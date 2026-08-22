const { app, BrowserWindow } = require('electron');
const path = require('path');
const { spawn } = require('child_process');

let mainWindow = null;
let serverProcess = null;
const SERVER_PORT = 8765;

function startBackend() {
  const pythonBin = process.env.VIRTUAL_ENV
    ? path.join(process.env.VIRTUAL_ENV, 'bin', 'python')
    : 'python3';

  serverProcess = spawn(pythonBin, ['-m', 'aether', '--server-only', '--port', String(SERVER_PORT)], {
    cwd: path.resolve(__dirname, '..', '..'),
    env: { ...process.env, PYTHONUNBUFFERED: '1' },
    stdio: 'ignore',
  });

  serverProcess.on('error', (err) => {
    console.warn('[Aether Desktop] Failed to spawn backend process:', err);
  });
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1360,
    height: 880,
    minWidth: 980,
    minHeight: 620,
    title: 'Aether Native · Codex Desktop Agent',
    backgroundColor: '#09090b',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    trafficLightPosition: { x: 16, y: 16 },
    webPreferences: {
      nodeIntegration: false,
      contextIsolation: true,
    },
  });

  const url = `http://127.0.0.1:${SERVER_PORT}/`;

  // Wait briefly for backend to initialize
  setTimeout(() => {
    mainWindow.loadURL(url).catch(() => {
      setTimeout(() => mainWindow.loadURL(url), 1000);
    });
  }, 500);

  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

app.whenReady().then(() => {
  startBackend();
  createWindow();

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on('window-all-closed', () => {
  if (serverProcess) {
    serverProcess.kill('SIGTERM');
  }
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('will-quit', () => {
  if (serverProcess) {
    serverProcess.kill('SIGTERM');
  }
});
