# Aether Desktop 1.0 visual specification

These three ImageGen concepts are the visual source of truth for the Wails/React
desktop surface:

- `chat-workspace.png`: code-agent conversation, approval, diff, agents, evidence and terminal.
- `simulation-evidence.png`: deterministic multi-domain simulation and Claim boundaries.
- `browser-permissions.png`: controlled browser, DOM/console evidence and per-app permissions.

## Design system

- True graphite background (`#090d12`) with cool dark surfaces; no warm tint.
- Electric blue is reserved for selected state and primary action. Green,
  orange and red are semantic evidence/permission colors.
- UI typography uses SF Pro/Inter fallbacks; code and traces use JetBrains Mono
  or SF Mono.
- Chrome uses one-pixel borders, 6–8px radii, compact 11–13px control text and
  50px titlebar. Avoid floating card grids, gloss and decorative gradients.
- The container model is a three-column desktop shell: project/thread rail,
  primary workspace, context/agents/evidence inspector. Terminal is a bottom
  rail, not a floating card.

## Allowed first-viewport copy

`Aether`, `agentic-embedded-lab`, `main`, `gpt-5.6`, `Workspace Write`,
`Chat`, `Diff`, `Browser`, `Simulation`, `Run experiment`, `Stop`,
`Approve once`, `Deny`, `Terminal`, `Active agents`, `Evidence`,
`Simulation validated`, `Hardware unverified`, `Production claim blocked`.

## Fidelity ledger

| Comparison point | Concept evidence | Browser implementation | Result |
|---|---|---|---|
| Shell geometry | 300px left rail, central workspace, ~330px inspector | 1600×1000 screenshot uses 300px/334px rails with no horizontal overflow | matched |
| Chat hierarchy | plan → tool calls → approval → diff → terminal | same order and visible labels; approval buttons repaired after first QA pass | matched |
| Typography and palette | compact cool-gray developer chrome, blue selection | computed layout uses locked token palette and deliberate 9–13px control text | matched |
| Simulation evidence | synchronized plots, timeline, assertions, immutable boundary | four plots, deterministic timeline, failed assertions, simulation/hardware/production statuses | matched; plots are code-native representative traces until live AEL events are wired |
| Browser permissions | DOM/console workspace plus per-site/per-app approval | Browser state includes page preview, DOM, console, site status, Allow once/Always/Deny and revoke | matched; preview data is development seed until bundled Chromium is installed |
| Responsive behavior | dense desktop surface with minimum readable regions | 1250px breakpoint narrows rails and hides the topology panel; no 1600px overflow | matched |
| Icons | thin, consistent system/developer symbols | Lucide icon family with consistent 13–21px optical sizes | matched |

## Intentional development deviations

- Monaco and xterm packages are installed but the first shell renders a
  lightweight code-native diff and terminal while backend binding is completed.
- Browser and simulation content are deterministic development data; the UI
  labels do not claim a live simulator or Chromium run.
- Native titlebar packaging, bundled Chromium, ScreenCaptureKit and
  Accessibility execution remain separate production gates.
