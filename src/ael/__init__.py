"""Agentic Embedded Lab core package."""

from importlib.metadata import PackageNotFoundError, version

try:
    __version__ = version("agentic-embedded-lab")
except PackageNotFoundError:
    __version__ = "0.2.0.dev0"

__all__ = ["__version__"]
