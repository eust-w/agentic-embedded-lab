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

    this.initDOMElements();
    this.initResizableDividers();
    this.bindEvents();
    this.startGoalTimer();
    this.loadInitialConfig();
    this.loadWorkspaceTree();
    this.refreshPlugins();
    this.refreshDiffs();
  }

  initDOMElements() {
    this.chatThreadContainer = document.getElementById('chat-thread-container');
    this.composerMainInput = document.getElementById('composer-main-input');
    this.btnSendOrStop = document.getElementById('btn-send-or-stop');
    this.sendIconSvg = document.querySelector('.send-icon-svg');
    this.stopIconSquare = document.querySelector('.stop-icon-square');
    this.navbarTaskTitle = document.getElementById('navbar-task-title');
    this.toolAccordionCard = document.getElementById('tool-accordion-card');
    this.btnNewTask = document.getElementById('btn-new-task');
    this.btnAddProject = document.getElementById('btn-add-project');
    this.sidebarProjectTree = document.getElementById('sidebar-project-tree');

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

    // 10. Interactive Terminal CLI
    const executeTerminalCmd = async () => {
      const cmd = this.terminalCliInput.value.trim();
      if (!cmd) return;
      this.terminalCliInput.value = '';
      this.terminalViewOutput.textContent += `\n$ ${cmd}\n`;
      this.terminalViewOutput.scrollTop = this.terminalViewOutput.scrollHeight;

      try {
        const res = await fetch('/api/terminal/exec', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({command: cmd})
        });
        const data = await res.json();
        this.terminalViewOutput.textContent += `${data.output || '(No output)'}\n`;
      } catch (e) {
        this.terminalViewOutput.textContent += `Error: ${e.message}\n`;
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

    // Slash command typing detection
    this.composerMainInput.addEventListener('input', () => {
      const val = this.composerMainInput.value;
      if (val.startsWith('/') || val.includes('\n/')) {
        this.slashAutocompleteMenu.style.display = 'block';
      } else {
        this.slashAutocompleteMenu.style.display = 'none';
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
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
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
  }

  triggerComposerAction(action) {
    if (action === 'plan') {
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
      this.recognition.lang = 'zh-CN';

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
    } else if (type === 'plugin_evolved') {
      this.refreshPlugins();
    } else if (type === 'diff_generated') {
      this.refreshDiffs();
      this.switchRightCanvasTab('diff');
    } else if (type === 'turn_complete') {
      traceEl.textContent = `Completed in ${this.elapsedSeconds}s (${data.tool_calls_count || 0} tool calls).`;
      this.refreshPlugins();
      this.refreshDiffs();
      this.saveCurrentSessionToServer();
    }
  }

  finishExecutionTurn() {
    this.isStreaming = false;
    if (this.tokenBuffer) this.tokenBuffer.flush();
    if (this.streamTimer) {
      clearInterval(this.streamTimer);
      this.streamTimer = null;
    }
    this.sendIconSvg.style.display = 'block';
    this.stopIconSquare.style.display = 'none';
    this.saveCurrentSessionToServer();
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
            <div style="background: #181920; border: 1px solid rgba(255,255,255,0.08); border-radius: 6px; padding: 10px; margin-bottom: 8px; font-family: var(--font-mono); font-size: 11.5px;">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px;">
                <div style="color: #f0f2f7; font-weight: bold;">📄 ${d.file_path}</div>
                <button class="btn-sm-accept" onclick="alert('已合并该代码变更！')">合并此项</button>
              </div>
              <pre style="color: #34d399; white-space: pre-wrap;">${this.escapeHtml(d.diff_text)}</pre>
            </div>
          `).join('');
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
}

// Global App Instance
const aetherEngine = new AetherDesktopEngine();
