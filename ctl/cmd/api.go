package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"utsusemi/ctl/internal/web"
)

// api 子命令：与 REST 同构的 JSON 桥（KSU WebUI 经 ksu.exec 调用）。
// 无视 --json 旗标，永远输出 Envelope 到 stdout
func newAPICmd() *cobra.Command {
	c := &cobra.Command{Use: "api", Short: "isomorphic JSON bridge for WebUI transports"}
	c.AddCommand(
		apiSimpleCmd("status", func(o web.Ops) (any, error) { return o.Status() }),
		apiSimpleCmd("server/start", func(o web.Ops) (any, error) { return o.ServerAction("start") }),
		apiSimpleCmd("server/stop", func(o web.Ops) (any, error) { return o.ServerAction("stop") }),
		apiSimpleCmd("server/restart", func(o web.Ops) (any, error) { return o.ServerAction("restart") }),
		apiPayloadCmd("server/set", func(o web.Ops, payload []byte) (any, error) { return o.ServerSet(payload) }),
		apiSimpleCmd("apps", func(o web.Ops) (any, error) { return o.Apps() }),
		apiSimpleCmd("gadget", func(o web.Ops) (any, error) { return o.GadgetRules() }),
		apiPayloadCmd("gadget/set", func(o web.Ops, payload []byte) (any, error) { return o.GadgetSet(payload) }),
		apiSimpleCmd("bin/list", func(o web.Ops) (any, error) { return o.BinList() }),
		apiSimpleCmd("bin/sources", func(o web.Ops) (any, error) { return o.BinSources() }),
		apiSimpleCmd("web/info", func(o web.Ops) (any, error) { return o.WebInfo() }),
		apiBinDownloadCmd(), apiBinImportCmd(), apiBinRemoveCmd(), apiBinUseCmd(),
		apiSimpleCmd("bin/cleanup", func(o web.Ops) (any, error) { return o.BinCleanup() }),
		apiLogsCmd(),
	)
	return c
}

// apiEmit/apiFail 强制 JSON 输出（api 子命令不用 emit/asJSON 路径）
func apiEmit(c *cobra.Command, env Envelope) {
	_ = json.NewEncoder(c.OutOrStdout()).Encode(env)
}

func apiFail(c *cobra.Command, err error) error {
	apiEmit(c, Envelope{OK: false, Error: err.Error()})
	return errSilent
}

func apiSimpleCmd(use string, fn func(web.Ops) (any, error)) *cobra.Command {
	return &cobra.Command{
		Use: use,
		RunE: func(c *cobra.Command, _ []string) error {
			data, err := fn(opsFor(paths()))
			if err != nil {
				return apiFail(c, err)
			}
			apiEmit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
}

func apiPayloadCmd(use string, fn func(web.Ops, []byte) (any, error)) *cobra.Command {
	var payload string
	c := &cobra.Command{
		Use: use,
		RunE: func(c *cobra.Command, _ []string) error {
			data, err := fn(opsFor(paths()), []byte(payload))
			if err != nil {
				return apiFail(c, err)
			}
			apiEmit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
	c.Flags().StringVar(&payload, "payload", "{}", "JSON body（与 REST 请求体一致）")
	return c
}

// api 子命令内下载/导入为同步执行（ksu.exec 阻塞语义）；REST 才走异步任务
func apiBinDownloadCmd() *cobra.Command {
	var variant, typ, version string
	c := &cobra.Command{
		Use: "bin/download",
		RunE: func(c *cobra.Command, _ []string) error {
			data, err := opsFor(paths()).BinDownload(variant, typ, version, false, nil)
			if err != nil {
				return apiFail(c, err)
			}
			apiEmit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
	c.Flags().StringVar(&variant, "variant", "official", "official|florida|undetected")
	c.Flags().StringVar(&typ, "type", "server", "server|gadget")
	c.Flags().StringVar(&version, "version", "", "default: latest with assets")
	return c
}

func apiBinImportCmd() *cobra.Command {
	var typ, file, version string
	c := &cobra.Command{
		Use: "bin/import",
		RunE: func(c *cobra.Command, _ []string) error {
			data, err := opsFor(paths()).BinImport(typ, file, version, false, nil)
			if err != nil {
				return apiFail(c, err)
			}
			apiEmit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
	c.Flags().StringVar(&typ, "type", "", "server|gadget (required)")
	_ = c.MarkFlagRequired("type")
	c.Flags().StringVar(&file, "file", "", "source path (required)")
	_ = c.MarkFlagRequired("file")
	c.Flags().StringVar(&version, "version", "", "default: unknown")
	return c
}

func apiBinRemoveCmd() *cobra.Command {
	var typ, file string
	c := &cobra.Command{
		Use:  "bin/remove",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			data, err := opsFor(paths()).BinRemove(typ, file)
			if err != nil {
				return apiFail(c, err)
			}
			apiEmit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
	c.Flags().StringVar(&typ, "type", "", "server|gadget (required)")
	_ = c.MarkFlagRequired("type")
	c.Flags().StringVar(&file, "file", "", "file key (required)")
	_ = c.MarkFlagRequired("file")
	return c
}

func apiBinUseCmd() *cobra.Command {
	var typ, file string
	c := &cobra.Command{
		Use:  "bin/use",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			data, err := opsFor(paths()).BinUse(typ, file)
			if err != nil {
				return apiFail(c, err)
			}
			apiEmit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
	c.Flags().StringVar(&typ, "type", "", "server|gadget (required)")
	_ = c.MarkFlagRequired("type")
	c.Flags().StringVar(&file, "file", "", "file key (required)")
	_ = c.MarkFlagRequired("file")
	return c
}

func apiLogsCmd() *cobra.Command {
	var name string
	var tail int
	c := &cobra.Command{
		Use:  "logs",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			data, err := opsFor(paths()).Logs(name, tail)
			if err != nil {
				return apiFail(c, err)
			}
			apiEmit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "ctl", "ctl|server|web")
	c.Flags().IntVar(&tail, "tail", 200, "last N lines")
	return c
}
