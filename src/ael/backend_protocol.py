from __future__ import annotations

from enum import StrEnum
from typing import Any, Literal

from pydantic import Field

from .contracts import Event, StrictModel

BACKEND_API_VERSION = "ael.dev/backend/v1"


class BackendOperation(StrEnum):
    PROBE = "probe"
    PREPARE = "prepare"
    INJECT = "inject"
    STEP = "step"
    SNAPSHOT = "snapshot"
    SHUTDOWN = "shutdown"


class BackendRequest(StrictModel):
    api_version: Literal["ael.dev/backend/v1"] = BACKEND_API_VERSION
    request_id: str
    operation: BackendOperation
    virtual_time_us: int | None = Field(default=None, ge=0)
    payload: dict[str, Any] = Field(default_factory=dict)


class BackendResponse(StrictModel):
    api_version: Literal["ael.dev/backend/v1"] = BACKEND_API_VERSION
    request_id: str
    ok: bool
    outputs: dict[str, Any] = Field(default_factory=dict)
    metrics: dict[str, Any] = Field(default_factory=dict)
    events: list[Event] = Field(default_factory=list)
    artifacts: dict[str, str] = Field(default_factory=dict)
    error: str | None = None

    @classmethod
    def failure(cls, request_id: str, error: str) -> BackendResponse:
        return cls(request_id=request_id, ok=False, error=error)
