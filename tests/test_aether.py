from __future__ import annotations

import asyncio
import json
from pathlib import Path
from typing import Any

from fastapi.testclient import TestClient

from aether.core.agent import AetherAgent, SubagentReviewer, TaskQueueManager
from aether.core.contracts import (
    DiffChunk,
    EvolutionRequest,
    PluginAuthor,
    PluginType,
)
from aether.core.evolution import EvolutionManager
from aether.core.registry import PluginRegistry
from aether.core.sandbox import PluginSandbox
from aether.plugins.loops import ReActLoopPlugin
from aether.plugins.memory import WorkingMemoryPlugin
from aether.plugins.models import (
    AnthropicModelPlugin,
    DeepSeekModelPlugin,
    OpenAIModelPlugin,
)
from aether.plugins.tools import (
    AskQuestionToolPlugin,
    AstAnalyzerToolPlugin,
    BashToolPlugin,
    EvolvePluginToolPlugin,
    FileEditorToolPlugin,
)
from aether.server import app


def test_registry_register_snapshot_and_rollback(tmp_path: Path) -> None:
    registry = PluginRegistry(str(tmp_path))
    tool = FileEditorToolPlugin()
    meta = registry.register(tool)
    assert meta.id == "file_editor"
    assert registry.get("file_editor") is not None
    assert len(registry.list_plugins()) == 1

    # Take snapshot
    snap_id = registry.create_snapshot("file_editor")
    assert snap_id is not None

    # Modify metadata and verify rollback
    tool.metadata.version = "2.0.0"
    assert registry.get("file_editor").metadata.version == "2.0.0"  # type: ignore

    restored = registry.rollback("file_editor", snap_id)
    assert restored.version == "1.1.0"


def test_sandbox_evaluates_and_rejects_forbidden_ast(tmp_path: Path) -> None:
    sandbox = PluginSandbox()

    valid_code = (
        "from aether.core.contracts import (\n"
        "    PluginMetadata, PluginType, PluginAuthor, PluginStatus, PluginContext\n"
        ")\n\n"
        "class HexDumpPlugin:\n"
        "    def __init__(self) -> None:\n"
        "        self.metadata = PluginMetadata(\n"
        "            id='hex_dumper',\n"
        "            name='Hex Dumper',\n"
        "            version='1.0.0',\n"
        "            type=PluginType.TOOL,\n"
        "            description='Converts text to hex bytes',\n"
        "            author=PluginAuthor.AGENT_SELF_EVOLVED,\n"
        "        )\n\n"
        "    def on_load(self, context: PluginContext) -> None:\n"
        "        pass\n\n"
        "    def on_unload(self) -> None:\n"
        "        pass\n\n"
        "    def get_schema(self) -> dict:\n"
        "        return {'action': 'to_hex'}\n\n"
        "    def self_test(self) -> dict:\n"
        "        return {'passed': True}\n\n"
        "    def execute(self, text: str) -> str:\n"
        "        return text.encode('utf-8').hex()\n"
    )

    test_code = (
        "res = plugin.execute('AEL')\n"
        "assert_equal(res, '41454c')\n"
        "assert_equal(plugin.self_test()['passed'], True)\n"
    )

    instance, test_res = sandbox.compile_and_instantiate(valid_code, test_code)
    assert test_res.passed is True
    assert instance is not None
    assert instance.metadata.id == "hex_dumper"

    # Invalid code with forbidden eval
    dangerous_code = valid_code + "\ndef bad():\n    eval('1+1')\n"
    inst2, test_res2 = sandbox.compile_and_instantiate(dangerous_code)
    assert test_res2.passed is False
    assert test_res2.security_passed is False or not test_res2.syntax_valid


def test_evolution_manager_hot_swaps_and_rolls_back(tmp_path: Path) -> None:
    registry = PluginRegistry(str(tmp_path))
    evolution = EvolutionManager(registry, allow_unsafe_in_process=True)

    plugin_code = (
        "from aether.core.contracts import (\n"
        "    PluginMetadata, PluginType, PluginAuthor, PluginStatus, PluginContext\n"
        ")\n\n"
        "class CalcTool:\n"
        "    def __init__(self) -> None:\n"
        "        self.metadata = PluginMetadata(\n"
        "            id='calc_tool',\n"
        "            name='Calculator Tool',\n"
        "            version='0.1.0',\n"
        "            type=PluginType.TOOL,\n"
        "            description='Basic math calculation tool',\n"
        "            author=PluginAuthor.AGENT_SELF_EVOLVED,\n"
        "        )\n\n"
        "    def on_load(self, context: PluginContext) -> None:\n"
        "        pass\n\n"
        "    def on_unload(self) -> None:\n"
        "        pass\n\n"
        "    def get_schema(self) -> dict:\n"
        "        return {}\n\n"
        "    def self_test(self) -> dict:\n"
        "        return {'passed': True}\n\n"
        "    def execute(self, a: int, b: int) -> int:\n"
        "        return a + b\n"
    )

    req = EvolutionRequest(
        target_plugin_id="calc_tool",
        plugin_name="Calculator Tool",
        plugin_type=PluginType.TOOL,
        description="Basic math calculation tool",
        plugin_code=plugin_code,
        test_code="assert_equal(plugin.execute(3, 4), 7)\n",
    )

    res = evolution.evolve_plugin(req)
    assert res.success is True
    assert res.status == "hot_swapped"
    assert registry.get("calc_tool") is not None

    # Rollback
    rb_res = evolution.rollback("calc_tool")
    assert rb_res is not None


def test_built_in_tool_plugins_execution(tmp_path: Path) -> None:
    registry = PluginRegistry(str(tmp_path))
    ctx = registry.get_context()

    # File Editor
    editor = FileEditorToolPlugin()
    editor.on_load(ctx)
    write_res = editor.execute("write", "hello.txt", content="Hello Aether!")
    assert write_res.success is True
    read_res = editor.execute("read", "hello.txt")
    assert read_res.output == "Hello Aether!"
    list_res = editor.execute("list_dir", ".")
    assert any(item["name"] == "hello.txt" for item in list_res.output)
    edit_res = editor.execute(
        "edit", "hello.txt", target_content="Hello", replacement_content="Greetings"
    )
    assert edit_res.success is True
    assert editor.execute("read", "hello.txt").output == "Greetings Aether!"

    # Bash tool
    bash = BashToolPlugin()
    bash.on_load(ctx)
    res_b = bash.execute("pwd")
    assert res_b.success is True
    assert str(tmp_path) in res_b.output["stdout"]
    rejected = bash.execute("echo 'aether-test'")
    assert rejected.success is False
    assert "not allow-listed" in rejected.error

    # AST Analyzer
    ast_tool = AstAnalyzerToolPlugin()
    res_ast = ast_tool.execute("class Demo:\n    def foo(self): pass\n")
    assert res_ast.success is True
    assert "Demo" in res_ast.output["classes"]
    assert "foo" in res_ast.output["functions"]

    # Loops and Memory
    loop = ReActLoopPlugin()
    assert loop.self_test()["passed"] is True
    mem = WorkingMemoryPlugin()
    assert mem.self_test()["passed"] is True

    # Models
    ds = DeepSeekModelPlugin(api_key="test-deepseek-key")
    assert ds.self_test()["passed"] is True
    oa = OpenAIModelPlugin(api_key="test-openai-key")
    assert oa.self_test()["passed"] is True
    anth = AnthropicModelPlugin(api_key="test-anthropic-key")
    assert anth.self_test()["passed"] is True

    # Meta Evolution Tool
    evo_tool = EvolvePluginToolPlugin(EvolutionManager(registry))
    assert evo_tool.self_test()["passed"] is True


def test_agent_run_turn_autonomous_stream(tmp_path: Path) -> None:
    async def _run() -> list[Any]:
        agent = AetherAgent(workspace=str(tmp_path), allow_unsafe_evolution=True)
        assert len(agent.registry.list_plugins()) >= 7

        events = []
        async for event in agent.run_turn("帮我新增一个分析插件"):
            events.append(event)

        sqlite_plugin = agent.registry.get("sqlite_monitor")
        assert sqlite_plugin is not None
        assert sqlite_plugin.metadata.id == "sqlite_monitor"
        assert sqlite_plugin.metadata.author == PluginAuthor.AGENT_SELF_EVOLVED
        return events

    events = asyncio.run(_run())
    event_types = [e.event_type for e in events]
    assert "turn_start" in event_types
    assert "thought_chunk" in event_types
    assert "tool_call_start" in event_types
    assert "tool_call_done" in event_types
    assert "plugin_evolved" in event_types
    assert "turn_complete" in event_types


def test_fastapi_server_endpoints() -> None:
    client = TestClient(app)

    # Health
    res = client.get("/api/health")
    assert res.status_code == 200
    assert res.json()["status"] == "healthy"

    # Plugins
    res = client.get("/api/plugins")
    assert res.status_code == 200
    assert isinstance(res.json(), list)

    # UI Static
    res = client.get("/")
    assert res.status_code == 200
    assert "Aether Native" in res.text

    # Diffs
    res = client.get("/api/diffs")
    assert res.status_code == 200

    # Workspace files & terminal exec
    res_files = client.get("/api/workspace/files")
    assert res_files.status_code == 200

    res_term = client.post("/api/terminal/exec", json={"command": "pwd"})
    assert res_term.status_code == 200
    assert "aether" in res_term.json().get("output", "")

    res_rejected = client.post(
        "/api/terminal/exec", json={"command": "echo 'aether-isolated'"}
    )
    assert res_rejected.status_code == 422

    res_escape = client.post("/api/workspace/create_project", json={"name": "../escape"})
    assert res_escape.status_code == 422

    # Save and load session
    res_save = client.post("/api/workspace/save_session", json={
        "task_id": "test-task-1",
        "project": "aether-agent-core",
        "title": "Test Task Title",
        "history_html": "<p>Hello Aether</p>",
    })
    assert res_save.status_code == 200

    res_load = client.get(
        "/api/workspace/load_session?project=aether-agent-core&task_id=test-task-1"
    )
    assert res_load.status_code == 200
    assert res_load.json()["title"] == "Test Task Title"


def test_desktop_server_thread_lifecycle() -> None:
    from aether.desktop_app import DesktopServerThread

    server_thread = DesktopServerThread(host="127.0.0.1", port=8769)
    assert server_thread.host == "127.0.0.1"
    assert server_thread.port == 8769
    server_thread.stop()


def test_evolution_is_disabled_by_default(tmp_path: Path) -> None:
    registry = PluginRegistry(str(tmp_path))
    evolution = EvolutionManager(registry)
    request = EvolutionRequest(
        target_plugin_id="disabled_plugin",
        plugin_name="Disabled Plugin",
        plugin_type=PluginType.TOOL,
        description="Must not execute without an explicit development gate",
        plugin_code="raise RuntimeError('must not execute')",
        test_code="",
    )

    result = evolution.evolve_plugin(request)
    assert result.success is False
    assert result.status == "disabled"
    assert registry.get("disabled_plugin") is None


def test_provider_plugins_require_explicit_credentials(monkeypatch: Any) -> None:
    for name in ("DEEPSEEK_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"):
        monkeypatch.delenv(name, raising=False)

    assert DeepSeekModelPlugin().self_test()["passed"] is False
    assert OpenAIModelPlugin().self_test()["passed"] is False
    assert AnthropicModelPlugin().self_test()["passed"] is False


def test_ask_question_tool_plugin() -> None:
    tool = AskQuestionToolPlugin()
    assert tool.self_test()["passed"] is True
    schema = tool.get_schema()
    assert schema["name"] == "ask_question"
    assert "question" in schema["parameters"]["required"]

    # Test execution with valid choices
    res = tool.execute(
        question="请选择底层内核架构：",
        options=["方案 A: 微内核", "方案 B: 单体内核"],
        is_multi_select=False,
        allow_custom=True,
        context="影响编译与仿真管线",
    )
    assert res.success is True
    assert res.output["question"] == "请选择底层内核架构："
    assert len(res.output["options"]) == 2
    parsed_card = json.loads(res.artifacts["ask_card"])
    assert parsed_card["options"] == ["方案 A: 微内核", "方案 B: 单体内核"]

    # Test execution with empty question
    res_err = tool.execute(question="")
    assert res_err.success is False
    assert "cannot be empty" in str(res_err.error)


def test_agent_runs_ask_question_interactive_turn(tmp_path: Path) -> None:
    agent = AetherAgent(workspace=str(tmp_path))
    assert agent.registry.get("ask_question") is not None

    async def _run() -> list[Any]:
        events = []
        async for ev in agent.run_turn("请针对当前内核方案发起提问和选择卡片"):
            events.append(ev)
        return events

    events = asyncio.run(_run())
    event_types = [ev.event_type for ev in events]
    assert "turn_start" in event_types
    assert "ask_question" in event_types
    assert "turn_complete" in event_types

    ask_ev = next(ev for ev in events if ev.event_type == "ask_question")
    assert "方案" in ask_ev.data["question"]
    assert len(ask_ev.data["options"]) >= 2


def test_agent_resolves_at_file_mentions(tmp_path: Path) -> None:
    test_file = tmp_path / "hello.py"
    test_file.write_text("print('Aether Native')", encoding="utf-8")

    agent = AetherAgent(workspace=str(tmp_path))
    resolved = agent._resolve_at_mentions("请阅读 @hello.py 并优化")
    assert "print('Aether Native')" in resolved
    assert "[Attached File Context from @hello.py]" in resolved

    # Non-existent mention is safely skipped
    resolved_none = agent._resolve_at_mentions("请阅读 @non_existent.py")
    assert resolved_none == "请阅读 @non_existent.py"


def test_task_queue_manager_lifecycle() -> None:
    queue = TaskQueueManager()
    assert queue.size() == 0

    async def _test() -> None:
        t1 = await queue.push("任务 1: 编写单元测试", priority=0)
        t2 = await queue.push("任务 2: 紧急修复 Bug", priority=10)
        assert queue.size() == 2

        # Priority 10 should come first
        peeked = await queue.peek()
        assert peeked is not None
        assert peeked.task_id == t2.task_id

        # Pop should pop t2
        popped = await queue.pop()
        assert popped is not None
        assert popped.task_id == t2.task_id
        assert queue.size() == 1

        # Remove t1
        removed = await queue.remove(t1.task_id)
        assert removed is True
        assert queue.size() == 0

        # Push and clear
        await queue.push("任务 3")
        await queue.push("任务 4")
        cleared = await queue.clear()
        assert cleared == 2
        assert queue.size() == 0

    asyncio.run(_test())


def test_subagent_reviewer_pipeline() -> None:
    sandbox = PluginSandbox()
    reviewer = SubagentReviewer(sandbox)

    # Clean diff
    clean_diff = DiffChunk(
        chunk_id="chunk-1",
        file_path="aether/core/agent.py",
        old_start=1,
        old_lines=5,
        new_start=1,
        new_lines=5,
        diff_text="+ def calculate(a, b):\n+     return a + b\n",
    )
    rep_clean = reviewer.review_diff(clean_diff)
    assert rep_clean.score == 100
    assert rep_clean.status == "completed"
    assert any("安全检查" in f for f in rep_clean.findings)

    # Dangerous diff with eval
    risky_diff = DiffChunk(
        chunk_id="chunk-2",
        file_path="aether/risky.py",
        old_start=1,
        old_lines=5,
        new_start=1,
        new_lines=5,
        diff_text="+ res = eval('1+1')\n",
    )
    rep_risky = reviewer.review_diff(risky_diff)
    assert rep_risky.score < 100
    assert any("eval" in f.lower() for f in rep_risky.findings)


def test_queue_and_subagent_api_endpoints() -> None:
    client = TestClient(app)

    # 1. Push queue
    res_push = client.post("/api/queue/push", json={"prompt": "API 排队测试任务", "priority": 5})
    assert res_push.status_code == 200
    task_id = res_push.json()["task"]["task_id"]

    # 2. List queue
    res_list = client.get("/api/queue")
    assert res_list.status_code == 200
    tasks = res_list.json()
    assert any(t["task_id"] == task_id for t in tasks)

    # 3. Pop queue
    res_pop = client.post("/api/queue/pop")
    assert res_pop.status_code == 200
    assert res_pop.json()["task"]["task_id"] == task_id

    # 4. Clear queue
    client.post("/api/queue/push", json={"prompt": "待清空任务"})
    res_clear = client.post("/api/queue/clear")
    assert res_clear.status_code == 200
    assert res_clear.json()["status"] == "success"
