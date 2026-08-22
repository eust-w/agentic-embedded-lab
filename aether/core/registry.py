from __future__ import annotations

import copy
import logging
import uuid
from collections.abc import Callable
from datetime import UTC, datetime

from .contracts import (
    AetherPlugin,
    PluginContext,
    PluginMetadata,
    PluginStatus,
    PluginType,
)

logger = logging.getLogger("aether.registry")


class RegistryError(Exception):
    pass


class PluginSnapshot:
    def __init__(self, plugin_id: str, metadata: PluginMetadata, instance: AetherPlugin) -> None:
        self.snapshot_id = uuid.uuid4().hex[:12]
        self.plugin_id = plugin_id
        self.metadata = copy.deepcopy(metadata)
        self.instance = instance
        self.created_at = datetime.now(UTC)


class PluginRegistry:
    def __init__(self, workspace: str = ".") -> None:
        self.workspace = workspace
        self._plugins: dict[str, AetherPlugin] = {}
        self._snapshots: dict[str, list[PluginSnapshot]] = {}
        self._listeners: list[Callable[[str, PluginMetadata], None]] = []

    def get_context(self) -> PluginContext:
        return PluginContext(registry=self, event_bus=None, workspace=self.workspace)

    def subscribe(self, callback: Callable[[str, PluginMetadata], None]) -> None:
        self._listeners.append(callback)

    def _notify(self, action: str, metadata: PluginMetadata) -> None:
        for callback in self._listeners:
            try:
                callback(action, metadata)
            except Exception as e:
                logger.warning(f"Error in registry listener: {e}")

    def register(self, plugin: AetherPlugin, auto_load: bool = True) -> PluginMetadata:
        meta = plugin.metadata
        if meta.id in self._plugins:
            raise RegistryError(
                f"Plugin with id '{meta.id}' is already registered. Use hot_swap instead."
            )

        for dep in meta.dependencies:
            if dep not in self._plugins:
                logger.warning(f"Plugin '{meta.id}' depends on '{dep}', which is not yet loaded.")

        self._plugins[meta.id] = plugin
        if auto_load:
            plugin.on_load(self.get_context())
            meta.status = PluginStatus.ACTIVE

        self._notify("registered", meta)
        logger.info(f"Registered plugin: {meta.id} ({meta.name}) [{meta.type}]")
        return meta

    def unregister(self, plugin_id: str) -> AetherPlugin:
        if plugin_id not in self._plugins:
            raise RegistryError(f"Plugin '{plugin_id}' not found.")
        plugin = self._plugins[plugin_id]
        plugin.on_unload()
        plugin.metadata.status = PluginStatus.STANDBY
        del self._plugins[plugin_id]
        self._notify("unregistered", plugin.metadata)
        logger.info(f"Unregistered plugin: {plugin_id}")
        return plugin

    def get(self, plugin_id: str) -> AetherPlugin | None:
        return self._plugins.get(plugin_id)

    def get_typed[T: AetherPlugin](self, plugin_id: str, expected_type: type[T]) -> T | None:
        plugin = self.get(plugin_id)
        if plugin and isinstance(plugin, expected_type):
            return plugin
        return None

    def list_plugins(self, plugin_type: PluginType | None = None) -> list[PluginMetadata]:
        result = []
        for plugin in self._plugins.values():
            if plugin_type is None or plugin.metadata.type == plugin_type:
                result.append(copy.deepcopy(plugin.metadata))
        return result

    def get_by_type(self, plugin_type: PluginType) -> list[AetherPlugin]:
        return [p for p in self._plugins.values() if p.metadata.type == plugin_type]

    def create_snapshot(self, plugin_id: str) -> str:
        if plugin_id not in self._plugins:
            raise RegistryError(f"Cannot snapshot non-existent plugin '{plugin_id}'.")
        plugin = self._plugins[plugin_id]
        snapshot = PluginSnapshot(plugin_id, plugin.metadata, plugin)
        self._snapshots.setdefault(plugin_id, []).append(snapshot)
        logger.info(f"Created snapshot '{snapshot.snapshot_id}' for plugin '{plugin_id}'")
        return snapshot.snapshot_id

    def rollback(self, plugin_id: str, snapshot_id: str | None = None) -> PluginMetadata:
        history = self._snapshots.get(plugin_id, [])
        if not history:
            raise RegistryError(f"No snapshot history available for plugin '{plugin_id}'.")

        if snapshot_id is None:
            snapshot = history.pop()
        else:
            idx = next((i for i, s in enumerate(history) if s.snapshot_id == snapshot_id), None)
            if idx is None:
                raise RegistryError(f"Snapshot '{snapshot_id}' not found for plugin '{plugin_id}'.")
            snapshot = history.pop(idx)

        # Unload current instance if running
        if plugin_id in self._plugins:
            try:
                self._plugins[plugin_id].on_unload()
            except Exception as err:
                logger.error(f"Error unloading plugin during rollback: {err}")

        # Restore snapshot instance
        restored = snapshot.instance
        restored.metadata = snapshot.metadata
        restored.metadata.updated_at = datetime.now(UTC)
        self._plugins[plugin_id] = restored
        restored.on_load(self.get_context())
        self._notify("rolled_back", restored.metadata)
        logger.info(f"Rolled back plugin '{plugin_id}' to snapshot '{snapshot.snapshot_id}'")
        return restored.metadata

    def hot_swap(self, new_plugin: AetherPlugin) -> PluginMetadata:
        meta = new_plugin.metadata
        target_id = meta.id

        if target_id in self._plugins:
            # Snapshot before replacement
            self.create_snapshot(target_id)
            old_plugin = self._plugins[target_id]
            try:
                old_plugin.on_unload()
            except Exception as err:
                logger.warning(f"Error unloading old plugin '{target_id}': {err}")

        self._plugins[target_id] = new_plugin
        new_plugin.on_load(self.get_context())
        meta.status = PluginStatus.ACTIVE
        meta.updated_at = datetime.now(UTC)
        self._notify("hot_swapped", meta)
        logger.info(f"Hot-swapped plugin: {meta.id} (v{meta.version})")
        return meta
