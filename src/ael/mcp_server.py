from __future__ import annotations

import os
from pathlib import Path
from typing import Any

from mcp.server.fastmcp import FastMCP

from .service import AelService

WORKSPACE = Path(os.environ.get("AEL_WORKSPACE", Path.cwd())).resolve()
service = AelService(WORKSPACE)
mcp = FastMCP("Agentic Embedded Lab")


@mcp.tool()
def inspect_project() -> dict[str, Any]:
    """Discover embedded experiment, model and backend capabilities with fidelity limits."""
    return service.inspect()


@mcp.tool()
def classify_problem(problem_path: str) -> dict[str, Any]:
    """Route a strict ProblemSpec to required backends or explicit validation gaps."""
    return service.classify(Path(problem_path))


@mcp.tool()
def plan_experiment(experiment_path: str) -> dict[str, Any]:
    """Validate an ExperimentSpec and return its executable multi-rate plan."""
    return service.validate_experiment(Path(experiment_path))


@mcp.tool()
def start_experiment(experiment_path: str) -> dict[str, str]:
    """Start an experiment asynchronously; returns immediately with a run_id."""
    return {"run_id": service.start_experiment(Path(experiment_path))}


@mcp.tool()
def get_experiment(run_id: str) -> dict[str, Any]:
    """Poll experiment state without returning raw logs."""
    return service.status(run_id)


@mcp.tool()
def get_evidence(run_id: str, offset: int = 0, limit: int = 100) -> dict[str, Any]:
    """Return bounded evidence summary and a paginated raw event page."""
    return {
        "summary": service.explain(run_id),
        "event_page": service.get_event_page(run_id, offset, limit),
    }


@mcp.tool()
def compare_experiments(left_run_id: str, right_run_id: str) -> dict[str, Any]:
    """Compare status, assertions and deterministic trace hashes for two runs."""
    return service.compare(left_run_id, right_run_id)


@mcp.tool()
def generate_missing_model(request_path: str) -> dict[str, Any]:
    """Generate a quarantined model package from grounded inputs such as CMSIS-SVD."""
    result = service.models.generate(Path(request_path), actor="mcp-agent")
    return result.package.model_dump(mode="json")


@mcp.tool()
def validate_model(model_id: str, evidence: list[str] | None = None) -> dict[str, Any]:
    """Apply static/conformance checks; never grant hardware or production approval."""
    package, _ = service.models.load(model_id)
    if package.state == "generated":
        package = service.models.static_validate(model_id, actor="mcp-agent")
    if package.state == "static_validated" and evidence:
        package = service.models.conformance_validate(
            model_id, actor="mcp-agent", evidence=evidence
        )
    return {
        "model": package.model_dump(mode="json"),
        "next_allowed": "conformance_validated" if package.state == "static_validated" else None,
        "boundary": "Agent validation cannot exceed conformance_validated.",
    }


def main() -> None:
    mcp.run(transport="stdio")
