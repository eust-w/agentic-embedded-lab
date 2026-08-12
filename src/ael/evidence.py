from __future__ import annotations

import re
import shutil
from datetime import UTC, datetime
from xml.etree import ElementTree

from .constants import TOOL_VERSIONS
from .contracts import (
    Assertion,
    Claim,
    ClaimStatus,
    Event,
    EvidenceBundle,
    ExperimentSpec,
    RunStatus,
    SystemManifest,
)
from .io import canonical_json, sha256_value, write_json
from .security import resolve_workspace_path
from .storage import ContentAddressedStore, WorkspaceLayout


class EvidenceRecorder:
    def __init__(
        self,
        layout: WorkspaceLayout,
        run_id: str,
        experiment: ExperimentSpec,
        system: SystemManifest,
        cas: object | None = None,
    ) -> None:
        self.layout = layout
        self.run_id = run_id
        self.experiment = experiment
        self.system = system
        self.run_dir = layout.runs_dir / run_id
        self.snapshot_dir = self.run_dir / "snapshots"
        self.run_dir.mkdir(parents=True, exist_ok=False)
        self.snapshot_dir.mkdir()
        self.events: list[Event] = []
        self.assertion_results: list[dict[str, object]] = []
        self.started_at = datetime.now(UTC)
        self.cas = cas or ContentAddressedStore(layout)
        write_json(self.run_dir / "manifest.resolved.json", system)
        write_json(self.run_dir / "experiment.resolved.json", experiment)

    def add_events(self, events: list[Event]) -> None:
        for event in events:
            self.events.append(event.model_copy(update={"sequence": len(self.events)}))

    def add_assertion(self, assertion: Assertion, actual: object, passed: bool) -> None:
        self.assertion_results.append(
            {
                "metric": assertion.metric,
                "operator": assertion.operator,
                "expected": assertion.expected,
                "actual": actual,
                "unit": assertion.unit,
                "critical": assertion.critical,
                "passed": passed,
            }
        )

    def add_artifacts(
        self, component_id: str, virtual_time_us: int, artifacts: dict[str, str]
    ) -> None:
        for label, reference in sorted(artifacts.items()):
            if not re.fullmatch(r"[A-Za-z0-9_.-]+", label):
                raise ValueError(f"unsafe backend artifact label: {label!r}")
            source = resolve_workspace_path(self.layout.root, reference, must_exist=True)
            destination = (
                self.run_dir
                / "artifacts"
                / component_id
                / f"{virtual_time_us:016d}"
                / label
            )
            destination.parent.mkdir(parents=True, exist_ok=True)
            if source.is_dir():
                shutil.copytree(source, destination)
            else:
                destination.mkdir()
                shutil.copy2(source, destination / source.name)

    def finalize(self, status: RunStatus, *, error: str | None = None) -> EvidenceBundle:
        events_path = self.run_dir / "events.jsonl"
        with events_path.open("w", encoding="utf-8") as stream:
            for event in self.events:
                stream.write(canonical_json(event).decode() + "\n")
        write_json(self.run_dir / "assertions.json", self.assertion_results)
        provenance = {
            "created_at": self.started_at.isoformat(),
            "completed_at": datetime.now(UTC).isoformat(),
            "ael_version": "0.1.0.dev0",
            "tools_expected": {tool.name: tool.version for tool in TOOL_VERSIONS},
            "source_tree_dirty": True,
            "error": error,
        }
        write_json(self.run_dir / "provenance.json", provenance)
        self._write_junit(status, error)
        self._write_summary(status, error)

        artifact_hashes: dict[str, str] = {}
        for path in sorted(self.run_dir.rglob("*")):
            if path.is_file() and path.name != "bundle.json":
                relative = str(path.relative_to(self.run_dir))
                digest, _ = self.cas.put_bytes(path.read_bytes())
                artifact_hashes[relative] = digest

        claims = [
            Claim(
                id=f"{self.run_id}-control-plane",
                statement="The recorded control-plane experiment produced the attached evidence.",
                status=ClaimStatus.UNVERIFIED,
                evidence_paths=["events.jsonl", "assertions.json"],
                limitations=[
                    "No hardware equivalence is implied.",
                    (
                        "Claims remain dependent on the declared component models and "
                        "fidelity metadata."
                    ),
                ],
            )
        ]
        model_hashes: dict[str, str] = {}
        for component in self.system.components:
            if not component.model:
                continue
            model_path = resolve_workspace_path(self.layout.root, component.model, must_exist=True)
            digest, _ = self.cas.put_bytes(model_path.read_bytes())
            model_hashes[component.id] = digest
        bundle = EvidenceBundle(
            run_id=self.run_id,
            status=status,
            created_at=self.started_at,
            completed_at=datetime.now(UTC),
            experiment_hash=sha256_value(self.experiment),
            system_hash=sha256_value(self.system),
            model_hashes=model_hashes,
            artifact_hashes=artifact_hashes,
            event_count=len(self.events),
            assertion_results=self.assertion_results,
            claims=claims,
            fidelity_boundaries=[
                "Simulation results are not hardware validation.",
                "Synthetic components are test-only and cannot support production claims.",
                *[f"{key}: {value}" for key, value in sorted(self.system.fidelity.items())],
            ],
        )
        write_json(self.run_dir / "bundle.json", bundle)
        return bundle

    def _write_junit(self, status: RunStatus, error: str | None) -> None:
        suite = ElementTree.Element(
            "testsuite",
            name=self.experiment.name,
            tests=str(max(1, len(self.assertion_results))),
            failures=str(sum(not bool(item["passed"]) for item in self.assertion_results)),
            errors="1" if error else "0",
        )
        if not self.assertion_results:
            case = ElementTree.SubElement(suite, "testcase", name="experiment-completed")
            if status not in {RunStatus.PASSED}:
                ElementTree.SubElement(case, "failure", message=error or status.value)
        for item in self.assertion_results:
            case = ElementTree.SubElement(suite, "testcase", name=str(item["metric"]))
            if not item["passed"]:
                ElementTree.SubElement(
                    case,
                    "failure",
                    message=(
                        f"expected {item['operator']} {item['expected']}; actual {item['actual']}"
                    ),
                )
        if error:
            ElementTree.SubElement(suite, "system-err").text = error
        ElementTree.ElementTree(suite).write(
            self.run_dir / "junit.xml", encoding="utf-8", xml_declaration=True
        )

    def _write_summary(self, status: RunStatus, error: str | None) -> None:
        failures = [item for item in self.assertion_results if not item["passed"]]
        lines = [
            f"# Experiment {self.experiment.name}",
            "",
            f"- Run: `{self.run_id}`",
            f"- Status: `{status}`",
            f"- Events: {len(self.events)}",
            f"- Failed assertions: {len(failures)}",
            "",
            "## Proven",
            "",
            "- The configuration was parsed under strict v1 contracts.",
            "- The attached event and assertion artifacts are content-addressed.",
            "",
            "## Not proven",
            "",
            "- Hardware behavior outside an approved Validation Envelope.",
            "- Electrical, timing, power, thermal, RF or EM equivalence unless separately claimed.",
        ]
        if error:
            lines.extend(["", "## Error", "", f"`{error}`"])
        (self.run_dir / "summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
