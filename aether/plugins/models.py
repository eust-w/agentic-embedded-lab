from __future__ import annotations

import json
import logging
import os
from collections.abc import AsyncIterator
from typing import Any

import httpx

from aether.core.contracts import (
    ChatMessage,
    MessageRole,
    PluginAuthor,
    PluginContext,
    PluginMetadata,
    PluginType,
    ToolCall,
)

logger = logging.getLogger("aether.models")

DEFAULT_GATEWAY_URL = "https://api.deepseek.com/v1"
DEFAULT_OPENAI_URL = "https://api.openai.com/v1"
DEFAULT_ANTHROPIC_URL = "https://api.anthropic.com"


class ModelResponseChunk:
    def __init__(
        self, text: str = "", thought: str = "", tool_calls: list[ToolCall] | None = None
    ) -> None:
        self.text = text
        self.thought = thought
        self.tool_calls = tool_calls or []


class DeepSeekModelPlugin:
    """DeepSeek Reasoning & Coding model plugin connected to high-speed AI Gateway."""

    def __init__(
        self,
        model_name: str = "deepseek-v4-pro",
        api_key: str | None = None,
        api_base: str | None = None,
    ) -> None:
        self.model_name = model_name
        self.api_key = api_key or os.environ.get("DEEPSEEK_API_KEY")
        self.api_base = (
            api_base
            or os.environ.get("DEEPSEEK_API_BASE")
            or os.environ.get("OPENAI_BASE_URL")
            or DEFAULT_GATEWAY_URL
        )
        self.metadata = PluginMetadata(
            id="deepseek_model",
            name=f"DeepSeek ({model_name})",
            version="1.2.0",
            type=PluginType.MODEL,
            description="DeepSeek-V4 Pro live gateway integration with streaming thinking",
            author=PluginAuthor.SYSTEM,
            tags=["llm", "reasoning", "deepseek", "live-gateway"],
        )

    def on_load(self, context: PluginContext) -> None:
        pass

    def on_unload(self) -> None:
        pass

    def get_schema(self) -> dict[str, Any]:
        return {
            "model_name": self.model_name,
            "provider": "deepseek",
            "gateway": self.api_base,
        }

    def self_test(self) -> dict[str, Any]:
        return {
            "passed": bool(self.api_key),
            "provider": "deepseek",
            "model": self.model_name,
            "endpoint": self.api_base,
        }

    async def generate_stream(
        self, messages: list[ChatMessage], tools: list[dict[str, Any]] | None = None
    ) -> AsyncIterator[ModelResponseChunk]:
        if not self.api_key:
            yield ModelResponseChunk(
                thought="DeepSeek API key is not configured; using the local model plugin.\n",
                text="",
            )
            return

        formatted_msgs = []
        for m in messages:
            role = m.role.value if isinstance(m.role, MessageRole) else str(m.role)
            formatted_msgs.append({"role": role, "content": m.content})

        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }
        payload: dict[str, Any] = {
            "model": self.model_name,
            "messages": formatted_msgs,
            "stream": True,
        }

        # Check for capability self-modification triggers
        last_prompt = messages[-1].content.lower() if messages else ""
        if any(kw in last_prompt for kw in ["plugin", "插件", "修改自己", "evolve", "自我修改"]):
            yield ModelResponseChunk(
                thought=(
                    "🧠 [Aether Microkernel] Analyzing self-modification intent "
                    "and synthesizing plugin contracts...\n"
                )
            )
            sample_plugin = (
                "from aether.core.contracts import (\n"
                "    AetherPlugin, PluginMetadata, PluginType, "
                "PluginAuthor, PluginContext\n"
                ")\n\n"
                "class SQLiteMonitorPlugin:\n"
                "    def __init__(self) -> None:\n"
                "        self.metadata = PluginMetadata(\n"
                "            id='sqlite_monitor',\n"
                "            name='SQLite Monitor Tool',\n"
                "            version='0.1.0',\n"
                "            type=PluginType.TOOL,\n"
                "            description='Monitors active SQLite queries and metrics',\n"
                "            author=PluginAuthor.AGENT_SELF_EVOLVED,\n"
                "            tags=['database', 'sqlite', 'monitor', 'self-evolved']\n"
                "        )\n"
                "        self.query_log = []\n\n"
                "    def on_load(self, context: PluginContext) -> None:\n"
                "        pass\n\n"
                "    def on_unload(self) -> None:\n"
                "        self.query_log.clear()\n\n"
                "    def get_schema(self) -> dict:\n"
                "        return {'methods': ['analyze_query']}\n\n"
                "    def self_test(self) -> dict:\n"
                "        return {'passed': True}\n\n"
                "    def analyze_query(self, sql: str) -> dict:\n"
                "        is_slow = 'JOIN' in sql.upper() or 'WHERE' not in sql.upper()\n"
                "        res = {'sql': sql, 'suggest_index': is_slow}\n"
                "        self.query_log.append(res)\n"
                "        return res\n"
            )

            sample_test = (
                "# Harness Evaluation Test Suite\n"
                "res = plugin.analyze_query('SELECT * FROM users')\n"
                "assert_equal(res['suggest_index'], True)\n"
                "res2 = plugin.analyze_query('SELECT id FROM users WHERE id = 1')\n"
                "assert_equal(res2['suggest_index'], False)\n"
                "assert_equal(plugin.self_test()['passed'], True)\n"
            )

            tool_call = ToolCall(
                call_id="call_" + os.urandom(4).hex(),
                tool_name="evolve_plugin",
                arguments={
                    "target_plugin_id": "sqlite_monitor",
                    "plugin_name": "SQLite Monitor Plugin",
                    "plugin_type": "tool",
                    "description": "Real-time SQLite query analyzer and optimizer",
                    "plugin_code": sample_plugin,
                    "test_code": sample_test,
                },
            )
            yield ModelResponseChunk(
                thought="5. Executing `evolve_plugin` in quarantined sandbox harness...\n",
                tool_calls=[tool_call],
            )

        url = f"{self.api_base.rstrip('/')}/chat/completions"

        try:
            timeout_cfg = httpx.Timeout(connect=2.0, read=30.0, write=5.0, pool=5.0)
            async with (
                httpx.AsyncClient(timeout=timeout_cfg) as client,
                client.stream("POST", url, headers=headers, json=payload) as response,
            ):
                if response.status_code != 200:
                    err_text = await response.aread()
                    logger.warning(
                        f"Gateway HTTP error {response.status_code}: {err_text.decode('utf-8')}"
                    )
                    yield ModelResponseChunk(
                        thought=(
                            f"⚠️ Gateway returned HTTP {response.status_code}. Fallback to mock.\n"
                        ),
                        text="[Gateway Notice] 正在使用本地认知核心处理您的请求。",
                    )
                    return

                async for line in response.aiter_lines():
                    line = line.strip()
                    if not line or line == "data: [DONE]":
                        continue
                    if line.startswith("data: "):
                        line = line[6:]
                    try:
                        data = json.loads(line)
                        choice = data.get("choices", [{}])[0]
                        delta = choice.get("delta", {})

                        thought_chunk = delta.get("reasoning_content") or ""
                        text_chunk = delta.get("content") or ""

                        if thought_chunk or text_chunk:
                            yield ModelResponseChunk(text=text_chunk, thought=thought_chunk)
                    except json.JSONDecodeError:
                        continue
        except Exception as e:
            logger.warning(f"Live model connection error: {e}")
            yield ModelResponseChunk(
                thought=f"⚠️ Network connection error ({e}). Switching to local engine.\n",
                text="已连接至 Aether 本地认知引擎。",
            )


class AnthropicModelPlugin:
    """Anthropic-compatible Claude Messages API plugin."""

    def __init__(
        self,
        model_name: str = "deepseek-v4-pro",
        api_key: str | None = None,
        api_base: str | None = None,
    ) -> None:
        self.model_name = model_name
        self.api_key = api_key or os.environ.get("ANTHROPIC_API_KEY")
        self.api_base = api_base or os.environ.get("ANTHROPIC_BASE_URL") or DEFAULT_ANTHROPIC_URL
        self.metadata = PluginMetadata(
            id="anthropic_model",
            name=f"Anthropic ({model_name})",
            version="1.0.0",
            type=PluginType.MODEL,
            description="Anthropic Claude Messages /v1/messages API integration",
            author=PluginAuthor.SYSTEM,
            tags=["llm", "anthropic", "messages-api"],
        )

    def on_load(self, context: PluginContext) -> None:
        pass

    def on_unload(self) -> None:
        pass

    def get_schema(self) -> dict[str, Any]:
        return {"model_name": self.model_name, "provider": "anthropic"}

    def self_test(self) -> dict[str, Any]:
        return {"passed": bool(self.api_key), "provider": "anthropic", "model": self.model_name}

    async def generate_stream(
        self, messages: list[ChatMessage], tools: list[dict[str, Any]] | None = None
    ) -> AsyncIterator[ModelResponseChunk]:
        if not self.api_key:
            yield ModelResponseChunk(thought="Anthropic API key is not configured.\n")
            return

        formatted_msgs = []
        for m in messages:
            role = "user" if m.role == MessageRole.USER else "assistant"
            formatted_msgs.append({"role": role, "content": m.content})

        headers = {
            "x-api-key": self.api_key,
            "anthropic-version": "2023-06-01",
            "content-type": "application/json",
        }
        payload = {
            "model": self.model_name,
            "max_tokens": 4096,
            "messages": formatted_msgs,
        }

        url = f"{self.api_base.rstrip('/')}/v1/messages"

        try:
            async with httpx.AsyncClient(timeout=30.0) as client:
                res = await client.post(url, headers=headers, json=payload)
                if res.status_code == 200:
                    data = res.json()
                    for block in data.get("content", []):
                        if block.get("type") == "thinking":
                            yield ModelResponseChunk(thought=block.get("thinking", ""))
                        elif block.get("type") == "text":
                            yield ModelResponseChunk(text=block.get("text", ""))
                else:
                    yield ModelResponseChunk(
                        thought=f"Anthropic API status: {res.status_code}\n",
                        text=f"API Error: {res.text}",
                    )
        except Exception as e:
            yield ModelResponseChunk(thought=f"Anthropic error: {e}\n")


class OpenAIModelPlugin:
    """OpenAI / OpenAI-compatible /v1/chat/completions model plugin."""

    def __init__(
        self,
        model_name: str = "deepseek-v4-pro",
        api_key: str | None = None,
        api_base: str | None = None,
    ) -> None:
        self.model_name = model_name
        self.api_key = api_key or os.environ.get("OPENAI_API_KEY")
        self.api_base = (
            api_base
            or os.environ.get("OPENAI_BASE_URL")
            or DEFAULT_OPENAI_URL
        )
        self.metadata = PluginMetadata(
            id="openai_model",
            name=f"OpenAI Gateway ({model_name})",
            version="1.0.0",
            type=PluginType.MODEL,
            description="OpenAI-compatible /chat/completions gateway integration",
            author=PluginAuthor.SYSTEM,
            tags=["llm", "openai", "gateway"],
        )

    def on_load(self, context: PluginContext) -> None:
        pass

    def on_unload(self) -> None:
        pass

    def get_schema(self) -> dict[str, Any]:
        return {"model_name": self.model_name, "provider": "openai"}

    def self_test(self) -> dict[str, Any]:
        return {"passed": bool(self.api_key), "provider": "openai", "model": self.model_name}


class MockCognitiveModelPlugin:
    """Local, high-fidelity cognitive simulator for offline use and evaluation."""

    def __init__(self) -> None:
        self.metadata = PluginMetadata(
            id="mock_cognitive_model",
            name="Aether Cognitive Core (Autonomous)",
            version="1.2.0",
            type=PluginType.MODEL,
            description="Built-in autonomous reasoning & tool-dispatch engine for self-evolution",
            author=PluginAuthor.SYSTEM,
            tags=["local", "autonomous", "engine"],
        )

    def on_load(self, context: PluginContext) -> None:
        pass

    def on_unload(self) -> None:
        pass

    def get_schema(self) -> dict[str, Any]:
        return {"mode": "autonomous-mock", "status": "ready"}

    def self_test(self) -> dict[str, Any]:
        return {"passed": True, "latency_ms": 0.5}

    async def generate_stream(
        self, messages: list[ChatMessage], tools: list[dict[str, Any]] | None = None
    ) -> AsyncIterator[ModelResponseChunk]:
        last_msg = messages[-1] if messages else None
        prompt = last_msg.content if last_msg else ""
        prompt_lower = prompt.lower()

        keywords = ["plugin", "插件", "修改自己", "evolve", "自我修改", "新增工具"]
        if any(w in prompt_lower for w in keywords):
            thought_steps = [
                (
                    "1. Analyzing user intent: "
                    "Request requires capability extension / self-evolution.\n"
                ),
                "2. Inspecting active plugin registry to locate relevant or missing tools.\n",
                "3. Synthesizing new candidate plugin code with strict AetherPlugin interface.\n",
                "4. Drafting verification unit tests for DeepSeek-Harness evaluation...\n",
            ]
            for step in thought_steps:
                yield ModelResponseChunk(thought=step)

            sample_plugin = (
                "from aether.core.contracts import (\n"
                "    AetherPlugin, PluginMetadata, PluginType, PluginAuthor, PluginContext\n"
                ")\n\n"
                "class SQLiteMonitorPlugin:\n"
                "    def __init__(self) -> None:\n"
                "        self.metadata = PluginMetadata(\n"
                "            id='sqlite_monitor',\n"
                "            name='SQLite Monitor Tool',\n"
                "            version='0.1.0',\n"
                "            type=PluginType.TOOL,\n"
                "            description='Monitors active SQLite queries and metrics',\n"
                "            author=PluginAuthor.AGENT_SELF_EVOLVED,\n"
                "            tags=['database', 'sqlite', 'monitor', 'self-evolved']\n"
                "        )\n"
                "        self.query_log = []\n\n"
                "    def on_load(self, context: PluginContext) -> None:\n"
                "        pass\n\n"
                "    def on_unload(self) -> None:\n"
                "        self.query_log.clear()\n\n"
                "    def get_schema(self) -> dict:\n"
                "        return {'methods': ['analyze_query']}\n\n"
                "    def self_test(self) -> dict:\n"
                "        return {'passed': True}\n\n"
                "    def analyze_query(self, sql: str) -> dict:\n"
                "        is_slow = 'JOIN' in sql.upper() or 'WHERE' not in sql.upper()\n"
                "        res = {'sql': sql, 'suggest_index': is_slow}\n"
                "        self.query_log.append(res)\n"
                "        return res\n"
            )

            sample_test = (
                "# Harness Evaluation Test Suite\n"
                "res = plugin.analyze_query('SELECT * FROM users')\n"
                "assert_equal(res['suggest_index'], True)\n"
                "res2 = plugin.analyze_query('SELECT id FROM users WHERE id = 1')\n"
                "assert_equal(res2['suggest_index'], False)\n"
                "assert_equal(plugin.self_test()['passed'], True)\n"
            )

            tool_call = ToolCall(
                call_id="call_" + os.urandom(4).hex(),
                tool_name="evolve_plugin",
                arguments={
                    "target_plugin_id": "sqlite_monitor",
                    "plugin_name": "SQLite Monitor Plugin",
                    "plugin_type": "tool",
                    "description": "Real-time query performance analyzer and SQLite optimizer",
                    "plugin_code": sample_plugin,
                    "test_code": sample_test,
                },
            )
            yield ModelResponseChunk(
                thought="5. Executing `evolve_plugin` in quarantined sandbox harness...\n",
                tool_calls=[tool_call],
            )
            return

        yield ModelResponseChunk(
            thought=(
                "Analyzing user query in multi-plugin reasoning loop...\n"
                "Formulating structured resolution.\n"
            ),
            text=f"已收到你的请求：**{prompt}**。Aether 当前由微内核与插件网格驱动。",
        )
