package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"utsusemi/ctl/internal/core"
)

// Envelope 是所有 --json 输出的统一信封
type Envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

var (
	dataRoot string
	stageDir string
	asJSON   bool
	Version  = "0.3.0"
)

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "utsusemi-ctl",
		Short:         "Utsusemi-Yadori Frida manager",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	// 支持 UTSUSEMI_DATA_DIR / UTSUSEMI_STAGE_DIR 环境变量作为默认值
	root.PersistentFlags().StringVar(&dataRoot, "data-root", core.DefaultDataDir(), "数据根目录")
	root.PersistentFlags().StringVar(&stageDir, "stage", core.DefaultStageDir(), "发布区目录（app 进程可读）")
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "以 JSON 信封输出")
	root.AddCommand(newVersionCmd(), newBinCmd(), newServerCmd(), newGadgetCmd(), newStatusCmd(), newBootCmd(), newAPICmd(), newWebCmd(), newAdbCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use: "version",
		RunE: func(c *cobra.Command, args []string) error {
			emit(c, Envelope{OK: true, Data: map[string]string{"version": Version}})
			return nil
		},
	}
}

func paths() core.Paths { return core.NewStage(dataRoot, stageDir) }

// emit 统一输出：--json 走信封到 stdout；否则人读文本
func emit(c *cobra.Command, env Envelope) {
	if asJSON {
		_ = json.NewEncoder(c.OutOrStdout()).Encode(env)
		return
	}
	if env.OK {
		if s, ok := env.Data.(fmt.Stringer); ok {
			fmt.Fprintln(c.OutOrStdout(), s)
		}
		return
	}
	fmt.Fprintf(c.ErrOrStderr(), "error: %s\n", env.Error)
}

// errSilent 让 cobra 静默退出而不重复打印错误（错误已由 emit 输出）
var errSilent = errors.New("silent")

// Execute 供 main 与测试调用；返回进程退出码
func Execute(args []string, stdout, stderr io.Writer) int {
	root := NewRoot()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		if errors.Is(err, errSilent) {
			return 1
		}
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}
