from __future__ import annotations

import ast
import subprocess
import sys
import time
from pathlib import Path
from typing import Any

from aether.core.contracts import (
    EvolutionRequest,
    PluginAuthor,
    PluginContext,
    PluginMetadata,
    PluginType,
    ToolResult,
)

ALLOWED_WORKSPACE_COMMANDS: dict[str, list[str]] = {
    "pwd": ["pwd"],
    "git status --short": ["git", "status", "--short"],
    "python --version": [sys.executable, "--version"],
}


def run_allowlisted_command(
    command: str, workspace: str, timeout_s: int = 30
) -> subprocess.CompletedProcess[str]:
    argv = ALLOWED_WORKSPACE_COMMANDS.get(command.strip())
    if argv is None:
        allowed = ", ".join(sorted(ALLOWED_WORKSPACE_COMMANDS))
        raise ValueError(f"Command is not allow-listed. Allowed operations: {allowed}")
    return subprocess.run(
        argv,
        cwd=workspace,
        capture_output=True,
        text=True,
        timeout=min(max(timeout_s, 1), 30),
        check=False,
    )


class FileEditorToolPlugin:
    def __init__(self) -> None:
        self.metadata = PluginMetadata(
            id="file_editor",
            name="File & Workspace Editor",
            version="1.1.0",
            type=PluginType.TOOL,
            description="Safe file reading, writing, contiguous editing and directory listing",
            author=PluginAuthor.SYSTEM,
            tags=["core", "filesystem", "editor"],
        )
        self.context: PluginContext | None = None

    def on_load(self, context: PluginContext) -> None:
        self.context = context

    def on_unload(self) -> None:
        self.context = None

    def get_schema(self) -> dict[str, Any]:
        return {
            "name": "file_editor",
            "description": "File system manipulation and file editor tool",
            "parameters": {
                "type": "object",
                "properties": {
                    "action": {"type": "string", "enum": ["read", "write", "edit", "list_dir"]},
                    "path": {"type": "string"},
                    "content": {"type": "string"},
                    "target_content": {"type": "string"},
                    "replacement_content": {"type": "string"},
                },
                "required": ["action", "path"],
            },
        }

    def self_test(self) -> dict[str, Any]:
        return {"passed": True, "workspace": self.context.workspace if self.context else "."}

    def execute(self, action: str, path: str, **kwargs: Any) -> ToolResult:
        start = time.monotonic()
        ws_root = Path(self.context.workspace if self.context else ".").resolve()
        target = (ws_root / path).resolve()

        if target != ws_root and ws_root not in target.parents:
            return ToolResult(
                call_id="call_file",
                tool_name="file_editor",
                success=False,
                output=None,
                error=f"Path escape detected: {path}",
                execution_time_ms=(time.monotonic() - start) * 1000,
            )

        try:
            if action == "list_dir":
                if not target.exists():
                    items = []
                else:
                    items = sorted([
                        {
                            "name": p.name,
                            "is_dir": p.is_dir(),
                            "size": p.stat().st_size if p.is_file() else 0,
                        }
                        for p in target.iterdir()
                        if not p.name.startswith((".git", ".pytest_cache", "__pycache__", ".venv"))
                    ], key=lambda x: (not x["is_dir"], x["name"]))
                return ToolResult(
                    call_id="call_file",
                    tool_name="file_editor",
                    success=True,
                    output=items,
                    execution_time_ms=(time.monotonic() - start) * 1000,
                )

            elif action == "read":
                if not target.is_file():
                    raise FileNotFoundError(f"File not found: {path}")
                content = target.read_text(encoding="utf-8")
                return ToolResult(
                    call_id="call_file",
                    tool_name="file_editor",
                    success=True,
                    output=content,
                    execution_time_ms=(time.monotonic() - start) * 1000,
                )

            elif action == "write":
                content = kwargs.get("content", "")
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(content, encoding="utf-8")
                return ToolResult(
                    call_id="call_file",
                    tool_name="file_editor",
                    success=True,
                    output=f"Successfully wrote {len(content)} bytes to {path}",
                    execution_time_ms=(time.monotonic() - start) * 1000,
                )

            elif action == "edit":
                target_str = kwargs.get("target_content", "")
                replacement = kwargs.get("replacement_content", "")
                if not target.is_file():
                    raise FileNotFoundError(f"File not found: {path}")
                original = target.read_text(encoding="utf-8")
                if target_str not in original:
                    raise ValueError("target_content not found in file")
                modified = original.replace(target_str, replacement, 1)
                target.write_text(modified, encoding="utf-8")
                return ToolResult(
                    call_id="call_file",
                    tool_name="file_editor",
                    success=True,
                    output=f"Successfully updated {path}",
                    artifacts={"diff": f"- {target_str}\n+ {replacement}"},
                    execution_time_ms=(time.monotonic() - start) * 1000,
                )

            return ToolResult(
                call_id="call_file",
                tool_name="file_editor",
                success=False,
                output=None,
                error=f"Unknown action: {action}",
                execution_time_ms=(time.monotonic() - start) * 1000,
            )

        except Exception as e:
            return ToolResult(
                call_id="call_file",
                tool_name="file_editor",
                success=False,
                output=None,
                error=str(e),
                execution_time_ms=(time.monotonic() - start) * 1000,
            )


class BashToolPlugin:
    def __init__(self) -> None:
        self.metadata = PluginMetadata(
            id="bash_executor",
            name="Allow-listed Workspace Commands",
            version="1.0.0",
            type=PluginType.TOOL,
            description="Run a fixed set of read-only workspace diagnostics",
            author=PluginAuthor.SYSTEM,
            tags=["core", "workspace", "diagnostics"],
        )
        self.context: PluginContext | None = None

    def on_load(self, context: PluginContext) -> None:
        self.context = context

    def on_unload(self) -> None:
        self.context = None

    def get_schema(self) -> dict[str, Any]:
        return {
            "name": "bash_executor",
            "description": "Run an allow-listed workspace diagnostic",
            "parameters": {
                "type": "object",
                "properties": {
                    "command": {
                        "type": "string",
                        "enum": sorted(ALLOWED_WORKSPACE_COMMANDS),
                    },
                    "timeout_s": {"type": "integer", "default": 30},
                },
                "required": ["command"],
            },
        }

    def self_test(self) -> dict[str, Any]:
        return {"passed": True}

    def execute(self, command: str, timeout_s: int = 30) -> ToolResult:
        start = time.monotonic()
        ws = self.context.workspace if self.context else "."
        try:
            res = run_allowlisted_command(command, ws, timeout_s)
            return ToolResult(
                call_id="call_bash",
                tool_name="bash_executor",
                success=res.returncode == 0,
                output={"stdout": res.stdout, "stderr": res.stderr, "returncode": res.returncode},
                execution_time_ms=(time.monotonic() - start) * 1000,
            )
        except Exception as e:
            return ToolResult(
                call_id="call_bash",
                tool_name="bash_executor",
                success=False,
                output=None,
                error=str(e),
                execution_time_ms=(time.monotonic() - start) * 1000,
            )


class AstAnalyzerToolPlugin:
    def __init__(self) -> None:
        self.metadata = PluginMetadata(
            id="ast_analyzer",
            name="Python AST & Code Intelligence",
            version="1.0.0",
            type=PluginType.TOOL,
            description="Static inspection of functions, classes, dependencies and imports",
            author=PluginAuthor.SYSTEM,
            tags=["analysis", "ast", "lint"],
        )

    def on_load(self, context: PluginContext) -> None:
        pass

    def on_unload(self) -> None:
        pass

    def get_schema(self) -> dict[str, Any]:
        return {
            "name": "ast_analyzer",
            "description": "Analyze Python source code AST",
            "parameters": {
                "type": "object",
                "properties": {"code": {"type": "string"}},
                "required": ["code"],
            },
        }

    def self_test(self) -> dict[str, Any]:
        return {"passed": True}

    def execute(self, code: str) -> ToolResult:
        start = time.monotonic()
        try:
            tree = ast.parse(code)
            classes = [n.name for n in ast.walk(tree) if isinstance(n, ast.ClassDef)]
            functions = [n.name for n in ast.walk(tree) if isinstance(n, ast.FunctionDef)]
            return ToolResult(
                call_id="call_ast",
                tool_name="ast_analyzer",
                success=True,
                output={
                    "classes": classes,
                    "functions": functions,
                    "node_count": len(list(ast.walk(tree))),
                },
                execution_time_ms=(time.monotonic() - start) * 1000,
            )
        except Exception as e:
            return ToolResult(
                call_id="call_ast",
                tool_name="ast_analyzer",
                success=False,
                output=None,
                error=str(e),
                execution_time_ms=(time.monotonic() - start) * 1000,
            )


class EvolvePluginToolPlugin:
    """Core meta-tool allowing the Agent to modify existing plugins or register new ones live."""

    def __init__(self, evolution_manager: Any = None) -> None:
        self.metadata = PluginMetadata(
            id="evolve_plugin",
            name="Meta Self-Evolution Tool",
            version="1.0.0",
            type=PluginType.TOOL,
            description="Author, verify in sandbox, and hot-swap Aether plugins live",
            author=PluginAuthor.SYSTEM,
            tags=["core", "meta", "self-modification", "hot-reload"],
        )
        self.evolution_manager = evolution_manager
        self.context: PluginContext | None = None

    def on_load(self, context: PluginContext) -> None:
        self.context = context

    def on_unload(self) -> None:
        self.context = None

    def get_schema(self) -> dict[str, Any]:
        return {
            "name": "evolve_plugin",
            "description": "Synthesize or update a plugin through verified sandbox evaluation",
            "parameters": {
                "type": "object",
                "properties": {
                    "target_plugin_id": {"type": "string"},
                    "plugin_name": {"type": "string"},
                    "plugin_type": {
                        "type": "string",
                        "enum": ["tool", "model", "cognitive_loop", "memory", "oracle"],
                    },
                    "description": {"type": "string"},
                    "plugin_code": {"type": "string"},
                    "test_code": {"type": "string"},
                },
                "required": [
                    "target_plugin_id",
                    "plugin_name",
                    "plugin_type",
                    "plugin_code",
                    "test_code",
                ],
            },
        }

    def self_test(self) -> dict[str, Any]:
        return {"passed": True, "has_manager": self.evolution_manager is not None}

    def execute(
        self,
        target_plugin_id: str,
        plugin_name: str,
        plugin_type: str,
        plugin_code: str,
        test_code: str,
        description: str = "",
        **kwargs: Any,
    ) -> ToolResult:
        start = time.monotonic()
        if not self.evolution_manager:
            return ToolResult(
                call_id="call_evolve",
                tool_name="evolve_plugin",
                success=False,
                output=None,
                error="EvolutionManager is not attached to tool.",
                execution_time_ms=(time.monotonic() - start) * 1000,
            )

        req = EvolutionRequest(
            target_plugin_id=target_plugin_id,
            plugin_name=plugin_name,
            plugin_type=PluginType(plugin_type),
            description=description,
            plugin_code=plugin_code,
            test_code=test_code,
            author=PluginAuthor.AGENT_SELF_EVOLVED,
        )

        res = self.evolution_manager.evolve_plugin(req)
        return ToolResult(
            call_id="call_evolve",
            tool_name="evolve_plugin",
            success=res.success,
            output={
                "status": res.status,
                "message": res.message,
                "snapshot_id": res.snapshot_id,
                "test_results": res.test_results,
            },
            error=None if res.success else res.message,
            execution_time_ms=(time.monotonic() - start) * 1000,
        )
