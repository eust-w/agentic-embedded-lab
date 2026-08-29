from __future__ import annotations

import json
import logging
import subprocess
from dataclasses import dataclass, field
from typing import Any

logger = logging.getLogger("aether.mcp")


@dataclass
class MCPTool:
    """A tool discovered from an MCP server."""
    name: str
    description: str
    input_schema: dict[str, Any] = field(default_factory=dict)
    server_name: str = ""


@dataclass
class MCPServer:
    """Represents a connected MCP server."""
    name: str
    command: list[str]
    status: str = "disconnected"  # disconnected, connected, error
    tools: list[MCPTool] = field(default_factory=list)
    error: str | None = None


class MCPClientManager:
    """Manages connections to multiple MCP servers.

    This is a simplified implementation that discovers tools
    by sending the MCP initialize + tools/list handshake over
    stdio to each configured server command.
    """

    def __init__(self) -> None:
        self.servers: dict[str, MCPServer] = {}

    def add_server(
        self, name: str, command: list[str],
    ) -> MCPServer:
        server = MCPServer(name=name, command=command)
        self.servers[name] = server
        return server

    def remove_server(self, name: str) -> bool:
        return self.servers.pop(name, None) is not None

    def connect(self, name: str) -> MCPServer:
        """Connect to an MCP server and discover its tools."""
        server = self.servers.get(name)
        if not server:
            raise ValueError(f"Unknown MCP server: {name}")

        try:
            # Send MCP initialize request
            init_req = {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {
                    "protocolVersion": "2024-11-05",
                    "capabilities": {},
                    "clientInfo": {
                        "name": "aether-native",
                        "version": "0.2.0",
                    },
                },
            }
            tools_req = {
                "jsonrpc": "2.0",
                "id": 2,
                "method": "tools/list",
                "params": {},
            }

            # Run server process with JSON-RPC stdin/stdout
            stdin_data = (
                json.dumps(init_req) + "\n"
                + json.dumps(tools_req) + "\n"
            )

            result = subprocess.run(
                server.command,
                input=stdin_data,
                capture_output=True,
                text=True,
                timeout=15,
            )

            # Parse tool responses
            tools: list[MCPTool] = []
            for line in result.stdout.strip().splitlines():
                try:
                    resp = json.loads(line)
                    if resp.get("id") == 2 and "result" in resp:
                        raw_tools = resp["result"].get(
                            "tools", [],
                        )
                        for t in raw_tools:
                            tools.append(MCPTool(
                                name=t.get("name", "unknown"),
                                description=t.get(
                                    "description", "",
                                ),
                                input_schema=t.get(
                                    "inputSchema", {},
                                ),
                                server_name=name,
                            ))
                except json.JSONDecodeError:
                    continue

            server.tools = tools
            server.status = "connected"
            server.error = None
            logger.info(
                "MCP server '%s' connected with %d tools",
                name, len(tools),
            )

        except subprocess.TimeoutExpired:
            server.status = "error"
            server.error = "Connection timed out"
        except FileNotFoundError:
            server.status = "error"
            server.error = (
                f"Command not found: {server.command[0]}"
            )
        except Exception as exc:
            server.status = "error"
            server.error = str(exc)

        return server

    def list_all_tools(self) -> list[MCPTool]:
        """List tools from all connected servers."""
        tools: list[MCPTool] = []
        for srv in self.servers.values():
            if srv.status == "connected":
                tools.extend(srv.tools)
        return tools

    def to_dict(self) -> list[dict[str, Any]]:
        """Serialize all servers for API responses."""
        return [
            {
                "name": s.name,
                "command": s.command,
                "status": s.status,
                "error": s.error,
                "tools": [
                    {
                        "name": t.name,
                        "description": t.description,
                    }
                    for t in s.tools
                ],
            }
            for s in self.servers.values()
        ]
