from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field


class AetherConfigModel(BaseModel):
    """Persistent configuration for Aether Native, strictly isolated from Codex Desktop."""
    version: str = "0.2.0"
    app_name: str = "Aether Native"
    active_model: str = "deepseek-v4-pro"
    reasoning_effort: str = "极高"
    permission_mode: str = "full_access"  # full_access | quarantined_sandbox
    theme: str = "dark"
    language: str = "zh"
    user_name: str = "Aether Developer"
    user_avatar: str = "AD"
    gateway_url: str = "https://api.deepseek.com/v1"
    target_device: str = "AEL Microkernel (Native)"
    custom_settings: dict[str, Any] = Field(default_factory=dict)


class AetherConfig:
    """Manages dedicated Aether directories and configuration to ensure complete isolation."""

    def __init__(self, custom_home: str | None = None) -> None:
        if custom_home:
            self.home_dir = Path(custom_home).resolve()
        else:
            env_home = os.getenv("AETHER_HOME")
            if env_home:
                self.home_dir = Path(env_home).resolve()
            else:
                self.home_dir = Path.home() / ".aether"

        self.workspace_dir = self.home_dir / "workspace"
        self.sessions_dir = self.home_dir / "sessions"
        self.plugins_dir = self.home_dir / "plugins"
        self.logs_dir = self.home_dir / "logs"
        self.config_file = self.home_dir / "config.json"

        self._ensure_dirs()
        self.data = self._load_config()
        self._ensure_default_aether_projects()

    def _ensure_dirs(self) -> None:
        self.home_dir.mkdir(parents=True, exist_ok=True)
        self.workspace_dir.mkdir(parents=True, exist_ok=True)
        self.sessions_dir.mkdir(parents=True, exist_ok=True)
        self.plugins_dir.mkdir(parents=True, exist_ok=True)
        self.logs_dir.mkdir(parents=True, exist_ok=True)

    def _ensure_default_aether_projects(self) -> None:
        """Seed default Aether-native isolated projects if empty, without touching external dirs."""
        default_projects = [
            ("aether-agent-core", "自主智能体微内核与自进化引擎"),
            ("embedded-contracts-lab", "嵌入式系统软硬件仿真与契约评测"),
            ("sandbox-telemetry", "沙箱安全与隔离执行环境"),
        ]
        for proj_name, _desc in default_projects:
            proj_dir = self.workspace_dir / proj_name
            if not proj_dir.exists():
                proj_dir.mkdir(parents=True, exist_ok=True)
                readme = proj_dir / "README.md"
                if not readme.exists():
                    txt = f"# {proj_name}\n\nAether Native 独立工程工作区。\n"
                    readme.write_text(txt, encoding="utf-8")

    def _load_config(self) -> AetherConfigModel:
        if self.config_file.exists():
            try:
                content = self.config_file.read_text(encoding="utf-8")
                config = AetherConfigModel.model_validate_json(content)
                # Rewrite legacy configs so previously persisted provider keys are removed.
                self._save_config(config)
                return config
            except Exception:
                pass
        cfg = AetherConfigModel()
        self._save_config(cfg)
        return cfg

    def _save_config(self, config: AetherConfigModel) -> None:
        self.config_file.write_text(config.model_dump_json(indent=2), encoding="utf-8")

    def update(self, **kwargs: Any) -> AetherConfigModel:
        updated_dict = self.data.model_dump()
        for k, v in kwargs.items():
            if hasattr(self.data, k):
                updated_dict[k] = v
        self.data = AetherConfigModel.model_validate(updated_dict)
        self._save_config(self.data)
        return self.data

    def get_projects_tree(self) -> list[dict[str, Any]]:
        """Returns structured projects and their tasks strictly inside ~/.aether/workspace."""
        tree = []
        if not self.workspace_dir.exists():
            return tree

        for item in sorted(self.workspace_dir.iterdir()):
            if item.is_dir() and not item.name.startswith("."):
                proj_name = item.name
                tasks = []
                session_files = list(self.sessions_dir.glob(f"{proj_name}_*.json"))
                if session_files:
                    for sf in session_files:
                        try:
                            sdata = json.loads(sf.read_text(encoding="utf-8"))
                            tasks.append({
                                "id": sdata.get("id", sf.stem),
                                "title": sdata.get("title", sf.stem),
                                "active": sdata.get("active", False)
                            })
                        except Exception:
                            tasks.append({"id": sf.stem, "title": sf.stem, "active": False})
                else:
                    if proj_name == "aether-agent-core":
                        tasks = [
                            {"id": "task-core-1", "title": "微内核插件热重载", "active": True},
                            {"id": "task-core-2", "title": "DeepSeek 推理验证", "active": False},
                        ]
                    elif proj_name == "embedded-contracts-lab":
                        tasks = [
                            {"id": "task-ecl-1", "title": "开源项目契约评测", "active": False},
                            {"id": "task-ecl-2", "title": "Zephyr 仿真链配置", "active": False},
                        ]
                    else:
                        tasks = [
                            {
                                "id": f"task-{proj_name}-1",
                                "title": f"构建 {proj_name}",
                                "active": False,
                            }
                        ]

                tree.append({
                    "name": proj_name,
                    "path": str(item),
                    "tasks": tasks
                })
        return tree


aether_config = AetherConfig()
