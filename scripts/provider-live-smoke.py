#!/usr/bin/env python3
"""Run minimal live structured-output calls without persisting credentials or prompts."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path

from ael.contracts import ModelGenerationConfig
from ael.model_providers import provider_for

PROMPT = """Grounding source examples/models/grounded-source.txt#lines:1-3 says:
The peripheral is 256 bytes. STATUS is a 32-bit read-only register at offset 0.
STATUS.READY is bit 0 and resets to zero.

Return HardwareBehaviorIR named ProviderLiveSmoke with only that documented register.
Every register must cite the exact grounding locator in the grounding map.
"""


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    configured = (
        ("openai", "OPENAI_API_KEY", os.getenv("AEL_OPENAI_SMOKE_MODEL", "gpt-5-mini")),
        (
            "anthropic",
            "ANTHROPIC_API_KEY",
            os.getenv("AEL_ANTHROPIC_SMOKE_MODEL", "claude-sonnet-4-5"),
        ),
    )
    results: dict[str, object] = {}
    for provider, key_name, model in configured:
        if not os.getenv(key_name):
            results[provider] = {"status": "not-run", "reason": f"{key_name} is absent"}
            continue
        result = provider_for(provider).generate(
            PROMPT,
            ModelGenerationConfig(provider=provider, model=model, max_attempts=1),
        )
        results[provider] = {
            "status": "passed",
            "model": model,
            "provider_request_id": result.request_id,
            "ir_name": result.ir.name,
            "register_count": len(result.ir.registers),
            "recorded": result.recorded,
        }
    payload = {
        "status": "passed",
        "results": results,
        "credentials_persisted": False,
        "hardware_validated": False,
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
