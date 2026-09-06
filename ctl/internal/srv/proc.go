package srv

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"utsusemi/ctl/internal/core"
)

type Status struct {
	Running   bool   `json:"running"`
	PID       int    `json:"pid"`
	Binary    string `json:"binary"`
	UptimeSec int64  `json:"uptime_sec"`
	Note      string `json:"note,omitempty"`
}

type Manager struct{ P core.Paths }

func New(p core.Paths) *Manager { return &Manager{P: p} }

// resolve 返回激活 server 的绝对路径；无激活或文件丢失返回错误
func (m *Manager) resolve() (string, core.Binary, error) {
	s, err := core.LoadSettings(m.P)
	if err != nil {
		return "", core.Binary{}, err
	}
	if s.Server.Active == "" {
		return "", core.Binary{}, fmt.Errorf("no active server binary")
	}
	man, err := core.LoadManifest(m.P)
	if err != nil {
		return "", core.Binary{}, err
	}
	b, ok := man.Find("server", s.Server.Active)
	if !ok {
		return "", core.Binary{}, fmt.Errorf("active %q not in manifest", s.Server.Active)
	}
	abs := m.P.ServerDir() + "/" + b.File
	if _, err := os.Stat(abs); err != nil {
		return "", b, fmt.Errorf("active binary missing: %s", abs)
	}
	return abs, b, nil
}

func readPid(p core.Paths) int {
	raw, err := os.ReadFile(p.PidFile())
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return pid
}

// aliveBasic 仅判 pid 是否存活（kill 0）
func aliveBasic(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// alive 校验 pid 存活且 cmdline 含 substr（防 stale pidfile 误判）。
// 注意 fork→execve 间隙 cmdline 仍是父进程内容，调用方对启动场景应带重试
func alive(pid int, substr string) bool {
	if !aliveBasic(pid) {
		return false
	}
	if substr == "" {
		return true
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ReplaceAll(string(raw), "\x00", " "), substr)
}

// waitReady 等待子进程完成 exec 且 cmdline 匹配；进程死亡返回 false
func waitReady(pid int, substr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !aliveBasic(pid) {
			return false // 已退出
		}
		if alive(pid, substr) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return alive(pid, substr)
}

func (m *Manager) Status() Status {
	abs, b, err := m.resolve()
	st := Status{Binary: b.File}
	if err != nil {
		st.Note = err.Error()
		return st
	}
	_ = abs
	pid := readPid(m.P)
	if alive(pid, b.File) {
		st.Running, st.PID = true, pid
		if fi, err := os.Stat(m.P.PidFile()); err == nil {
			st.UptimeSec = int64(time.Since(fi.ModTime()).Seconds())
		}
	}
	return st
}

func (m *Manager) Start() (Status, error) {
	s, err := core.LoadSettings(m.P)
	if err != nil {
		return Status{}, err
	}
	abs, b, err := m.resolve()
	if err != nil {
		_ = UpdateDescription(m.P, false, "no binary")
		return Status{}, err
	}
	if pid := readPid(m.P); alive(pid, b.File) {
		return m.Status(), nil // 幂等
	}
	if err := os.MkdirAll(m.P.Logs(), 0o755); err != nil {
		return Status{}, err
	}
	log, err := os.OpenFile(m.P.ServerLog(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return Status{}, err
	}
	defer log.Close()
	cmd := exec.Command(abs, s.Server.Args...)
	cmd.Stdout, cmd.Stderr = log, log
	// 脱离会话组：ctl 退出不连带 frida-server
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return Status{}, err
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return Status{}, err
	}
	if err := os.WriteFile(m.P.PidFile(), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return Status{}, err
	}
	// 轮询等待 exec 完成且进程存活（fork→execve 有竞态窗口，须带重试）
	if !waitReady(pid, b.File, 3*time.Second) {
		os.Remove(m.P.PidFile())
		_ = UpdateDescription(m.P, false, "crashed at start")
		return Status{}, fmt.Errorf("frida-server exited immediately, see %s", m.P.ServerLog())
	}
	_ = UpdateDescription(m.P, true, "")
	return m.Status(), nil
}

func (m *Manager) Stop() (Status, error) {
	pid := readPid(m.P)
	if !alive(pid, "") {
		os.Remove(m.P.PidFile())
		_ = UpdateDescription(m.P, false, "")
		return m.Status(), nil // 幂等
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	for i := 0; i < 30; i++ {
		if !alive(pid, "") {
			os.Remove(m.P.PidFile())
			_ = UpdateDescription(m.P, false, "")
			return m.Status(), nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	time.Sleep(200 * time.Millisecond)
	os.Remove(m.P.PidFile())
	_ = UpdateDescription(m.P, false, "")
	return m.Status(), nil
}
