package cmd

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"utsusemi/ctl/internal/core"
	"utsusemi/ctl/internal/web"
	"utsusemi/ctl/webui"
)

func newWebCmd() *cobra.Command {
	c := &cobra.Command{Use: "web", Short: "on-demand remote web service"}
	c.AddCommand(webStartCmd(), webStopCmd(), webStatusCmd(), webServeCmd())
	return c
}

// genToken 生成 32 字节 base64url 强随机 token（43 字符）
func genToken() string {
	return core.GenToken()
}

func readWebPid(p core.Paths) int {
	raw, err := os.ReadFile(p.WebPidFile())
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return pid
}

// waitWebReady 轮询 web.port 文件出现且端口可连接（serve fork 后异步就绪）
func waitWebReady(p core.Paths, timeout time.Duration) (int, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(p.WebPortFile()); err == nil {
			if port, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && port > 0 {
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
				if err == nil {
					conn.Close()
					return port, true
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0, false
}

func webStartCmd() *cobra.Command {
	var port int
	var tokenFlag string
	c := &cobra.Command{
		Use:  "start",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p := paths()
			s, err := core.LoadSettings(p)
			if err != nil {
				return fail(c, err)
			}
			if c.Flags().Changed("port") {
				s.Web.Port = port
			}
			if tokenFlag != "" {
				s.Web.Token = tokenFlag
			}
			if s.Web.Token == "" {
				s.Web.Token = genToken()
			}
			s.Web.Enabled = true
			if err := core.SaveSettings(p, s); err != nil {
				return fail(c, err)
			}
			// 幂等：已运行则直接报信息
			if pid := readWebPid(p); pid > 0 && syscall.Kill(pid, 0) == nil {
				data, err := opsFor(p).WebInfo()
				if err != nil {
					return fail(c, err)
				}
				emit(c, Envelope{OK: true, Data: data})
				return nil
			}
			// fork 可执行文件：优先环境变量注入（测试传真实 ctl 二进制；go test 二进制无 cobra 入口）
			exe := os.Getenv("UTSUSEMI_CTL_EXE")
			if exe == "" {
				exe, err = os.Executable()
				if err != nil {
					return fail(c, err)
				}
			}
			if err := os.MkdirAll(p.Logs(), 0o755); err != nil {
				return fail(c, err)
			}
			log, err := os.OpenFile(p.WebLog(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return fail(c, err)
			}
			defer log.Close()
			// fork 自身进入 serve 子命令；透传 --data-root/--stage 保证测试隔离
			args := []string{"web", "serve", "--port", strconv.Itoa(s.Web.Port), "--token", s.Web.Token,
				"--data-root", p.Root, "--stage", p.Stage}
			cmd := exec.Command(exe, args...)
			cmd.Stdout, cmd.Stderr = log, log
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			if err := cmd.Start(); err != nil {
				return fail(c, err)
			}
			_ = cmd.Process.Release()
			if _, ok := waitWebReady(p, 5*time.Second); !ok {
				return fail(c, fmt.Errorf("web service not ready in 5s, see %s", p.WebLog()))
			}
			// 就绪后统一走 ops.WebInfo（含实际端口/token）
			data, err := opsFor(p).WebInfo()
			if err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
	c.Flags().IntVar(&port, "port", 0, "监听端口（0=OS 分配，回写 web.port）")
	c.Flags().StringVar(&tokenFlag, "token", "", "固定 token（默认自动生成并写回 settings）")
	return c
}

func webStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:  "stop",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p := paths()
			pid := readWebPid(p)
			if pid > 0 && syscall.Kill(pid, 0) == nil {
				_ = syscall.Kill(pid, syscall.SIGTERM)
				for i := 0; i < 30; i++ {
					if syscall.Kill(pid, 0) != nil {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
				if syscall.Kill(pid, 0) == nil {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			}
			os.Remove(p.WebPidFile())
			os.Remove(p.WebPortFile())
			// 维护期望状态
			if s, err := core.LoadSettings(p); err == nil {
				s.Web.Enabled = false
				_ = core.SaveSettings(p, s)
			}
			emit(c, Envelope{OK: true, Data: map[string]bool{"stopped": true}})
			return nil
		},
	}
}

func webStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:  "status",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			data, err := opsFor(paths()).WebInfo()
			if err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
}

// webServeCmd 内部子命令：由 web start 后台拉起，前台跑 Gin 直至 OnStop/信号
func webServeCmd() *cobra.Command {
	var port int
	var token string
	c := &cobra.Command{
		Use:   "serve",
		Short: "internal: run gin server in foreground",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p := paths()
			ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
			if err != nil {
				return fmt.Errorf("listen: %w", err)
			}
			// 实际端口与 pid 落盘（--port 0 场景 start 侧靠 web.port 就绪探测）
			realPort := ln.Addr().(*net.TCPAddr).Port
			if err := os.WriteFile(p.WebPortFile(), []byte(strconv.Itoa(realPort)), 0o644); err != nil {
				return err
			}
			if err := os.WriteFile(p.WebPidFile(), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
				return err
			}
			defer func() {
				os.Remove(p.WebPidFile())
				os.Remove(p.WebPortFile())
			}()

			srv := web.NewServer(opsFor(p), web.NewTaskManager(), token, webui.FS())
			stopped := make(chan struct{})
			srv.OnStop = func() { close(stopped) }
			httpS := &http.Server{Handler: srv.Handler()}
			errCh := make(chan error, 1)
			go func() { errCh <- httpS.Serve(ln) }()

			// SIGTERM/SIGINT 与 /api/web/stop 同等触发优雅退出
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
			select {
			case <-stopped:
			case <-sig:
			case err := <-errCh:
				if err != nil && err != http.ErrServerClosed {
					return err
				}
			}
			_ = httpS.Close()
			return nil
		},
	}
	c.Flags().IntVar(&port, "port", 23333, "listen port (0=auto)")
	c.Flags().StringVar(&token, "token", "", "auth token (empty=no auth)")
	return c
}
