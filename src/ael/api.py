from __future__ import annotations

import os
from pathlib import Path
from typing import Any

from fastapi import Depends, FastAPI, Header, HTTPException
from pydantic import BaseModel, ConfigDict

from .auth import (
    AuthenticationError,
    OidcVerifier,
    oidc_config_from_environment,
    verify_worker_fingerprint,
)
from .contracts import (
    WorkerHeartbeat,
    WorkerRegistration,
    WorkerTask,
    WorkerTaskResult,
)
from .service import AelService


class PathRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    path: str


def create_app(workspace: Path | None = None) -> FastAPI:
    root = workspace or Path(os.environ.get("AEL_WORKSPACE", Path.cwd()))
    service = AelService(root)
    oidc_config = oidc_config_from_environment()
    oidc_verifier = OidcVerifier(oidc_config) if oidc_config else None
    app = FastAPI(title="Agentic Embedded Lab", version="0.2.0-dev", openapi_url="/v1/openapi.json")

    def translate(call: Any) -> Any:
        try:
            return call()
        except KeyError as exception:
            raise HTTPException(404, str(exception)) from exception
        except (ValueError, PermissionError, FileNotFoundError) as exception:
            raise HTTPException(422, str(exception)) from exception

    def require_user(authorization: str | None = Header(default=None)) -> dict[str, Any]:
        if oidc_verifier is None:
            return {"sub": "local-development"}
        if not authorization or not authorization.startswith("Bearer "):
            raise HTTPException(401, "OIDC bearer token required")
        try:
            return oidc_verifier.verify(authorization.removeprefix("Bearer "))
        except AuthenticationError as exception:
            raise HTTPException(401, str(exception)) from exception

    def require_worker(
        worker_id: str,
        x_client_cert_sha256: str | None = Header(default=None),
    ) -> WorkerRegistration:
        registration = service.store.worker(worker_id)
        if registration is None:
            raise HTTPException(403, "worker is not registered")
        if x_client_cert_sha256 is None:
            raise HTTPException(401, "verified client certificate fingerprint required")
        try:
            verify_worker_fingerprint(x_client_cert_sha256, registration.certificate_fingerprint)
        except AuthenticationError as exception:
            raise HTTPException(403, str(exception)) from exception
        return registration

    @app.get("/v1/doctor")
    def doctor() -> dict[str, Any]:
        return service.doctor()

    @app.get("/v1/healthz")
    def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/v1/project")
    def inspect_project(
        _: dict[str, Any] = Depends(require_user),  # noqa: B008
    ) -> dict[str, Any]:
        return service.inspect()

    @app.post("/v1/problems/classify")
    def classify_problem(
        request: PathRequest,
        _: dict[str, Any] = Depends(require_user),  # noqa: B008
    ) -> dict[str, Any]:
        return translate(lambda: service.classify(Path(request.path)))

    @app.post("/v1/experiments")
    def start_experiment(
        request: PathRequest,
        _: dict[str, Any] = Depends(require_user),  # noqa: B008
    ) -> dict[str, Any]:
        return translate(lambda: {"run_id": service.start_experiment(Path(request.path))})

    @app.get("/v1/experiments/{run_id}")
    def get_experiment(
        run_id: str,
        _: dict[str, Any] = Depends(require_user),  # noqa: B008
    ) -> dict[str, Any]:
        return translate(lambda: service.status(run_id))

    @app.get("/v1/experiments/{run_id}/evidence")
    def get_evidence(
        run_id: str,
        _: dict[str, Any] = Depends(require_user),  # noqa: B008
    ) -> Any:
        return translate(lambda: service.get_evidence(run_id))

    @app.get("/v1/experiments/{run_id}/events")
    def get_events(
        run_id: str,
        offset: int = 0,
        limit: int = 100,
        _: dict[str, Any] = Depends(require_user),  # noqa: B008
    ) -> dict[str, Any]:
        return translate(lambda: service.get_event_page(run_id, offset, limit))

    @app.post("/v1/workers/register")
    def register_worker(
        registration: WorkerRegistration,
        x_client_cert_sha256: str | None = Header(default=None),
    ) -> dict[str, Any]:
        if x_client_cert_sha256 is None:
            raise HTTPException(401, "verified client certificate fingerprint required")
        try:
            verify_worker_fingerprint(x_client_cert_sha256, registration.certificate_fingerprint)
        except AuthenticationError as exception:
            raise HTTPException(403, str(exception)) from exception
        return translate(lambda: service.register_worker(registration))

    @app.post("/v1/tasks")
    def enqueue_task(
        task: WorkerTask,
        _: dict[str, Any] = Depends(require_user),  # noqa: B008
    ) -> dict[str, Any]:
        return translate(lambda: service.enqueue_task(task).model_dump(mode="json"))

    @app.post("/v1/tasks/{task_id}/cancel")
    def cancel_task(
        task_id: str,
        _: dict[str, Any] = Depends(require_user),  # noqa: B008
    ) -> dict[str, Any]:
        return translate(lambda: service.cancel_task(task_id).model_dump(mode="json"))

    @app.get("/v1/tasks/{task_id}")
    def get_task(
        task_id: str,
        _: dict[str, Any] = Depends(require_user),  # noqa: B008
    ) -> dict[str, Any]:
        return translate(lambda: service.task_status(task_id).model_dump(mode="json"))

    @app.post("/v1/workers/{worker_id}/lease")
    def lease_task(
        worker_id: str,
        _: WorkerRegistration = Depends(require_worker),  # noqa: B008
    ) -> dict[str, Any] | None:
        return translate(
            lambda: (
                task.model_dump(mode="json")
                if (task := service.lease_task(worker_id)) is not None
                else None
            )
        )

    @app.post("/v1/workers/{worker_id}/heartbeat")
    def heartbeat_task(
        worker_id: str,
        heartbeat: WorkerHeartbeat,
        _: WorkerRegistration = Depends(require_worker),  # noqa: B008
    ) -> dict[str, Any]:
        return translate(
            lambda: service.heartbeat_task(worker_id, heartbeat).model_dump(mode="json")
        )

    @app.post("/v1/workers/{worker_id}/complete")
    def complete_task(
        worker_id: str,
        result: WorkerTaskResult,
        _: WorkerRegistration = Depends(require_worker),  # noqa: B008
    ) -> dict[str, Any]:
        return translate(lambda: service.complete_task(worker_id, result).model_dump(mode="json"))

    return app


app = create_app()


def main() -> None:
    import uvicorn

    uvicorn.run("ael.api:app", host="127.0.0.1", port=8765)
