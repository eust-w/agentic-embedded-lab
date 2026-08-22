from __future__ import annotations

import logging
import uuid
from pathlib import Path

from .contracts import EvolutionRequest, EvolutionResult
from .registry import PluginRegistry
from .sandbox import PluginSandbox

logger = logging.getLogger("aether.evolution")


class EvolutionManager:
    """Orchestrates self-modification, quarantined validation, and hot-swap rollback."""

    def __init__(
        self,
        registry: PluginRegistry,
        sandbox: PluginSandbox | None = None,
        *,
        allow_unsafe_in_process: bool = False,
    ) -> None:
        self.registry = registry
        self.sandbox = sandbox or PluginSandbox()
        self.allow_unsafe_in_process = allow_unsafe_in_process
        self.history: list[EvolutionResult] = []

    def evolve_plugin(self, request: EvolutionRequest) -> EvolutionResult:
        request_id = uuid.uuid4().hex[:8]
        if not self.allow_unsafe_in_process:
            result = EvolutionResult(
                request_id=request_id,
                target_plugin_id=request.target_plugin_id,
                success=False,
                status="disabled",
                message=(
                    "In-process plugin evolution is disabled. Use an isolated model "
                    "validation worker or explicitly enable development-only execution."
                ),
            )
            self.history.append(result)
            return result

        logger.info(
            f"Initiating self-evolution [{request_id}] for plugin: '{request.target_plugin_id}'"
        )

        # 1. Quarantined Compile & Harness Evaluation
        instance, test_res = self.sandbox.compile_and_instantiate(
            request.plugin_code, request.test_code
        )

        if not test_res.passed or instance is None:
            logger.warning(
                f"Self-evolution [{request_id}] failed validation: {test_res.error_message}"
            )
            result = EvolutionResult(
                request_id=request_id,
                target_plugin_id=request.target_plugin_id,
                success=False,
                status="conformance_failed",
                message=test_res.error_message or "Validation harness failed",
                test_results=test_res.to_dict(),
            )
            self.history.append(result)
            return result

        # 2. Update metadata with author and source
        instance.metadata.author = request.author
        instance.metadata.source_code = request.plugin_code
        if request.description:
            instance.metadata.description = request.description

        # 3. Create snapshot of current plugin if exists
        snapshot_id = None
        if self.registry.get(request.target_plugin_id) is not None:
            snapshot_id = self.registry.create_snapshot(request.target_plugin_id)

        # 4. Perform live Hot-Swap
        try:
            evolved_meta = self.registry.hot_swap(instance)

            # Persist to evolved storage directory
            evolved_dir = Path(self.registry.workspace) / ".aether" / "plugins" / "evolved"
            evolved_dir.mkdir(parents=True, exist_ok=True)
            target_file = evolved_dir / f"{request.target_plugin_id}.py"
            target_file.write_text(request.plugin_code, encoding="utf-8")

            logger.info(
                f"Self-evolution [{request_id}] SUCCESS. "
                f"Plugin '{request.target_plugin_id}' is live."
            )
            result = EvolutionResult(
                request_id=request_id,
                target_plugin_id=request.target_plugin_id,
                success=True,
                status="hot_swapped",
                message=(
                    f"Plugin '{request.target_plugin_id}' successfully evolved "
                    f"and hot-reloaded into runtime."
                ),
                test_results=test_res.to_dict(),
                snapshot_id=snapshot_id,
                evolved_plugin_metadata=evolved_meta,
            )
            self.history.append(result)
            return result

        except Exception as e:
            logger.error(f"Error during hot-swap, attempting rollback: {e}")
            if snapshot_id:
                try:
                    self.registry.rollback(request.target_plugin_id, snapshot_id)
                except Exception as rb_err:
                    logger.critical(f"Rollback failed: {rb_err}")

            result = EvolutionResult(
                request_id=request_id,
                target_plugin_id=request.target_plugin_id,
                success=False,
                status="rolled_back",
                message=f"Hot-swap failed: {e}. Reverted to snapshot.",
                test_results=test_res.to_dict(),
                snapshot_id=snapshot_id,
            )
            self.history.append(result)
            return result

    def rollback(self, plugin_id: str, snapshot_id: str | None = None) -> EvolutionResult:
        request_id = uuid.uuid4().hex[:8]
        try:
            restored_meta = self.registry.rollback(plugin_id, snapshot_id)
            result = EvolutionResult(
                request_id=request_id,
                target_plugin_id=plugin_id,
                success=True,
                status="rolled_back",
                message=f"Successfully rolled back plugin '{plugin_id}' to previous snapshot.",
                snapshot_id=snapshot_id,
                evolved_plugin_metadata=restored_meta,
            )
            self.history.append(result)
            return result
        except Exception as e:
            result = EvolutionResult(
                request_id=request_id,
                target_plugin_id=plugin_id,
                success=False,
                status="failed",
                message=f"Rollback failed: {e}",
            )
            self.history.append(result)
            return result
