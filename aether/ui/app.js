/**
 * Aether Native · Complete Project & Session Management, Plan/Goal Modes & Slash Engine
 */

class AetherStreamingBuffer {
  constructor(onFlush) {
    this.buffer = [];
    this.onFlush = onFlush;
    this.rafId = null;
    this.isFlushing = false;
  }

  push(text) {
    if (!text) return;
    this.buffer.push(text);
    if (!this.isFlushing) {
      this.isFlushing = true;
      this.rafId = requestAnimationFrame(() => this.flush());
    }
  }

  flush() {
    if (this.buffer.length > 0) {
      const chunk = this.buffer.join('');
      this.buffer = [];
      this.onFlush(chunk);
    }
    if (this.buffer.length > 0) {
      this.rafId = requestAnimationFrame(() => this.flush());
    } else {
      this.isFlushing = false;
    }
  }

  clear() {
    this.buffer = [];
    if (this.rafId) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
    this.isFlushing = false;
  }
}

class AetherDesktopEngine {
  constructor() {
    this.activeTaskId = 'task-core-1';
    this.activeProject = 'aether-agent-core';
    this.isStreaming = false;
    this.streamTimer = null;
    this.elapsedSeconds = 28;
    this.goalTimer = null;
    this.goalElapsedSeconds = 28;
    this.eventSource = null;
    this.scrollRafId = null;
    this.recognition = null;
    this.isRecordingVoice = false;

    // Modes
    this.isFullAccess = true;
    this.isGoalMode = false;
    this.isPlanMode = false;

    // History stack for back/forward navigation
    this.taskHistoryStack = ['task-core-1'];
    this.historyPointer = 0;

    // Multi-task session store
    this.taskStore = {
      'task-core-1': {
        title: '微内核插件热重载',
        project: 'aether-agent-core',
        goal: '验证 Aether 微内核自主演化机制与插件沙箱隔离',
        historyHtml: null,
      }
    };

    this.models = [
      { name: "DeepSeek-V4 Pro 极高", badge: "DeepSeek-V4", id: "deepseek-v4-pro" },
      { name: "5.0 Sol 极高", badge: "5.0 Sol", id: "5.0-sol" },
      { name: "Claude 3.7 Sonnet", badge: "Claude 3.7", id: "claude-3-7-sonnet" },
      { name: "Qwen 2.5 Max", badge: "Qwen 2.5", id: "qwen-2-5-max" }
    ];
    this.currentModelIdx = 0;
    this.currentEffort = "极高";

    // Terminal Multi-Tab: per-tab output buffers
    this.terminalBuffers = {
      'main': '> Aether Sandbox Terminal Ready.\n$ ',
      'build': '> Build Log Terminal Ready.\n$ ',
    };
    this.activeTerminalTab = 'main';

    this.initDOMElements();

    // Load saved theme
    const savedTheme = localStorage.getItem('aether_theme') || 'dark';
    document.body.dataset.theme = savedTheme;
    if (this.settingsThemeSelect) this.settingsThemeSelect.value = savedTheme;

    this.initResizableDividers();
    this.bindEvents();
    this.startGoalTimer();
    this.loadInitialConfig();
    this.loadWorkspaceTree();
    this.refreshPlugins();
    this.refreshDiffs();
    this.refreshQueue();
    this.loadSessionHistory();
  }

  initDOMElements() {
    this.chatThreadContainer = document.getElementById('chat-thread-container');
    this.composerMainInput = document.getElementById('composer-main-input');
    this.dropZoneOverlay = document.getElementById('drop-zone-overlay');
    this.btnSendOrStop = document.getElementById('btn-send-or-stop');
    this.sendIconSvg = document.querySelector('.send-icon-svg');
    this.stopIconSquare = document.querySelector('.stop-icon-square');
    this.navbarTaskTitle = document.getElementById('navbar-task-title');
    this.toolAccordionCard = document.getElementById('tool-accordion-card');
    this.btnNewTask = document.getElementById('btn-new-task');
    this.btnAddProject = document.getElementById('btn-add-project');
    this.sidebarProjectTree = document.getElementById('sidebar-project-tree');
    this.sessionHistoryList = document.getElementById('session-history-list');
    this.sessionCountBadge = document.getElementById('session-count-badge');

    // Top Navigation & Menus
    this.btnNavBack = document.getElementById('btn-nav-back');
    this.btnNavForward = document.getElementById('btn-nav-forward');
    this.btnTaskOptions = document.getElementById('btn-task-options');
    this.taskOptionsMenu = document.getElementById('task-options-menu');
    this.btnModelDropdown = document.getElementById('btn-model-dropdown');
    this.modelTopBadge = document.getElementById('model-top-badge');
    this.modelPickerMenu = document.getElementById('model-picker-menu');
    this.btnSearchTasks = document.getElementById('btn-search-tasks');

    // Sidebar & Brand Popover
    this.aetherSidebar = document.getElementById('aether-sidebar');
    this.resizerLeft = document.getElementById('resizer-left');
    this.btnToggleSidebar = document.getElementById('btn-toggle-sidebar');
    this.brandDropdownBtn = document.getElementById('brand-dropdown-btn');
    this.aetherBrandMenu = document.getElementById('aether-brand-menu');
    this.userAvatarBadge = document.getElementById('user-avatar-badge');
    this.userDisplayName = document.getElementById('user-display-name');
    this.btnAppSettings = document.getElementById('btn-app-settings');

    // Right Canvas & Resizer
    this.aetherRightCanvas = document.getElementById('aether-right-canvas');
    this.resizerRight = document.getElementById('resizer-right');
    this.btnToggleSplitView = document.getElementById('btn-toggle-split-view');
    this.btnToggleDiffPanel = document.getElementById('btn-toggle-diff-panel');
    this.btnTogglePluginMesh = document.getElementById('btn-toggle-plugin-mesh');
    this.btnBrowseMarketplace = document.getElementById('btn-browse-marketplace');
    this.btnConnectMcp = document.getElementById('btn-connect-mcp');
    this.marketplacePanel = document.getElementById('marketplace-panel');
    this.marketplaceList = document.getElementById('marketplace-list');
    this.mcpServersList = document.getElementById('mcp-servers-list');
    this.mcpServerName = document.getElementById('mcp-server-name');
    this.mcpServerCmd = document.getElementById('mcp-server-cmd');
    this.btnMcpConnect = document.getElementById('btn-mcp-connect');
    this.btnToggleTerminalPane = document.getElementById('btn-toggle-terminal-pane');
    this.btnToggleOutline = document.getElementById('btn-toggle-outline');
    this.btnCloseCanvas = document.getElementById('btn-close-canvas');
    this.btnMaximizeCanvas = document.getElementById('btn-maximize-canvas');
    this.pluginMeshGrid = document.getElementById('plugin-mesh-grid');
    this.diffStreamContainer = document.getElementById('diff-stream-container');
    this.canvasDiffCount = document.getElementById('canvas-diff-count');
    this.btnTabLaunchStudio = document.getElementById('btn-tab-launch-studio');

    // Terminal interactive CLI
    this.terminalViewOutput = document.getElementById('terminal-view-output');
    this.terminalCliInput = document.getElementById('terminal-cli-input');
    this.btnSendTerminalCmd = document.getElementById('btn-send-terminal-cmd');

    // Diff batch actions
    this.btnAcceptAllDiffs = document.getElementById('btn-accept-all-diffs');
    this.btnRejectAllDiffs = document.getElementById('btn-reject-all-diffs');

    // Composer Controls & Modes
    this.btnToggleAccess = document.getElementById('btn-toggle-access');
    this.accessModeLabel = document.getElementById('access-mode-label');
    this.btnToggleGoal = document.getElementById('btn-toggle-goal');
    this.btnTogglePlan = document.getElementById('btn-toggle-plan');
    this.btnComposerActions = document.getElementById('btn-composer-actions');
    this.composerActionsMenu = document.getElementById('composer-actions-menu');
    this.slashAutocompleteMenu = document.getElementById('slash-autocomplete-menu');
    this.atAutocompleteMenu = document.getElementById('at-autocomplete-menu');
    this.atMenuItemsContainer = document.getElementById('at-menu-items-container');
    this.cachedWorkspaceFiles = [];

    this.btnAttachContext = document.getElementById('btn-attach-context');
    this.contextAttachMenu = document.getElementById('context-attach-menu');
    this.btnRefreshState = document.getElementById('btn-refresh-state');
    this.btnModelEffortSelector = document.getElementById('btn-model-effort-selector');
    this.modelEffortLabel = document.getElementById('model-effort-label');
    this.effortPickerMenu = document.getElementById('effort-picker-menu');
    this.btnMic = document.getElementById('btn-mic');

    // Goal & Plan Banners
    this.activeGoalBanner = document.getElementById('active-goal-banner');
    this.activeGoalText = document.getElementById('active-goal-text');
    this.goalTimerLabel = document.getElementById('goal-timer-label');
    this.btnEditGoal = document.getElementById('btn-edit-goal');
    this.btnPauseGoal = document.getElementById('btn-pause-goal');
    this.btnDeleteGoal = document.getElementById('btn-delete-goal');

    this.activePlanBanner = document.getElementById('active-plan-banner');
    this.activePlanSummary = document.getElementById('active-plan-summary');
    this.btnExecutePlan = document.getElementById('btn-execute-plan');
    this.btnDismissPlan = document.getElementById('btn-dismiss-plan');

    // Queue Drawer & Controls
    this.queueDrawer = document.getElementById('aether-queue-drawer');
    this.queueCountBadge = document.getElementById('queue-count-badge');
    this.queueTasksList = document.getElementById('queue-tasks-list');
    this.btnClearQueue = document.getElementById('btn-clear-queue');
    this.btnToggleQueue = document.getElementById('btn-toggle-queue');
    this.btnQueueTask = document.getElementById('btn-queue-task');

    // Subagent Review Elements
    this.subagentReviewCard = document.getElementById('subagent-review-card');
    this.subagentScoreBadge = document.getElementById('subagent-score-badge');
    this.subagentFindingsList = document.getElementById('subagent-findings-list');

    this.terminalTabsNav = document.getElementById('terminal-tabs-nav');

    // Git Panel Elements
    this.gitBranchLabel = document.getElementById('git-branch-label');
    this.gitStatusList = document.getElementById('git-status-list');
    this.gitLogList = document.getElementById('git-log-list');
    this.gitCommitMsg = document.getElementById('git-commit-msg');
    this.btnGitCommit = document.getElementById('btn-git-commit');
    this.btnGitRefresh = document.getElementById('btn-git-refresh');

    // Code Editor Elements
    this.editorTextarea = document.getElementById('editor-textarea');
    this.editorFileTitle = document.getElementById('editor-file-title');
    this.btnSaveEditor = document.getElementById('btn-save-editor');
    this.editorCurrentPath = null;

    // Modals
    this.modalPalette = document.getElementById('modal-command-palette');
    this.paletteSearchInput = document.getElementById('palette-search-input');
    this.paletteResultsList = document.getElementById('palette-results-list');

    this.modalSettings = document.getElementById('modal-app-settings');
    this.btnCloseSettings = document.getElementById('btn-close-settings-modal');
    this.btnCancelSettings = document.getElementById('btn-cancel-settings');
    this.btnSaveSettings = document.getElementById('btn-save-settings');
    this.settingsGatewayUrl = document.getElementById('settings-gateway-url');
    this.settingsApiKey = document.getElementById('settings-api-key');
    this.settingsDefaultModel = document.getElementById('settings-default-model');
    this.settingsDefaultPermission = document.getElementById('settings-default-permission');
    this.settingsUserName = document.getElementById('settings-user-name');
    this.settingsFeedback = document.getElementById('settings-feedback-alert');
    this.settingsThemeSelect = document.getElementById('settings-theme-select');
    this.settingsVoiceLang = document.getElementById('settings-voice-lang');

    this.modalStudio = document.getElementById('modal-evolution-studio');
    this.btnCloseStudio = document.getElementById('btn-close-studio-modal');
    this.btnCancelStudio = document.getElementById('btn-cancel-studio');
    this.btnExecuteHotswap = document.getElementById('btn-execute-hotswap');
    this.inputStudioPluginId = document.getElementById('input-studio-plugin-id');
    this.selectStudioPluginType = document.getElementById('select-studio-plugin-type');
    this.textareaStudioPluginCode = document.getElementById('textarea-studio-plugin-code');
    this.textareaStudioTestCode = document.getElementById('textarea-studio-test-code');
    this.studioFeedbackAlert = document.getElementById('studio-eval-feedback-alert');
  }

  initResizableDividers() {
    const savedSidebarWidth = localStorage.getItem('aether_sidebar_width');
    if (savedSidebarWidth && this.aetherSidebar) {
      this.aetherSidebar.style.width = savedSidebarWidth + 'px';
    }

    let isResizingLeft = false;
    this.resizerLeft.addEventListener('mousedown', (e) => {
      isResizingLeft = true;
      document.body.classList.add('is-resizing');
      this.resizerLeft.classList.add('is-dragging');
      e.preventDefault();
    });

    const savedRightWidth = localStorage.getItem('aether_right_width');
    if (savedRightWidth && this.aetherRightCanvas) {
      this.aetherRightCanvas.style.width = savedRightWidth + 'px';
    }

    let isResizingRight = false;
    this.resizerRight.addEventListener('mousedown', (e) => {
      isResizingRight = true;
      document.body.classList.add('is-resizing');
      this.resizerRight.classList.add('is-dragging');
      e.preventDefault();
    });

    window.addEventListener('mousemove', (e) => {
      if (isResizingLeft) {
        const newWidth = Math.min(Math.max(e.clientX, 180), 480);
        this.aetherSidebar.style.width = `${newWidth}px`;
        localStorage.setItem('aether_sidebar_width', newWidth);
      } else if (isResizingRight) {
        const newWidth = Math.min(Math.max(window.innerWidth - e.clientX, 280), window.innerWidth * 0.7);
        this.aetherRightCanvas.style.width = `${newWidth}px`;
        localStorage.setItem('aether_right_width', newWidth);
      }
    });

    window.addEventListener('mouseup', () => {
      if (isResizingLeft || isResizingRight) {
        isResizingLeft = false;
        isResizingRight = false;
        document.body.classList.remove('is-resizing');
        this.resizerLeft.classList.remove('is-dragging');
        this.resizerRight.classList.remove('is-dragging');
      }
    });

    this.resizerLeft.addEventListener('dblclick', () => this.toggleSidebar());
    this.resizerRight.addEventListener('dblclick', () => this.toggleRightCanvas());
  }

  toggleSidebar() {
    const isHidden = this.aetherSidebar.style.display === 'none';
    this.aetherSidebar.style.display = isHidden ? 'flex' : 'none';
    this.resizerLeft.style.display = isHidden ? 'block' : 'none';
  }

  toggleRightCanvas() {
    this.aetherRightCanvas.classList.toggle('collapsed');
    this.resizerRight.style.display = this.aetherRightCanvas.classList.contains('collapsed') ? 'none' : 'block';
    this.btnToggleSplitView.classList.toggle('active-btn', !this.aetherRightCanvas.classList.contains('collapsed'));
  }

  switchRightCanvasTab(tabName) {
    if (this.aetherRightCanvas.classList.contains('collapsed')) {
      this.toggleRightCanvas();
    }
    document.querySelectorAll('.canvas-tab-pill').forEach(btn => {
      btn.classList.toggle('active', btn.dataset.view === tabName);
    });
    document.querySelectorAll('.canvas-tab-content').forEach(content => {
      content.classList.toggle('active', content.id === `content-view-${tabName}`);
    });
  }

  async loadInitialConfig() {
    try {
      const res = await fetch('/api/config');
      if (res.ok) {
        const cfg = await res.json();
        if (cfg.active_model) {
          const idx = this.models.findIndex(m => m.id === cfg.active_model || m.name.toLowerCase().includes(cfg.active_model.toLowerCase()));
          if (idx !== -1) {
            this.currentModelIdx = idx;
            this.modelEffortLabel.textContent = `${this.models[this.currentModelIdx].badge} ${this.currentEffort}`;
            this.modelTopBadge.textContent = this.models[this.currentModelIdx].badge;
          }
        }
        if (cfg.user_name && this.userDisplayName) {
          this.userDisplayName.textContent = cfg.user_name;
        }
        if (cfg.user_avatar && this.userAvatarBadge) {
          this.userAvatarBadge.textContent = cfg.user_avatar;
        }
        if (this.settingsGatewayUrl) this.settingsGatewayUrl.value = cfg.gateway_url || "https://api.deepseek.com/v1";
        if (this.settingsApiKey) {
          this.settingsApiKey.value = "";
          this.settingsApiKey.placeholder = cfg.api_key_configured ? "Configured via environment" : "Set DEEPSEEK_API_KEY before launch";
        }
        if (this.settingsDefaultModel) this.settingsDefaultModel.value = cfg.active_model || "deepseek-v4-pro";
        if (this.settingsDefaultPermission) this.settingsDefaultPermission.value = cfg.permission_mode || "full_access";
        if (this.settingsUserName) this.settingsUserName.value = cfg.user_name || "Aether Developer";
      }
    } catch (e) {
      console.warn("Using default isolated config", e);
    }
  }

  async loadWorkspaceTree() {
    try {
      const res = await fetch('/api/workspace/tree');
      if (res.ok) {
        const tree = await res.json();
        this.renderWorkspaceTree(tree);
      }
    } catch (e) {
      console.warn("Failed to load workspace tree", e);
    }
  }

  renderWorkspaceTree(tree) {
    if (!this.sidebarProjectTree) return;

    let html = `
      <div class="tree-section-header">
        <span>工程项目</span>
        <button class="icon-tiny-btn" id="btn-add-project-dynamic" title="新建独立工程目录">+</button>
      </div>
    `;

    tree.forEach(proj => {
      html += `
        <div class="project-group" data-project="${this.escapeHtml(proj.name)}">
          <div class="project-folder-row" data-project="${this.escapeHtml(proj.name)}">
            <div class="project-folder-left">
              <svg class="folder-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
              <span class="project-name">${this.escapeHtml(proj.name)}</span>
            </div>
            <div class="project-actions">
              <button class="btn-item-action add-session-btn" data-project="${this.escapeHtml(proj.name)}" title="在此工程新建会话">+</button>
              <button class="btn-item-action delete-action delete-project-btn" data-project="${this.escapeHtml(proj.name)}" title="删除此工程项目">🗑️</button>
            </div>
          </div>
          <div class="task-items-list">
      `;

      proj.tasks.forEach(t => {
        const isActive = (t.id === this.activeTaskId || t.active);
        html += `
          <div class="task-item ${isActive ? 'active' : ''}" data-task-id="${this.escapeHtml(t.id)}" data-project="${this.escapeHtml(proj.name)}" data-title="${this.escapeHtml(t.title)}">
            <div class="task-left-meta">
              <span class="task-title">${this.escapeHtml(t.title)}</span>
              ${isActive ? '<span class="task-spinner-icon">◌</span>' : ''}
            </div>
            <div class="task-actions">
              <button class="btn-item-action rename-task-btn" data-task-id="${this.escapeHtml(t.id)}" data-project="${this.escapeHtml(proj.name)}" data-title="${this.escapeHtml(t.title)}" title="重命名会话">✏️</button>
              <button class="btn-item-action delete-action delete-task-btn" data-task-id="${this.escapeHtml(t.id)}" data-project="${this.escapeHtml(proj.name)}" title="删除此会话">🗑️</button>
            </div>
          </div>
        `;
      });

      html += `
          </div>
        </div>
      `;
    });

    this.sidebarProjectTree.innerHTML = html;

    // Bind item click to switch task
    this.sidebarProjectTree.querySelectorAll('.task-item').forEach(item => {
      item.addEventListener('click', (e) => {
        if (e.target.closest('.task-actions')) return;
        this.switchTask(item.dataset.taskId, item.dataset.project, item.dataset.title);
      });
      const fileEl = item;
      const filePath = item.dataset.title;
      fileEl.addEventListener('dblclick', () => this.openFileInEditor(filePath));
    });

    // Bind Add Session (+) per project
    this.sidebarProjectTree.querySelectorAll('.add-session-btn').forEach(btn => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const proj = btn.dataset.project;
        this.createTaskInProject(proj);
      });
    });

    // Bind Delete Project (🗑️) per project
    this.sidebarProjectTree.querySelectorAll('.delete-project-btn').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        e.stopPropagation();
        const proj = btn.dataset.project;
        if (confirm(`确定要彻底删除工程项目【${proj}】及其全部会话记录吗？\n（此操作仅影响 ~/.aether/workspace/${proj} 隔离环境）`)) {
          await fetch('/api/workspace/delete_project', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({project: proj})
          });
          await this.loadWorkspaceTree();
        }
      });
    });

    // Bind Rename Task (✏️)
    this.sidebarProjectTree.querySelectorAll('.rename-task-btn').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        e.stopPropagation();
        const taskId = btn.dataset.taskId;
        const currentTitle = btn.dataset.title;
        const newTitle = prompt("重命名此任务会话:", currentTitle);
        if (newTitle && newTitle !== currentTitle) {
          if (this.activeTaskId === taskId) this.navbarTaskTitle.textContent = newTitle;
          if (this.taskStore[taskId]) this.taskStore[taskId].title = newTitle;
          await fetch('/api/workspace/rename_task', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({task_id: taskId, new_title: newTitle})
          });
          this.loadWorkspaceTree();
        }
      });
    });

    // Bind Delete Task (🗑️)
    this.sidebarProjectTree.querySelectorAll('.delete-task-btn').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        e.stopPropagation();
        const taskId = btn.dataset.taskId;
        if (confirm("确定要删除此任务会话吗？")) {
          await fetch('/api/workspace/delete_task', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({task_id: taskId})
          });
          delete this.taskStore[taskId];
          await this.loadWorkspaceTree();
          if (this.activeTaskId === taskId) this.createNewTask();
        }
      });
    });

    const btnAdd = document.getElementById('btn-add-project-dynamic') || document.getElementById('btn-add-project');
    if (btnAdd) {
      btnAdd.onclick = () => this.createNewProject();
    }
  }

  async createNewProject() {
    const projName = prompt("新建独立工程名称 (New Project Name):", `project-${Date.now().toString().slice(-4)}`);
    if (!projName) return;

    try {
      await fetch('/api/workspace/create_project', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({name: projName})
      });
      await this.loadWorkspaceTree();
      this.activeProject = projName;
      this.createTaskInProject(projName);
    } catch (e) {
      alert(`创建工程失败: ${e.message}`);
    }
  }

  async createTaskInProject(projectName) {
    const taskName = prompt(`在工程【${projectName}】中新建会话名称:`, `任务 ${new Date().toLocaleTimeString()}`);
    if (!taskName) return;

    try {
      const res = await fetch('/api/workspace/create_task', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          project: projectName,
          title: taskName
        })
      });
      const data = await res.json();
      await this.loadWorkspaceTree();
      this.switchTask(data.task.id, projectName, taskName);
      this.composerMainInput.focus();
    } catch {
      const newTaskId = `task-${projectName}-${Date.now()}`;
      this.switchTask(newTaskId, projectName, taskName);
    }
  }

  bindEvents() {
    // Global copy button handler for code cards
    document.addEventListener('click', (e) => {
      const copyBtn = e.target.closest('.md-copy-btn');
      if (copyBtn) {
        const codeText = copyBtn.dataset.code || '';
        navigator.clipboard.writeText(codeText).then(() => {
          const orig = copyBtn.innerHTML;
          copyBtn.classList.add('copied');
          copyBtn.innerHTML = '✓ 已复制';
          setTimeout(() => {
            copyBtn.classList.remove('copied');
            copyBtn.innerHTML = orig;
          }, 1500);
        });
      }
    });

    // Marketplace & MCP Events
    if (this.btnBrowseMarketplace) {
      this.btnBrowseMarketplace.addEventListener('click', () => {
        this.toggleMarketplace();
      });
    }
    if (this.btnConnectMcp) {
      this.btnConnectMcp.addEventListener('click', () => {
        this.toggleMarketplace();
      });
    }
    if (this.btnMcpConnect) {
      this.btnMcpConnect.addEventListener('click', () => this.connectMcpServer());
    }

    // 1. Brand dropdown popover
    this.brandDropdownBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      const isVisible = this.aetherBrandMenu.style.display === 'block';
      this.closeAllMenus();
      this.aetherBrandMenu.style.display = isVisible ? 'none' : 'block';
    });

    document.getElementById('menu-open-studio').addEventListener('click', () => this.openEvolutionStudio());
    document.getElementById('menu-open-settings').addEventListener('click', () => this.openSettingsModal());
    document.getElementById('menu-switch-workspace').addEventListener('click', async () => {
      const newWs = prompt("输入工作区目录 (Workspace Path):", "~/.aether/workspace");
      if (newWs) {
        await fetch('/api/config', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({workspace_dir: newWs})
        });
        this.loadWorkspaceTree();
      }
    });

    document.getElementById('menu-reload-plugins').addEventListener('click', () => {
      this.refreshPlugins();
      alert("所有微内核插件已执行热重载并同步完成！");
    });

    // 2. Navigation Back / Forward
    this.btnNavBack.addEventListener('click', () => {
      if (this.historyPointer > 0) {
        this.historyPointer--;
        const prevId = this.taskHistoryStack[this.historyPointer];
        if (this.taskStore[prevId]) {
          this.switchTask(prevId, this.taskStore[prevId].project, this.taskStore[prevId].title, false);
        }
      }
    });

    this.btnNavForward.addEventListener('click', () => {
      if (this.historyPointer < this.taskHistoryStack.length - 1) {
        this.historyPointer++;
        const nextId = this.taskHistoryStack[this.historyPointer];
        if (this.taskStore[nextId]) {
          this.switchTask(nextId, this.taskStore[nextId].project, this.taskStore[nextId].title, false);
        }
      }
    });

    // 3. Task Options (···) and Direct Title Click
    this.btnTaskOptions.addEventListener('click', (e) => {
      e.stopPropagation();
      const isVisible = this.taskOptionsMenu.style.display === 'block';
      this.closeAllMenus();
      this.taskOptionsMenu.style.display = isVisible ? 'none' : 'block';
    });

    this.navbarTaskTitle.addEventListener('click', async () => {
      const currentTitle = this.navbarTaskTitle.textContent;
      const newTitle = prompt("重命名当前任务:", currentTitle);
      if (newTitle && newTitle !== currentTitle) {
        this.navbarTaskTitle.textContent = newTitle;
        if (this.taskStore[this.activeTaskId]) this.taskStore[this.activeTaskId].title = newTitle;
        await fetch('/api/workspace/rename_task', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({task_id: this.activeTaskId, new_title: newTitle})
        });
        this.loadWorkspaceTree();
      }
    });

    document.getElementById('menu-task-rename').addEventListener('click', async () => {
      const currentTitle = this.navbarTaskTitle.textContent;
      const newTitle = prompt("重命名当前任务:", currentTitle);
      if (newTitle && newTitle !== currentTitle) {
        this.navbarTaskTitle.textContent = newTitle;
        if (this.taskStore[this.activeTaskId]) this.taskStore[this.activeTaskId].title = newTitle;
        await fetch('/api/workspace/rename_task', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({task_id: this.activeTaskId, new_title: newTitle})
        });
        this.loadWorkspaceTree();
      }
    });

    document.getElementById('menu-task-export').addEventListener('click', () => {
      const text = this.chatThreadContainer.innerText;
      const blob = new Blob([text], {type: 'text/markdown;charset=utf-8'});
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `${this.navbarTaskTitle.textContent}.md`;
      a.click();
    });

    document.getElementById('menu-task-clear').addEventListener('click', () => {
      if (confirm("确定要清空此任务的对话记录吗？")) {
        this.chatThreadContainer.innerHTML = '';
        if (this.taskStore[this.activeTaskId]) this.taskStore[this.activeTaskId].historyHtml = '';
        this.saveCurrentSessionToServer();
      }
    });

    document.getElementById('menu-task-delete').addEventListener('click', async () => {
      if (confirm(`确定要删除任务【${this.navbarTaskTitle.textContent}】吗？`)) {
        await fetch('/api/workspace/delete_task', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({task_id: this.activeTaskId})
        });
        delete this.taskStore[this.activeTaskId];
        await this.loadWorkspaceTree();
        this.createNewTask();
      }
    });

    // 4. Model Dropdown & Selector
    this.btnModelDropdown.addEventListener('click', (e) => {
      e.stopPropagation();
      const isVisible = this.modelPickerMenu.style.display === 'block';
      this.closeAllMenus();
      this.modelPickerMenu.style.display = isVisible ? 'none' : 'block';
    });

    document.querySelectorAll('.model-option-item').forEach(item => {
      item.addEventListener('click', async () => {
        const modelName = item.dataset.model;
        const found = this.models.find(m => m.name === modelName);
        if (found) {
          this.currentModelIdx = this.models.indexOf(found);
          this.modelEffortLabel.textContent = `${found.badge} ${this.currentEffort}`;
          this.modelTopBadge.textContent = found.badge;
          document.querySelectorAll('.model-option-item').forEach(i => i.classList.remove('active'));
          item.classList.add('active');
          await fetch('/api/config', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({active_model: found.id})
          });
        }
        this.modelPickerMenu.style.display = 'none';
      });
    });

    // 5. Effort Selector
    this.btnModelEffortSelector.addEventListener('click', (e) => {
      e.stopPropagation();
      const isVisible = this.effortPickerMenu.style.display === 'block';
      this.closeAllMenus();
      this.effortPickerMenu.style.display = isVisible ? 'none' : 'block';
    });

    this.effortPickerMenu.querySelectorAll('.menu-action-item').forEach(item => {
      item.addEventListener('click', () => {
        this.currentEffort = item.dataset.effort;
        const currentModel = this.models[this.currentModelIdx].badge;
        this.modelEffortLabel.textContent = `${currentModel} ${this.currentEffort}`;
        this.effortPickerMenu.style.display = 'none';
      });
    });

    // 6. Context Attachment (+)
    this.btnAttachContext.addEventListener('click', (e) => {
      e.stopPropagation();
      const isVisible = this.contextAttachMenu.style.display === 'block';
      this.closeAllMenus();
      this.contextAttachMenu.style.display = isVisible ? 'none' : 'block';
    });

    document.getElementById('attach-opt-file').addEventListener('click', async () => {
      try {
        const res = await fetch('/api/workspace/files');
        const files = await res.json();
        if (files.length === 0) {
          const custom = prompt("输入文件相对路径 (@file):", "aether/core/agent.py");
          if (custom) this.injectContext(`@${custom} `);
        } else {
          const listStr = files.slice(0, 10).map((f, i) => `${i + 1}. ${f.path}`).join('\n');
          const choice = prompt(`选择要附加的文件序号或输入路径:\n${listStr}`, "1");
          if (choice) {
            const idx = parseInt(choice, 10) - 1;
            const chosenFile = files[idx] ? files[idx].path : choice;
            this.injectContext(`@${chosenFile} `);
          }
        }
      } catch {
        const custom = prompt("输入文件路径 (@file):", "aether/core/agent.py");
        if (custom) this.injectContext(`@${custom} `);
      }
      this.contextAttachMenu.style.display = 'none';
    });

    document.getElementById('attach-opt-terminal').addEventListener('click', () => {
      const logs = this.terminalViewOutput.textContent.slice(-500);
      this.injectContext(`\n\`\`\`terminal_output\n${logs}\n\`\`\`\n`);
      this.contextAttachMenu.style.display = 'none';
    });

    document.getElementById('attach-opt-diff').addEventListener('click', async () => {
      const res = await fetch('/api/diffs');
      const diffs = await res.json();
      if (diffs.length > 0) {
        this.injectContext(`\n\`\`\`diff\n${diffs[0].diff_text}\n\`\`\`\n`);
      } else {
        alert("当前工作区无待审查的代码变更！");
      }
      this.contextAttachMenu.style.display = 'none';
    });

    document.getElementById('attach-opt-plugins').addEventListener('click', async () => {
      const res = await fetch('/api/plugins');
      const plugins = await res.json();
      const summary = plugins.map(p => `- ${p.name} (v${p.version}): ${p.description}`).join('\n');
      this.injectContext(`\n\`\`\`plugin_mesh\n${summary}\n\`\`\`\n`);
      this.contextAttachMenu.style.display = 'none';
    });

    // 7. Refresh State (⟳)
    this.btnRefreshState.addEventListener('click', async () => {
      const icon = this.btnRefreshState.querySelector('.refresh-icon-svg');
      icon.classList.add('is-spinning');
      await Promise.all([this.loadWorkspaceTree(), this.refreshPlugins(), this.refreshDiffs()]);
      setTimeout(() => icon.classList.remove('is-spinning'), 600);
    });

    // 8. Voice Speech-to-Text (🎙️)
    this.btnMic.addEventListener('click', () => this.toggleVoiceRecording());

    // 9. Command Palette (⌘K)
    this.btnSearchTasks.addEventListener('click', () => this.openCommandPalette());
    this.paletteSearchInput.addEventListener('input', (e) => this.filterPalette(e.target.value));
    this.paletteResultsList.querySelectorAll('.palette-item').forEach(item => {
      item.addEventListener('click', () => this.executePaletteAction(item.dataset.action));
    });

    // 10. Interactive Terminal CLI (with per-tab buffer isolation)
    const executeTerminalCmd = async () => {
      const cmd = this.terminalCliInput.value.trim();
      if (!cmd) return;
      this.terminalCliInput.value = '';
      this.appendToTerminalBuffer(`\n$ ${cmd}\n`);

      try {
        const res = await fetch('/api/terminal/exec', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({command: cmd})
        });
        const data = await res.json();
        this.appendToTerminalBuffer(`${data.output || '(No output)'}\n`);
      } catch (e) {
        this.appendToTerminalBuffer(`Error: ${e.message}\n`);
      }
      this.terminalViewOutput.scrollTop = this.terminalViewOutput.scrollHeight;
    };

    this.btnSendTerminalCmd.addEventListener('click', executeTerminalCmd);
    this.terminalCliInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        executeTerminalCmd();
      }
    });

    // 11. Diff Accept / Reject batch actions
    this.btnAcceptAllDiffs.addEventListener('click', () => {
      this.diffStreamContainer.innerHTML = `
        <div class="canvas-empty-state">
          <div class="empty-icon">✓</div>
          <div class="empty-title">所有修改已成功应用合并</div>
          <div class="empty-desc">代码变更已写入工作区并自动同步至沙箱执行环境中。</div>
        </div>
      `;
      this.canvasDiffCount.textContent = '0';
    });

    this.btnRejectAllDiffs.addEventListener('click', () => {
      this.diffStreamContainer.innerHTML = `
        <div class="canvas-empty-state">
          <div class="empty-icon">✕</div>
          <div class="empty-title">所有修改已被拒绝并回滚</div>
        </div>
      `;
      this.canvasDiffCount.textContent = '0';
    });

    // 12. Settings Modal
    this.btnAppSettings.addEventListener('click', () => this.openSettingsModal());
    document.getElementById('btn-user-profile').addEventListener('click', () => this.openSettingsModal());
    this.btnCloseSettings.addEventListener('click', () => this.modalSettings.classList.remove('show'));
    this.btnCancelSettings.addEventListener('click', () => this.modalSettings.classList.remove('show'));
    this.btnSaveSettings.addEventListener('click', () => this.saveSettings());

    // 13. Studio launch from Tab
    if (this.btnTabLaunchStudio) {
      this.btnTabLaunchStudio.addEventListener('click', () => this.openEvolutionStudio());
    }

    // 14. Composer Modes (Goal / Plan / Access / Actions)
    this.btnToggleAccess.addEventListener('click', () => {
      this.isFullAccess = !this.isFullAccess;
      this.btnToggleAccess.classList.toggle('active', this.isFullAccess);
      this.accessModeLabel.textContent = this.isFullAccess ? '完全访问' : '沙箱隔离';
    });

    this.btnToggleGoal.addEventListener('click', () => {
      this.isGoalMode = !this.isGoalMode;
      this.btnToggleGoal.classList.toggle('active', this.isGoalMode);
      this.btnToggleGoal.classList.toggle('mode-goal', this.isGoalMode);
      this.activeGoalBanner.style.display = this.isGoalMode ? 'flex' : 'none';
    });

    this.btnTogglePlan.addEventListener('click', () => {
      this.isPlanMode = !this.isPlanMode;
      this.btnTogglePlan.classList.toggle('active', this.isPlanMode);
      this.btnTogglePlan.classList.toggle('mode-plan', this.isPlanMode);
      this.activePlanBanner.style.display = this.isPlanMode ? 'flex' : 'none';
    });

    this.btnComposerActions.addEventListener('click', (e) => {
      e.stopPropagation();
      const isVisible = this.composerActionsMenu.style.display === 'block';
      this.closeAllMenus();
      this.composerActionsMenu.style.display = isVisible ? 'none' : 'block';
    });

    this.composerActionsMenu.querySelectorAll('.menu-action-item').forEach(item => {
      item.addEventListener('click', () => {
        const action = item.dataset.action;
        this.triggerComposerAction(action);
        this.composerActionsMenu.style.display = 'none';
      });
    });

    // Slash command and @ Context Mention typing detection
    this.composerMainInput.addEventListener('input', async () => {
      const val = this.composerMainInput.value;
      const cursorPos = this.composerMainInput.selectionStart || val.length;
      const textBeforeCursor = val.slice(0, cursorPos);

      if (this.isStreaming && val.trim().length > 0) {
        if (this.btnQueueTask) this.btnQueueTask.style.display = 'inline-flex';
      } else {
        if (this.btnQueueTask) this.btnQueueTask.style.display = 'none';
      }

      if (val.startsWith('/') || val.includes('\n/')) {
        this.slashAutocompleteMenu.style.display = 'block';
        if (this.atAutocompleteMenu) this.atAutocompleteMenu.style.display = 'none';
      } else {
        this.slashAutocompleteMenu.style.display = 'none';
      }

      // Check if cursor is right after an '@' token e.g. '@' or '@app'
      const atMatch = textBeforeCursor.match(/@([a-zA-Z0-9_\-\.\/]*)$/);
      if (atMatch && !val.startsWith('/')) {
        const query = atMatch[1].toLowerCase();
        await this.showAtAutocomplete(query, atMatch[0]);
      } else {
        if (this.atAutocompleteMenu) this.atAutocompleteMenu.style.display = 'none';
      }
    });

    this.slashAutocompleteMenu.querySelectorAll('.menu-action-item').forEach(item => {
      item.addEventListener('click', () => {
        const slash = item.dataset.slash;
        this.composerMainInput.value = `${slash} `;
        this.slashAutocompleteMenu.style.display = 'none';
        this.composerMainInput.focus();
      });
    });

    // Queue Controls
    if (this.btnQueueTask) {
      this.btnQueueTask.addEventListener('click', async () => {
        const text = this.composerMainInput.value.trim();
        if (text) {
          await this.pushQueueTask(text);
          this.composerMainInput.value = '';
          this.btnQueueTask.style.display = 'none';
        }
      });
    }

    if (this.btnClearQueue) {
      this.btnClearQueue.addEventListener('click', () => this.clearQueue());
    }

    if (this.btnToggleQueue) {
      this.btnToggleQueue.addEventListener('click', () => {
        const isHidden = this.queueTasksList.style.display === 'none';
        this.queueTasksList.style.display = isHidden ? 'flex' : 'none';
      });
    }

    // Terminal Tabs Navigation with per-tab output isolation
    if (this.terminalTabsNav) {
      this.terminalTabsNav.querySelectorAll('.terminal-tab-item').forEach(tab => {
        tab.addEventListener('click', () => this.switchTerminalTab(tab.dataset.tab, tab));
      });
      const btnClear = document.getElementById('btn-clear-term');
      if (btnClear) {
        btnClear.addEventListener('click', () => {
          this.terminalBuffers[this.activeTerminalTab] = `> Terminal cleared.\n$ `;
          this.terminalViewOutput.textContent = this.terminalBuffers[this.activeTerminalTab];
        });
      }
      const btnAdd = document.getElementById('btn-add-term-tab');
      if (btnAdd) {
        btnAdd.addEventListener('click', () => {
          const tabId = `tab-${Date.now()}`;
          const tabCount = this.terminalTabsNav.querySelectorAll('.terminal-tab-item').length + 1;
          this.terminalBuffers[tabId] = `> 终端 #${tabCount} Ready.\n$ `;
          const newTab = document.createElement('button');
          newTab.className = 'terminal-tab-item';
          newTab.dataset.tab = tabId;
          newTab.innerHTML = `<span>终端 #${tabCount}</span>`;
          newTab.onclick = () => this.switchTerminalTab(tabId, newTab);
          this.terminalTabsNav.insertBefore(newTab, btnAdd);
          // Immediately switch to the new tab
          this.switchTerminalTab(tabId, newTab);
        });
      }
    }

    this.btnExecutePlan.addEventListener('click', () => {
      const planPrompt = `请立即执行已确认的实施计划，按步骤调用相关工具并在沙箱中验证。`;
      this.startAgentExecutionTurn(planPrompt);
    });

    this.btnDismissPlan.addEventListener('click', () => {
      this.activePlanBanner.style.display = 'none';
      this.isPlanMode = false;
      this.btnTogglePlan.classList.remove('active', 'mode-plan');
    });

    // 15. Global Keyboard Shortcuts
    document.addEventListener('keydown', (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's' && this.editorCurrentPath) {
        e.preventDefault();
        this.saveEditorFile();
      } else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        this.openCommandPalette();
      } else if ((e.metaKey || e.ctrlKey) && e.key === ',') {
        e.preventDefault();
        this.openSettingsModal();
      } else if (e.key === 'Escape') {
        this.closeAllModals();
      }
    });

    document.addEventListener('click', () => this.closeAllMenus());

    // Standard Composer and Layout
    this.composerMainInput.addEventListener('input', () => {
      requestAnimationFrame(() => {
        this.composerMainInput.style.height = 'auto';
        this.composerMainInput.style.height = Math.min(this.composerMainInput.scrollHeight, 140) + 'px';
      });
    });

    this.composerMainInput.addEventListener('keydown', (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
        e.preventDefault();
        this.handleSendOrStop();
      } else if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        this.handleSendOrStop();
      }
    });

    document.addEventListener('keydown', (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'b') {
        e.preventDefault();
        this.toggleSidebar();
      } else if ((e.metaKey || e.ctrlKey) && e.key === '\\') {
        e.preventDefault();
        this.toggleRightCanvas();
      } else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'n') {
        e.preventDefault();
        this.createNewTask();
      }
    });

    this.btnSendOrStop.addEventListener('click', () => this.handleSendOrStop());
    if (this.toolAccordionCard) {
      this.toolAccordionCard.addEventListener('click', () => this.toolAccordionCard.classList.toggle('open'));
    }
    this.btnNewTask.addEventListener('click', () => this.createNewTask());

    this.btnToggleSidebar.addEventListener('click', () => this.toggleSidebar());
    this.btnToggleSplitView.addEventListener('click', () => this.toggleRightCanvas());
    this.btnToggleDiffPanel.addEventListener('click', () => this.switchRightCanvasTab('diff'));
    this.btnTogglePluginMesh.addEventListener('click', () => this.switchRightCanvasTab('plugins'));
    this.btnToggleTerminalPane.addEventListener('click', () => this.switchRightCanvasTab('terminal'));
    this.btnToggleOutline.addEventListener('click', () => this.switchRightCanvasTab('outline'));

    document.querySelectorAll('.canvas-tab-pill').forEach(pill => {
      pill.addEventListener('click', () => this.switchRightCanvasTab(pill.dataset.view));
    });

    this.btnCloseCanvas.addEventListener('click', () => this.toggleRightCanvas());
    this.btnMaximizeCanvas.addEventListener('click', () => this.aetherRightCanvas.classList.toggle('maximized'));

    // Git Panel Events
    if (this.btnGitCommit) {
      this.btnGitCommit.addEventListener('click', () => this.gitCommit());
    }
    if (this.btnGitRefresh) {
      this.btnGitRefresh.addEventListener('click', () => this.refreshGitPanel());
    }

    // Code Editor Events
    if (this.btnSaveEditor) {
      this.btnSaveEditor.addEventListener('click', () => this.saveEditorFile());
    }

    this.btnEditGoal.addEventListener('click', () => {
      const newGoal = prompt("编辑当前目标 (Edit Active Goal):", this.activeGoalText.textContent);
      if (newGoal) this.activeGoalText.textContent = newGoal;
    });

    this.btnPauseGoal.addEventListener('click', () => {
      if (this.goalTimer) {
        clearInterval(this.goalTimer);
        this.goalTimer = null;
        this.btnPauseGoal.textContent = '▶️';
      } else {
        this.startGoalTimer();
        this.btnPauseGoal.textContent = '⏸️';
      }
    });

    this.btnDeleteGoal.addEventListener('click', () => {
      this.activeGoalBanner.style.display = 'none';
      this.isGoalMode = false;
      this.btnToggleGoal.classList.remove('active', 'mode-goal');
    });

    this.btnCloseStudio.addEventListener('click', () => this.modalStudio.classList.remove('show'));
    this.btnCancelStudio.addEventListener('click', () => this.modalStudio.classList.remove('show'));
    this.btnExecuteHotswap.addEventListener('click', () => this.executeHotSwap());

    // Theme Toggle
    if (this.settingsThemeSelect) {
      this.settingsThemeSelect.addEventListener('change', (e) => {
        const theme = e.target.value;
        document.body.dataset.theme = theme;
        localStorage.setItem('aether_theme', theme);
      });
    }

    if (this.settingsVoiceLang) {
      this.settingsVoiceLang.addEventListener('change', (e) => {
        localStorage.setItem('aether_voice_lang', e.target.value);
      });
    }
  }

  triggerComposerAction(action) {
    if (action === 'ask') {
      this.composerMainInput.value = `/ask 请针对当前技术方案提供交互式决策选项卡片`;
    } else if (action === 'plan') {
      this.isPlanMode = true;
      this.btnTogglePlan.classList.add('active', 'mode-plan');
      this.activePlanBanner.style.display = 'flex';
      this.composerMainInput.value = `/plan 请帮我制定详细的实施方案并列出所需调用的工具步骤`;
    } else if (action === 'goal') {
      this.isGoalMode = true;
      this.btnToggleGoal.classList.add('active', 'mode-goal');
      this.activeGoalBanner.style.display = 'flex';
      this.composerMainInput.value = `/goal `;
    } else if (action === 'evolve') {
      this.openEvolutionStudio();
    } else if (action === 'review') {
      this.switchRightCanvasTab('diff');
    } else if (action === 'terminal') {
      this.switchRightCanvasTab('terminal');
    } else if (action === 'doctor') {
      this.composerMainInput.value = `/doctor 运行 AEL 嵌入式环境与工具链健康体检`;
    } else if (action === 'fix') {
      this.composerMainInput.value = `/fix 分析并自动修复当前代码/编译报错`;
    }
    this.composerMainInput.focus();
  }

  closeAllMenus() {
    this.aetherBrandMenu.style.display = 'none';
    this.taskOptionsMenu.style.display = 'none';
    this.modelPickerMenu.style.display = 'none';
    this.contextAttachMenu.style.display = 'none';
    this.effortPickerMenu.style.display = 'none';
    this.composerActionsMenu.style.display = 'none';
    this.slashAutocompleteMenu.style.display = 'none';
    if (this.atAutocompleteMenu) this.atAutocompleteMenu.style.display = 'none';
  }

  closeAllModals() {
    this.modalPalette.classList.remove('show');
    this.modalSettings.classList.remove('show');
    this.modalStudio.classList.remove('show');
    this.closeAllMenus();
  }

  injectContext(text) {
    this.composerMainInput.value = text + this.composerMainInput.value;
    this.composerMainInput.focus();
  }

  openCommandPalette() {
    this.closeAllMenus();
    this.modalPalette.classList.add('show');
    this.paletteSearchInput.value = '';
    this.paletteSearchInput.focus();
    this.filterPalette('');
  }

  filterPalette(query) {
    const q = query.toLowerCase();
    this.paletteResultsList.querySelectorAll('.palette-item').forEach(item => {
      const match = item.textContent.toLowerCase().includes(q);
      item.style.display = match ? 'flex' : 'none';
    });
  }

  executePaletteAction(action) {
    this.modalPalette.classList.remove('show');
    if (action === 'new-task') this.createNewTask();
    else if (action === 'toggle-plan') this.btnTogglePlan.click();
    else if (action === 'toggle-goal') this.btnToggleGoal.click();
    else if (action === 'toggle-split') this.toggleRightCanvas();
    else if (action === 'open-diff') this.switchRightCanvasTab('diff');
    else if (action === 'open-terminal') this.switchRightCanvasTab('terminal');
    else if (action === 'open-studio') this.openEvolutionStudio();
    else if (action === 'open-settings') this.openSettingsModal();
  }

  openSettingsModal() {
    this.closeAllMenus();
    this.modalSettings.classList.add('show');
  }

  async saveSettings() {
    this.settingsFeedback.style.display = 'block';
    this.settingsFeedback.style.color = '#38bdf8';
    this.settingsFeedback.textContent = '正在保存并同步设置到 ~/.aether/config.json...';

    try {
      const res = await fetch('/api/config', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          gateway_url: this.settingsGatewayUrl.value,
          active_model: this.settingsDefaultModel.value,
          permission_mode: this.settingsDefaultPermission.value,
          user_name: this.settingsUserName.value,
        })
      });
      if (res.ok) {
        this.settingsFeedback.style.color = '#10b981';
        this.settingsFeedback.textContent = '✓ 设置已成功保存并立即生效！';
        this.userDisplayName.textContent = this.settingsUserName.value;
        setTimeout(() => this.modalSettings.classList.remove('show'), 800);
      }
    } catch (e) {
      this.settingsFeedback.style.color = '#ef4444';
      this.settingsFeedback.textContent = `保存失败: ${e.message}`;
    }
  }

  toggleVoiceRecording() {
    const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SpeechRecognition) {
      alert("当前 Webview 或系统暂不支持 SpeechRecognition 原生接口。");
      return;
    }

    if (this.isRecordingVoice) {
      if (this.recognition) this.recognition.stop();
      this.isRecordingVoice = false;
      this.btnMic.classList.remove('is-recording');
    } else {
      this.recognition = new SpeechRecognition();
      this.recognition.continuous = true;
      this.recognition.interimResults = true;
      this.recognition.lang = this.settingsVoiceLang?.value || localStorage.getItem('aether_voice_lang') || 'zh-CN';

      this.recognition.onresult = (event) => {
        let transcript = '';
        for (let i = event.resultIndex; i < event.results.length; ++i) {
          transcript += event.results[i][0].transcript;
        }
        this.composerMainInput.value = transcript;
      };

      this.recognition.onerror = () => {
        this.isRecordingVoice = false;
        this.btnMic.classList.remove('is-recording');
      };

      this.recognition.onend = () => {
        this.isRecordingVoice = false;
        this.btnMic.classList.remove('is-recording');
      };

      this.recognition.start();
      this.isRecordingVoice = true;
      this.btnMic.classList.add('is-recording');
    }

    // File Drag & Drop Upload
    const composerArea = this.composerMainInput.closest('.composer-main-area') || this.composerMainInput.parentElement;
    composerArea.addEventListener('dragover', (e) => {
      e.preventDefault();
      if (this.dropZoneOverlay) this.dropZoneOverlay.style.display = 'flex';
    });
    composerArea.addEventListener('dragleave', (e) => {
      if (!composerArea.contains(e.relatedTarget)) {
        if (this.dropZoneOverlay) this.dropZoneOverlay.style.display = 'none';
      }
    });
    composerArea.addEventListener('drop', async (e) => {
      e.preventDefault();
      if (this.dropZoneOverlay) this.dropZoneOverlay.style.display = 'none';
      const files = e.dataTransfer?.files;
      if (!files || files.length === 0) return;
      for (const file of files) {
        await this.handleFileUpload(file);
      }
    });
  }

  openEvolutionStudio() {
    this.closeAllMenus();
    this.modalStudio.classList.add('show');
    this.textareaStudioPluginCode.value = `from aether.plugins.base import AetherPlugin, PluginMetadata, ToolResult

class SQLiteMonitorPlugin(AetherPlugin):
    metadata = PluginMetadata(
        id="sqlite_monitor",
        name="SQLite Database Telemetry",
        version="1.0.0",
        description="High-frequency SQLite query monitor and lock analyzer",
        plugin_type="tool"
    )
    def execute(self, db_path: str = ":memory:") -> ToolResult:
        return ToolResult(success=True, output=f"Analyzed {db_path} with 0 lock contention.")
`;
    this.textareaStudioTestCode.value = `assert plugin.metadata.id == "sqlite_monitor"\nres = plugin.execute()\nassert res.success is True`;
  }

  async executeHotSwap() {
    this.studioFeedbackAlert.style.display = 'block';
    this.studioFeedbackAlert.style.color = '#38bdf8';
    this.studioFeedbackAlert.textContent = "Running quarantined DeepSeek-Harness verification...";

    try {
      const res = await fetch('/api/plugins/evolve', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          plugin_id: this.inputStudioPluginId.value,
          plugin_type: this.selectStudioPluginType.value,
          code: this.textareaStudioPluginCode.value,
          eval_assertions: this.textareaStudioTestCode.value
        })
      });
      const data = await res.json();
      if (data.status === 'success' || data.success) {
        this.studioFeedbackAlert.style.color = '#10b981';
        this.studioFeedbackAlert.textContent = `✓ 验证通过！插件已成功热重载到 Aether 微内核 (v${data.version || '1.0.0'})`;
        this.refreshPlugins();
        setTimeout(() => this.modalStudio.classList.remove('show'), 1200);
      } else {
        this.studioFeedbackAlert.style.color = '#ef4444';
        this.studioFeedbackAlert.textContent = `✕ 验证失败: ${data.message || data.error}`;
      }
    } catch (e) {
      this.studioFeedbackAlert.style.color = '#ef4444';
      this.studioFeedbackAlert.textContent = `✕ 执行异常: ${e.message}`;
    }
  }

  startGoalTimer() {
    this.goalTimer = setInterval(() => {
      this.goalElapsedSeconds++;
      this.goalTimerLabel.textContent = `${this.goalElapsedSeconds}s`;
    }, 1000);
  }

  async saveCurrentSessionToServer() {
    try {
      await fetch('/api/workspace/save_session', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          task_id: this.activeTaskId,
          project: this.activeProject,
          title: this.navbarTaskTitle.textContent,
          history_html: this.chatThreadContainer.innerHTML,
        })
      });
      this.loadSessionHistory();
    } catch (e) {
      console.warn("Session auto-save failed", e);
    }
  }

  async switchTask(taskId, project, title, recordHistory = true) {
    if (this.activeTaskId === taskId) return;

    if (this.taskStore[this.activeTaskId]) {
      this.taskStore[this.activeTaskId].historyHtml = this.chatThreadContainer.innerHTML;
      this.saveCurrentSessionToServer();
    }

    if (recordHistory) {
      this.taskHistoryStack.push(taskId);
      this.historyPointer = this.taskHistoryStack.length - 1;
    }

    document.querySelectorAll('.task-item').forEach(i => i.classList.remove('active'));
    const targetItem = document.querySelector(`.task-item[data-task-id="${taskId}"]`);
    if (targetItem) targetItem.classList.add('active');

    this.activeTaskId = taskId;
    this.activeProject = project || 'aether-agent-core';
    this.navbarTaskTitle.textContent = title;

    // Attempt to load from server session file if not loaded
    if (!this.taskStore[taskId] || !this.taskStore[taskId].historyHtml) {
      try {
        const res = await fetch(`/api/workspace/load_session?project=${encodeURIComponent(this.activeProject)}&task_id=${encodeURIComponent(taskId)}`);
        if (res.ok) {
          const sess = await res.json();
          if (sess.history_html) {
            this.taskStore[taskId] = {
              title: sess.title || title,
              project: this.activeProject,
              historyHtml: sess.history_html,
            };
          }
        }
      } catch (e) {
        console.warn("Failed to load server session", e);
      }
    }

    if (!this.taskStore[taskId]) {
      this.taskStore[taskId] = { title, project: this.activeProject, historyHtml: null };
    }

    if (this.taskStore[taskId].historyHtml) {
      this.chatThreadContainer.innerHTML = this.taskStore[taskId].historyHtml;
      const acc = document.getElementById('tool-accordion-card');
      if (acc) acc.onclick = () => acc.classList.toggle('open');
    } else {
      this.chatThreadContainer.innerHTML = `
        <div class="message-row assistant-row">
          <div class="assistant-content-flow">
            <div class="assistant-prose">
              <p>已切换至独立任务 <strong>${this.escapeHtml(title)}</strong>（工程：<code>${this.escapeHtml(this.activeProject)}</code>）。</p>
              <p>向 Aether 下达任务目标或代码修改需求：</p>
            </div>
          </div>
        </div>
      `;
    }
  }

  async createNewTask() {
    this.createTaskInProject(this.activeProject);
  }

  handleSendOrStop() {
    if (this.isStreaming) {
      this.stopStreaming();
      return;
    }

    let text = this.composerMainInput.value.trim();
    if (!text) return;

    this.composerMainInput.value = '';
    this.composerMainInput.style.height = 'auto';
    this.slashAutocompleteMenu.style.display = 'none';

    if (text.startsWith('/compare')) {
      const promptText = text.replace('/compare', '').trim();
      this.startABComparison(promptText);
      return;
    }

    if (this.isPlanMode && !text.startsWith('/plan') && !text.startsWith('[Plan')) {
      this.activePlanSummary.textContent = text.slice(0, 45) + '...';
      this.activePlanBanner.style.display = 'flex';
      text = `[Plan Mode 规划任务]: ${text}\n请先生成详细实施方案（包含步骤、文件修改点和测试计划），等待用户确认后再执行。`;
    } else if (this.isGoalMode && !text.startsWith('/goal') && !text.startsWith('[Goal')) {
      this.activeGoalText.textContent = text.slice(0, 45);
      this.activeGoalBanner.style.display = 'flex';
      text = `[Goal 目标驱动模式]: ${text}\n请自主循环推进目标，直到全部断言或仿真验证通过。`;
    }

    this.appendUserMessage(text);
    this.startAgentExecutionTurn(text);
  }

  appendUserMessage(text) {
    const row = document.createElement('div');
    row.className = 'message-row user-row';
    row.innerHTML = `
      <div class="user-bubble-capsule">${this.escapeHtml(text)}</div>
    `;
    this.chatThreadContainer.appendChild(row);
    this.smoothScrollToBottom();
  }

  startAgentExecutionTurn(prompt) {
    this.isStreaming = true;
    this.sendIconSvg.style.display = 'none';
    this.stopIconSquare.style.display = 'block';

    this.elapsedSeconds = 0;
    const timeBadgeId = `time-badge-${Date.now()}`;
    const proseId = `prose-${Date.now()}`;
    const traceId = `trace-${Date.now()}`;
    const accordionId = `tool-acc-${Date.now()}`;

    const assistantRow = document.createElement('div');
    assistantRow.className = 'message-row assistant-row';
    assistantRow.innerHTML = `
      <div class="assistant-content-flow">
        <div class="processing-time-badge" id="${timeBadgeId}">已处理 0s</div>
        <div class="assistant-prose" id="${proseId}"></div>
        <div class="aether-tool-accordion" id="${accordionId}">
          <div class="tool-accordion-summary">
            <div class="tool-summary-left">
              <svg class="wrench-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>
              <span class="tool-summary-text">已加载工具读取文件运行了多个命令</span>
            </div>
            <svg class="accordion-chevron" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>
          </div>
          <div class="tool-accordion-details">
            <div class="tool-step-item">✓ [quarantined_sandbox] 初始化隔离环境与上下文 (~/.aether/workspace)</div>
          </div>
        </div>
        <div class="status-trace-text" id="${traceId}">Reasoning with ${this.models[this.currentModelIdx].name}...</div>
      </div>
    `;
    this.chatThreadContainer.appendChild(assistantRow);

    const timeBadgeEl = document.getElementById(timeBadgeId);
    const proseEl = document.getElementById(proseId);
    const traceEl = document.getElementById(traceId);
    const accordionEl = document.getElementById(accordionId);

    accordionEl.addEventListener('click', () => accordionEl.classList.toggle('open'));

    this.streamTimer = setInterval(() => {
      this.elapsedSeconds++;
      const mins = Math.floor(this.elapsedSeconds / 60);
      const secs = this.elapsedSeconds % 60;
      timeBadgeEl.textContent = mins > 0 ? `已处理 ${mins}m ${secs}s` : `已处理 ${secs}s`;
    }, 1000);

    let rawStreamAccumulator = "";
    this.tokenBuffer = new AetherStreamingBuffer((batchText) => {
      rawStreamAccumulator += batchText;
      proseEl.innerHTML = this.formatProse(rawStreamAccumulator);
      this.smoothScrollToBottom();
    });

    this.streamFromSSE(prompt, proseEl, traceEl, accordionEl);
  }

  streamFromSSE(prompt, proseEl, traceEl, accordionEl) {
    const activeModelId = this.models[this.currentModelIdx].id;
    const sseUrl = `/api/chat/stream?message=${encodeURIComponent(prompt)}&model=${encodeURIComponent(activeModelId)}&effort=${encodeURIComponent(this.currentEffort)}`;
    this.eventSource = new EventSource(sseUrl);

    this.eventSource.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data);
        this.processStreamEvent(event, proseEl, traceEl, accordionEl);
        if (event.type === 'turn_complete' || event.type === 'error') {
          this.eventSource.close();
          this.finishExecutionTurn();
        }
      } catch (err) {
        console.error("SSE parse error", err);
      }
    };

    this.eventSource.onerror = () => {
      if (this.eventSource) this.eventSource.close();
      this.finishExecutionTurn();
    };
  }

  processStreamEvent(event, proseEl, traceEl, accordionEl) {
    const type = event.type;
    const data = event.data || {};

    if (type === 'thought_chunk' && data.chunk) {
      traceEl.textContent = data.chunk.slice(-90);
    } else if (type === 'text_chunk' && data.chunk) {
      this.tokenBuffer.push(data.chunk);
    } else if (type === 'tool_call_start') {
      const details = accordionEl.querySelector('.tool-accordion-details');
      const step = document.createElement('div');
      step.className = 'tool-step-item';
      step.textContent = `✓ [Step ${data.step || 1}: ${data.tool_name}] ${JSON.stringify(data.arguments || {})}`;
      details.appendChild(step);
      traceEl.textContent = `[Step ${data.step || 1}] Executing tool: ${data.tool_name}...`;
    } else if (type === 'ask_question') {
      this.renderAskCard(data, proseEl);
    } else if (type === 'tool_call_done' && data.tool_name === 'ask_question' && data.artifacts && data.artifacts.ask_card) {
      if (!proseEl.querySelector(`[data-call-id="${data.call_id}"]`)) {
        this.renderAskCard({ ...data.artifacts.ask_card, call_id: data.call_id, step: data.step }, proseEl);
      }
    } else if (type === 'plugin_evolved') {
      this.refreshPlugins();
    } else if (type === 'diff_generated') {
      this.refreshDiffs();
      this.switchRightCanvasTab('diff');
    } else if (type === 'plan_checklist') {
      this.renderPlanChecklist(data, proseEl);
    } else if (type === 'subagent_review') {
      this.renderSubagentReview(data);
    } else if (type === 'turn_complete') {
      traceEl.textContent = `Completed in ${this.elapsedSeconds}s (${data.tool_calls_count || 0} tool calls).`;
      this.refreshPlugins();
      this.refreshDiffs();
      this.saveCurrentSessionToServer();
    }
  }

  finishExecutionTurn() {
    this.isStreaming = false;
    if (this.btnQueueTask) this.btnQueueTask.style.display = 'none';
    if (this.tokenBuffer) this.tokenBuffer.flush();
    if (this.streamTimer) {
      clearInterval(this.streamTimer);
      this.streamTimer = null;
    }
    this.sendIconSvg.style.display = 'block';
    this.stopIconSquare.style.display = 'none';
    this.saveCurrentSessionToServer();

    // Auto-dequeue next task in execution queue
    this.checkAndAutoDequeue();
  }

  stopStreaming() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    if (this.tokenBuffer) this.tokenBuffer.clear();
    this.finishExecutionTurn();
  }

  async refreshPlugins() {
    try {
      const res = await fetch('/api/plugins');
      const plugins = await res.json();
      if (this.pluginMeshGrid) {
        this.pluginMeshGrid.innerHTML = plugins.map(p => `
          <div style="background: #181920; border: 1px solid rgba(255,255,255,0.08); border-radius: 8px; padding: 10px;">
            <div style="font-weight: 600; color: #f0f2f7;">${p.name} <span style="font-size: 10px; color: #38bdf8; background: rgba(56,189,248,0.12); padding: 1px 4px; border-radius: 3px;">${p.type}</span></div>
            <div style="font-size: 11.5px; color: #9aa0b2; margin-top: 4px;">${p.description}</div>
            <div style="font-size: 10.5px; color: #10b981; margin-top: 6px;">● active (v${p.version}) · ${p.author}</div>
          </div>
        `).join('');
      }
    } catch (e) {
      console.warn("Failed to fetch plugins", e);
    }
  }

  async refreshDiffs() {
    try {
      const res = await fetch('/api/diffs');
      const diffs = await res.json();
      this.canvasDiffCount.textContent = diffs.length;
      if (this.diffStreamContainer) {
        if (diffs.length === 0) {
          this.diffStreamContainer.innerHTML = `
            <div class="canvas-empty-state">
              <div class="empty-icon">📝</div>
              <div class="empty-title">没有待审查的代码修改</div>
              <div class="empty-desc">当 Agent 执行任务或演化插件时，代码变更将实时流式渲染于此，支持逐行差异审查与合并。</div>
            </div>
          `;
        } else {
          this.diffStreamContainer.innerHTML = diffs.map(d => `
            <div class="diff-chunk-card" data-chunk-id="${d.chunk_id}" style="background: #181920; border: 1px solid rgba(255,255,255,0.08); border-radius: 8px; padding: 10px; margin-bottom: 10px; font-family: var(--font-mono); font-size: 11.5px;">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px;">
                <div style="color: #f0f2f7; font-weight: bold;">📄 ${this.escapeHtml(d.file_path)} <span style="font-size:10px;color:${d.status === 'accepted' ? '#34d399' : (d.status === 'rejected' ? '#f87171' : '#a5b4fc')};">(${d.status})</span></div>
                <div style="display: flex; gap: 4px;">
                  <button class="btn-sm-accept btn-chunk-accept" data-chunk-id="${d.chunk_id}" style="padding: 2px 8px; font-size: 11px;">✓ 接受</button>
                  <button class="btn-sm-reject btn-chunk-reject" data-chunk-id="${d.chunk_id}" style="padding: 2px 8px; font-size: 11px;">✕ 拒绝</button>
                </div>
              </div>
              <pre style="color: #34d399; white-space: pre-wrap; background: rgba(0,0,0,0.3); padding: 8px; border-radius: 4px; overflow-x: auto;">${this.escapeHtml(d.diff_text)}</pre>
            </div>
          `).join('');

          this.diffStreamContainer.querySelectorAll('.btn-chunk-accept').forEach(btn => {
            btn.onclick = async () => {
              const chunkId = btn.dataset.chunkId;
              await fetch('/api/diffs/action', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ chunk_id: chunkId, action: 'accept' })
              });
              await this.refreshDiffs();
            };
          });

          this.diffStreamContainer.querySelectorAll('.btn-chunk-reject').forEach(btn => {
            btn.onclick = async () => {
              const chunkId = btn.dataset.chunkId;
              await fetch('/api/diffs/action', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ chunk_id: chunkId, action: 'reject' })
              });
              await this.refreshDiffs();
            };
          });
        }
      }
    } catch (e) {
      console.warn("Failed to fetch diffs", e);
    }
  }

  smoothScrollToBottom() {
    if (this.scrollRafId) return;
    this.scrollRafId = requestAnimationFrame(() => {
      this.chatThreadContainer.scrollTop = this.chatThreadContainer.scrollHeight;
      this.scrollRafId = null;
    });
  }

  escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  formatProse(raw) {
    if (!raw) return '';

    // 1. Extract and format fenced code blocks
    const codeBlocks = [];
    let processed = raw.replace(/```([a-zA-Z0-9_-]*)\n([\s\S]*?)```/g, (match, lang, code) => {
      const idx = codeBlocks.length;
      const cleanLang = lang.trim() || 'code';
      const cleanCode = code.trim();
      codeBlocks.push({ lang: cleanLang, code: cleanCode });
      return `___AETHER_CODE_BLOCK_${idx}___`;
    });

    // 2. Format prose elements
    processed = this.escapeHtml(processed);
    processed = processed.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
    processed = processed.replace(/`([^`]+)`/g, '<code class="tag-code">$1</code>');
    processed = processed.replace(/^### (.*$)/gim, '<h3 style="font-size:14px;margin:8px 0 4px;color:#f0f2f7;">$1</h3>');
    processed = processed.replace(/^## (.*$)/gim, '<h2 style="font-size:15px;margin:10px 0 4px;color:#f0f2f7;">$1</h2>');
    processed = processed.replace(/^# (.*$)/gim, '<h1 style="font-size:16px;margin:12px 0 6px;color:#f0f2f7;">$1</h1>');
    processed = processed.replace(/\n/g, '<br>');

    // 3. Re-inject code cards
    codeBlocks.forEach((cb, idx) => {
      const cardHtml = `
        <div class="md-code-card">
          <div class="md-code-header">
            <span class="md-code-lang">${this.escapeHtml(cb.lang)}</span>
            <button class="md-copy-btn" data-code="${this.escapeHtml(cb.code)}">
              <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
              <span>复制代码</span>
            </button>
          </div>
          <pre class="md-code-pre"><code>${this.escapeHtml(cb.code)}</code></pre>
        </div>
      `;
      processed = processed.replace(`___AETHER_CODE_BLOCK_${idx}___`, cardHtml);
    });

    return processed;
  }

  renderAskCard(data, proseEl) {
    const question = data.question || "请确认接下来的执行方案：";
    const options = data.options || ["确认继续", "取消操作"];
    const isMulti = Boolean(data.is_multi_select);
    const allowCustom = data.allow_custom !== false;
    const context = data.context || "";
    const callId = data.call_id || `ask-${Date.now()}`;
    const cardId = `ask-card-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;

    // Avoid duplicate cards
    if (proseEl.querySelector(`[data-call-id="${callId}"]`)) {
      return;
    }

    const card = document.createElement('div');
    card.className = 'aether-ask-card';
    card.id = cardId;
    card.dataset.callId = callId;

    let optionsHtml = '';
    options.forEach((opt, idx) => {
      const optId = `${cardId}-opt-${idx}`;
      const inputType = isMulti ? 'checkbox' : 'radio';
      const inputName = `${cardId}-choice`;
      optionsHtml += `
        <label class="aether-ask-option-item ${idx === 0 && !isMulti ? 'selected' : ''}" for="${optId}">
          <input type="${inputType}" id="${optId}" name="${inputName}" value="${this.escapeHtml(opt)}" ${idx === 0 && !isMulti ? 'checked' : ''} />
          <div class="aether-ask-option-content">
            <span class="aether-ask-option-label">${this.escapeHtml(opt)}</span>
          </div>
        </label>
      `;
    });

    card.innerHTML = `
      <div class="aether-ask-header">
        <div class="aether-ask-badge">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
          决策与确认 / Decision Required
        </div>
        ${data.step ? `<span class="aether-ask-step">Step ${data.step}</span>` : ''}
      </div>
      <div class="aether-ask-question">${this.escapeHtml(question)}</div>
      ${context ? `<div class="aether-ask-context">${this.escapeHtml(context)}</div>` : ''}
      <div class="aether-ask-options-list">
        ${optionsHtml}
      </div>
      ${allowCustom ? `
        <div class="aether-ask-custom-wrapper">
          <input type="text" class="aether-ask-custom-input" placeholder="输入自定义意见或补充说明 (可选)..." />
        </div>
      ` : ''}
      <div class="aether-ask-actions">
        <button class="aether-ask-submit-btn">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
          确认并继续
        </button>
        <button class="aether-ask-skip-btn">跳过</button>
      </div>
    `;

    proseEl.appendChild(card);
    this.smoothScrollToBottom();

    // Bind selection highlight
    const optionLabels = card.querySelectorAll('.aether-ask-option-item');
    const updateSelectedStyles = () => {
      optionLabels.forEach(lbl => {
        const inp = lbl.querySelector('input');
        if (inp && inp.checked) {
          lbl.classList.add('selected');
        } else {
          lbl.classList.remove('selected');
        }
      });
    };
    optionLabels.forEach(lbl => {
      lbl.addEventListener('change', updateSelectedStyles);
    });

    // Bind submit action
    const submitBtn = card.querySelector('.aether-ask-submit-btn');
    const skipBtn = card.querySelector('.aether-ask-skip-btn');
    const customInput = card.querySelector('.aether-ask-custom-input');

    const handleSubmit = (isSkip = false) => {
      let selectedValues = [];
      if (!isSkip) {
        const checkedInputs = card.querySelectorAll('.aether-ask-options-list input:checked');
        checkedInputs.forEach(inp => selectedValues.push(inp.value));
        const customVal = customInput ? customInput.value.trim() : '';
        if (customVal) {
          selectedValues.push(`补充说明: ${customVal}`);
        }
      }

      const responseText = isSkip ? "已跳过该决策，按默认流程继续。" : `已选定决策: ${selectedValues.join("；")}`;

      // Update card state to answered
      card.classList.add('answered');
      card.innerHTML = `
        <div class="aether-ask-answered-banner">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
          <span>${this.escapeHtml(responseText)}</span>
        </div>
      `;

      this.saveCurrentSessionToServer();

      // Submit user choice back to the agent turn
      this.composerMainInput.value = responseText;
      this.handleSendOrStop();
    };

    submitBtn.addEventListener('click', () => handleSubmit(false));
    skipBtn.addEventListener('click', () => handleSubmit(true));
  }

  async showAtAutocomplete(query, fullToken) {
    if (!this.atAutocompleteMenu || !this.atMenuItemsContainer) return;

    if (this.cachedWorkspaceFiles.length === 0) {
      try {
        const res = await fetch('/api/workspace/files');
        this.cachedWorkspaceFiles = await res.json();
      } catch (err) {
        console.warn("Failed to load workspace files for @ mention", err);
      }
    }

    let itemsHtml = '';
    // Standard context options
    const contextOptions = [
      { id: 'files', label: '@Files', desc: '浏览并引用工作区文件', icon: '📄' },
      { id: 'terminal', label: '@Terminal', desc: '引用终端执行日志', icon: '🖥️' },
      { id: 'diff', label: '@Diff', desc: '引用当前代码变更差异', icon: '🔀' },
      { id: 'plugins', label: '@Plugins', desc: '引用微内核插件网格', icon: '🧪' },
    ];

    const matchedContext = contextOptions.filter(o =>
      !query || o.label.toLowerCase().includes(query) || o.desc.toLowerCase().includes(query)
    );
    matchedContext.forEach(opt => {
      itemsHtml += `
        <div class="menu-action-item at-select-item" data-at-type="${opt.id}" data-at-label="${opt.label}">
          <span class="action-icon">${opt.icon}</span>
          <span><strong>${this.escapeHtml(opt.label)}</strong> - ${this.escapeHtml(opt.desc)}</span>
        </div>
      `;
    });

    // Matched workspace files
    const matchedFiles = this.cachedWorkspaceFiles.filter(f =>
      !query || f.path.toLowerCase().includes(query) || f.name.toLowerCase().includes(query)
    ).slice(0, 8);

    if (matchedFiles.length > 0) {
      itemsHtml += `<div class="menu-divider" style="height:1px;background:rgba(255,255,255,0.06);margin:4px 0;"></div>`;
      matchedFiles.forEach(f => {
        itemsHtml += `
          <div class="menu-action-item at-select-item" data-at-type="file" data-at-path="${this.escapeHtml(f.path)}">
            <span class="action-icon">📄</span>
            <span><strong>@${this.escapeHtml(f.name)}</strong> <small style="color:#64748b;font-size:11px;margin-left:4px;">${this.escapeHtml(f.path)}</small></span>
          </div>
        `;
      });
    }

    this.atMenuItemsContainer.innerHTML = itemsHtml;
    this.atAutocompleteMenu.style.display = 'block';

    // Bind click events on at-select-item
    this.atMenuItemsContainer.querySelectorAll('.at-select-item').forEach(item => {
      item.addEventListener('click', (e) => {
        e.stopPropagation();
        const type = item.dataset.atType;
        const val = this.composerMainInput.value;
        const cursorPos = this.composerMainInput.selectionStart || val.length;
        const textBefore = val.slice(0, cursorPos);
        const textAfter = val.slice(cursorPos);
        const replacedBefore = textBefore.replace(/@([a-zA-Z0-9_\-\.\/]*)$/, '');

        if (type === 'file') {
          const filePath = item.dataset.atPath;
          this.composerMainInput.value = `${replacedBefore}@${filePath} ${textAfter}`;
        } else if (type === 'terminal') {
          const logs = this.terminalViewOutput.textContent.slice(-500);
          this.composerMainInput.value = `${replacedBefore}\n\`\`\`terminal_output\n${logs}\n\`\`\`\n${textAfter}`;
        } else if (type === 'diff') {
          this.composerMainInput.value = `${replacedBefore}@Diff ${textAfter}`;
        } else if (type === 'plugins') {
          this.composerMainInput.value = `${replacedBefore}@Plugins ${textAfter}`;
        } else if (type === 'files') {
          this.composerMainInput.value = `${replacedBefore}@${this.cachedWorkspaceFiles[0]?.path || 'file'} ${textAfter}`;
        }

        this.atAutocompleteMenu.style.display = 'none';
        this.composerMainInput.focus();
      });
    });
  }

  switchTerminalTab(tabId, tabEl) {
    if (!this.terminalTabsNav) return;
    // Save current tab's output to its buffer
    this.terminalBuffers[this.activeTerminalTab] = this.terminalViewOutput.textContent;
    // Switch active
    this.activeTerminalTab = tabId;
    this.terminalTabsNav.querySelectorAll('.terminal-tab-item').forEach(t => t.classList.remove('active'));
    if (tabEl) tabEl.classList.add('active');
    // Load new tab's buffer
    if (!this.terminalBuffers[tabId]) {
      this.terminalBuffers[tabId] = `> Terminal Ready.\n$ `;
    }
    this.terminalViewOutput.textContent = this.terminalBuffers[tabId];
    this.terminalViewOutput.scrollTop = this.terminalViewOutput.scrollHeight;
  }

  appendToTerminalBuffer(text) {
    // Write to both the active display and the buffer
    this.terminalViewOutput.textContent += text;
    this.terminalBuffers[this.activeTerminalTab] = this.terminalViewOutput.textContent;
  }

  async refreshQueue() {
    if (!this.queueDrawer || !this.queueTasksList) return;
    try {
      const res = await fetch('/api/queue');
      const tasks = await res.json();
      this.queueCountBadge.textContent = tasks.length;

      if (tasks.length === 0) {
        this.queueDrawer.style.display = 'none';
        this.queueTasksList.innerHTML = '';
        return;
      }

      this.queueDrawer.style.display = 'block';
      this.queueTasksList.innerHTML = tasks.map((t, idx) => `
        <div class="queue-task-item" data-task-id="${t.task_id}">
          <span class="queue-task-text"><strong>#${idx + 1}</strong> ${this.escapeHtml(t.prompt)}</span>
          <button class="queue-task-remove" data-task-id="${t.task_id}" title="移除此任务">✕</button>
        </div>
      `).join('');

      this.queueTasksList.querySelectorAll('.queue-task-remove').forEach(btn => {
        btn.onclick = async (e) => {
          e.stopPropagation();
          await this.removeQueueTask(btn.dataset.taskId);
        };
      });
    } catch (e) {
      console.warn("Failed to refresh queue", e);
    }
  }

  async pushQueueTask(prompt) {
    try {
      await fetch('/api/queue/push', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt, priority: 0 })
      });
      await this.refreshQueue();
    } catch (e) {
      console.warn("Failed to push queue", e);
    }
  }

  async removeQueueTask(taskId) {
    try {
      await fetch('/api/queue/remove', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ task_id: taskId })
      });
      await this.refreshQueue();
    } catch (e) {
      console.warn("Failed to remove queue task", e);
    }
  }

  async clearQueue() {
    try {
      await fetch('/api/queue/clear', { method: 'POST' });
      await this.refreshQueue();
    } catch (e) {
      console.warn("Failed to clear queue", e);
    }
  }

  async checkAndAutoDequeue() {
    try {
      const res = await fetch('/api/queue/pop', { method: 'POST' });
      const data = await res.json();
      await this.refreshQueue();
      if (data.task && data.task.prompt) {
        setTimeout(() => {
          this.startAgentExecutionTurn(data.task.prompt);
        }, 400);
      }
    } catch (e) {
      console.warn("Failed to pop queue", e);
    }
  }

  renderPlanChecklist(data, proseEl) {
    const steps = data.steps || [];
    if (steps.length === 0) return;

    const checklistCard = document.createElement('div');
    checklistCard.className = 'plan-checklist-card';
    checklistCard.innerHTML = `
      <div class="plan-checklist-header">
        <span>📋 Plan 逐步实施清单 (${steps.length} 步骤)</span>
        <span style="font-size: 11px; color: #34d399;">● 自动调度中</span>
      </div>
      <div class="plan-checklist-items">
        ${steps.map((s, idx) => `
          <div class="checklist-step-item ${s.status}">
            <span>${s.status === 'completed' ? '🟢' : (s.status === 'running' ? '🟡' : '⚪')}</span>
            <span><strong>第 ${idx + 1} 步:</strong> ${this.escapeHtml(s.title)}</span>
          </div>
        `).join('')}
      </div>
    `;
    proseEl.appendChild(checklistCard);
    this.smoothScrollToBottom();
  }

  renderSubagentReview(data) {
    if (!this.subagentReviewCard) return;
    const score = data.score !== undefined ? data.score : 100;
    const findings = data.findings || [];
    const suggestions = data.suggestions || [];

    if (this.subagentScoreBadge) {
      this.subagentScoreBadge.textContent = `安全分: ${score}`;
      this.subagentScoreBadge.style.color = score >= 80 ? '#34d399' : '#f87171';
    }

    if (this.subagentFindingsList) {
      let html = findings.map(f => `<div class="subagent-finding-item">${this.escapeHtml(f)}</div>`).join('');
      if (suggestions.length > 0) {
        html += suggestions.map(s => `<div class="subagent-finding-item" style="color:#94a3b8;">💡 ${this.escapeHtml(s)}</div>`).join('');
      }
      this.subagentFindingsList.innerHTML = html;
    }

    this.subagentReviewCard.style.display = 'block';
  }

  async loadSessionHistory() {
    if (!this.sessionHistoryList) return;
    try {
      const res = await fetch('/api/workspace/sessions');
      const sessions = await res.json();
      this.sessionCountBadge.textContent = sessions.length;

      if (sessions.length === 0) {
        this.sessionHistoryList.innerHTML = '<div style="color:#6b7280;font-size:11.5px;padding:8px 12px;">暂无历史会话</div>';
        return;
      }

      this.sessionHistoryList.innerHTML = sessions.map(s => {
        const timeStr = s.updated_at ? new Date(s.updated_at).toLocaleString('zh-CN', {month:'short', day:'numeric', hour:'2-digit', minute:'2-digit'}) : '';
        return `
          <div class="session-history-item" data-task-id="${s.id}" data-project="${s.project}" data-title="${this.escapeHtml(s.title)}">
            <div class="session-item-title">${this.escapeHtml(s.title)}</div>
            <div class="session-item-meta">${timeStr}</div>
          </div>
        `;
      }).join('');

      this.sessionHistoryList.querySelectorAll('.session-history-item').forEach(item => {
        item.addEventListener('click', () => {
          const taskId = item.dataset.taskId;
          const project = item.dataset.project;
          const title = item.dataset.title;
          this.switchTask(taskId, project, title);
        });
      });
    } catch (e) {
      console.warn('Failed to load session history', e);
    }
  }

  async handleFileUpload(file) {
    if (file.type.startsWith('image/')) {
      // Read image as base64 and inject into prompt
      const reader = new FileReader();
      reader.onload = () => {
        const b64 = reader.result;
        this.composerMainInput.value += `\n[Attached Image: ${file.name}]\n`;
      };
      reader.readAsDataURL(file);
    } else {
      // Upload text file to server
      const formData = new FormData();
      formData.append('file', file);
      try {
        const res = await fetch('/api/upload', {
          method: 'POST',
          body: formData,
        });
        const data = await res.json();
        if (data.success && data.preview) {
          const preview = data.preview.length > 3000 ? data.preview.slice(0, 3000) + '\n... [truncated]' : data.preview;
          this.composerMainInput.value += `\n[Attached File: ${data.filename}]\n\`\`\`\n${preview}\n\`\`\`\n`;
        }
      } catch (e) {
        console.warn('File upload failed', e);
      }
    }
    this.composerMainInput.focus();
  }

  async refreshGitPanel() {
    try {
      const [statusRes, logRes] = await Promise.all([
        fetch('/api/git/status'),
        fetch('/api/git/log'),
      ]);
      const status = await statusRes.json();
      const log = await logRes.json();

      if (this.gitBranchLabel) {
        this.gitBranchLabel.textContent = `分支: ${status.branch || 'unknown'}`;
      }

      if (this.gitStatusList) {
        if (status.clean) {
          this.gitStatusList.innerHTML = '<div style="color:#34d399;padding:6px 0;">✓ 工作区干净</div>';
        } else {
          this.gitStatusList.innerHTML = (status.files || []).map(f => {
            const color = f.status === 'M' ? '#fbbf24' : (f.status === 'A' || f.status === '?' ? '#34d399' : '#f87171');
            return `<div style="padding:3px 0;"><span style="color:${color};font-weight:600;width:24px;display:inline-block;">${this.escapeHtml(f.status)}</span> ${this.escapeHtml(f.path)}</div>`;
          }).join('');
        }
      }

      if (this.gitLogList) {
        this.gitLogList.innerHTML = (Array.isArray(log) ? log : []).filter(c => !c.error).map(c =>
          `<div style="padding:3px 0;color:#9ca3af;"><span style="color:#a5b4fc;">${this.escapeHtml(c.short_hash)}</span> ${this.escapeHtml(c.message)} <span style="color:#6b7280;">${this.escapeHtml(c.time_ago)}</span></div>`
        ).join('');
      }
    } catch (e) {
      console.warn('Failed to refresh git panel', e);
    }
  }

  async gitCommit() {
    const msg = this.gitCommitMsg?.value?.trim();
    if (!msg) return;
    try {
      const res = await fetch('/api/git/commit', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({message: msg}),
      });
      const data = await res.json();
      if (data.success) {
        this.gitCommitMsg.value = '';
        await this.refreshGitPanel();
      }
    } catch (e) {
      console.warn('Git commit failed', e);
    }
  }

  async openFileInEditor(filePath) {
    try {
      const res = await fetch(`/api/workspace/read_file?path=${encodeURIComponent(filePath)}`);
      const data = await res.json();
      if (data.error) {
        console.warn('Failed to open file:', data.error);
        return;
      }
      this.editorCurrentPath = filePath;
      if (this.editorFileTitle) this.editorFileTitle.textContent = filePath;
      if (this.editorTextarea) this.editorTextarea.value = data.content;
      this.switchRightCanvasTab('editor');
    } catch (e) {
      console.warn('Failed to open file in editor', e);
    }
  }

  async saveEditorFile() {
    if (!this.editorCurrentPath || !this.editorTextarea) return;
    try {
      const res = await fetch('/api/workspace/write_file', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          path: this.editorCurrentPath,
          content: this.editorTextarea.value,
        }),
      });
      const data = await res.json();
      if (data.success && this.editorFileTitle) {
        this.editorFileTitle.textContent = `${this.editorCurrentPath} ✓`;
      }
    } catch (e) {
      console.warn('Failed to save file', e);
    }
  }
  async startABComparison(prompt) {
    if (!prompt || this.models.length < 2) return;
    const modelA = this.models[0];
    const modelB = this.models[1];

    const comparisonHtml = `
      <div class="ab-comparison-card" style="display:grid;grid-template-columns:1fr 1fr;gap:12px;margin:12px 0;">
        <div class="ab-column" style="background:rgba(99,102,241,0.08);border:1px solid rgba(99,102,241,0.2);border-radius:10px;padding:12px;">
          <div style="color:#a5b4fc;font-weight:600;font-size:12px;margin-bottom:8px;">${modelA.badge}</div>
          <div class="ab-response-a" id="ab-response-a" style="color:#d1d5db;font-size:13px;line-height:1.6;">Generating...</div>
        </div>
        <div class="ab-column" style="background:rgba(52,211,153,0.08);border:1px solid rgba(52,211,153,0.2);border-radius:10px;padding:12px;">
          <div style="color:#6ee7b7;font-weight:600;font-size:12px;margin-bottom:8px;">${modelB.badge}</div>
          <div class="ab-response-b" id="ab-response-b" style="color:#d1d5db;font-size:13px;line-height:1.6;">Generating...</div>
        </div>
      </div>
    `;

    this.appendUserMessage(`[A/B 对比] ${prompt}`);

    const compDiv = document.createElement('div');
    compDiv.innerHTML = comparisonHtml;
    this.chatThreadContainer.appendChild(compDiv);
    this.chatThreadContainer.scrollTop = this.chatThreadContainer.scrollHeight;

    const respA = compDiv.querySelector('#ab-response-a');
    const respB = compDiv.querySelector('#ab-response-b');

    // Fetch both models in parallel
    const fetchModel = async (modelId, targetEl) => {
      try {
        const evtSource = new EventSource(`/api/chat/stream?prompt=${encodeURIComponent(prompt)}&model=${encodeURIComponent(modelId)}`);
        let text = '';
        evtSource.addEventListener('text_chunk', (e) => {
          const data = JSON.parse(e.data);
          text += data.chunk || '';
          targetEl.textContent = text;
        });
        evtSource.addEventListener('turn_complete', () => {
          evtSource.close();
        });
        evtSource.onerror = () => evtSource.close();
      } catch (e) {
        targetEl.textContent = `Error: ${e.message}`;
      }
    };

    fetchModel(modelA.id, respA);
    fetchModel(modelB.id, respB);
  }

  toggleMarketplace() {
    if (!this.marketplacePanel) return;
    const isHidden = this.marketplacePanel.style.display === 'none';
    this.marketplacePanel.style.display = isHidden ? 'block' : 'none';
    if (isHidden) {
      this.loadMarketplace();
      this.loadMcpServers();
    }
  }

  async loadMarketplace() {
    if (!this.marketplaceList) return;
    try {
      const res = await fetch('/api/plugins/marketplace');
      const plugins = await res.json();
      this.marketplaceList.innerHTML = plugins.map(p => `
        <div style="display:flex;justify-content:space-between;align-items:center;padding:8px;margin:4px 0;background:rgba(255,255,255,0.03);border-radius:8px;border:1px solid rgba(255,255,255,0.06);">
          <div>
            <div style="color:#e2e8f0;font-size:12px;font-weight:600;">${this.escapeHtml(p.name)}</div>
            <div style="color:#6b7280;font-size:11px;margin-top:2px;">${this.escapeHtml(p.description)}</div>
          </div>
          <button class="btn-sm-accept" onclick="window.aetherApp.installPlugin('${p.id}')" style="font-size:10px;white-space:nowrap;">${p.installed ? '✓ 已安装' : '安装'}</button>
        </div>
      `).join('');
    } catch (e) {
      console.warn('Failed to load marketplace', e);
    }
  }

  async installPlugin(pluginId) {
    try {
      await fetch('/api/plugins/install', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({plugin_id: pluginId}),
      });
      this.loadMarketplace();
    } catch (e) {
      console.warn('Plugin install failed', e);
    }
  }

  async loadMcpServers() {
    if (!this.mcpServersList) return;
    try {
      const res = await fetch('/api/mcp/servers');
      const servers = await res.json();
      if (servers.length === 0) {
        this.mcpServersList.innerHTML = '<div style="color:#6b7280;font-size:11px;padding:4px 0;">未连接任何 MCP 服务器</div>';
        return;
      }
      this.mcpServersList.innerHTML = servers.map(s => {
        const statusColor = s.status === 'connected' ? '#34d399' : (s.status === 'error' ? '#f87171' : '#6b7280');
        const toolsText = s.tools.map(t => t.name).join(', ') || '无工具';
        return `
          <div style="padding:6px 8px;margin:3px 0;background:rgba(255,255,255,0.03);border-radius:6px;border:1px solid rgba(255,255,255,0.06);">
            <div style="display:flex;justify-content:space-between;align-items:center;">
              <span style="color:#e2e8f0;font-size:12px;font-weight:600;">${this.escapeHtml(s.name)}</span>
              <span style="color:${statusColor};font-size:10px;">● ${s.status}</span>
            </div>
            <div style="color:#6b7280;font-size:10px;margin-top:3px;">工具: ${this.escapeHtml(toolsText)}</div>
            ${s.error ? `<div style="color:#f87171;font-size:10px;margin-top:2px;">${this.escapeHtml(s.error)}</div>` : ''}
          </div>
        `;
      }).join('');
    } catch (e) {
      console.warn('Failed to load MCP servers', e);
    }
  }

  async connectMcpServer() {
    const name = this.mcpServerName?.value?.trim();
    const cmd = this.mcpServerCmd?.value?.trim();
    if (!name || !cmd) return;
    try {
      const res = await fetch('/api/mcp/connect', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({name, command: cmd.split(/\\s+/)}),
      });
      await res.json();
      this.mcpServerName.value = '';
      this.mcpServerCmd.value = '';
      this.loadMcpServers();
    } catch (e) {
      console.warn('MCP connect failed', e);
    }
  }
}

// Global App Instance
const aetherEngine = new AetherDesktopEngine();
window.aetherApp = aetherEngine;
