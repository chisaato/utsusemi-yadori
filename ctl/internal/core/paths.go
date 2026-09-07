package core

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
)

const DefaultRoot = "/data/adb/utsusemi"
const ModuleID = "utsusemi"
const DefaultStage = "/data/local/tmp/" + ModuleID

// DefaultDataDir 返回数据根目录，优先 UTSUSEMI_DATA_DIR 环境变量
func DefaultDataDir() string {
	if v := os.Getenv("UTSUSEMI_DATA_DIR"); v != "" {
		return v
	}
	return DefaultRoot
}

// DefaultStageDir 返回发布区目录，优先 UTSUSEMI_STAGE_DIR 环境变量
func DefaultStageDir() string {
	if v := os.Getenv("UTSUSEMI_STAGE_DIR"); v != "" {
		return v
	}
	return DefaultStage
}

// GenToken 生成 32 字节 base64url 强随机 token（43 字符）
func GenToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Paths 汇聚所有数据落点。
// Root: 控制区（root 才能读写）；Stage: 发布区（app 进程可读，zygisk 从此加载 gadget）。
// 两者均可被 --data-root/--stage 覆盖以便测试
type Paths struct {
	Root  string
	Stage string
}

func New(root string) Paths         { return Paths{Root: root, Stage: DefaultStageDir()} }
func NewStage(root, stage string) Paths { return Paths{Root: root, Stage: stage} }
func Default() Paths                { return NewStage(DefaultDataDir(), DefaultStageDir()) }

func (p Paths) Ensure() error {
	for _, d := range []string{p.Root, p.ServerDir(), p.GadgetDir(), p.Logs()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// EnsureStage 创建发布区（0755，app 进程可进入读取）
func (p Paths) EnsureStage() error { return os.MkdirAll(p.Stage, 0o755) }

func (p Paths) Settings() string  { return filepath.Join(p.Root, "settings.json") }
func (p Paths) Manifest() string  { return filepath.Join(p.Root, "manifest.json") }
func (p Paths) Rules() string     { return filepath.Join(p.Root, "rules.json") }
func (p Paths) PidFile() string   { return filepath.Join(p.Root, "frida-server.pid") }
func (p Paths) ServerDir() string { return filepath.Join(p.Root, "frida-bin", "server") }
func (p Paths) GadgetDir() string { return filepath.Join(p.Root, "frida-bin", "gadget") }
func (p Paths) Logs() string     { return filepath.Join(p.Root, "logs") }
func (p Paths) ServerLog() string { return filepath.Join(p.Logs(), "frida-server.log") }
func (p Paths) CtlLog() string    { return filepath.Join(p.Logs(), "ctl.log") }
func (p Paths) WebLog() string    { return filepath.Join(p.Logs(), "web.log") }

// Web 进程（Gin 远程服务）运行态文件
func (p Paths) WebPidFile() string  { return filepath.Join(p.Root, "web.pid") }
func (p Paths) WebPortFile() string { return filepath.Join(p.Root, "web.port") }

// DownloadTaskFile 哨兵文件路径（ksu 模式单一任务进度）
func (p Paths) DownloadTaskFile() string { return filepath.Join(p.Root, "download-task.json") }

// GadgetConfig 位于发布区：zygisk so 运行在 app 进程，读不到 /data/adb
func (p Paths) GadgetConfig() string { return filepath.Join(p.Stage, "gadget.json") }

// ModuleProp 派生自数据根兄弟目录，使 --data-root 测试可用
func (p Paths) ModuleProp() string {
	return filepath.Join(filepath.Dir(p.Root), "modules", ModuleID, "module.prop")
}
