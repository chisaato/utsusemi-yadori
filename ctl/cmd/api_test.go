package cmd

import (
	"encoding/json"
	"testing"
)

// api 子命令永远输出 JSON 信封（无视 --json 旗标），与 REST 同构。
// 用 runJSON 复用 --json 注入的 Envelope 断言路径

func TestAPIStatusShape(t *testing.T) {
	root := t.TempDir()
	env := runJSON(t, "--data-root", root, "api", "status")
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var st map[string]any
	json.Unmarshal(raw, &st)
	for _, k := range []string{"server", "servers", "gadgets", "rules", "web", "settings"} {
		if _, ok := st[k]; !ok {
			t.Fatalf("api status missing %q: %v", k, st)
		}
	}
}

func TestAPIServerSetAndStart(t *testing.T) {
	root := t.TempDir()
	with := func(args ...string) []string { return append([]string{"--data-root", root}, args...) }

	env := runJSON(t, with("api", "server/set", "--payload", `{"autostart":false,"args":["-l","127.0.0.1:27043"]}`)...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	// 回读：settings.server 段已更新
	env = runJSON(t, with("api", "status")...)
	raw, _ := json.Marshal(env.Data)
	var st struct {
		Settings struct {
			Server struct {
				Autostart bool     `json:"autostart"`
				Args      []string `json:"args"`
			} `json:"server"`
		} `json:"settings"`
	}
	json.Unmarshal(raw, &st)
	if st.Settings.Server.Autostart || len(st.Settings.Server.Args) != 2 {
		t.Fatalf("server/set not persisted: %+v", st.Settings.Server)
	}
	// 无激活二进制时 start → ok:false 且 error 可读
	env = runJSON(t, with("api", "server/start")...)
	if env.OK || env.Error == "" {
		t.Fatalf("server/start without binary: %+v", env)
	}
}

func TestAPIGadgetSetAndRules(t *testing.T) {
	root := t.TempDir()
	with := func(args ...string) []string { return append([]string{"--data-root", root}, args...) }

	// 无激活 gadget：规则保存成功，apply 失败须在 apply_error 中体现而不整体失败
	env := runJSON(t, with("api", "gadget/set", "--payload", `{"rules":[{"app_name":"com.x","enabled":true}]}`)...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var d struct {
		Saved      bool   `json:"saved"`
		Applied    int    `json:"applied"`
		ApplyError string `json:"apply_error"`
	}
	json.Unmarshal(raw, &d)
	if !d.Saved || d.Applied != 0 || d.ApplyError == "" {
		t.Fatalf("gadget/set data: %+v", d)
	}
	// 回读规则一致
	env = runJSON(t, with("api", "gadget")...)
	raw, _ = json.Marshal(env.Data)
	var rr struct {
		Rules []map[string]any `json:"rules"`
	}
	json.Unmarshal(raw, &rr)
	if len(rr.Rules) != 1 || rr.Rules[0]["app_name"] != "com.x" {
		t.Fatalf("api gadget rules: %+v", rr)
	}
}

func TestAPIBinListLogsWebInfo(t *testing.T) {
	root := t.TempDir()
	with := func(args ...string) []string { return append([]string{"--data-root", root}, args...) }

	env := runJSON(t, with("api", "bin/list")...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var bins map[string][]any
	json.Unmarshal(raw, &bins)
	if _, ok := bins["servers"]; !ok {
		t.Fatalf("bin/list data: %v", bins)
	}

	env = runJSON(t, with("api", "logs", "--name", "ctl")...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ = json.Marshal(env.Data)
	var lg struct {
		Name  string   `json:"name"`
		Lines []string `json:"lines"`
	}
	json.Unmarshal(raw, &lg)
	if lg.Name != "ctl" || lg.Lines == nil {
		t.Fatalf("logs data: %+v", lg)
	}

	env = runJSON(t, with("api", "web/info")...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ = json.Marshal(env.Data)
	var wi map[string]any
	json.Unmarshal(raw, &wi)
	if _, ok := wi["running"]; !ok {
		t.Fatalf("web/info data: %v", wi)
	}
}

func TestAPITasksCurrent(t *testing.T) {
	root := t.TempDir()
	with := func(args ...string) []string { return append([]string{"--data-root", root}, args...) }

	// 哨兵文件不存在：ok: false
	env := runJSON(t, with("api", "tasks/current")...)
	if env.OK {
		t.Fatalf("expected ok:false when no current task, got %+v", env)
	}
}

func TestAPIWebToken(t *testing.T) {
	root := t.TempDir()
	with := func(args ...string) []string { return append([]string{"--data-root", root}, args...) }

	// 1. 无 payload 读取当前 token
	env := runJSON(t, with("api", "web/token")...)
	if !env.OK {
		t.Fatalf("web/token read failed: %s", env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var res struct {
		Token           string `json:"token"`
		RestartRequired bool   `json:"restart_required"`
	}
	json.Unmarshal(raw, &res)

	// 2. generate 生成新 token
	envGen := runJSON(t, with("api", "web/token", "--payload", `{"generate":true}`)...)
	if !envGen.OK {
		t.Fatalf("web/token generate failed: %s", envGen.Error)
	}
	var resGen struct {
		Token           string `json:"token"`
		RestartRequired bool   `json:"restart_required"`
	}
	rawGen, _ := json.Marshal(envGen.Data)
	json.Unmarshal(rawGen, &resGen)
	if len(resGen.Token) != 43 {
		t.Fatalf("expected 43-char base64url token, got %q (len %d)", resGen.Token, len(resGen.Token))
	}

	// 3. 自定义 token
	envCustom := runJSON(t, with("api", "web/token", "--payload", `{"token":"custom-token-123456"}`)...)
	if !envCustom.OK {
		t.Fatalf("web/token custom failed: %s", envCustom.Error)
	}
	var resCustom struct {
		Token           string `json:"token"`
		RestartRequired bool   `json:"restart_required"`
	}
	rawCustom, _ := json.Marshal(envCustom.Data)
	json.Unmarshal(rawCustom, &resCustom)
	if resCustom.Token != "custom-token-123456" {
		t.Fatalf("expected custom-token-123456, got %s", resCustom.Token)
	}

	// 4. 非法 token 校验拒绝：过短
	envShort := runJSON(t, with("api", "web/token", "--payload", `{"token":"short"}`)...)
	if envShort.OK {
		t.Fatal("expected rejection for short token")
	}

	// 5. 非法 token 校验拒绝：含空白
	envSpace := runJSON(t, with("api", "web/token", "--payload", `{"token":"token with space"}`)...)
	if envSpace.OK {
		t.Fatal("expected rejection for token with space")
	}
}
