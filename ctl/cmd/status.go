package cmd

import (
	"strconv"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use: "status",
		RunE: func(c *cobra.Command, _ []string) error {
			// 与 api status / GET /api/status 完全同构（复用 ops 层）
			data, err := opsFor(paths()).Status()
			if err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: data})
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
