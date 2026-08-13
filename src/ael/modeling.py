from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from pathlib import Path
from xml.etree import ElementTree

from .contracts import (
    BehaviorField,
    BehaviorRegister,
    GenerationReceipt,
    GroundingManifest,
    GroundingReference,
    HardwareBehaviorIR,
    ModelConformanceEvidence,
    ModelGenerationRequest,
    ModelPackage,
    ModelState,
)
from .io import load_document, sha256_file, write_json
from .model_providers import provider_for
from .security import resolve_workspace_path
from .storage import StateStore, WorkspaceLayout

STATE_TRANSITIONS: dict[ModelState, set[ModelState]] = {
    ModelState.DRAFT: {ModelState.GENERATED, ModelState.DEPRECATED},
    ModelState.GENERATED: {ModelState.STATIC_VALIDATED, ModelState.DEPRECATED},
    ModelState.STATIC_VALIDATED: {ModelState.CONFORMANCE_VALIDATED, ModelState.DEPRECATED},
    ModelState.CONFORMANCE_VALIDATED: {ModelState.HARDWARE_VALIDATED, ModelState.DEPRECATED},
    ModelState.HARDWARE_VALIDATED: {ModelState.PRODUCTION_APPROVED, ModelState.DEPRECATED},
    ModelState.PRODUCTION_APPROVED: {ModelState.DEPRECATED},
    ModelState.DEPRECATED: set(),
}

AGENT_MAX_STATE = ModelState.CONFORMANCE_VALIDATED


@dataclass(frozen=True)
class GenerationResult:
    package: ModelPackage
    package_path: Path
    ir_path: Path


def import_systemrdl(path: Path, name: str) -> HardwareBehaviorIR:
    try:
        from systemrdl import RDLCompiler
        from systemrdl.node import FieldNode, RegNode
    except ImportError as exception:
        raise RuntimeError("install AEL with the modeling extra") from exception
    compiler = RDLCompiler()
    compiler.compile_file(str(path))
    root = compiler.elaborate().top
    registers: list[BehaviorRegister] = []
    max_end = 0
    for node in root.descendants(unroll=True):
        if not isinstance(node, RegNode):
            continue
        fields: list[BehaviorField] = []
        register_reset = 0
        for field in node.fields():
            if not isinstance(field, FieldNode):
                continue
            sw = str(field.get_property("sw"))
            access = {"r": "ro", "w": "wo", "rw": "rw", "r/w": "rw"}.get(sw, "rw")
            field_reset = int(field.get_property("reset") or 0)
            register_reset |= field_reset << int(field.low)
            fields.append(
                BehaviorField(
                    name=field.inst_name,
                    lsb=int(field.low),
                    width=int(field.width),
                    access=access,
                    reset=field_reset,
                )
            )
        offset = int(node.absolute_address - root.absolute_address)
        width = int(node.get_property("regwidth") or 32)
        if width not in {8, 16, 32, 64}:
            raise ValueError(f"unsupported SystemRDL register width: {width}")
        registers.append(
            BehaviorRegister(
                name=node.inst_name,
                offset=offset,
                width=width,
                reset=register_reset,
                fields=fields,
            )
        )
        max_end = max(max_end, offset + width // 8)
    if not registers:
        raise ValueError("SystemRDL contains no registers")
    return HardwareBehaviorIR(name=name, size=max(4, max_end), registers=registers)


def generate_renode_csharp(ir: HardwareBehaviorIR, namespace: str = "Ael.Generated") -> str:
    class_name = "".join(part.title() for part in re_split_identifier(ir.name))
    register_definitions: list[str] = []
    enum_entries: list[str] = []
    field_declarations: list[str] = []
    for register in ir.registers:
        builder = [f"            Registers.{register.name}.Define(this, 0x{register.reset:x})"]
        for field in register.fields:
            method = "WithFlag" if field.width == 1 else "WithValueField"
            mode = {
                "ro": "FieldMode.Read",
                "wo": "FieldMode.Write",
                "rw": "FieldMode.Read | FieldMode.Write",
                "w1c": "FieldMode.Read | FieldMode.WriteOneToClear",
                "w1s": "FieldMode.Read | FieldMode.Set",
            }[field.access]
            variable = (
                register.name[:1].lower()
                + register.name[1:]
                + field.name[:1].upper()
                + field.name[1:]
            )
            field_declarations.append(
                "        private "
                + ("IFlagRegisterField" if field.width == 1 else "IValueRegisterField")
                + f" {variable};"
            )
            width_argument = "" if field.width == 1 else f", {field.width}"
            builder.append(
                f"                .{method}({field.lsb}{width_argument}, out {variable}, "
                f'mode: {mode}, name: "{field.name}")'
            )
        builder[-1] += ";"
        register_definitions.append("\n".join(builder))
        enum_entries.append(f"            {register.name} = 0x{register.offset:x},")
    return f"""// SPDX-License-Identifier: Apache-2.0
using Antmicro.Renode.Core;
using Antmicro.Renode.Core.Structure.Registers;
using Antmicro.Renode.Peripherals.Bus;

namespace {namespace}
{{
    public sealed class {class_name} : BasicDoubleWordPeripheral, IKnownSize
    {{
        public {class_name}(IMachine machine) : base(machine)
        {{
{chr(10).join(register_definitions)}
        }}

        public long Size => 0x{ir.size:x};

{chr(10).join(field_declarations)}

        private enum Registers : long
        {{
{chr(10).join(enum_entries)}
        }}
    }}
}}
"""


def re_split_identifier(value: str) -> list[str]:
    import re

    return [part for part in re.split(r"[^A-Za-z0-9]+", value) if part]


def import_svd(path: Path, name: str) -> HardwareBehaviorIR:
    root = ElementTree.parse(path).getroot()

    def local(tag: str) -> str:
        return tag.rsplit("}", 1)[-1]

    def child_text(element: ElementTree.Element, child_name: str, default: str = "") -> str:
        for child in element:
            if local(child.tag) == child_name:
                return (child.text or default).strip()
        return default

    registers: list[BehaviorRegister] = []
    max_end = 0
    for element in root.iter():
        if local(element.tag) != "register":
            continue
        register_name = child_text(element, "name")
        offset_text = child_text(element, "addressOffset", "0")
        size_text = child_text(element, "size", "32")
        reset_text = child_text(element, "resetValue", "0")
        width = int(size_text, 0)
        if width not in {8, 16, 32, 64}:
            width = 32
        fields: list[BehaviorField] = []
        for candidate in element.iter():
            if local(candidate.tag) != "field":
                continue
            field_name = child_text(candidate, "name")
            offset = int(child_text(candidate, "bitOffset", "0"), 0)
            field_width = int(child_text(candidate, "bitWidth", "1"), 0)
            access = child_text(candidate, "access", "read-write")
            access_map = {
                "read-only": "ro",
                "write-only": "wo",
                "read-write": "rw",
                "read-writeOnce": "rw",
                "writeOnce": "wo",
            }
            fields.append(
                BehaviorField(
                    name=field_name,
                    lsb=offset,
                    width=field_width,
                    access=access_map.get(access, "rw"),
                )
            )
        offset = int(offset_text, 0)
        registers.append(
            BehaviorRegister(
                name=register_name,
                offset=offset,
                width=width,
                reset=int(reset_text, 0),
                fields=fields,
            )
        )
        max_end = max(max_end, offset + width // 8)
    if not registers:
        raise ValueError("SVD contains no registers")
    return HardwareBehaviorIR(name=name, size=max(4, max_end), registers=registers)


class ModelRegistry:
    def __init__(self, layout: WorkspaceLayout, store: StateStore | None = None) -> None:
        self.layout = layout
        self.store = store or StateStore(layout)

    def generate(self, request_path: Path, *, actor: str = "agent") -> GenerationResult:
        request = load_document(request_path, ModelGenerationRequest, self.layout.root)
        if request.systemrdl is not None:
            return self._generate_systemrdl(request, actor)
        if request.svd is None:
            return self._generate_grounded(request, actor)
        svd_path = resolve_workspace_path(self.layout.root, request.svd, must_exist=True)
        ir = import_svd(svd_path, request.name)
        model_dir = self.layout.models_dir / request.id / request.version
        model_dir.mkdir(parents=True, exist_ok=True)
        ir_path = model_dir / "behavior.ir.json"
        package_path = model_dir / "package.json"
        write_json(ir_path, ir)
        package = ModelPackage(
            id=request.id,
            name=request.name,
            version=request.version,
            backend=request.backend,
            state=ModelState.GENERATED,
            source_paths=[request.svd, *request.datasheets, *request.drivers],
            source_hashes={request.svd: sha256_file(svd_path)},
            ir_path=str(ir_path.relative_to(self.layout.root)),
            artifact_paths=[],
            generated_by=request.generator,
        )
        self.store.save_model(package, package_path)
        self.store.audit(
            actor, "model.generate", package.id, json.dumps({"version": package.version})
        )
        return GenerationResult(package, package_path, ir_path)

    def _generate_grounded(self, request: ModelGenerationRequest, actor: str) -> GenerationResult:
        if request.generation is None:
            raise ValueError(
                "document-only generation requires generation.provider, model, and template version"
            )
        source_names = [
            *request.datasheets,
            *request.drivers,
            *request.reference_models,
            *request.hardware_traces,
        ]
        if not source_names:
            raise ValueError(
                "grounded Agent generation requires document, driver, model, or trace input"
            )
        documents: list[str] = []
        references: list[GroundingReference] = []
        source_hashes: dict[str, str] = {}
        for source_name in source_names:
            source_path = resolve_workspace_path(self.layout.root, source_name, must_exist=True)
            digest = sha256_file(source_path)
            source_hashes[source_name] = digest
            text, locator = _read_grounding_source(source_path)
            references.append(
                GroundingReference(
                    source_path=source_name,
                    source_sha256=digest,
                    locator=locator,
                    purpose="datasheet/driver/reference evidence for Hardware Behavior IR",
                )
            )
            documents.append(
                f"<source path={json.dumps(source_name)} sha256={json.dumps(digest)} "
                f"locator={json.dumps(locator)}>\n{text}\n</source>"
            )
        prompt = (
            f"Target model name: {request.name}\n"
            "Return a strict HardwareBehaviorIR. The grounding map must map every modeled "
            "register, interrupt, DMA request, transaction, fault, clock, timer, and power state "
            "JSON pointer to one or more `path#locator` entries present below. Omit behavior that "
            "is not established by the sources. SI/UCUM units are required.\n\n"
            + "\n\n".join(documents)
        )
        if len(prompt) > 1_500_000:
            raise ValueError("grounding input exceeds the 1,500,000 character safety limit")
        provider = provider_for(request.generation.provider)
        last_error: Exception | None = None
        result = None
        for _attempt in range(1, request.generation.max_attempts + 1):
            try:
                result = provider.generate(prompt, request.generation)
                break
            except (RuntimeError, ValueError) as error:
                last_error = error
        if result is None:
            raise RuntimeError(
                "grounded model generation failed after "
                f"{request.generation.max_attempts} attempts: "
                f"{last_error}"
            )
        _validate_grounding_map(result.ir, references)
        model_dir = self.layout.models_dir / request.id / request.version
        model_dir.mkdir(parents=True, exist_ok=True)
        ir_path = model_dir / "behavior.ir.json"
        package_path = model_dir / "package.json"
        grounding_path = model_dir / "grounding-manifest.json"
        receipt_path = model_dir / "generation-receipt.json"
        write_json(ir_path, result.ir)
        write_json(grounding_path, GroundingManifest(sources=references))
        write_json(
            receipt_path,
            GenerationReceipt(
                provider=request.generation.provider,
                model=request.generation.model,
                prompt_template_version=request.generation.prompt_template_version,
                request_sha256=hashlib.sha256(prompt.encode("utf-8")).hexdigest(),
                response_sha256=hashlib.sha256(result.raw_text.encode("utf-8")).hexdigest(),
                provider_request_id=result.request_id,
                attempts=_attempt,
                recorded=result.recorded,
            ),
        )
        artifact_paths: list[str] = []
        if request.backend.value == "renode":
            generated = model_dir / "GeneratedPeripheral.cs"
            generated.write_text(generate_renode_csharp(result.ir), encoding="utf-8")
            artifact_paths.append(str(generated.relative_to(self.layout.root)))
        package = ModelPackage(
            id=request.id,
            name=request.name,
            version=request.version,
            backend=request.backend,
            state=ModelState.GENERATED,
            source_paths=source_names,
            source_hashes=source_hashes,
            ir_path=str(ir_path.relative_to(self.layout.root)),
            artifact_paths=artifact_paths,
            generated_by=f"{request.generation.provider}:{request.generation.model}",
            grounding_manifest_path=str(grounding_path.relative_to(self.layout.root)),
            generation_receipt_path=str(receipt_path.relative_to(self.layout.root)),
        )
        self.store.save_model(package, package_path)
        self.store.audit(
            actor,
            "model.generate.grounded",
            package.id,
            json.dumps(
                {"provider": request.generation.provider, "version": package.version},
                sort_keys=True,
            ),
        )
        return GenerationResult(package, package_path, ir_path)

    def generate_from_systemrdl(
        self, request_path: Path, *, actor: str = "agent"
    ) -> GenerationResult:
        request = load_document(request_path, ModelGenerationRequest, self.layout.root)
        return self._generate_systemrdl(request, actor)

    def _generate_systemrdl(self, request: ModelGenerationRequest, actor: str) -> GenerationResult:
        sources = [*request.datasheets, *request.drivers]
        rdl_source = request.systemrdl or next(
            (item for item in sources if item.endswith((".rdl", ".systemrdl"))), None
        )
        if rdl_source is None:
            raise ValueError("SystemRDL generation requires a .rdl source path")
        source_path = resolve_workspace_path(self.layout.root, rdl_source, must_exist=True)
        ir = import_systemrdl(source_path, request.name)
        model_dir = self.layout.models_dir / request.id / request.version
        model_dir.mkdir(parents=True, exist_ok=True)
        ir_path = model_dir / "behavior.ir.json"
        source_path_cs = model_dir / "GeneratedPeripheral.cs"
        package_path = model_dir / "package.json"
        write_json(ir_path, ir)
        source_path_cs.write_text(generate_renode_csharp(ir), encoding="utf-8")
        package = ModelPackage(
            id=request.id,
            name=request.name,
            version=request.version,
            backend=request.backend,
            state=ModelState.GENERATED,
            source_paths=[rdl_source],
            source_hashes={rdl_source: sha256_file(source_path)},
            ir_path=str(ir_path.relative_to(self.layout.root)),
            artifact_paths=[str(source_path_cs.relative_to(self.layout.root))],
            generated_by=request.generator,
        )
        self.store.save_model(package, package_path)
        self.store.audit(actor, "model.generate", package.id, "systemrdl")
        return GenerationResult(package, package_path, ir_path)

    def generate_renode_source(self, model_id: str) -> Path:
        package, package_path = self.load(model_id)
        if package.ir_path is None:
            raise ValueError("model has no Hardware Behavior IR")
        ir_path = resolve_workspace_path(self.layout.root, package.ir_path, must_exist=True)
        ir = HardwareBehaviorIR.model_validate_json(ir_path.read_text(encoding="utf-8"))
        destination = package_path.parent / "GeneratedPeripheral.cs"
        destination.write_text(generate_renode_csharp(ir), encoding="utf-8")
        relative = str(destination.relative_to(self.layout.root))
        if relative not in package.artifact_paths:
            package = package.model_copy(
                update={"artifact_paths": [*package.artifact_paths, relative]}
            )
            self.store.save_model(package, package_path)
        return destination

    def static_validate(self, model_id: str, *, actor: str) -> ModelPackage:
        package, _ = self.load(model_id)
        if package.state != ModelState.GENERATED:
            raise ValueError("static validation requires a generated model")
        if package.ir_path is None:
            raise ValueError("generated model has no Hardware Behavior IR")
        ir_path = resolve_workspace_path(self.layout.root, package.ir_path, must_exist=True)
        HardwareBehaviorIR.model_validate_json(ir_path.read_text(encoding="utf-8"))
        for source, expected_hash in package.source_hashes.items():
            source_path = resolve_workspace_path(self.layout.root, source, must_exist=True)
            if sha256_file(source_path) != expected_hash:
                raise ValueError(f"grounding source changed after generation: {source}")
        return self.transition(
            model_id,
            ModelState.STATIC_VALIDATED,
            actor=actor,
            evidence=[str(ir_path.relative_to(self.layout.root))],
        )

    def conformance_validate(
        self, model_id: str, *, actor: str, evidence: list[str]
    ) -> ModelPackage:
        package, package_path = self.load(model_id)
        if package.state != ModelState.STATIC_VALIDATED:
            raise ValueError("conformance validation requires a static_validated model")
        if not evidence:
            raise ValueError("conformance validation requires independent evidence")
        model_root = package_path.parent.resolve()
        resolved: list[str] = []
        independent = False
        validated_report = False
        for item in evidence:
            path = resolve_workspace_path(self.layout.root, item, must_exist=True)
            if path.is_dir():
                raise ValueError("conformance evidence must be a file")
            if model_root not in path.resolve().parents:
                independent = True
            try:
                report = ModelConformanceEvidence.model_validate_json(
                    path.read_text(encoding="utf-8")
                )
            except (UnicodeDecodeError, ValueError):
                report = None
            if report is not None:
                if report.model_id != model_id:
                    raise ValueError("conformance evidence model_id does not match package")
                checks = (
                    report.source_independent,
                    report.register_layout_passed,
                    report.compile_passed,
                    report.driver_tests_passed,
                    report.property_tests_passed,
                    report.reference_trace_passed,
                    not report.generated_tests_are_only_evidence,
                    report.sandbox_read_only,
                )
                if all(checks):
                    validated_report = True
            resolved.append(str(path.relative_to(self.layout.root)))
        if not independent:
            raise ValueError(
                "at least one conformance artifact must be independent of the model package"
            )
        if not validated_report:
            raise ValueError(
                "conformance requires an independent ModelConformanceEvidence report with "
                "layout, compile, driver, property, reference-trace and offline sandbox checks"
            )
        return self.transition(
            model_id,
            ModelState.CONFORMANCE_VALIDATED,
            actor=actor,
            evidence=resolved,
            _conformance_checked=True,
        )

    def load(self, model_id: str) -> tuple[ModelPackage, Path]:
        record = self.store.get_model_record(model_id)
        if record is None:
            raise KeyError(f"unknown model: {model_id}")
        path = resolve_workspace_path(self.layout.root, record["package_path"], must_exist=True)
        return ModelPackage.model_validate_json(path.read_text(encoding="utf-8")), path

    def transition(
        self,
        model_id: str,
        target: ModelState,
        *,
        actor: str,
        human_approved: bool = False,
        evidence: list[str] | None = None,
        signature: str | None = None,
        _conformance_checked: bool = False,
    ) -> ModelPackage:
        package, path = self.load(model_id)
        if target not in STATE_TRANSITIONS[package.state]:
            raise ValueError(f"invalid model transition: {package.state} -> {target}")
        if target in {ModelState.HARDWARE_VALIDATED, ModelState.PRODUCTION_APPROVED}:
            if not human_approved:
                raise PermissionError(f"{target} requires explicit human approval")
            if not evidence:
                raise ValueError(f"{target} requires independent validation evidence")
        if target == ModelState.PRODUCTION_APPROVED and not signature:
            raise ValueError("production approval requires a signature")
        if target == ModelState.CONFORMANCE_VALIDATED and not _conformance_checked:
            raise PermissionError("use conformance_validate with an independent evidence report")
        for evidence_path in evidence or []:
            resolve_workspace_path(self.layout.root, evidence_path, must_exist=True)
        updated = package.model_copy(
            update={
                "state": target,
                "validation_evidence": [*package.validation_evidence, *(evidence or [])],
                "signature": signature or package.signature,
            }
        )
        self.store.save_model(updated, path)
        self.store.audit(actor, "model.transition", model_id, f"{package.state}->{target}")
        return updated


def _read_grounding_source(path: Path) -> tuple[str, str]:
    if path.suffix.lower() == ".pdf":
        try:
            from pypdf import PdfReader
        except ImportError as error:
            raise RuntimeError(
                "install AEL with the modeling extra to read PDF datasheets"
            ) from error
        reader = PdfReader(str(path))
        pages = [
            f"[page {index}]\n{page.extract_text() or ''}"
            for index, page in enumerate(reader.pages, 1)
        ]
        text = "\n\n".join(pages)
        locator = f"pages:1-{len(reader.pages)}"
    else:
        text = path.read_text(encoding="utf-8", errors="replace")
        locator = f"lines:1-{max(1, len(text.splitlines()))}"
    if len(text) > 500_000:
        raise ValueError(f"grounding source is too large after extraction: {path.name}")
    return text, locator


def _validate_grounding_map(ir: HardwareBehaviorIR, references: list[GroundingReference]) -> None:
    if not ir.grounding:
        raise ValueError("Agent-generated HardwareBehaviorIR must include a grounding map")
    valid_prefixes = {f"{item.source_path}#" for item in references}
    for pointer, citations in ir.grounding.items():
        if not pointer.startswith("/") or not citations:
            raise ValueError("grounding entries require a JSON pointer and at least one citation")
        for citation in citations:
            if not any(citation.startswith(prefix) for prefix in valid_prefixes):
                raise ValueError(
                    f"grounding citation does not reference an input source: {citation}"
                )
    required: list[str] = []
    required.extend(f"/registers/{index}" for index, _ in enumerate(ir.registers))
    required.extend(f"/interrupts/{index}" for index, _ in enumerate(ir.interrupts))
    required.extend(f"/dma_requests/{index}" for index, _ in enumerate(ir.dma_requests))
    required.extend(f"/transactions/{index}" for index, _ in enumerate(ir.transactions))
    missing = [pointer for pointer in required if pointer not in ir.grounding]
    if missing:
        raise ValueError(f"Agent-generated IR has ungrounded modeled behavior: {missing}")
