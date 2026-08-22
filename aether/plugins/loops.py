from __future__ import annotations

from typing import Any

from aether.core.contracts import (
    PluginAuthor,
    PluginContext,
    PluginMetadata,
    PluginType,
)


class ReActLoopPlugin:
    """Standard Reason+Act iterative loop plugin."""

    def __init__(self, max_iterations: int = 8) -> None:
        self.max_iterations = max_iterations
        self.metadata = PluginMetadata(
            id="react_loop",
            name="ReAct (Reason + Act) Strategy",
            version="1.0.0",
            type=PluginType.COGNITIVE_LOOP,
            description="Interleaves thought and action execution with environment feedback",
            author=PluginAuthor.SYSTEM,
            tags=["loop", "react", "strategy"],
        )

    def on_load(self, context: PluginContext) -> None:
        pass

    def on_unload(self) -> None:
        pass

    def get_schema(self) -> dict[str, Any]:
        return {"max_iterations": self.max_iterations, "mode": "react"}

    def self_test(self) -> dict[str, Any]:
        return {"passed": True, "max_iterations": self.max_iterations}
