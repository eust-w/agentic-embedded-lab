package terminal

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
)

const maxBufferedBytes = 2 * 1024 * 1024

type Info struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace"`
	Shell     string    `json:"shell"`
	Running   bool      `json:"running"`
	ExitCode  int       `json:"exit_code"`
	CreatedAt time.Time `json:"created_at"`
}

type Snapshot struct {
	Info
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"next_offset"`
	DataBase64 string `json:"data_base64"`
	Truncated  bool   `json:"truncated"`
}

type session struct {
	mu         sync.Mutex
	info       Info
	command    *exec.Cmd
	pty        *os.File
	buffer     []byte
	baseOffset int64
	nextOffset int64
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*session
	allowed  map[string]bool
}

func NewManager(ctx context.Context) *Manager {
	manager := &Manager{sessions: make(map[string]*session), allowed: make(map[string]bool)}
	go func() {
		<-ctx.Done()
		manager.Close()
	}()
	return manager
}

func (m *Manager) RegisterWorkspace(workspace string) error {
	root, err := canonicalDirectory(workspace)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.allowed[root] = true
	m.mu.Unlock()
	return nil
}

func (m *Manager) Start(workspace string, columns, rows uint16) (Info, error) {
	root, err := canonicalDirectory(workspace)
	if err != nil {
		return Info{}, err
	}
	m.mu.RLock()
	allowed := m.allowed[root]
	m.mu.RUnlock()
	if !allowed {
		return Info{}, errors.New("terminal workspace is not registered")
	}
	if columns == 0 {
		columns = 120
	}
	if rows == 0 {
		rows = 30
	}
	command := exec.Command("/bin/zsh", "-l")
	command.Dir = root
	command.Env = []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin",
		"HOME=" + os.Getenv("HOME"),
		"LANG=" + environmentOr("LANG", "zh_CN.UTF-8"),
		"LC_ALL=" + environmentOr("LC_ALL", "zh_CN.UTF-8"),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	}
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Cols: columns, Rows: rows})
	if err != nil {
		return Info{}, err
	}
	value := &session{info: Info{ID: uuid.NewString(), Workspace: root, Shell: "/bin/zsh -l", Running: true, ExitCode: -1, CreatedAt: time.Now().UTC()}, command: command, pty: terminal}
	m.mu.Lock()
	m.sessions[value.info.ID] = value
	m.mu.Unlock()
	go value.capture(terminal)
	go value.wait()
	return value.info, nil
}

func (m *Manager) List() []Info {
	m.mu.RLock()
	values := make([]*session, 0, len(m.sessions))
	for _, value := range m.sessions {
		values = append(values, value)
	}
	m.mu.RUnlock()
	result := make([]Info, 0, len(values))
	for _, value := range values {
		value.mu.Lock()
		result = append(result, value.info)
		value.mu.Unlock()
	}
	return result
}

func (m *Manager) Read(id string, after int64, limit int) (Snapshot, error) {
	value, err := m.get(id)
	if err != nil {
		return Snapshot{}, err
	}
	if limit <= 0 || limit > 256*1024 {
		limit = 64 * 1024
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	truncated := after < value.baseOffset
	if after < value.baseOffset {
		after = value.baseOffset
	}
	if after > value.nextOffset {
		after = value.nextOffset
	}
	start := int(after - value.baseOffset)
	end := min(len(value.buffer), start+limit)
	data := append([]byte(nil), value.buffer[start:end]...)
	return Snapshot{Info: value.info, Offset: after, NextOffset: after + int64(len(data)), DataBase64: base64.StdEncoding.EncodeToString(data), Truncated: truncated}, nil
}

func (m *Manager) Write(id string, data []byte) error {
	if len(data) == 0 || len(data) > 64*1024 {
		return errors.New("terminal input must be between 1 and 65536 bytes")
	}
	value, err := m.get(id)
	if err != nil {
		return err
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if !value.info.Running || value.pty == nil {
		return errors.New("terminal is not running")
	}
	_, err = value.pty.Write(data)
	return err
}

func (m *Manager) Resize(id string, columns, rows uint16) error {
	if columns < 20 || rows < 5 || columns > 1000 || rows > 500 {
		return errors.New("terminal size is outside the allowed range")
	}
	value, err := m.get(id)
	if err != nil {
		return err
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.pty == nil {
		return errors.New("terminal is closed")
	}
	return pty.Setsize(value.pty, &pty.Winsize{Cols: columns, Rows: rows})
}

func (m *Manager) Stop(id string) error {
	value, err := m.get(id)
	if err != nil {
		return err
	}
	return value.stop()
}

func (m *Manager) Close() {
	m.mu.RLock()
	values := make([]*session, 0, len(m.sessions))
	for _, value := range m.sessions {
		values = append(values, value)
	}
	m.mu.RUnlock()
	for _, value := range values {
		_ = value.stop()
	}
}

func (m *Manager) get(id string) (*session, error) {
	if id == "" {
		return nil, errors.New("terminal id is required")
	}
	m.mu.RLock()
	value := m.sessions[id]
	m.mu.RUnlock()
	if value == nil {
		return nil, errors.New("terminal session not found")
	}
	return value, nil
}

func (s *session) capture(terminal *os.File) {
	chunk := make([]byte, 32*1024)
	for {
		count, err := terminal.Read(chunk)
		if count > 0 {
			s.mu.Lock()
			s.buffer = append(s.buffer, chunk[:count]...)
			s.nextOffset += int64(count)
			if len(s.buffer) > maxBufferedBytes {
				drop := len(s.buffer) - maxBufferedBytes
				s.buffer = append([]byte(nil), s.buffer[drop:]...)
				s.baseOffset += int64(drop)
			}
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *session) wait() {
	err := s.command.Wait()
	s.mu.Lock()
	s.info.Running = false
	s.info.ExitCode = 0
	if exitError := (*exec.ExitError)(nil); errors.As(err, &exitError) {
		s.info.ExitCode = exitError.ExitCode()
	} else if err != nil {
		s.info.ExitCode = -1
	}
	if s.pty != nil {
		_ = s.pty.Close()
		s.pty = nil
	}
	s.mu.Unlock()
}

func (s *session) stop() error {
	s.mu.Lock()
	if !s.info.Running || s.command == nil || s.command.Process == nil {
		s.mu.Unlock()
		return nil
	}
	pid := s.command.Process.Pid
	s.mu.Unlock()
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		running := s.info.Running
		s.mu.Unlock()
		if !running {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return syscall.Kill(-pid, syscall.SIGKILL)
}

func canonicalDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("terminal workspace must be absolute")
	}
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", errors.New("terminal workspace must be an accessible directory")
	}
	return root, nil
}

func environmentOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
