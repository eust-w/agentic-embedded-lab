from __future__ import annotations


def test_mcp_server_exposes_only_domain_tools() -> None:
    from ael import mcp_server

    functions = {
        "inspect_project",
        "classify_problem",
        "plan_experiment",
        "start_experiment",
        "get_experiment",
        "get_evidence",
        "compare_experiments",
        "generate_missing_model",
        "validate_model",
    }
    assert functions <= set(mcp_server.__dict__)
    assert "shell" not in mcp_server.__dict__
    assert "renode_monitor" not in mcp_server.__dict__
