import logging
import threading
import time

import uvicorn
import webview

from aether.server import app

logger = logging.getLogger("aether.desktop")


class DesktopServerThread(threading.Thread):
    def __init__(self, host: str = "127.0.0.1", port: int = 8765) -> None:
        super().__init__(daemon=True)
        self.host = host
        self.port = port
        self.server: uvicorn.Server | None = None

    def run(self) -> None:
        config = uvicorn.Config(
            app=app,
            host=self.host,
            port=self.port,
            log_level="warning",
            access_log=False,
        )
        self.server = uvicorn.Server(config)
        self.server.run()

    def stop(self) -> None:
        if self.server:
            self.server.should_exit = True


def launch_native_desktop(host: str = "127.0.0.1", port: int = 8765) -> None:
    """Launch Aether as a true native macOS Cocoa / Windows WebView2 Desktop Application."""
    server_thread = DesktopServerThread(host=host, port=port)
    server_thread.start()

    # Allow server a brief moment to bind socket
    time.sleep(0.3)

    target_url = f"http://{host}:{port}/"
    logger.info(f"Opening native desktop window pointing to {target_url}")

    # Native Window Settings
    _window = webview.create_window(
        title="Aether Native · Codex Desktop Agent",
        url=target_url,
        width=1340,
        height=880,
        min_size=(980, 620),
        background_color="#09090b",
        text_select=True,
        zoomable=True,
    )
    del _window

    try:
        # Start native GUI event loop (Cocoa on macOS, Edge WebView2 on Windows, WebKitGTK on Linux)
        webview.start(debug=False)
    finally:
        server_thread.stop()


def main() -> None:
    launch_native_desktop()


if __name__ == "__main__":
    main()
