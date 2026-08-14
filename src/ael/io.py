from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any

import yaml
from pydantic import BaseModel

from .security import resolve_workspace_path


def canonical_json(value: Any) -> bytes:
    if isinstance(value, BaseModel):
        value = value.model_dump(mode="json")
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def sha256_value(value: Any) -> str:
    return hashlib.sha256(canonical_json(value)).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def load_document[ModelT: BaseModel](path: Path, model: type[ModelT], workspace: Path) -> ModelT:
    safe_path = resolve_workspace_path(workspace, path, must_exist=True)
    text = safe_path.read_text(encoding="utf-8")
    if safe_path.suffix.lower() == ".json":
        data = json.loads(text)
    elif safe_path.suffix.lower() in {".yaml", ".yml"}:
        data = yaml.safe_load(text)
    else:
        raise ValueError(f"unsupported document extension: {safe_path.suffix}")
    return model.model_validate(data)


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if isinstance(value, BaseModel):
        value = value.model_dump(mode="json")
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")
