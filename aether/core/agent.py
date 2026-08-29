from __future__ import annotations

import ast
import asyncio
import logging
import re
import uuid
from collections.abc import AsyncIterator
from datetime import UTC, datetime
from pathlib import Path
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
    AskQuestionToolPlugin,
    AstAnalyzerToolPlugin,
    BashToolPlugin,
    EvolvePluginToolPlugin,
    FileEditorToolPlugin,
)

from .contracts import (
    ChatMessage,
    DiffChunk,
    MessageRole,
    PlanStepItem,
    PluginType,
    QueueTaskItem,
    SubagentReport,
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


class TaskQueueManager:
    """In-memory thread-safe user-queued task manager for continuous execution."""

    def __init__(self) -> None:
        self._queue: list[QueueTaskItem] = []
        self._lock = asyncio.Lock()

    async def push(
        self, prompt: str, priority: int = 0, metadata: dict[str, Any] | None = None
    ) -> QueueTaskItem:
        async with self._lock:
            task_id = f"queue-{uuid.uuid4().hex[:6]}"
            item = QueueTaskItem(
                task_id=task_id,
                prompt=prompt,
                priority=priority,
                metadata=metadata or {},
            )
            inserted = False
            for idx, existing in enumerate(self._queue):
                if priority > existing.priority:
                    self._queue.insert(idx, item)
                    inserted = True
                    break
            if not inserted:
                self._queue.append(item)
            return item

    async def pop(self) -> QueueTaskItem | None:
        async with self._lock:
            return self._queue.pop(0) if self._queue else None

    async def peek(self) -> QueueTaskItem | None:
        async with self._lock:
            return self._queue[0] if self._queue else None

    async def remove(self, task_id: str) -> bool:
        async with self._lock:
            for idx, item in enumerate(self._queue):
                if item.task_id == task_id:
                    self._queue.pop(idx)
                    return True
            return False

    async def list_tasks(self) -> list[QueueTaskItem]:
        async with self._lock:
            return list(self._queue)

    async def clear(self) -> int:
        async with self._lock:
            count = len(self._queue)
            self._queue.clear()
            return count

    def size(self) -> int:
        return len(self._queue)


class SubagentReviewer:
    """Background autonomous code review & security auditing subagent.

    Uses real ``ast`` analysis to detect dangerous patterns, falling back
    to heuristic text scanning for non-parseable diffs.
    """

    DANGEROUS_CALLS: set[str] = {"eval", "exec", "compile", "__import__", "breakpoint"}
    DANGEROUS_ATTRS: set[str] = {
        "os.system", "os.popen", "os.exec", "os.execv", "os.execvp",
        "subprocess.call", "subprocess.run", "subprocess.Popen",
        "shutil.rmtree",
    }

    def __init__(self, sandbox: PluginSandbox) -> None:
        self.sandbox = sandbox

    def review_diff(self, diff_chunk: DiffChunk) -> SubagentReport:
        subagent_id = f"subagent-rev-{uuid.uuid4().hex[:6]}"
        findings: list[str] = []
        suggestions: list[str] = []
        score = 100

        # Extract added Python lines from the diff
        added_lines: list[str] = []
        for raw_line in diff_chunk.diff_text.splitlines():
            if raw_line.startswith("+") and not raw_line.startswith("+++"):
                added_lines.append(raw_line[1:])

        source = "\n".join(added_lines)

        # Try real AST analysis first
        ast_parsed = False
        try:
            tree = ast.parse(source, mode="exec")
            ast_parsed = True
            self._walk_ast(tree, findings, suggestions)
        except SyntaxError:
            # Diff fragment is not valid Python — fall back to text heuristics
            self._heuristic_scan(source, findings, suggestions)

        # Deduct score based on findings severity
        for f in findings:
            if "eval/exec/compile" in f or "os.system" in f or "subprocess" in f:
                score -= 30
            elif "__import__" in f or "breakpoint" in f:
                score -= 20
            elif "star import" in f or "超过 100 行" in f:
                score -= 10

        if not findings:
            method = "AST 深度分析" if ast_parsed else "启发式扫描"
            findings.append(f"✓ 代码差异通过{method}安全检查，未发现越权或危险调用。")
            suggestions.append("建议运行完整的 pytest 与 ael doctor 门禁。")

        return SubagentReport(
            subagent_id=subagent_id,
            role="reviewer",
            status="completed",
            score=max(0, score),
            findings=findings,
            suggestions=suggestions,
            diff_id=diff_chunk.chunk_id,
        )

    def _walk_ast(
        self,
        tree: ast.AST,
        findings: list[str],
        suggestions: list[str],
    ) -> None:
        """Walk the AST looking for dangerous patterns."""
        for node in ast.walk(tree):
            # Direct dangerous function calls: eval(...), exec(...), etc.
            if isinstance(node, ast.Call):
                func = node.func
                if isinstance(func, ast.Name) and func.id in self.DANGEROUS_CALLS:
                    findings.append(
                        f"⚠️ 检测到危险调用 (eval/exec/compile): `{func.id}()` — 存在沙箱逃逸风险。"
                    )
                    suggestions.append(f"推荐使用 AST 解析或静态字典映射替换 `{func.id}()`。")

                # Attribute calls: os.system(...), subprocess.run(...), etc.
                if isinstance(func, ast.Attribute):
                    qual = self._qualified_name(func)
                    if qual and any(qual.endswith(d) for d in self.DANGEROUS_ATTRS):
                        findings.append(f"⚠️ 检测到危险系统调用: `{qual}()`。")
                        suggestions.append(
                            "使用标准沙箱隔离子进程 run_allowlisted_command。"
                        )

            # Star imports: from x import *
            if isinstance(node, ast.ImportFrom) and node.names:
                for alias in node.names:
                    if alias.name == "*":
                        msg = (
                            f"ℹ️ 检测到 star import: "
                            f"`from {node.module} import *` — 可能引入不安全符号。"
                        )
                        findings.append(msg)

    def _heuristic_scan(
        self,
        text: str,
        findings: list[str],
        suggestions: list[str],
    ) -> None:
        """Fallback text-based heuristic when AST parsing fails."""
        danger_patterns = [
            (r"\beval\s*\(", "eval/exec/compile", "推荐使用 AST 解析或静态字典映射替换动态执行。"),
            (r"\bexec\s*\(", "eval/exec/compile", "推荐使用 AST 解析或静态字典映射替换动态执行。"),
            (r"\bcompile\s*\(", "eval/exec/compile", "避免使用 compile() 动态编译代码。"),
            (r"\bos\.system\s*\(", "os.system", "使用标准沙箱隔离子进程 run_allowlisted_command。"),
            (r"\bsubprocess\.(call|run|Popen)\s*\(", "subprocess", "使用标准沙箱隔离子进程。"),
            (r"\b__import__\s*\(", "__import__", "使用标准 import 语句。"),
            (r"\bbreakpoint\s*\(", "breakpoint", "移除调试用 breakpoint() 调用。"),
        ]
        seen: set[str] = set()
        for pattern, label, suggestion in danger_patterns:
            if re.search(pattern, text) and label not in seen:
                seen.add(label)
                findings.append(f"⚠️ 检测到潜在危险代码 ({label})。")
                suggestions.append(suggestion)

        if len(text.splitlines()) > 100:
            findings.append("ℹ️ 单个代码变更块超过 100 行。")
            suggestions.append("建议拆分为多个原子提交以便于进行分层测试。")

    @staticmethod
    def _qualified_name(node: ast.Attribute) -> str | None:
        """Reconstruct a dotted name from an ast.Attribute chain."""
        parts: list[str] = [node.attr]
        current = node.value
        while isinstance(current, ast.Attribute):
            parts.append(current.attr)
            current = current.value
        if isinstance(current, ast.Name):
            parts.append(current.id)
            return ".".join(reversed(parts))
        return None


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
        self.queue_manager = TaskQueueManager()
        self.subagent_reviewer = SubagentReviewer(self.sandbox)
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
        self.registry.register(AskQuestionToolPlugin())
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

    def _resolve_at_mentions(self, prompt: str) -> str:
        """Safely resolve @file mentions and attach content to prompt."""
        ws_path = Path(self.workspace).resolve()
        matches = re.findall(r'@([a-zA-Z0-9_\-\.\/]+)', prompt)
        if not matches:
            return prompt

        injected = []
        for ref in matches:
            target = (ws_path / ref).resolve()
            if target.is_file() and (target == ws_path or ws_path in target.parents):
                try:
                    content = target.read_text(encoding="utf-8", errors="replace")
                    if len(content) > 50_000:
                        content = content[:50_000] + "\n... [truncated]"
                    injected.append(f"\n[Attached File Context from @{ref}]:\n```\n{content}\n```")
                except Exception as err:
                    logger.debug(f"Failed to read file mention @{ref}: {err}")

        if injected:
            return prompt + "\n\n" + "\n".join(injected)
        return prompt

    def _extract_plan_steps(self, text: str) -> list[PlanStepItem]:
        """Extract structured plan steps from plan response text."""
        steps = []
        lines = text.splitlines()
        for _idx, line in enumerate(lines):
            clean = line.strip()
            m = re.match(r'^(?:(?:\d+\.|\-|\*)\s*(?:\[\s*\])?\s*|\d+\.\s+)(.+)$', clean)
            if m and len(m.group(1).strip()) > 3 and not clean.startswith("```"):
                step_title = m.group(1).strip()
                steps.append(
                    PlanStepItem(
                        step_id=f"step-{len(steps) + 1}",
                        title=step_title,
                        status="pending" if len(steps) > 0 else "running",
                    )
                )
        return steps[:8]

    @staticmethod
    def _parse_hunk_header(diff_text: str) -> tuple[int, int, int, int]:
        """Parse the first unified diff hunk header ``@@ -a,b +c,d @@``.

        Returns ``(old_start, old_lines, new_start, new_lines)``.
        Falls back to ``(1, 1, 1, 1)`` if no hunk header is found.
        """
        m = re.search(r"@@\s*-(\d+)(?:,(\d+))?\s*\+(\d+)(?:,(\d+))?\s*@@", diff_text)
        if m:
            return (
                int(m.group(1)),
                int(m.group(2)) if m.group(2) else 1,
                int(m.group(3)),
                int(m.group(4)) if m.group(4) else 1,
            )
        # Fallback: count added/removed lines manually
        lines = diff_text.splitlines()
        added = sum(
            1 for ln in lines
            if ln.startswith("+") and not ln.startswith("+++")
        )
        removed = sum(
            1 for ln in lines
            if ln.startswith("-") and not ln.startswith("---")
        )
        return (1, max(removed, 1), 1, max(added, 1))

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

        resolved_prompt = self._resolve_at_mentions(user_prompt)

        memory = self.get_memory()
        if memory:
            memory.append(ChatMessage(role=MessageRole.USER, content=resolved_prompt))

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

                if call.tool_name == "ask_question" and res.success:
                    ask_data = res.output if isinstance(res.output, dict) else {}
                    yield AgentEvent("ask_question", {
                        "call_id": res.call_id,
                        "question": ask_data.get("question", ""),
                        "options": ask_data.get("options", []),
                        "is_multi_select": ask_data.get("is_multi_select", False),
                        "allow_custom": ask_data.get("allow_custom", True),
                        "context": ask_data.get("context", ""),
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
                    diff_text = res.artifacts["diff"]
                    old_s, old_l, new_s, new_l = self._parse_hunk_header(diff_text)
                    diff_chunk = DiffChunk(
                        chunk_id=uuid.uuid4().hex[:8],
                        file_path=call.arguments.get("path", "unknown"),
                        old_start=old_s,
                        old_lines=old_l,
                        new_start=new_s,
                        new_lines=new_l,
                        diff_text=diff_text,
                    )
                    self.pending_diffs.append(diff_chunk)
                    yield AgentEvent("diff_generated", diff_chunk.model_dump(mode="json"))

                    # Parallel subagent reviewer evaluates diff security & safety
                    report = self.subagent_reviewer.review_diff(diff_chunk)
                    yield AgentEvent("subagent_review", report.model_dump(mode="json"))

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

        # If user initiated a plan, extract checklist steps
        _prompt_lower = user_prompt.lower().strip()
        _is_plan = (
            _prompt_lower.startswith("/plan")
            or _prompt_lower.startswith("[plan")
            or _prompt_lower.startswith("plan:")
            or any(kw in _prompt_lower for kw in ("计划", "方案", "步骤", "roadmap", "实施清单"))
        )
        if _is_plan:
            steps = self._extract_plan_steps(final_response)
            if steps:
                yield AgentEvent("plan_checklist", {
                    "steps": [s.model_dump(mode="json") for s in steps]
                })

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
