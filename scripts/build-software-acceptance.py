#!/usr/bin/env python3
"""Build a strict software ReleaseProfile manifest from machine evidence."""

from __future__ import annotations

import argparse
import json
import os
from datetime import UTC, datetime
from pathlib import Path
from xml.etree import ElementTree

from ael.contracts import AcceptanceEntry, AcceptanceManifest, ReleaseProfile
from ael.io import sha256_file, write_json
from ael.provenance import detect_platform


def load_json(path: Path) -> object:
    return json.loads(path.read_text(encoding="utf-8"))


def write_entry(
    workspace: Path, name: str, source: Path, checks: dict[str, object]
) -> AcceptanceEntry:
    destination = workspace / "acceptance/evidence" / f"software-{name.split(':')[1]}.json"
    payload = {
        "name": name,
        "status": "passed",
        "source": str(source.relative_to(workspace)),
        "source_sha256": sha256_file(source),
        "checks": checks,
        "hardware_validated": False,
    }
    write_json(destination, payload)
    return AcceptanceEntry(
        name=name,
        status="passed",
        evidence_path=str(destination.relative_to(workspace)),
        evidence_sha256=sha256_file(destination),
        limitations=["Software production topology only; no hardware validation."],
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--workspace", type=Path, default=Path.cwd())
    parser.add_argument("--compose-report", type=Path, required=True)
    parser.add_argument("--junit", type=Path, required=True)
    parser.add_argument("--sbom", type=Path, required=True)
    parser.add_argument("--signature", type=Path, required=True)
    parser.add_argument("--licenses", type=Path, required=True)
    parser.add_argument("--source-revision", default=os.environ.get("GITHUB_SHA", "working-tree"))
    args = parser.parse_args()
    workspace = args.workspace.resolve()
    sources = [
        args.compose_report.resolve(),
        args.junit.resolve(),
        args.sbom.resolve(),
        args.signature.resolve(),
        args.licenses.resolve(),
    ]
    if any(workspace not in source.parents for source in sources):
        raise ValueError("all acceptance inputs must be inside the workspace")
    compose = load_json(sources[0])
    if not isinstance(compose, dict) or compose.get("passed") is not True:
        raise ValueError("Compose acceptance report did not pass")
    junit_root = ElementTree.parse(sources[1]).getroot()
    suites = (
        [junit_root]
        if junit_root.tag.endswith("testsuite")
        else list(junit_root.iter("testsuite"))
    )
    if not suites:
        raise ValueError("software test JUnit contains no test suites")
    failures = sum(int(suite.attrib.get("failures", "0")) for suite in suites)
    errors = sum(int(suite.attrib.get("errors", "0")) for suite in suites)
    if failures or errors:
        raise ValueError("software test JUnit contains failures")
    sbom = load_json(sources[2])
    signature = load_json(sources[3])
    licenses = load_json(sources[4])
    if not isinstance(sbom, dict) or sbom.get("bomFormat") != "CycloneDX":
        raise ValueError("software SBOM is not CycloneDX JSON")
    if not isinstance(signature, dict) or not signature:
        raise ValueError("Sigstore bundle is empty")
    if not isinstance(licenses, list) or not licenses:
        raise ValueError("license inventory is empty")
    checks = compose["checks"]
    entries = [
        write_entry(
            workspace,
            "deployment:compose",
            sources[0],
            {
                "compose_passed": True,
                "oidc_access": checks["oidc_authorized_project"],
                "migration_upgrade_and_rollback": checks["migration_upgrade_and_rollback"],
            },
        ),
        write_entry(
            workspace,
            "storage:postgres-s3",
            sources[0],
            {
                "postgres_restart": checks["postgres_restart_recovery"],
                "s3_retransmit": checks["s3_outage_and_retransmit"],
            },
        ),
        write_entry(
            workspace,
            "security:oidc-mtls",
            sources[0],
            {
                "anonymous_rejected": checks["oidc_rejects_anonymous"],
                "mtls_required": checks["mtls_rejects_missing_client_certificate"],
            },
        ),
        write_entry(
            workspace,
            "worker:lease-recovery",
            sources[0],
            {
                "execution": checks["worker_executes_and_uploads_evidence"],
                "lease_recovery": checks["expired_lease_recovered_idempotently"],
                "cancellation": checks["leased_task_cancellation"],
            },
        ),
        write_entry(
            workspace,
            "supply-chain:sbom-signature",
            sources[2],
            {
                "sbom_sha256": sha256_file(sources[2]),
                "signature_sha256": sha256_file(sources[3]),
                "licenses_sha256": sha256_file(sources[4]),
                "software_test_junit_sha256": sha256_file(sources[1]),
            },
        ),
    ]
    manifest = AcceptanceManifest(
        profile=ReleaseProfile.SOFTWARE,
        source_revision=args.source_revision,
        platform=detect_platform(),
        entries=entries,
        created_at=datetime.now(UTC),
    )
    write_json(workspace / "acceptance/software.json", manifest)


if __name__ == "__main__":
    main()
