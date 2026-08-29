from __future__ import annotations

import json
import logging
import os
import re
from collections.abc import AsyncIterator
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException, Request, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import HTMLResponse, StreamingResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

from aether.core.agent import AetherAgent
from aether.core.config import aether_config
from aether.core.contracts import DiffChunk, EvolutionRequest
from aether.core.mcp_client import MCPClientManager
from aether.plugins.tools import run_allowlisted_command

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(name)s: %(message)s")
logger = logging.getLogger("aether.server")

app = FastAPI(title="Aether Agentic Platform", version="0.2.0")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://127.0.0.1:8765", "http://localhost:8765"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

WORKSPACE_DIR = aether_config.workspace_dir
agent = AetherAgent(workspace=str(WORKSPACE_DIR))
mcp_manager = MCPClientManager()
UI_DIR = Path(__file__).parent / "ui"
SAFE_IDENTIFIER = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")


def _safe_identifier(value: str, label: str) -> str:
    if not SAFE_IDENTIFIER.fullmatch(value):
        raise HTTPException(status_code=422, detail=f"Invalid {label}")
    return value


class ChatRequest(BaseModel):
    message: str


class DiffActionRequest(BaseModel):
    chunk_id: str
    action: str


class RollbackRequest(BaseModel):
    plugin_id: str
    snapshot_id: str | None = None


class ConfigUpdateRequest(BaseModel):
    gateway_url: str | None = None
    active_model: str | None = None
    reasoning_effort: str | None = None
    permission_mode: str | None = None
    theme: str | None = None
    user_name: str | None = None


@app.get("/api/health")
async def health() -> dict[str, Any]:
    return {
        "status": "healthy",
        "agent": "Aether-Native",
        "plugins_count": len(agent.registry.list_plugins()),
        "workspace": str(WORKSPACE_DIR),
        "config_home": str(aether_config.home_dir),
    }


@app.get("/api/config")
async def get_config() -> dict[str, Any]:
    return {
        **aether_config.data.model_dump(),
        "api_key_configured": any(
            os.getenv(name)
            for name in ("DEEPSEEK_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY")
        ),
    }


@app.post("/api/config")
async def update_config(req: ConfigUpdateRequest) -> dict[str, Any]:
    updates = {k: v for k, v in req.model_dump().items() if v is not None}
    updated = aether_config.update(**updates)
    return updated.model_dump()


class CreateProjectRequest(BaseModel):
    name: str


class CreateTaskRequest(BaseModel):
    project: str
    title: str


@app.get("/api/workspace/tree")
async def get_workspace_tree() -> list[dict[str, Any]]:
    return aether_config.get_projects_tree()


@app.post("/api/workspace/create_project")
async def create_project(req: CreateProjectRequest) -> dict[str, Any]:
    name = _safe_identifier(req.name, "project name")
    proj_dir = aether_config.workspace_dir / name
    proj_dir.mkdir(parents=True, exist_ok=True)
    return {"status": "success", "project": name, "path": str(proj_dir)}


@app.post("/api/workspace/create_task")
async def create_task(req: CreateTaskRequest) -> dict[str, Any]:
    project = _safe_identifier(req.project, "project")
    task_id = f"task-{project}-{int(Path(__file__).stat().st_mtime * 1000)}"
    session_file = aether_config.sessions_dir / f"{project}_{task_id}.json"
    session_data = {
        "id": task_id,
        "project": project,
        "title": req.title,
        "created_at": str(Path(__file__).stat().st_mtime),
    }
    dumped = json.dumps(session_data, indent=2, ensure_ascii=False)
    session_file.write_text(dumped, encoding="utf-8")
    return {"status": "success", "task": session_data}


class RenameTaskRequest(BaseModel):
    task_id: str
    new_title: str


class DeleteTaskRequest(BaseModel):
    task_id: str


class TerminalExecRequest(BaseModel):
    command: str


@app.get("/api/workspace/files")
async def get_workspace_files() -> list[dict[str, Any]]:
    files = []
    ws = aether_config.workspace_dir
    if ws.exists():
        for path in ws.rglob("*"):
            if path.is_file() and not any(p.startswith(".") for p in path.parts):
                rel = path.relative_to(ws)
                files.append({
                    "path": str(rel),
                    "name": path.name,
                    "size": path.stat().st_size,
                })
    return files[:100]


@app.post("/api/workspace/rename_task")
async def rename_task(req: RenameTaskRequest) -> dict[str, Any]:
    task_id = _safe_identifier(req.task_id, "task id")
    for sf in aether_config.sessions_dir.glob("*.json"):
        try:
            data = json.loads(sf.read_text(encoding="utf-8"))
            if data.get("id") == task_id:
                data["title"] = req.new_title
                sf.write_text(json.dumps(data, indent=2, ensure_ascii=False), encoding="utf-8")
                return {"status": "success", "task": data}
        except Exception:
            pass
    return {"status": "success", "task": {"id": task_id, "title": req.new_title}}


class DeleteProjectRequest(BaseModel):
    project: str


@app.post("/api/workspace/delete_project")
async def delete_project(req: DeleteProjectRequest) -> dict[str, Any]:
    import shutil
    project = _safe_identifier(req.project, "project")
    proj_dir = aether_config.workspace_dir / project
    if proj_dir.exists() and proj_dir.is_dir():
        shutil.rmtree(proj_dir, ignore_errors=True)
    for sf in aether_config.sessions_dir.glob(f"{project}_*.json"):
        sf.unlink(missing_ok=True)
    return {"status": "success", "deleted_project": project}


@app.post("/api/workspace/delete_task")
async def delete_task(req: DeleteTaskRequest) -> dict[str, Any]:
    task_id = _safe_identifier(req.task_id, "task id")
    for sf in aether_config.sessions_dir.glob("*.json"):
        try:
            data = json.loads(sf.read_text(encoding="utf-8"))
            if data.get("id") == task_id or sf.stem.endswith(task_id):
                sf.unlink(missing_ok=True)
                return {"status": "success", "deleted": task_id}
        except Exception:
            pass
    return {"status": "success", "deleted": task_id}


@app.post("/api/terminal/exec")
async def terminal_exec(req: TerminalExecRequest) -> dict[str, Any]:
    cmd = req.command.strip()
    if not cmd:
        return {"output": "", "exit_code": 0}
    try:
        res = run_allowlisted_command(cmd, str(aether_config.workspace_dir), 10)
        out = (res.stdout or "") + (res.stderr or "")
        return {"output": out, "exit_code": res.returncode}
    except Exception as e:
        raise HTTPException(status_code=422, detail=str(e)) from e


@app.get("/api/plugins")
async def list_plugins() -> list[dict[str, Any]]:
    return [p.model_dump(mode="json") for p in agent.registry.list_plugins()]


@app.post("/api/plugins/evolve")
async def evolve_plugin(req: EvolutionRequest) -> dict[str, Any]:
    res = agent.evolution_manager.evolve_plugin(req)
    return res.model_dump(mode="json")


@app.post("/api/plugins/rollback")
async def rollback_plugin(req: RollbackRequest) -> dict[str, Any]:
    res = agent.evolution_manager.rollback(req.plugin_id, req.snapshot_id)
    return res.model_dump(mode="json")


@app.get("/api/diffs")
async def list_diffs() -> list[dict[str, Any]]:
    return [d.model_dump(mode="json") for d in agent.pending_diffs]


@app.post("/api/diffs/action")
async def diff_action(req: DiffActionRequest) -> dict[str, Any]:
    if req.action not in ("accept", "reject"):
        return {
            "success": False,
            "error": f"Invalid action: {req.action!r}. Must be 'accept' or 'reject'.",
        }

    for i, d in enumerate(agent.pending_diffs):
        if d.chunk_id == req.chunk_id:
            if req.action == "accept":
                # Actually apply the diff: extract new lines and write to file
                applied = _apply_diff_to_file(d, agent.workspace)
                d.status = "accepted"
                agent.pending_diffs.pop(i)
                return {"success": True, "applied": applied, "chunk": d.model_dump(mode="json")}
            else:
                d.status = "rejected"
                agent.pending_diffs.pop(i)
                return {"success": True, "applied": False, "chunk": d.model_dump(mode="json")}
    return {"success": False, "error": "Diff chunk not found"}


def _apply_diff_to_file(chunk: DiffChunk, workspace: str) -> bool:
    """Apply accepted diff content to the target file.

    Extracts lines prefixed with '+' (additions) from the unified diff text
    and writes them to the file.  The approach is intentionally conservative:
    it only appends new content rather than doing a full patch merge, which
    avoids destructive edits when the hunk metadata is incomplete.
    """
    target = Path(workspace) / chunk.file_path
    # Safety: resolve and reject path escapes
    try:
        resolved = target.resolve()
        ws_resolved = Path(workspace).resolve()
        if not str(resolved).startswith(str(ws_resolved)):
            logger.warning("Diff apply blocked: path escape detected for %s", chunk.file_path)
            return False
    except (OSError, ValueError):
        return False

    new_lines: list[str] = []
    for raw_line in chunk.diff_text.splitlines():
        if raw_line.startswith("+ ") or raw_line.startswith("+\t"):
            new_lines.append(raw_line[1:])  # strip leading '+'
        elif raw_line.startswith("+") and len(raw_line) > 1 and not raw_line.startswith("+++"):
            new_lines.append(raw_line[1:])

    if not new_lines:
        return False

    try:
        target.parent.mkdir(parents=True, exist_ok=True)
        existing = target.read_text(encoding="utf-8") if target.exists() else ""
        # Append new content separated by a newline
        new_content = "\n".join(new_lines) + "\n"
        merged = (
            existing.rstrip("\n") + "\n" + new_content
            if existing
            else new_content
        )
        target.write_text(merged, encoding="utf-8")
        logger.info("Diff applied to %s (%d new lines)", chunk.file_path, len(new_lines))
        return True
    except OSError as exc:
        logger.error("Failed to apply diff to %s: %s", chunk.file_path, exc)
        return False


class QueuePushRequest(BaseModel):
    prompt: str
    priority: int = 0
    metadata: dict[str, Any] = {}


class QueueRemoveRequest(BaseModel):
    task_id: str


@app.get("/api/queue")
async def list_queue() -> list[dict[str, Any]]:
    tasks = await agent.queue_manager.list_tasks()
    return [t.model_dump(mode="json") for t in tasks]


@app.post("/api/queue/push")
async def push_queue(req: QueuePushRequest) -> dict[str, Any]:
    task = await agent.queue_manager.push(req.prompt, req.priority, req.metadata)
    return {
        "status": "success",
        "task": task.model_dump(mode="json"),
        "size": agent.queue_manager.size(),
    }


@app.post("/api/queue/pop")
async def pop_queue() -> dict[str, Any]:
    task = await agent.queue_manager.pop()
    return {
        "status": "success",
        "task": task.model_dump(mode="json") if task else None,
        "size": agent.queue_manager.size(),
    }


@app.post("/api/queue/remove")
async def remove_queue(req: QueueRemoveRequest) -> dict[str, Any]:
    removed = await agent.queue_manager.remove(req.task_id)
    return {
        "status": "success",
        "removed": removed,
        "size": agent.queue_manager.size(),
    }


@app.post("/api/queue/clear")
async def clear_queue() -> dict[str, Any]:
    cleared = await agent.queue_manager.clear()
    return {"status": "success", "cleared": cleared}


@app.post("/api/subagents/review")
async def review_subagent(req: DiffActionRequest) -> dict[str, Any]:
    chunk = next((d for d in agent.pending_diffs if d.chunk_id == req.chunk_id), None)
    if not chunk:
        return {"status": "error", "message": "Diff chunk not found"}
    report = agent.subagent_reviewer.review_diff(chunk)
    return {"status": "success", "report": report.model_dump(mode="json")}


@app.post("/api/chat")
async def chat_endpoint(req: ChatRequest) -> dict[str, Any]:
    """Fallback non-streaming turn execution."""
    full_text: list[str] = []
    full_thought: list[str] = []
    async for event in agent.run_turn(req.message):
        if event.event_type == "text_chunk":
            full_text.append(event.data.get("chunk", ""))
        elif event.event_type == "thought_chunk":
            full_thought.append(event.data.get("chunk", ""))
    return {
        "response": "".join(full_text),
        "thought": "".join(full_thought),
        "plugins_count": len(agent.registry.list_plugins()),
    }


@app.get("/api/git/status")
async def git_status() -> dict[str, Any]:
    """Run git status in the workspace."""
    import subprocess
    try:
        result = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=agent.workspace,
            capture_output=True, text=True, timeout=10,
        )
        files = []
        for line in result.stdout.strip().splitlines():
            if len(line) >= 3:
                status_code = line[:2].strip()
                filepath = line[3:]
                files.append({"status": status_code, "path": filepath})
        branch_res = subprocess.run(
            ["git", "branch", "--show-current"],
            cwd=agent.workspace,
            capture_output=True, text=True, timeout=5,
        )
        branch = branch_res.stdout.strip()
        return {"branch": branch, "files": files, "clean": len(files) == 0}
    except Exception as e:
        return {"error": str(e), "files": [], "branch": "unknown"}


@app.get("/api/git/log")
async def git_log() -> list[dict[str, Any]]:
    """Return recent git commits."""
    import subprocess
    try:
        result = subprocess.run(
            ["git", "log", "--oneline", "-20", "--format=%H|%h|%s|%an|%ar"],
            cwd=agent.workspace,
            capture_output=True, text=True, timeout=10,
        )
        commits = []
        for line in result.stdout.strip().splitlines():
            parts = line.split("|", 4)
            if len(parts) == 5:
                commits.append({
                    "hash": parts[0],
                    "short_hash": parts[1],
                    "message": parts[2],
                    "author": parts[3],
                    "time_ago": parts[4],
                })
        return commits
    except Exception as e:
        return [{"error": str(e)}]


@app.get("/api/git/diff")
async def git_diff() -> dict[str, Any]:
    """Return current git diff."""
    import subprocess
    try:
        result = subprocess.run(
            ["git", "diff"],
            cwd=agent.workspace,
            capture_output=True, text=True, timeout=10,
        )
        return {"diff": result.stdout[:50000]}
    except Exception as e:
        return {"error": str(e), "diff": ""}


class GitCommitRequest(BaseModel):
    message: str


@app.post("/api/git/commit")
async def git_commit(req: GitCommitRequest) -> dict[str, Any]:
    """Stage all changes and commit."""
    import subprocess
    try:
        subprocess.run(
            ["git", "add", "-A"],
            cwd=agent.workspace, timeout=10,
        )
        result = subprocess.run(
            ["git", "commit", "-m", req.message],
            cwd=agent.workspace,
            capture_output=True, text=True, timeout=10,
        )
        return {
            "success": result.returncode == 0,
            "output": result.stdout + result.stderr,
        }
    except Exception as e:
        return {"success": False, "output": str(e)}


@app.get("/api/workspace/read_file")
async def read_workspace_file(path: str) -> dict[str, Any]:
    """Read a file from the workspace."""
    ws = Path(agent.workspace).resolve()
    target = (ws / path).resolve()
    if not str(target).startswith(str(ws)):
        return {"error": "Path escape detected"}
    if not target.is_file():
        return {"error": "File not found"}
    try:
        content = target.read_text(encoding="utf-8", errors="replace")
        if len(content) > 200_000:
            content = content[:200_000] + "\n... [truncated]"
        return {
            "path": path,
            "content": content,
            "size": target.stat().st_size,
        }
    except Exception as e:
        return {"error": str(e)}


class WriteFileRequest(BaseModel):
    path: str
    content: str


@app.post("/api/workspace/write_file")
async def write_workspace_file(req: WriteFileRequest) -> dict[str, Any]:
    """Write content to a file in the workspace."""
    ws = Path(agent.workspace).resolve()
    target = (ws / req.path).resolve()
    if not str(target).startswith(str(ws)):
        return {"success": False, "error": "Path escape detected"}
    try:
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(req.content, encoding="utf-8")
        return {
            "success": True,
            "path": req.path,
            "size": len(req.content.encode("utf-8")),
        }
    except Exception as e:
        return {"success": False, "error": str(e)}


class MCPConnectRequest(BaseModel):
    name: str
    command: list[str]


@app.get("/api/mcp/servers")
async def list_mcp_servers() -> list[dict[str, Any]]:
    return mcp_manager.to_dict()


@app.post("/api/mcp/connect")
async def connect_mcp(req: MCPConnectRequest) -> dict[str, Any]:
    mcp_manager.add_server(req.name, req.command)
    server = mcp_manager.connect(req.name)
    return {
        "name": server.name,
        "status": server.status,
        "error": server.error,
        "tools_count": len(server.tools),
        "tools": [
            {"name": t.name, "description": t.description}
            for t in server.tools
        ],
    }


@app.get("/api/mcp/tools")
async def list_mcp_tools() -> list[dict[str, Any]]:
    return [
        {
            "name": t.name,
            "description": t.description,
            "server": t.server_name,
        }
        for t in mcp_manager.list_all_tools()
    ]


@app.get("/api/plugins/marketplace")
async def plugin_marketplace() -> list[dict[str, Any]]:
    """Return available plugins for the marketplace."""
    return [
        {
            "id": "tool-web-search",
            "name": "网络搜索工具",
            "description": "调用搜索引擎 API 实现实时信息检索",
            "category": "tool",
            "installed": False,
        },
        {
            "id": "tool-code-interpreter",
            "name": "代码解释器",
            "description": "沙箱内安全执行 Python 代码并返回结果",
            "category": "tool",
            "installed": False,
        },
        {
            "id": "model-adapter-ollama",
            "name": "Ollama 本地模型适配器",
            "description": "连接本地 Ollama 服务运行开源模型",
            "category": "model",
            "installed": False,
        },
        {
            "id": "loop-tree-of-thought",
            "name": "Tree-of-Thought 推理循环",
            "description": "树状思维推理循环插件，用于复杂问题分解",
            "category": "loop",
            "installed": False,
        },
        {
            "id": "memory-vector-store",
            "name": "向量记忆库",
            "description": "基于向量检索的长期记忆插件",
            "category": "memory",
            "installed": False,
        },
    ]


class PluginInstallRequest(BaseModel):
    plugin_id: str


@app.post("/api/plugins/install")
async def install_plugin(
    req: PluginInstallRequest,
) -> dict[str, Any]:
    """Install a plugin from the marketplace (placeholder)."""
    return {
        "success": True,
        "plugin_id": req.plugin_id,
        "message": f"Plugin {req.plugin_id} queued for install.",
    }


class SaveSessionRequest(BaseModel):
    task_id: str
    project: str
    title: str
    history_html: str
    messages: list[dict[str, Any]] = []


@app.get("/api/workspace/sessions")
async def list_sessions() -> list[dict[str, Any]]:
    """List all saved sessions sorted by updated_at descending."""
    sessions = []
    for sf in aether_config.sessions_dir.glob("*.json"):
        try:
            data = json.loads(sf.read_text(encoding="utf-8"))
            sessions.append({
                "id": data.get("id", sf.stem),
                "project": data.get("project", "unknown"),
                "title": data.get("title", sf.stem),
                "updated_at": data.get("updated_at", ""),
            })
        except Exception:
            pass
    sessions.sort(key=lambda s: s.get("updated_at", ""), reverse=True)
    return sessions


@app.post("/api/upload")
async def upload_file(req: Request) -> dict[str, Any]:
    """Accept a file upload and save to workspace uploads dir."""
    form = await req.form()
    uploaded = form.get("file")
    if not uploaded:
        return {"success": False, "error": "No file provided"}

    filename = getattr(uploaded, "filename", "upload")
    safe_name = re.sub(r'[^a-zA-Z0-9._\-]', '_', filename)
    uploads_dir = Path(agent.workspace) / ".aether_uploads"
    uploads_dir.mkdir(parents=True, exist_ok=True)
    target = uploads_dir / safe_name

    # Safety check
    if not str(target.resolve()).startswith(
        str(Path(agent.workspace).resolve())
    ):
        return {"success": False, "error": "Path escape"}

    content = await uploaded.read()
    target.write_bytes(content)

    # For text files, return content preview
    preview = ""
    try:
        text = content.decode("utf-8", errors="replace")
        preview = text[:5000]
    except Exception:
        preview = f"[Binary file: {len(content)} bytes]"

    return {
        "success": True,
        "filename": safe_name,
        "path": str(target.relative_to(Path(agent.workspace))),
        "size": len(content),
        "preview": preview,
    }


@app.get("/api/workspace/export_session")
async def export_session(
    task_id: str, project: str, fmt: str = "md",
) -> dict[str, Any]:
    """Export a session as structured Markdown or JSON."""
    project = _safe_identifier(project, "project")
    task_id = _safe_identifier(task_id, "task id")
    sf = aether_config.sessions_dir / f"{project}_{task_id}.json"
    if not sf.exists():
        return {"error": "Session not found"}
    data = json.loads(sf.read_text(encoding="utf-8"))

    if fmt == "json":
        return data

    # Build Markdown
    lines = [
        f"# {data.get('title', task_id)}",
        "",
        f"**Project**: {data.get('project', 'N/A')}",
        f"**Updated**: {data.get('updated_at', 'N/A')}",
        "",
        "---",
        "",
    ]
    for msg in data.get("messages", []):
        role = msg.get("role", "unknown").upper()
        content = msg.get("content", "")
        lines.append(f"### {role}")
        lines.append("")
        lines.append(content)
        lines.append("")

    if not data.get("messages"):
        lines.append("*Session content available as HTML only.*")

    return {"markdown": "\n".join(lines), "title": data.get("title", task_id)}


@app.post("/api/workspace/save_session")
async def save_session(req: SaveSessionRequest) -> dict[str, Any]:
    project = _safe_identifier(req.project, "project")
    task_id = _safe_identifier(req.task_id, "task id")
    sf = aether_config.sessions_dir / f"{project}_{task_id}.json"
    session_data = {
        "id": task_id,
        "project": project,
        "title": req.title,
        "history_html": req.history_html,
        "messages": req.messages,
        "updated_at": str(datetime.now(UTC).isoformat()),
    }
    dumped = json.dumps(session_data, indent=2, ensure_ascii=False)
    sf.write_text(dumped, encoding="utf-8")
    return {"status": "success", "session": session_data}


@app.get("/api/workspace/load_session")
async def load_session(project: str, task_id: str) -> dict[str, Any]:
    project = _safe_identifier(project, "project")
    task_id = _safe_identifier(task_id, "task id")
    sf = aether_config.sessions_dir / f"{project}_{task_id}.json"
    if sf.exists():
        try:
            return json.loads(sf.read_text(encoding="utf-8"))
        except Exception as e:
            logger.warning(f"Failed to read session {sf}: {e}")
    return {"id": task_id, "project": project, "title": task_id, "history_html": "", "messages": []}


@app.get("/api/chat/stream")
async def chat_sse_stream(
    message: str,
    model: str | None = None,
    effort: str | None = None,
) -> StreamingResponse:
    """Ultra-fast Server-Sent Events (SSE) stream for real-time agent output."""
    async def sse_generator() -> AsyncIterator[str]:
        async for event in agent.run_turn(message, model_name=model, reasoning_effort=effort):
            yield f"data: {json.dumps(event.to_dict(), ensure_ascii=False)}\n\n"


    return StreamingResponse(
        sse_generator(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no",
        },
    )


@app.websocket("/ws")
async def websocket_endpoint(websocket: WebSocket) -> None:
    await websocket.accept()
    logger.info("WebSocket client connected.")
    try:
        while True:
            raw_data = await websocket.receive_text()
            data = json.loads(raw_data)
            action = data.get("action")

            if action == "chat":
                prompt = data.get("prompt", "")
                async for event in agent.run_turn(prompt):
                    await websocket.send_json(event.to_dict())

            elif action == "get_plugins":
                plugins = [p.model_dump(mode="json") for p in agent.registry.list_plugins()]
                await websocket.send_json({"type": "plugins_list", "data": plugins})

    except WebSocketDisconnect:
        logger.info("WebSocket client disconnected.")
    except Exception as e:
        logger.error(f"WebSocket error: {e}")


if UI_DIR.is_dir():
    app.mount("/static", StaticFiles(directory=str(UI_DIR)), name="static")

    @app.get("/")
    async def serve_ui() -> HTMLResponse:
        index_file = UI_DIR / "index.html"
        return HTMLResponse(content=index_file.read_text(encoding="utf-8"))
