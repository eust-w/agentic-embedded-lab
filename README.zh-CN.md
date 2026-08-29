<div align="center">
  <img src="docs/design/aether-desktop-1.0/chat-workspace.png" alt="Aether Desktop代码工作区" width="100%" />
  <h1>Aether Desktop + AEL Engine</h1>
  <p><b>macOS原生工程Agent与证据驱动的嵌入式实验室。</b></p>
  <p><a href="README.md">English</a> | <b>简体中文</b></p>
</div>

> [!IMPORTANT]
> 当前分支正在进行Aether Desktop 1.0全量Go重构。Go Foundation与React生产
> 构建已通过，但五个仿真后端、24项机制基准、内置Chromium、LaunchAgent、
> 签名和公证尚未全部完成，不能据此声明仿真或硬件通过。

## 产品关系

- **Aether Desktop**：Thread、审批、终端、Git/Worktree、多Agent、Skills、
  Plugins、MCP、Memory、Browser、Computer Use和后台自动化。
- **AEL Engine**：确定性调度、虚拟硬件、多物理域后端、Evidence、Fidelity、
  Claim与Validation Envelope。

## 当前Go实现

- Wails v2 + React/TypeScript/Vite桌面壳，包含Chat、Diff、Browser和Simulation。
- `aether.desktop/v1` Thread/Turn/Item/Approval/Agent/Worktree/Automation契约。
- SQLite WAL、CAS、持久Thread、Memory、Permission、Automation和Job。
- OpenAI Responses流式调用、取消、幂等、分类错误和重试；默认`gpt-5.6`。
- macOS Keychain凭证和Unix Socket daemon认证。
- 类型化审批、Seatbelt执行器、Git/Worktree、AGENTS层级发现。
- Skills、Hooks、签名插件、WASM、MCP、脱敏Memory和独立子Agent Thread。
- RRULE后台调度、受控CDP浏览器和逐应用Computer Use权限。
- AEL v2契约、确定性Scheduler、Trace Hash、Fidelity和Backend JSONL协议。

## 尚未通过的门

- 五个仿真器的Go Worker与容器尚未全部接通。
- 24项faulty/fixed基准尚未全部通过Go Runner。
- Chromium、Chrome Native Messaging、ScreenCaptureKit完整链路尚未完成。
- `SMAppService`、Sparkle、Developer ID签名与Notarization需要完整Xcode。
- 没有真实实验室时，Hardware与Production Claim继续阻塞。

## 开发检查

```bash
npm --prefix frontend install
npm --prefix frontend run build
go test ./...
go vet ./...
go run ./cmd/schema-export --output schemas/v2
```

视觉规范见[设计文档](docs/design/aether-desktop-1.0/DESIGN.md)，旧Python实现保存在
[归档分支](https://github.com/eust-w/agentic-embedded-lab/tree/archive/python-aether-pre-go)。

核心使用[Apache-2.0](LICENSE)，外部模拟器保持各自上游许可证。
