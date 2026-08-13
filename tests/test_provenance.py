from __future__ import annotations

from ael.provenance import AUTHORITATIVE_SIMULATION_PLATFORM, detect_platform


def test_detect_platform_never_returns_an_empty_label() -> None:
    assert detect_platform().strip()


def test_authoritative_platform_is_explicit_and_stable() -> None:
    assert AUTHORITATIVE_SIMULATION_PLATFORM == "Ubuntu 24.04 x86_64"
