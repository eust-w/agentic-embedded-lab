from __future__ import annotations

import argparse
import os
import sys

import uvicorn


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Aether-Native: Agentic Microkernel & Codex Desktop Application"
    )
    parser.add_argument("--host", default="127.0.0.1", help="Host to bind server to")
    parser.add_argument("--port", type=int, default=8765, help="Port to bind server to")
    parser.add_argument(
        "--server-only",
        "--headless",
        action="store_true",
        help="Run headless backend server without opening native desktop window",
    )
    parser.add_argument("--reload", action="store_true", help="Enable auto-reload")
    args = parser.parse_args()

    loopback_hosts = {"127.0.0.1", "localhost", "::1"}
    if args.host not in loopback_hosts and os.getenv("AETHER_ALLOW_REMOTE") != "1":
        parser.error(
            "Remote binding is disabled by default. Set AETHER_ALLOW_REMOTE=1 only "
            "behind an authenticated reverse proxy."
        )

    if not args.server_only:
        try:
            from aether.desktop_app import launch_native_desktop

            print("=" * 60)
            print("⚡ Starting Aether-Native Desktop Application")
            print("🖥️ Mode: Native macOS Cocoa / Windows WebView2 Window")
            print("🧩 Philosophy: 'Everything is a Plugin' & Safe Self-Evolution")
            print("=" * 60)
            launch_native_desktop(host=args.host, port=args.port)
            sys.exit(0)
        except Exception as e:
            print(f"Notice: Native window failed to start ({e}), falling back to server mode.")

    print("=" * 60)
    print("⚡ Starting Aether-Native Server")
    print(f"🔗 Local Web Interface: http://{args.host}:{args.port}")
    print("=" * 60)

    uvicorn.run("aether.server:app", host=args.host, port=args.port, reload=args.reload)


if __name__ == "__main__":
    main()
