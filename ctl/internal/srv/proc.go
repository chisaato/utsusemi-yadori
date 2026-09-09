package srv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"utsusemi/ctl/internal/core"
)

// defaultListenPort 与 frida-server 默认监听端口保持一致
const defaultListenPort = 27042

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

// parseListenPort 从 server 启动参数解析监听端口。
// 支持 -l/--listen HOST:PORT、--listen=HOST:PORT、-l=HOST:PORT 及裸端口；
// 未指定、无效或为 0 时回退 defaultListenPort
func parseListenPort(args []string) int {
	val := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-l" || a == "--listen":
			if i+1 < len(args) {
				val = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--listen="):
			val = strings.TrimPrefix(a, "--listen=")
		case strings.HasPrefix(a, "-l="):
			val = strings.TrimPrefix(a, "-l=")
		}
	}
	if val == "" {
		return defaultListenPort
	}
	portStr := val
	if idx := strings.LastIndex(val, ":"); idx >= 0 {
		portStr = val[idx+1:]
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port <= 0 || port > 65535 {
		return defaultListenPort
	}
	return port
}

// socketInode 从 /proc/<pid>/fd 链接目标 socket:[12345] 中提取 inode
func socketInode(link string) (string, bool) {
	const prefix, suffix = "socket:[", "]"
	if strings.HasPrefix(link, prefix) && strings.HasSuffix(link, suffix) {
		return link[len(prefix) : len(link)-len(suffix)], true
	}
	return "", false
}

// listenInodes 解析 /proc/net/tcp{,6}，返回 LISTEN 状态且端口匹配的 socket inode 集合
func listenInodes(port int) map[string]bool {
	want := strings.ToUpper(strconv.FormatInt(int64(port), 16))
	out := map[string]bool{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if i == 0 {
				continue // 表头
			}
			f := strings.Fields(line)
			if len(f) < 10 || f[3] != "0A" {
				continue // st == 0A 表示 LISTEN
			}
			local := f[1]
			idx := strings.LastIndex(local, ":")
			if idx < 0 || !strings.EqualFold(local[idx+1:], want) {
				continue
			}
			out[f[9]] = true // inode
		}
	}
	return out
}

// findListeningPIDs 扫描 /proc/<pid>/fd，返回持有目标端口 LISTEN socket 的 PID（排除自身）
func findListeningPIDs(port int) []int {
	inodes := listenInodes(port)
	if len(inodes) == 0 {
		return nil
	}
	self := os.Getpid()
	seen := map[int]bool{}
	var pids []int
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	for _, e := range procs {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if ino, ok := socketInode(link); ok && inodes[ino] {
				seen[pid] = true
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids
}

// procExe 返回 /proc/<pid>/exe 指向的可执行文件路径，失败返回空串
func procExe(pid int) string {
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(exe, " (deleted)")
}

// isServerName 判断可执行文件名是否属于已知的 frida-server 命名范围
func isServerName(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "frida-server") || strings.Contains(n, "frida_server")
}

// isSelfProcess 判断 pid 是否为当前 utsusemi-ctl 可执行程序本身
func isSelfProcess(pid int) bool {
	if pid == os.Getpid() {
		return true
	}
	base := strings.ToLower(filepath.Base(procExe(pid)))
	return base == "utsusemi-ctl" || base == "utsusemi"
}

// isServerProcess 判断 pid 是否为 frida-server 进程：
// exe 指向 serverDir 之下或文件名命中已知范围，或 cmdline 含 frida-server
func isServerProcess(pid int, serverDir string) bool {
	if pid <= 0 || isSelfProcess(pid) {
		return false
	}
	exe := procExe(pid)
	if exe != "" {
		if serverDir != "" {
			dir := filepath.Clean(serverDir)
			if exe == dir || strings.HasPrefix(exe, dir+string(os.PathSeparator)) {
				return true
			}
		}
		if isServerName(filepath.Base(exe)) {
			return true
		}
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	return strings.Contains(strings.ReplaceAll(string(raw), "\x00", " "), "frida-server")
}

// KillProcess 优雅终止进程：SIGTERM 后每 50ms 检查存活，超时未退则 SIGKILL
func KillProcess(pid int, timeout time.Duration) {
	if pid <= 0 || pid == os.Getpid() || !aliveBasic(pid) {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !aliveBasic(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if aliveBasic(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// collectResidualPIDs 汇总需要清理/提示的 frida-server 进程：
// pidfile 记录、占用监听端口的 server、以及全量扫描到的 server 进程
func collectResidualPIDs(p core.Paths) []int {
	seen := map[int]bool{}
	var pids []int
	add := func(pid int) {
		if pid <= 0 || pid == os.Getpid() || seen[pid] {
			return
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	// a) pidfile 记录且仍存活
	if pid := readPid(p); pid > 0 && aliveBasic(pid) {
		add(pid)
	}
	port := defaultListenPort
	if s, err := core.LoadSettings(p); err == nil {
		port = parseListenPort(s.Server.Args)
	}
	// b) 占用监听端口的 server 进程
	for _, pid := range findListeningPIDs(port) {
		if isServerProcess(pid, p.ServerDir()) {
			add(pid)
		}
	}
	// c) 全量扫描 server 进程
	ents, err := os.ReadDir("/proc")
	if err == nil {
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			pid, err := strconv.Atoi(e.Name())
			if err != nil {
				continue
			}
			if isServerProcess(pid, p.ServerDir()) {
				add(pid)
			}
		}
	}
	sort.Ints(pids)
	return pids
}

// KillResiduals 清理 pidfile 记录、端口占用及残留的 frida-server 进程，
// 复位 pidfile 与 module.prop，返回被处理的 PID 列表
func KillResiduals(p core.Paths) ([]int, error) {
	if _, err := core.LoadSettings(p); err != nil {
		return nil, err
	}
	pids := collectResidualPIDs(p)
	for _, pid := range pids {
		KillProcess(pid, 3*time.Second)
	}
	os.Remove(p.PidFile())
	_ = UpdateDescription(p, false, "")
	return pids, nil
}

func (m *Manager) Status() Status {
	abs, b, err := m.resolve()
	st := Status{Binary: b.File}
	pid := readPid(m.P)
	if err == nil && alive(pid, b.File) {
		st.Running, st.PID = true, pid
		if fi, statErr := os.Stat(m.P.PidFile()); statErr == nil {
			st.UptimeSec = int64(time.Since(fi.ModTime()).Seconds())
		}
		return st
	}
	if err != nil {
		st.Note = err.Error()
	}
	_ = abs
	// active 未匹配 pidfile：探测残留/旧核心，避免界面误报 stopped
	residual := collectResidualPIDs(m.P)
	if len(residual) == 0 {
		return st
	}
	st.Running = true
	if st.PID == 0 {
		st.PID = residual[0]
	}
	note := fmt.Sprintf("residual frida-server pid=%v", residual)
	if st.Note != "" {
		st.Note += "; " + note
	} else {
		st.Note = note
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
	// 启动前清理残留与端口占用，避免新旧核心冲突
	if _, err := KillResiduals(m.P); err != nil {
		return Status{}, err
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
	if alive(pid, "") {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if !alive(pid, "") {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if alive(pid, "") {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			time.Sleep(200 * time.Millisecond)
		}
	}
	// 兜底清理残留/端口占用，并复位 pidfile 与 module.prop
	_, _ = KillResiduals(m.P)
	return m.Status(), nil
}
