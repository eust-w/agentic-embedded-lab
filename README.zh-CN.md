<div align="center">
  <img src="docs/design/aether-desktop-1.0/chat-workspace.png" alt="Aether Desktop代码工作区" width="100%" />
  <h1>Aether Desktop + AEL Engine</h1>
  <p><b>macOS原生工程Agent与证据驱动的嵌入式实验室。</b></p>
  <p><a href="README.md">English</a> | <b>简体中文</b></p>
</div>

> [!IMPORTANT]
> 当前分支正在进行Aether Desktop 1.0全量Go重构。Foundation、软件拓扑、
> AEL完整仿真门和arm64开发包已经通过；正式1.0仍受完整Xcode、Developer ID、
> Apple公证凭据以及真机Validation Envelope约束。

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
- 隔离gRPC进程插件、WASM插件、固定Chromium、Chrome Native Messaging和
  动态加载的Sparkle 2更新器。
- AEL v2契约、FMI/SSP、六类执行适配器、24项真实faulty/fixed机制、
  ARM/RISC-V Firmware、Evidence/Fidelity和20次确定性Trace验收。

## 尚未通过的发布门

- 当前开发包使用ad-hoc签名；Developer ID、Notarization、Sparkle Feed签名和
  正式DMG需要完整Xcode及发布者凭据。
- Chrome Native Messaging安装器已提供，但不会静默创建持久集成，必须由用户明确启用。
- 没有真实实验室时，Hardware与Production Claim继续阻塞；仿真通过不等于真机通过。

## 开发检查

```bash
npm --prefix frontend install
npm --prefix frontend run build
go test ./...
go vet ./...
go run ./cmd/schema-export --output schemas/v2
./scripts/fetch_macos_dependencies.sh
./scripts/build_mac_app.sh --development
go run ./cmd/aether-package-check
```

视觉规范见[设计文档](docs/design/aether-desktop-1.0/DESIGN.md)，旧Python实现保存在
[归档分支](https://github.com/eust-w/agentic-embedded-lab/tree/archive/python-aether-pre-go)。

核心使用[Apache-2.0](LICENSE)，外部模拟器保持各自上游许可证。
