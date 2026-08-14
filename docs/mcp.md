# MCP configuration

Install the optional MCP dependencies and run the server over STDIO:

```bash
python -m pip install -e '.[mcp]'
AEL_WORKSPACE=/absolute/path/to/agentic-embedded-lab ael-mcp
```

Generic MCP client configuration:

```json
{
  "mcpServers": {
    "agentic-embedded-lab": {
      "command": "/absolute/path/to/.venv/bin/ael-mcp",
      "env": {
        "AEL_WORKSPACE": "/absolute/path/to/agentic-embedded-lab"
      }
    }
  }
}
```

The tools return compact status and fidelity boundaries. Events are paged up to
1000 at a time. There is no shell tool and no path outside `AEL_WORKSPACE`.
