from __future__ import annotations

import shutil
from pathlib import Path

import pytest


@pytest.fixture
def workspace(tmp_path: Path) -> Path:
    source = Path(__file__).parents[1]
    for directory in ("examples", "benchmarks", "firmware", "lab"):
        shutil.copytree(
            source / directory,
            tmp_path / directory,
            ignore=shutil.ignore_patterns("build-*", "__pycache__", "*.pyc"),
        )
    return tmp_path
