package core

import (
	"os"
	"path/filepath"
)

const DefaultRoot = "/data/adb/utsusemi"
const ModuleID = "utsusemi"

// Paths 汇聚所有数据落点；Root 可被 --data-root 覆盖以便测试
type Paths struct{ Root string }

func New(root string) Paths { return Paths{Root: root} }
func Default() Paths        { return Paths{Root: DefaultRoot} }

func (p Paths) Ensure() error {
	for _, d := range []string{p.Root, p.ServerDir(), p.GadgetDir(), p.Logs()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p Paths) Settings() string     { return filepath.Join(p.Root, "settings.json") }
func (p Paths) Manifest() string     { return filepath.Join(p.Root, "manifest.json") }
func (p Paths) Rules() string        { return filepath.Join(p.Root, "rules.json") }
func (p Paths) GadgetConfig() string { return filepath.Join(p.Root, "gadget.json") }
func (p Paths) PidFile() string      { return filepath.Join(p.Root, "frida-server.pid") }
func (p Paths) ServerDir() string    { return filepath.Join(p.Root, "frida-bin", "server") }
func (p Paths) GadgetDir() string    { return filepath.Join(p.Root, "frida-bin", "gadget") }
func (p Paths) Logs() string         { return filepath.Join(p.Root, "logs") }
func (p Paths) ServerLog() string    { return filepath.Join(p.Logs(), "frida-server.log") }
func (p Paths) CtlLog() string       { return filepath.Join(p.Logs(), "ctl.log") }

// ModuleProp 派生自数据根兄弟目录，使 --data-root 测试可用
func (p Paths) ModuleProp() string {
	return filepath.Join(filepath.Dir(p.Root), "modules", ModuleID, "module.prop")
}
