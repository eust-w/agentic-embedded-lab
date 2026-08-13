from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any, Protocol

from .contracts import HardwareBehaviorIR, ModelGenerationConfig


@dataclass(frozen=True)
class ProviderResult:
    ir: HardwareBehaviorIR
    raw_text: str
    request_id: str | None
    recorded: bool = False


class ModelProvider(Protocol):
    def generate(self, prompt: str, config: ModelGenerationConfig) -> ProviderResult: ...


def _post_json(
    url: str, headers: dict[str, str], payload: dict[str, Any]
) -> tuple[dict[str, Any], str | None]:
    request = urllib.request.Request(
        url,
        data=json.dumps(payload, separators=(",", ":")).encode("utf-8"),
        headers={"content-type": "application/json", **headers},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=180) as response:
            body = json.loads(response.read().decode("utf-8"))
            return body, response.headers.get("request-id") or response.headers.get("x-request-id")
    except urllib.error.HTTPError as error:
        detail = error.read().decode("utf-8", errors="replace")[:4000]
        raise RuntimeError(f"model provider returned HTTP {error.code}: {detail}") from error
    except urllib.error.URLError as error:
        raise RuntimeError(f"model provider request failed: {error.reason}") from error


def _recorded_response(provider: str) -> dict[str, Any] | None:
    fixture_dir = os.environ.get("AEL_MODEL_PROVIDER_REPLAY_DIR")
    if not fixture_dir:
        return None
    path = os.path.join(fixture_dir, f"{provider}.json")
    try:
        with open(path, encoding="utf-8") as stream:
            value = json.load(stream)
    except FileNotFoundError as error:
        raise RuntimeError(f"recorded provider response is missing: {path}") from error
    if not isinstance(value, dict):
        raise RuntimeError("recorded provider response must be a JSON object")
    return value


class OpenAIProvider:
    def generate(self, prompt: str, config: ModelGenerationConfig) -> ProviderResult:
        recorded = _recorded_response("openai")
        api_key = os.environ.get("OPENAI_API_KEY")
        if recorded is None and not api_key:
            raise RuntimeError("OPENAI_API_KEY is required for provider=openai")
        schema = HardwareBehaviorIR.model_json_schema()
        payload = {
            "model": config.model,
            "store": False,
            "input": [
                {
                    "role": "developer",
                    "content": (
                        "Generate only a grounded AEL HardwareBehaviorIR. Never invent "
                        "undocumented registers or behavior."
                    ),
                },
                {"role": "user", "content": prompt},
            ],
            "text": {
                "format": {
                    "type": "json_schema",
                    "name": "ael_hardware_behavior_ir",
                    "strict": True,
                    "schema": schema,
                }
            },
        }
        base = os.environ.get("AEL_OPENAI_BASE_URL", "https://api.openai.com/v1").rstrip("/")
        if recorded is None:
            response, header_request_id = _post_json(
                f"{base}/responses", {"authorization": f"Bearer {api_key}"}, payload
            )
        else:
            response, header_request_id = recorded, "ael-recorded-openai"
        raw = response.get("output_text")
        if not isinstance(raw, str):
            parts: list[str] = []
            for item in response.get("output", []):
                if item.get("type") != "message":
                    continue
                for content in item.get("content", []):
                    if content.get("type") == "output_text" and isinstance(
                        content.get("text"), str
                    ):
                        parts.append(content["text"])
            raw = "".join(parts)
        if not raw:
            raise RuntimeError("OpenAI response contained no output_text")
        return ProviderResult(
            HardwareBehaviorIR.model_validate_json(raw),
            raw,
            response.get("id") or header_request_id,
            recorded is not None,
        )


class AnthropicProvider:
    def generate(self, prompt: str, config: ModelGenerationConfig) -> ProviderResult:
        recorded = _recorded_response("anthropic")
        api_key = os.environ.get("ANTHROPIC_API_KEY")
        if recorded is None and not api_key:
            raise RuntimeError("ANTHROPIC_API_KEY is required for provider=anthropic")
        schema = HardwareBehaviorIR.model_json_schema()
        payload = {
            "model": config.model,
            "max_tokens": 8192,
            "system": (
                "Generate only a grounded AEL HardwareBehaviorIR. Never invent "
                "undocumented registers or behavior."
            ),
            "messages": [{"role": "user", "content": prompt}],
            "output_config": {"format": {"type": "json_schema", "schema": schema}},
        }
        base = os.environ.get("AEL_ANTHROPIC_BASE_URL", "https://api.anthropic.com/v1").rstrip("/")
        if recorded is None:
            response, header_request_id = _post_json(
                f"{base}/messages",
                {"x-api-key": api_key, "anthropic-version": "2023-06-01"},
                payload,
            )
        else:
            response, header_request_id = recorded, "ael-recorded-anthropic"
        raw = "".join(
            block["text"]
            for block in response.get("content", [])
            if block.get("type") == "text" and isinstance(block.get("text"), str)
        )
        if not raw:
            raise RuntimeError("Anthropic response contained no text block")
        if response.get("stop_reason") in {"max_tokens", "refusal"}:
            raise RuntimeError(
                f"Anthropic structured output stopped with {response['stop_reason']}"
            )
        return ProviderResult(
            HardwareBehaviorIR.model_validate_json(raw),
            raw,
            response.get("id") or header_request_id,
            recorded is not None,
        )


def provider_for(name: str) -> ModelProvider:
    if name == "openai":
        return OpenAIProvider()
    if name == "anthropic":
        return AnthropicProvider()
    raise ValueError(f"unsupported model provider: {name}")
