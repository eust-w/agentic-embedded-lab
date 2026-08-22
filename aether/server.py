from __future__ import annotations

import json
import logging
import os
import re
from collections.abc import AsyncIterator
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import HTMLResponse, StreamingResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

from aether.core.agent import AetherAgent
from aether.core.config import aether_config
from aether.core.contracts import EvolutionRequest
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
    for d in agent.pending_diffs:
        if d.chunk_id == req.chunk_id:
            d.status = "accepted" if req.action == "accept" else "rejected"
            return {"success": True, "chunk": d.model_dump(mode="json")}
    return {"success": False, "error": "Diff chunk not found"}


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


class SaveSessionRequest(BaseModel):
    task_id: str
    project: str
    title: str
    history_html: str
    messages: list[dict[str, Any]] = []


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
