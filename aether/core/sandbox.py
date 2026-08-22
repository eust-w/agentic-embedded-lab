from __future__ import annotations

import ast
import logging
import time
import traceback
from typing import Any

from .contracts import AetherPlugin, PluginMetadata, PluginStatus, PluginType

logger = logging.getLogger("aether.sandbox")


class SandboxSecurityError(Exception):
    pass


class SandboxEvaluationError(Exception):
    pass


class SandboxTestResult:
    def __init__(
        self,
        passed: bool,
        duration_ms: float,
        syntax_valid: bool = True,
        security_passed: bool = True,
        self_test_passed: bool = True,
        unit_tests_passed: bool = True,
        test_details: list[dict[str, Any]] | None = None,
        error_message: str | None = None,
    ) -> None:
        self.passed = passed
        self.duration_ms = duration_ms
        self.syntax_valid = syntax_valid
        self.security_passed = security_passed
        self.self_test_passed = self_test_passed
        self.unit_tests_passed = unit_tests_passed
        self.test_details = test_details or []
        self.error_message = error_message

    def to_dict(self) -> dict[str, Any]:
        return {
            "passed": self.passed,
            "duration_ms": round(self.duration_ms, 2),
            "syntax_valid": self.syntax_valid,
            "security_passed": self.security_passed,
            "self_test_passed": self.self_test_passed,
            "unit_tests_passed": self.unit_tests_passed,
            "test_details": self.test_details,
            "error_message": self.error_message,
        }


class PluginSandbox:
    """Quarantined execution & evaluation harness for candidate plugins."""

    FORBIDDEN_CALLS = {"__import__('os').system", "subprocess.call", "shutil.rmtree"}

    def static_analysis(self, source_code: str) -> None:
        """Parse AST and verify syntax & basic safety constraints."""
        try:
            tree = ast.parse(source_code)
        except SyntaxError as e:
            raise SandboxEvaluationError(f"Syntax Error in plugin code: {e}") from e

        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                raise SandboxSecurityError("Direct imports are forbidden in candidate plugins.")
            if isinstance(node, ast.ImportFrom) and node.module != "aether.core.contracts":
                raise SandboxSecurityError(
                    "Candidate plugins may import only aether.core.contracts."
                )
            if (
                isinstance(node, ast.Call)
                and isinstance(node.func, ast.Name)
                and node.func.id
                in {
                    "breakpoint",
                    "compile",
                    "eval",
                    "exec",
                    "globals",
                    "input",
                    "locals",
                    "open",
                    "vars",
                }
            ):
                raise SandboxSecurityError(
                    f"Use of '{node.func.id}' is forbidden in candidate plugins."
                )

    def compile_and_instantiate(
        self, source_code: str, test_code: str = ""
    ) -> tuple[AetherPlugin, SandboxTestResult]:
        """Compile candidate plugin, instantiate it, and run comprehensive test suite."""
        start_time = time.monotonic()
        test_details: list[dict[str, Any]] = []

        # 1. Static AST Analysis
        try:
            self.static_analysis(source_code)
            test_details.append({"name": "AST Syntax & Static Analysis", "status": "passed"})
        except Exception as e:
            duration = (time.monotonic() - start_time) * 1000
            test_details.append({
                "name": "AST Syntax & Static Analysis",
                "status": "failed",
                "error": str(e),
            })
            return None, SandboxTestResult(  # type: ignore[return-value]
                passed=False,
                duration_ms=duration,
                syntax_valid=False,
                test_details=test_details,
                error_message=str(e),
            )

        # 2. Dynamic Namespace Compilation
        namespace: dict[str, Any] = {
            "PluginMetadata": PluginMetadata,
            "PluginType": PluginType,
            "PluginStatus": PluginStatus,
            "AetherPlugin": AetherPlugin,
        }

        try:
            compiled = compile(source_code, "<aether-quarantine-plugin>", "exec")
            exec(compiled, namespace)
            test_details.append({"name": "Compilation & Namespace Binding", "status": "passed"})
        except Exception as e:
            duration = (time.monotonic() - start_time) * 1000
            test_details.append({"name": "Compilation", "status": "failed", "error": str(e)})
            return None, SandboxTestResult(  # type: ignore[return-value]
                passed=False,
                duration_ms=duration,
                test_details=test_details,
                error_message=f"Compilation failure: {e}",
            )

        # 3. Locate Plugin Class
        plugin_class = None
        ignored_names = {
            "AetherPlugin",
            "PluginMetadata",
            "PluginType",
            "PluginStatus",
            "PluginContext",
        }
        for item_name, item_val in namespace.items():
            if (
                isinstance(item_val, type)
                and item_name not in ignored_names
                and hasattr(item_val, "on_load")
                and hasattr(item_val, "on_unload")
            ):
                plugin_class = item_val
                break

        if plugin_class is None:
            duration = (time.monotonic() - start_time) * 1000
            err_msg = "No class implementing AetherPlugin protocol found in code."
            test_details.append({
                "name": "Protocol Verification",
                "status": "failed",
                "error": err_msg,
            })
            return None, SandboxTestResult(  # type: ignore[return-value]
                passed=False,
                duration_ms=duration,
                test_details=test_details,
                error_message=err_msg,
            )

        # 4. Instantiate Plugin
        try:
            instance: AetherPlugin = plugin_class()
            if not hasattr(instance, "metadata") or not isinstance(
                instance.metadata, PluginMetadata
            ):
                raise TypeError(
                    "Plugin instance must possess a valid 'metadata: PluginMetadata' attribute"
                )
            instance.metadata.source_code = source_code
            test_details.append({"name": "Instantiation Check", "status": "passed"})
        except Exception as e:
            duration = (time.monotonic() - start_time) * 1000
            test_details.append({
                "name": "Instantiation Check",
                "status": "failed",
                "error": str(e),
            })
            return None, SandboxTestResult(  # type: ignore[return-value]
                passed=False,
                duration_ms=duration,
                test_details=test_details,
                error_message=f"Instantiation error: {e}",
            )

        # 5. Run Built-in Self-Test
        try:
            self_test_res = instance.self_test()
            if isinstance(self_test_res, dict) and not self_test_res.get("passed", True):
                err = self_test_res.get("error", "unknown")
                raise SandboxEvaluationError(f"Plugin self_test returned failed: {err}")
            test_details.append({
                "name": "Plugin Self-Test",
                "status": "passed",
                "output": self_test_res,
            })
        except Exception as e:
            duration = (time.monotonic() - start_time) * 1000
            test_details.append({"name": "Plugin Self-Test", "status": "failed", "error": str(e)})
            return None, SandboxTestResult(  # type: ignore[return-value]
                passed=False,
                duration_ms=duration,
                self_test_passed=False,
                test_details=test_details,
                error_message=f"Self-test failed: {e}",
            )

        # 6. Run External DeepSeek-Harness Style Evaluation Code (if supplied)
        if test_code and test_code.strip():
            test_ns = {
                **namespace,
                "plugin": instance,
                "assert_equal": lambda a, b: self._assert_eq(a, b),
            }
            try:
                test_compiled = compile(test_code, "<aether-eval-harness>", "exec")
                exec(test_compiled, test_ns)
                test_details.append({"name": "Harness Eval Test Suite", "status": "passed"})
            except Exception as e:
                duration = (time.monotonic() - start_time) * 1000
                test_details.append({
                    "name": "Harness Eval Test Suite",
                    "status": "failed",
                    "error": f"{type(e).__name__}: {e}\n{traceback.format_exc()}",
                })
                return None, SandboxTestResult(  # type: ignore[return-value]
                    passed=False,
                    duration_ms=duration,
                    unit_tests_passed=False,
                    test_details=test_details,
                    error_message=f"Harness eval failed: {e}",
                )

        duration = (time.monotonic() - start_time) * 1000
        return instance, SandboxTestResult(
            passed=True,
            duration_ms=duration,
            syntax_valid=True,
            security_passed=True,
            self_test_passed=True,
            unit_tests_passed=True,
            test_details=test_details,
        )

    @staticmethod
    def _assert_eq(a: Any, b: Any) -> None:
        if a != b:
            raise AssertionError(f"Expected {b!r}, but got {a!r}")
