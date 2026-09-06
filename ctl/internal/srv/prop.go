package srv

import (
	"os"
	"strings"

	"utsusemi/ctl/internal/core"
)

// UpdateDescription 回写 module.prop 的 description= 行；文件缺失静默跳过
func UpdateDescription(p core.Paths, running bool, note string) error {
	path := p.ModuleProp()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	desc := "description=frida-server: ○ stopped"
	if running {
		desc = "description=frida-server: ● running"
	}
	if note != "" {
		desc += " (" + note + ")"
	}
	lines := strings.Split(string(raw), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "description=") {
			lines[i] = desc
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
