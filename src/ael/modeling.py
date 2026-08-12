from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from xml.etree import ElementTree

from .contracts import (
    BehaviorField,
    BehaviorRegister,
    HardwareBehaviorIR,
    ModelGenerationRequest,
    ModelPackage,
    ModelState,
)
from .io import load_document, sha256_file, write_json
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
                f"mode: {mode}, name: \"{field.name}\")"
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
            raise NotImplementedError(
                "the deterministic v0 importer requires CMSIS-SVD; document-only generation "
                "must run in the quarantined model-generation service"
            )
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

    def generate_from_systemrdl(
        self, request_path: Path, *, actor: str = "agent"
    ) -> GenerationResult:
        request = load_document(request_path, ModelGenerationRequest, self.layout.root)
        return self._generate_systemrdl(request, actor)

    def _generate_systemrdl(
        self, request: ModelGenerationRequest, actor: str
    ) -> GenerationResult:
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
        if target == ModelState.CONFORMANCE_VALIDATED and not evidence:
            raise ValueError("conformance validation requires independent test evidence")
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
