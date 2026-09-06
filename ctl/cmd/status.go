package cmd

import (
	"strconv"

	"github.com/spf13/cobra"

	"utsusemi/ctl/internal/binmgr"
	"utsusemi/ctl/internal/gadgetcfg"
	"utsusemi/ctl/internal/srv"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use: "status",
		RunE: func(c *cobra.Command, _ []string) error {
			p := paths()
			m, err := binmgr.New(p)
			if err != nil {
				return fail(c, err)
			}
			r, _ := gadgetcfg.LoadRules(p)
			emit(c, Envelope{OK: true, Data: map[string]any{
				"server":  srv.New(p).Status(),
				"servers": m.List("server"),
				"gadgets": m.List("gadget"),
				"rules":   r,
				"web":     m.S.Web,
			}})
			return nil
		},
	}
}

// fail 统一错误输出；返回 errSilent 让 Execute 静默处理（错误已由 emit 输出）
func fail(c *cobra.Command, err error) error {
	emit(c, Envelope{OK: false, Error: err.Error()})
	return errSilent
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
