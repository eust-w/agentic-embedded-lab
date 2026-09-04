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
- 可配置Responses模型与项目权限；运行中Turn可取消，任务列表支持`⌘K`检索。
- 子Agent支持消息、转向、等待、中断、结果读取、关闭，以及带冲突预检和Patch哈希的Worktree Handoff。
- Browser与Computer Use的DOM/AX树、截图、点击、输入和下载均通过显式授权与敏感操作确认执行。
- RRULE、手动运行和插件事件自动化共享同一持久Job状态，支持启停、取消、恢复和删除。
- Skills、Hooks、签名插件、WASM、MCP、脱敏Memory和独立子Agent Thread。
- daemon托管真实zsh PTY、xterm.js、离线Monaco Diff、只读AI代码审查，以及
  需确认的Stage/Restore/Commit/Push/GitHub草稿PR工作流。
- 跨Turn历史恢复、Responses长上下文自动压缩、目录级AGENTS规则注入。
- 持久项目、租约恢复RRULE后台调度、受控CDP浏览器和一次性/持久Computer Use权限。
- 隔离gRPC进程插件、WASM插件、固定Chromium、Chrome Native Messaging和
  动态加载的Sparkle 2更新器。
- AEL v2契约、FMI/SSP、六类执行适配器、24项真实faulty/fixed机制、
  ARM/RISC-V Firmware、Evidence/Fidelity和20次确定性Trace验收。
- 可选Verilator 5.050 RTL扩展：独立容器真实编译并执行SystemVerilog faulty/fixed资产；
  当前为功能级扩展证据，不声明综合、时序收敛、FPGA或硅等价。
- 可选多物理模型包：OpenModelica机电电机、电池老化、传感器动态误差，以及
  ngspice PCB串联寄生和EFT钳位；均具有真实faulty/fixed运行证据，但尚未校准。
- 扩展模型不再只检查单一failure：RTL覆盖复位/保持/回绕/IRQ，电机覆盖动态速度、稳态误差和温升，
  电池覆盖循环衰减、温度、内阻和带载电压，传感器覆盖偏置/比例/饱和/响应，PCB与EFT覆盖瞬态、恢复和峰值。
- 六个扩展固定模型均要求20次事件Trace与断言结果哈希一致；主24项矩阵同时校验Trace和Oracle结果。
- 硬件实验室软件包含严格校准记录、仪器证据、Validation Envelope和白名单操作契约；原始SCPI禁止暴露。

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
go run ./cmd/aether-capability-acceptance
go run ./cmd/ael-extension-acceptance --workspace .
go run ./cmd/ael capabilities list
go run ./cmd/ael acceptance report
go run ./cmd/ael release check --profile foundation
go run ./cmd/ael release check --profile simulation
go run ./cmd/ael release check --profile software
# 必须在没有真机证据时失败：
go run ./cmd/ael release check --profile production
```

## 数据边界

OpenAI密钥只保存在macOS Keychain。Prompt、用户选择的图片和工具结果会发送到
所配置的OpenAI API。为在单个Turn内通过`previous_response_id`继续工具调用，
Aether开启Responses存储；最终保留期/ZDR策略以OpenAI项目配置为准。本地Memory
默认关闭，启用后会脱敏，并可查看、删除或再次关闭。

视觉规范见[设计文档](docs/design/aether-desktop-1.0/DESIGN.md)，旧Python实现保存在
[归档分支](https://github.com/eust-w/agentic-embedded-lab/tree/archive/python-aether-pre-go)。

核心使用[Apache-2.0](LICENSE)，外部模拟器保持各自上游许可证。
