package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestMain 构建真实 ctl 二进制注入 fork 路径（go test 二进制无 cobra 入口，serve 起不来）
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "utsusemi-ctl-test-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mktemp:", err)
		os.Exit(1)
	}
	exe := filepath.Join(tmp, "utsusemi-ctl")
	if out, err := exec.Command("go", "build", "-o", exe, "..").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "go build: %v\n%s\n", err, out)
		os.Exit(1)
	}
	os.Setenv("UTSUSEMI_CTL_EXE", exe)
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// webAliveData 解析 web status 输出
func webStatusData(t *testing.T, root string) map[string]any {
	t.Helper()
	env := runJSON(t, "--data-root", root, "web", "status")
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var wi map[string]any
	json.Unmarshal(raw, &wi)
	return wi
}

func TestWebLifecycle(t *testing.T) {
	root := t.TempDir()

	// 初始未运行
	if wi := webStatusData(t, root); wi["running"] != false {
		t.Fatalf("initial: %v", wi)
	}

	// start（--port 0 由 OS 分配，实际端口回写 web.port）
	env := runJSON(t, "--data-root", root, "web", "start", "--port", "0")
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var started map[string]any
	json.Unmarshal(raw, &started)
	token, _ := started["token"].(string)
	if token == "" {
		t.Fatalf("no token generated: %v", started)
	}

	// 轮询直到 web status running=true
	deadline := time.Now().Add(5 * time.Second)
	running := false
	for time.Now().Before(deadline) {
		if wi := webStatusData(t, root); wi["running"] == true {
			running = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !running {
		t.Fatal("web service never became ready")
	}

	// HTTP 探活：带 token 的 /api/status 返回信封
	wi := webStatusData(t, root)
	port := int(wi["port"].(float64))
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/status?token=%s", port, token))
	if err != nil {
		t.Fatalf("http probe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe status: %d", resp.StatusCode)
	}

	// 停止
	env = runJSON(t, "--data-root", root, "web", "stop")
	if !env.OK {
		t.Fatal(env.Error)
	}
	// 进程退出后 running=false（pid 死亡判定）
	deadline = time.Now().Add(5 * time.Second)
	stopped := false
	for time.Now().Before(deadline) {
		if wi := webStatusData(t, root); wi["running"] == false {
			stopped = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !stopped {
		// 诊断：残留进程
		out, _ := exec.Command("ps", "-ef").Output()
		t.Fatalf("web not stopped; ps:\n%s", out)
	}
}
