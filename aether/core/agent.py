from __future__ import annotations

import asyncio
import logging
import uuid
from collections.abc import AsyncIterator
from datetime import UTC, datetime
from typing import Any

from aether.plugins.loops import ReActLoopPlugin
from aether.plugins.memory import WorkingMemoryPlugin
from aether.plugins.models import (
    AnthropicModelPlugin,
    DeepSeekModelPlugin,
    MockCognitiveModelPlugin,
    OpenAIModelPlugin,
)
from aether.plugins.tools import (
    AstAnalyzerToolPlugin,
    BashToolPlugin,
    EvolvePluginToolPlugin,
    FileEditorToolPlugin,
)

from .contracts import (
    ChatMessage,
    DiffChunk,
    MessageRole,
    PluginType,
    ToolCall,
    ToolResult,
)
from .evolution import EvolutionManager
from .registry import PluginRegistry
from .sandbox import PluginSandbox

logger = logging.getLogger("aether.agent")


class AgentEvent:
    def __init__(self, event_type: str, data: dict[str, Any]) -> None:
        self.event_type = event_type
        self.data = data
        self.timestamp = datetime.now(UTC).isoformat()

    def to_dict(self) -> dict[str, Any]:
        return {"type": self.event_type, "data": self.data, "timestamp": self.timestamp}


class AetherAgent:
    """The central Agent runtime orchestrating the cognitive loop and dynamic plugin mesh."""

    def __init__(
        self,
        workspace: str | None = None,
        *,
        allow_unsafe_evolution: bool = False,
    ) -> None:
        from aether.core.config import aether_config
        self.workspace = workspace if workspace is not None else str(aether_config.workspace_dir)
        self.registry = PluginRegistry(self.workspace)
        self.sandbox = PluginSandbox()
        self.evolution_manager = EvolutionManager(
            self.registry,
            self.sandbox,
            allow_unsafe_in_process=allow_unsafe_evolution,
        )
        self.pending_diffs: list[DiffChunk] = []
        self._init_default_plugins()

    def _init_default_plugins(self) -> None:
        self.registry.register(DeepSeekModelPlugin(model_name="deepseek-v4-pro"))
        self.registry.register(AnthropicModelPlugin(model_name="claude-3-7-sonnet"))
        self.registry.register(OpenAIModelPlugin(model_name="gpt-4o"))
        self.registry.register(MockCognitiveModelPlugin())

        self.registry.register(FileEditorToolPlugin())
        self.registry.register(BashToolPlugin())
        self.registry.register(AstAnalyzerToolPlugin())
        self.registry.register(EvolvePluginToolPlugin(self.evolution_manager))
        self.registry.register(ReActLoopPlugin())
        self.registry.register(WorkingMemoryPlugin())

    def get_active_model(self, model_name: str | None = None) -> Any:
        models = self.registry.get_by_type(PluginType.MODEL)
        if not models:
            return None

        available = [model for model in models if model.self_test().get("passed", False)]
        if not available:
            return None

        if model_name:
            norm = model_name.lower()
            if "claude" in norm or "anthropic" in norm:
                found = next((m for m in available if isinstance(m, AnthropicModelPlugin)), None)
                if found:
                    return found
            elif "openai" in norm or "gpt" in norm or "5.0" in norm:
                found = next((m for m in available if isinstance(m, OpenAIModelPlugin)), None)
                if found:
                    return found
            elif "deepseek" in norm or "qwen" in norm:
                found = next((m for m in available if isinstance(m, DeepSeekModelPlugin)), None)
                if found:
                    return found

        local = next((m for m in available if isinstance(m, MockCognitiveModelPlugin)), None)
        return local or available[0]

    def get_memory(self) -> WorkingMemoryPlugin | None:
        memories = self.registry.get_by_type(PluginType.MEMORY)
        return memories[0] if memories and isinstance(memories[0], WorkingMemoryPlugin) else None

    async def run_turn(
        self,
        user_prompt: str,
        model_name: str | None = None,
        reasoning_effort: str | None = None,
        max_steps: int = 5,
    ) -> AsyncIterator[AgentEvent]:
        """Execute a conversational/task turn with autonomous multi-step ReAct cognitive loop."""
        turn_id = uuid.uuid4().hex[:8]
        yield AgentEvent("turn_start", {"turn_id": turn_id, "prompt": user_prompt})

        memory = self.get_memory()
        if memory:
            memory.append(ChatMessage(role=MessageRole.USER, content=user_prompt))

        model = self.get_active_model(model_name)
        if model is None:
            yield AgentEvent("error", {"message": "No active model plugin available."})
            return

        total_tool_calls = 0
        all_turn_text: list[str] = []

        for step in range(1, max_steps + 1):
            messages = memory.get_messages() if memory else [
                ChatMessage(role=MessageRole.USER, content=user_prompt)
            ]

            step_thought: list[str] = []
            step_text: list[str] = []
            step_tool_calls: list[ToolCall] = []

            async for chunk in model.generate_stream(messages):
                if chunk.thought:
                    step_thought.append(chunk.thought)
                    yield AgentEvent("thought_chunk", {"chunk": chunk.thought, "step": step})
                    await asyncio.sleep(0.02)

                if chunk.text:
                    step_text.append(chunk.text)
                    all_turn_text.append(chunk.text)
                    yield AgentEvent("text_chunk", {"chunk": chunk.text, "step": step})
                    await asyncio.sleep(0.01)

                if chunk.tool_calls:
                    step_tool_calls.extend(chunk.tool_calls)

            # If no tool calls produced in this step, cognitive turn is complete
            if not step_tool_calls:
                break

            total_tool_calls += len(step_tool_calls)
            step_tool_results: list[ToolResult] = []

            for call in step_tool_calls:
                yield AgentEvent("tool_call_start", {
                    "call_id": call.call_id,
                    "tool_name": call.tool_name,
                    "arguments": call.arguments,
                    "step": step,
                })

                res = await self._execute_tool_call(call)
                step_tool_results.append(res)

                yield AgentEvent("tool_call_done", {
                    "call_id": res.call_id,
                    "tool_name": res.tool_name,
                    "success": res.success,
                    "output": res.output,
                    "error": res.error,
                    "artifacts": res.artifacts,
                    "execution_time_ms": res.execution_time_ms,
                    "step": step,
                })

                if call.tool_name == "evolve_plugin" and res.success:
                    yield AgentEvent("plugin_evolved", {
                        "plugin_id": call.arguments.get("target_plugin_id"),
                        "status": res.output.get("status") if isinstance(res.output, dict) else "",
                        "snapshot_id": (
                            res.output.get("snapshot_id") if isinstance(res.output, dict) else None
                        ),
                        "plugin_mesh": [
                            p.model_dump(mode="json") for p in self.registry.list_plugins()
                        ],
                    })

                if "diff" in res.artifacts:
                    diff_chunk = DiffChunk(
                        chunk_id=uuid.uuid4().hex[:8],
                        file_path=call.arguments.get("path", "unknown"),
                        old_start=1,
                        old_lines=1,
                        new_start=1,
                        new_lines=1,
                        diff_text=res.artifacts["diff"],
                    )
                    self.pending_diffs.append(diff_chunk)
                    yield AgentEvent("diff_generated", diff_chunk.model_dump(mode="json"))

            # Append intermediate assistant step & tool results to working memory
            if memory:
                memory.append(
                    ChatMessage(
                        role=MessageRole.ASSISTANT,
                        content="".join(step_text),
                        thought="".join(step_thought) or None,
                        tool_calls=step_tool_calls,
                        tool_results=step_tool_results,
                    )
                )
                for tr in step_tool_results:
                    out_str = (
                        str(tr.output)
                        if tr.output is not None
                        else (f"Error: {tr.error}" if tr.error else "")
                    )
                    memory.append(
                        ChatMessage(
                            role=MessageRole.TOOL,
                            content=f"Tool [{tr.tool_name}] output: {out_str}",
                        )
                    )

        final_response = "".join(all_turn_text)
        if not final_response:
            final_response = "✓ 任务与工具调用执行完毕。"
            yield AgentEvent("text_chunk", {"chunk": final_response})

        yield AgentEvent(
            "turn_complete",
            {
                "turn_id": turn_id,
                "response": final_response,
                "tool_calls_count": total_tool_calls,
                "plugins_count": len(self.registry.list_plugins()),
            },
        )

    async def _execute_tool_call(self, call: ToolCall) -> ToolResult:
        plugin = self.registry.get(call.tool_name)
        if plugin is None:
            tools = self.registry.get_by_type(PluginType.TOOL)
            plugin = next((t for t in tools if t.metadata.id == call.tool_name), None)

        if plugin is None:
            return ToolResult(
                call_id=call.call_id,
                tool_name=call.tool_name,
                success=False,
                output=None,
                error=f"Tool plugin '{call.tool_name}' not found in active registry.",
            )

        if not hasattr(plugin, "execute"):
            return ToolResult(
                call_id=call.call_id,
                tool_name=call.tool_name,
                success=False,
                output=None,
                error=f"Plugin '{call.tool_name}' does not implement 'execute' method.",
            )

        loop = asyncio.get_running_loop()
        try:
            return await loop.run_in_executor(None, lambda: plugin.execute(**call.arguments))
        except Exception as e:
            return ToolResult(
                call_id=call.call_id,
                tool_name=call.tool_name,
                success=False,
                output=None,
                error=f"Tool execution exception: {e}",
            )
