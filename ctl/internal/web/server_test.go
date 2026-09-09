package web

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// mockOps 记录调用并返回固定数据，覆盖 Ops 全方法
type mockOps struct {
	lastAction    string
	lastServerSet []byte
	lastGadgetSet []byte
	lastDownload  [3]string
	lastImport    [3]string
	lastRemove    [2]string
	lastUse       [2]string
	lastLogName   string
	stopped       bool
}

func (m *mockOps) Status() (any, error) { return map[string]any{"ok": "status"}, nil }
func (m *mockOps) ServerAction(a string) (any, error) {
	m.lastAction = a
	return map[string]any{"action": a}, nil
}
func (m *mockOps) ServerSet(p []byte) (any, error) {
	m.lastServerSet = p
	return map[string]any{"saved": true}, nil
}
func (m *mockOps) Apps() (any, error) { return []any{}, nil }
func (m *mockOps) GadgetRules() (any, error) {
	return map[string]any{"rules": []any{}}, nil
}
func (m *mockOps) GadgetSet(p []byte) (any, error) {
	m.lastGadgetSet = p
	return map[string]any{"saved": true, "applied": 0}, nil
}
func (m *mockOps) BinList() (any, error) {
	return map[string]any{"servers": []any{}, "gadgets": []any{}}, nil
}
func (m *mockOps) BinSources() (any, error) { return []any{}, nil }
func (m *mockOps) BinDownload(variant, typ, ver string, async bool, tm *TaskManager) (any, error) {
	m.lastDownload = [3]string{variant, typ, ver}
	if !async {
		return map[string]any{"file": "x"}, nil
	}
	id := NewTaskID()
	tm.Start(id, func(upd func(phase, detail string) error) error {
		_ = upd("download", variant)
		time.Sleep(30 * time.Millisecond)
		return nil
	})
	return map[string]string{"task_id": id}, nil
}
func (m *mockOps) BinImport(typ, path, ver string, async bool, tm *TaskManager) (any, error) {
	m.lastImport = [3]string{typ, path, ver}
	return map[string]any{"file": "imported"}, nil
}
func (m *mockOps) BinRemove(typ, file string) (any, error) {
	m.lastRemove = [2]string{typ, file}
	return map[string]string{"removed": file}, nil
}
func (m *mockOps) BinUse(typ, file string) (any, error) {
	m.lastUse = [2]string{typ, file}
	return map[string]string{"active": file}, nil
}
func (m *mockOps) BinCleanup() (any, error) {
	return map[string][]string{"removed": {}}, nil
}
func (m *mockOps) Logs(name string, tail int) (any, error) {
	m.lastLogName = name
	return map[string]any{"name": name, "lines": []string{"a"}}, nil
}
func (m *mockOps) WebInfo() (any, error) {
	return map[string]any{"running": true, "port": 23333, "token": "tok"}, nil
}
func (m *mockOps) WebToken(p []byte) (any, error) {
	return map[string]any{"token": "newtok", "restart_required": false}, nil
}
func (m *mockOps) TaskCurrent() (any, error) {
	return Task{ID: "ksu", State: "running", Phase: "download"}, nil
}
func (m *mockOps) AdbStatus() (any, error) {
	return map[string]any{"current": map[string]any{"adbd_running": true}, "settings": map[string]any{"usb_enabled": true}}, nil
}
func (m *mockOps) AdbSet(p []byte) (any, error) {
	return map[string]any{"saved": true, "applied": false}, nil
}
func (m *mockOps) AdbUsb(p []byte) (any, error) {
	return map[string]any{"usb_enabled": true}, nil
}
func (m *mockOps) AdbTcpip(p []byte) (any, error) {
	return map[string]any{"tcpip_enabled": true, "tcpip_port": 5555}, nil
}
func (m *mockOps) AdbRestart() (any, error) {
	return map[string]any{"restarted": true}, nil
}
func (m *mockOps) AdbApply() (any, error) {
	return map[string]any{"applied": true}, nil
}

func newTestServer(t *testing.T) (*mockOps, *httptest.Server) {
	t.Helper()
	ops := &mockOps{}
	srv := NewServer(ops, NewTaskManager(), "tok", nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ops, ts
}

// decode 解析统一信封
func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("bad envelope: %v", err)
	}
	return env
}

func TestTokenAuth(t *testing.T) {
	_, ts := newTestServer(t)

	// 无 token → 401
	resp, _ := http.Get(ts.URL + "/api/status")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", resp.StatusCode)
	}
	// query token → 200
	resp, _ = http.Get(ts.URL + "/api/status?token=tok")
	if env := decode(t, resp); env["ok"] != true {
		t.Fatalf("query token: %v", env)
	}
	// bearer token → 200
	req, _ := http.NewRequest("GET", ts.URL+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, _ = http.DefaultClient.Do(req)
	if env := decode(t, resp); env["ok"] != true {
		t.Fatalf("bearer: %v", env)
	}
	// 错 token → 401
	resp, _ = http.Get(ts.URL + "/api/status?token=bad")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: want 401, got %d", resp.StatusCode)
	}
}

func TestOpsErrorMapsToEnvelope(t *testing.T) {
	// mock 返回 error 的路径：直接用一个 errorOps 验证 handler 包装
	srv := NewServer(&errOps{}, NewTaskManager(), "", nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	resp, _ := http.Get(ts.URL + "/api/status")
	env := decode(t, resp)
	if resp.StatusCode != http.StatusOK || env["ok"] != false {
		t.Fatalf("error mapping: %d %v", resp.StatusCode, env)
	}
	if !strings.Contains(env["error"].(string), "boom") {
		t.Fatalf("error content: %v", env)
	}
}

type errOps struct{ mockOps }

func (*errOps) Status() (any, error) {
	return nil, errBoom
}

var errBoom = errors.New("boom")

var _ = fs.ValidPath // 占位避免未用 import（fstest 在下文使用）

func TestEndpointsMatrix(t *testing.T) {
	ops, ts := newTestServer(t)
	q := ts.URL + "/api"

	post := func(path, body string) map[string]any {
		resp, err := http.Post(q+path+"?token=tok", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return decode(t, resp)
	}
	put := func(path, body string) map[string]any {
		req, _ := http.NewRequest("PUT", q+path+"?token=tok", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return decode(t, resp)
	}
	get := func(path string) map[string]any {
		resp, err := http.Get(q + path + "&token=tok")
		if err != nil {
			t.Fatal(err)
		}
		return decode(t, resp)
	}

	if env := get("/status?"); env["ok"] != true {
		t.Fatalf("status: %v", env)
	}
	if env := post("/server/start", ""); env["ok"] != true || ops.lastAction != "start" {
		t.Fatalf("server/start: %v action=%s", env, ops.lastAction)
	}
	if env := post("/server/stop", ""); ops.lastAction != "stop" {
		t.Fatalf("server/stop: %v", env)
	}
	if env := post("/server/restart", ""); ops.lastAction != "restart" {
		t.Fatalf("server/restart: %v", env)
	}
	if env := put("/server", `{"autostart":false}`); env["ok"] != true || string(ops.lastServerSet) != `{"autostart":false}` {
		t.Fatalf("PUT server: %v %s", env, ops.lastServerSet)
	}
	if env := get("/apps?"); env["ok"] != true {
		t.Fatalf("apps: %v", env)
	}
	if env := get("/gadget?"); env["ok"] != true {
		t.Fatalf("gadget: %v", env)
	}
	if env := put("/gadget", `{"rules":[]}`); env["ok"] != true || string(ops.lastGadgetSet) != `{"rules":[]}` {
		t.Fatalf("PUT gadget: %v", env)
	}
	if env := get("/bin?"); env["ok"] != true {
		t.Fatalf("bin: %v", env)
	}
	if env := get("/bin/sources?"); env["ok"] != true {
		t.Fatalf("bin/sources: %v", env)
	}

	// 异步下载 → task_id → 轮询 done
	env := post("/bin/download", `{"variant":"florida","type":"gadget","version":"17.9.1"}`)
	if env["ok"] != true || ops.lastDownload != [3]string{"florida", "gadget", "17.9.1"} {
		t.Fatalf("download: %v %+v", env, ops.lastDownload)
	}
	taskID := env["data"].(map[string]any)["task_id"].(string)
	done := false
	for i := 0; i < 50; i++ {
		tEnv := get("/tasks/" + taskID + "?")
		if st := tEnv["data"].(map[string]any)["state"]; st == "done" {
			done = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !done {
		t.Fatal("task never done")
	}

	// bin/use、bin/cleanup
	if env := post("/bin/use", `{"type":"server","file":"f1"}`); env["ok"] != true || ops.lastUse != [2]string{"server", "f1"} {
		t.Fatalf("bin/use: %v", env)
	}
	if env := post("/bin/cleanup", ""); env["ok"] != true {
		t.Fatalf("bin/cleanup: %v", env)
	}

	// DELETE bin/:file
	req, _ := http.NewRequest("DELETE", q+"/bin/f2?type=gadget&token=tok", nil)
	resp, _ := http.DefaultClient.Do(req)
	if env := decode(t, resp); env["ok"] != true || ops.lastRemove != [2]string{"gadget", "f2"} {
		t.Fatalf("DELETE bin: %v", env)
	}

	// logs
	if env := get("/logs?name=server&tail=10"); env["ok"] != true || ops.lastLogName != "server" {
		t.Fatalf("logs: %v", env)
	}

	// web/info
	if env := get("/web/info?"); env["ok"] != true {
		t.Fatalf("web/info: %v", env)
	}

	// PUT /web/token
	reqToken, _ := http.NewRequest("PUT", q+"/web/token?token=tok", strings.NewReader(`{"generate":true}`))
	reqToken.Header.Set("Content-Type", "application/json")
	respToken, errToken := http.DefaultClient.Do(reqToken)
	if errToken != nil {
		t.Fatal(errToken)
	}
	if env := decode(t, respToken); env["ok"] != true {
		t.Fatalf("PUT /web/token: %v", env)
	}

	// adb endpoints
	if env := get("/adb?"); env["ok"] != true {
		t.Fatalf("GET /adb: %v", env)
	}
	if env := put("/adb/settings", `{"usb_enabled":true,"tcpip_enabled":true,"port":5555}`); env["ok"] != true {
		t.Fatalf("PUT /adb/settings: %v", env)
	}
	if env := post("/adb/usb", `{"enabled":true}`); env["ok"] != true {
		t.Fatalf("POST /adb/usb: %v", env)
	}
	if env := post("/adb/tcpip", `{"enabled":true,"port":5555}`); env["ok"] != true {
		t.Fatalf("POST /adb/tcpip: %v", env)
	}
	if env := post("/adb/restart", `{}`); env["ok"] != true {
		t.Fatalf("POST /adb/restart: %v", env)
	}
	if env := post("/adb/apply", `{}`); env["ok"] != true {
		t.Fatalf("POST /adb/apply: %v", env)
	}

	// web/stop：200 信封 + 触发 OnStop + 响应头标记（供 serve 循环识别）
	stopped := make(chan struct{})
	srv := NewServer(ops, NewTaskManager(), "tok", nil)
	srv.OnStop = func() { close(stopped) }
	ts2 := httptest.NewServer(srv.Handler())
	defer ts2.Close()
	resp, _ = http.Post(ts2.URL+"/api/web/stop?token=tok", "", nil)
	if env := decode(t, resp); env["ok"] != true {
		t.Fatalf("web/stop: %v", env)
	}
	if resp.Header.Get("X-Utsusemi-Stop") != "1" {
		t.Fatal("missing stop header")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("OnStop not fired")
	}
}

func TestStaticSPA(t *testing.T) {
	// 静态资源不鉴权；未知路径回落 index.html（仅非 /api）
	static := fstest.MapFS{
		"index.html":         &fstest.MapFile{Data: []byte("<h1>utsusemi</h1>")},
		"assets/app.js":      &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	srv := NewServer(&mockOps{}, NewTaskManager(), "tok", static)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/")
	if resp.StatusCode != 200 || !strings.Contains(readAll(t, resp), "utsusemi") {
		t.Fatalf("index: %d", resp.StatusCode)
	}
	resp, _ = http.Get(ts.URL + "/assets/app.js")
	if resp.StatusCode != 200 {
		t.Fatalf("asset: %d", resp.StatusCode)
	}
	// SPA 回落：任意未知路径 → index.html
	resp, _ = http.Get(ts.URL + "/bin/manage")
	if resp.StatusCode != 200 || !strings.Contains(readAll(t, resp), "utsusemi") {
		t.Fatalf("spa fallback: %d", resp.StatusCode)
	}
	// /api 未知路径仍走信封（不回落静态）
	resp, _ = http.Get(ts.URL + "/api/whatever?token=tok")
	if env := decode(t, resp); env["ok"] != false {
		t.Fatalf("api 404 should be envelope: %v", env)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	return string(buf[:n])
}
