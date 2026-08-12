from __future__ import annotations

import json
from pathlib import Path
from typing import Annotated

import typer

from .contracts import ModelState, ReleaseProfile
from .schemas import export_schemas
from .service import AelService

app = typer.Typer(no_args_is_help=True, help="Agentic Embedded Lab control-plane CLI")
model_app = typer.Typer(no_args_is_help=True, help="Generate and govern hardware models")
lab_app = typer.Typer(no_args_is_help=True, help="Operate explicitly configured hardware workers")
schema_app = typer.Typer(no_args_is_help=True, help="Export stable JSON schemas")
release_app = typer.Typer(no_args_is_help=True, help="Evaluate production release gates")
benchmark_app = typer.Typer(no_args_is_help=True, help="Run executable benchmark pairs")
app.add_typer(model_app, name="model")
app.add_typer(lab_app, name="lab")
app.add_typer(schema_app, name="schema")
app.add_typer(release_app, name="release")
app.add_typer(benchmark_app, name="benchmark")


def service(workspace: Path) -> AelService:
    return AelService(workspace)


def emit(value: object) -> None:
    if hasattr(value, "model_dump"):
        value = value.model_dump(mode="json")  # type: ignore[union-attr]
    typer.echo(json.dumps(value, indent=2, sort_keys=True, default=str))


WorkspaceOption = Annotated[Path, typer.Option("--workspace", "-w", resolve_path=True)]
DEFAULT_WORKSPACE = Path.cwd()


@app.command()
def doctor(workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    """Probe pinned tools without substituting missing backends."""
    emit(service(workspace).doctor())


@app.command()
def inspect(workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    """Inspect workspace runs, models and capabilities."""
    emit(service(workspace).inspect())


@app.command()
def classify(problem: Path, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    emit(service(workspace).classify(problem))


@app.command()
def validate(experiment: Path, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    emit(service(workspace).validate_experiment(experiment))


@app.command()
def run(experiment: Path, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    result = service(workspace).run_experiment(experiment)
    emit(
        {
            "run_id": result.run_id,
            "status": result.status,
            "evidence_dir": str(result.evidence_dir),
            "error": result.error,
        }
    )
    if result.status not in {"passed"}:
        raise typer.Exit(1)


@app.command()
def status(run_id: str, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    emit(service(workspace).status(run_id))


@app.command()
def replay(run_id: str, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    result = service(workspace).replay(run_id)
    emit({"run_id": result.run_id, "status": result.status, "evidence_dir": result.evidence_dir})


@app.command()
def compare(left: str, right: str, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    emit(service(workspace).compare(left, right))


@app.command()
def explain(run_id: str, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    emit(service(workspace).explain(run_id))


@model_app.command("generate")
def model_generate(request: Path, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    result = service(workspace).models.generate(request)
    emit(
        {
            "model": result.package,
            "package_path": result.package_path,
            "ir_path": result.ir_path,
        }
    )


@model_app.command("validate")
def model_validate(model_id: str, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    registry = service(workspace).models
    updated = registry.static_validate(model_id, actor="cli")
    emit(updated)


@model_app.command("promote")
def model_promote(
    model_id: str,
    target: ModelState,
    evidence: Annotated[list[str] | None, typer.Option("--evidence")] = None,
    human_approved: Annotated[bool, typer.Option("--human-approved")] = False,
    signature: Annotated[str | None, typer.Option("--signature")] = None,
    workspace: WorkspaceOption = DEFAULT_WORKSPACE,
) -> None:
    updated = service(workspace).models.transition(
        model_id,
        target,
        actor="cli",
        evidence=evidence,
        human_approved=human_approved,
        signature=signature,
    )
    emit(updated)


@schema_app.command("export")
def schema_export(
    destination: Annotated[Path, typer.Argument()] = Path("schemas/v1"),
    workspace: WorkspaceOption = DEFAULT_WORKSPACE,
) -> None:
    target = workspace / destination
    emit({"schemas": [str(path) for path in export_schemas(target)]})


@lab_app.command("calibrate")
def lab_calibrate(target: str, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    emit(
        {
            "target": target,
            "status": "blocked",
            "reason": "no mutually authenticated Lab Worker is configured",
            "workspace": str(workspace),
        }
    )
    raise typer.Exit(2)


@lab_app.command("validate")
def lab_validate(target: str, workspace: WorkspaceOption = DEFAULT_WORKSPACE) -> None:
    emit(
        {
            "target": target,
            "status": "blocked",
            "reason": "hardware differential validation requires a configured Lab Worker",
            "workspace": str(workspace),
        }
    )
    raise typer.Exit(2)


@benchmark_app.command("run")
def benchmark_run(
    case_id: Annotated[list[int] | None, typer.Option("--case-id")] = None,
    source_revision: Annotated[str, typer.Option("--source-revision")] = "working-tree",
    workspace: WorkspaceOption = DEFAULT_WORKSPACE,
) -> None:
    manifest = service(workspace).run_benchmarks(set(case_id) if case_id else None, source_revision)
    emit(manifest)
    if any(entry["status"] != "passed" for entry in manifest["entries"]):
        raise typer.Exit(2)


@release_app.command("check")
def release_check(
    profile: Annotated[ReleaseProfile, typer.Option("--profile")] = ReleaseProfile.PRODUCTION,
    workspace: WorkspaceOption = DEFAULT_WORKSPACE,
) -> None:
    result = service(workspace).release_check(profile)
    emit(result)
    if not result["ready"]:
        raise typer.Exit(2)
