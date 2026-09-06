package cmd

import (
	"github.com/spf13/cobra"

	"utsusemi/ctl/internal/core"
	"utsusemi/ctl/internal/gadgetcfg"
	"utsusemi/ctl/internal/srv"
)

// newBootCmd 开机入口：service.sh 调用；尊重 autostart 并同步 gadget 发布区
func newBootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "boot",
		Short: "开机入口：按 autostart 启动 server 并同步 gadget 发布区",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := core.LoadSettings(paths())
			if err != nil {
				return fail(c, err)
			}
			note := ""
			var st srv.Status
			if s.Server.Autostart {
				st, err = srv.New(paths()).Start()
				if err != nil {
					note = err.Error()
				}
			}
			// gadget 发布区同步失败不阻断 boot（无规则/无 gadget 属正常冷启动状态）
			if aerr := gadgetcfg.Apply(paths()); aerr != nil && note == "" {
				note = "gadget: " + aerr.Error()
			}
			emit(c, Envelope{OK: true, Data: map[string]any{
				"autostart": s.Server.Autostart,
				"server":    st,
				"note":      note,
			}})
			return nil
		},
	}
}
