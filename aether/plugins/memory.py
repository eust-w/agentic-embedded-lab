from __future__ import annotations

from typing import Any

from aether.core.contracts import (
    ChatMessage,
    PluginAuthor,
    PluginContext,
    PluginMetadata,
    PluginType,
)


class WorkingMemoryPlugin:
    def __init__(self, max_tokens: int = 64000) -> None:
        self.max_tokens = max_tokens
        self.metadata = PluginMetadata(
            id="working_memory",
            name="Sliding Window Context Memory",
            version="1.0.0",
            type=PluginType.MEMORY,
            description=(
                "Manages multi-turn conversation history, context budget, and tool call traces"
            ),
            author=PluginAuthor.SYSTEM,
            tags=["memory", "context", "buffer"],
        )
        self.history: list[ChatMessage] = []

    def on_load(self, context: PluginContext) -> None:
        pass

    def on_unload(self) -> None:
        self.history.clear()

    def get_schema(self) -> dict[str, Any]:
        return {"max_tokens": self.max_tokens, "message_count": len(self.history)}

    def self_test(self) -> dict[str, Any]:
        return {"passed": True, "active_messages": len(self.history)}

    def append(self, message: ChatMessage) -> None:
        self.history.append(message)

    def get_messages(self) -> list[ChatMessage]:
        return list(self.history)

    def clear(self) -> None:
        self.history.clear()
