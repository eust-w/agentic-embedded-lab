from __future__ import annotations

from datetime import UTC, datetime
from enum import StrEnum
from typing import Any, Protocol, runtime_checkable

from pydantic import BaseModel, ConfigDict, Field


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid", validate_assignment=True)


class PluginType(StrEnum):
    MODEL = "model"
    TOOL = "tool"
    COGNITIVE_LOOP = "cognitive_loop"
    MEMORY = "memory"
    ORACLE = "oracle"
    UI_PANEL = "ui_panel"


class PluginAuthor(StrEnum):
    SYSTEM = "system"
    USER = "user"
    AGENT_SELF_EVOLVED = "agent_self_evolved"


class PluginStatus(StrEnum):
    ACTIVE = "active"
    STANDBY = "standby"
    QUARANTINED = "quarantined"
    DEPRECATED = "deprecated"
    FAILED = "failed"


class PluginMetadata(StrictModel):
    id: str = Field(pattern=r"^[a-zA-Z0-9_-]+$")
    name: str
    version: str = "0.1.0"
    type: PluginType
    description: str
    author: PluginAuthor = PluginAuthor.SYSTEM
    status: PluginStatus = PluginStatus.ACTIVE
    dependencies: list[str] = Field(default_factory=list)
    tags: list[str] = Field(default_factory=list)
    source_code: str | None = None
    created_at: datetime = Field(default_factory=lambda: datetime.now(UTC))
    updated_at: datetime = Field(default_factory=lambda: datetime.now(UTC))


class PluginContext:
    def __init__(self, registry: Any, event_bus: Any, workspace: str) -> None:
        self.registry = registry
        self.event_bus = event_bus
        self.workspace = workspace


@runtime_checkable
class AetherPlugin(Protocol):
    metadata: PluginMetadata

    def on_load(self, context: PluginContext) -> None:
        ...

    def on_unload(self) -> None:
        ...

    def get_schema(self) -> dict[str, Any]:
        ...

    def self_test(self) -> dict[str, Any]:
        ...


class ToolCall(StrictModel):
    call_id: str
    tool_name: str
    arguments: dict[str, Any] = Field(default_factory=dict)


class ToolResult(StrictModel):
    call_id: str
    tool_name: str
    success: bool
    output: Any
    error: str | None = None
    artifacts: dict[str, str] = Field(default_factory=dict)
    execution_time_ms: float = 0.0


class MessageRole(StrEnum):
    SYSTEM = "system"
    USER = "user"
    ASSISTANT = "assistant"
    TOOL = "tool"
    THINKING = "thinking"


class ChatMessage(StrictModel):
    role: MessageRole
    content: str
    thought: str | None = None
    tool_calls: list[ToolCall] = Field(default_factory=list)
    tool_results: list[ToolResult] = Field(default_factory=list)
    timestamp: datetime = Field(default_factory=lambda: datetime.now(UTC))


class DiffChunk(StrictModel):
    chunk_id: str
    file_path: str
    old_start: int
    old_lines: int
    new_start: int
    new_lines: int
    diff_text: str
    status: str = "pending"  # pending, accepted, rejected


class EvolutionRequest(StrictModel):
    target_plugin_id: str
    plugin_name: str
    plugin_type: PluginType
    description: str
    plugin_code: str
    test_code: str
    author: PluginAuthor = PluginAuthor.AGENT_SELF_EVOLVED
    reason: str = "User requested capability extension or dynamic self-adaptation"


class EvolutionResult(StrictModel):
    request_id: str
    target_plugin_id: str
    success: bool
    status: str  # "hot_swapped", "conformance_failed", "rolled_back", "syntax_error"
    message: str
    test_results: dict[str, Any] = Field(default_factory=dict)
    snapshot_id: str | None = None
    evolved_plugin_metadata: PluginMetadata | None = None


class QueueTaskItem(StrictModel):
    task_id: str
    prompt: str
    created_at: datetime = Field(default_factory=lambda: datetime.now(UTC))
    status: str = "queued"  # queued, running, completed, cancelled
    priority: int = 0
    metadata: dict[str, Any] = Field(default_factory=dict)


class PlanStepItem(StrictModel):
    step_id: str
    title: str
    description: str = ""
    status: str = "pending"  # pending, running, completed, failed, skipped
    tool_name: str | None = None
    result_summary: str | None = None


class SubagentReport(StrictModel):
    subagent_id: str
    role: str  # "reviewer", "security_auditor", "tester"
    status: str = "completed"  # running, completed, error
    score: int = 100
    findings: list[str] = Field(default_factory=list)
    suggestions: list[str] = Field(default_factory=list)
    diff_id: str | None = None
    timestamp: datetime = Field(default_factory=lambda: datetime.now(UTC))
