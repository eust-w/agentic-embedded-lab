from __future__ import annotations

from ael.adapters import AdapterCatalog
from ael.contracts import ProblemCategory, ProblemSpec
from ael.router import CATEGORY_BACKENDS, RouteStatus, classify_problem


def test_every_problem_category_has_a_route() -> None:
    assert set(CATEGORY_BACKENDS) == set(ProblemCategory)


def test_missing_real_backend_is_explicit() -> None:
    problem = ProblemSpec(
        name="uart", title="UART failure", category=ProblemCategory.SERIAL_BUS, symptoms=["loss"]
    )
    result = classify_problem(problem, AdapterCatalog())
    assert result.status == RouteStatus.MODEL_GENERATION_REQUIRED
    assert result.missing_backends
    assert "equivalence" in result.fidelity_boundary
