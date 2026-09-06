package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"utsusemi/ctl/internal/binmgr"
	"utsusemi/ctl/internal/core"
	"utsusemi/ctl/internal/dl"
)

func newBinCmd() *cobra.Command {
	c := &cobra.Command{Use: "bin", Short: "frida binary management"}
	c.AddCommand(
		binListCmd(), binSourcesCmd(), binDownloadCmd(),
		binImportCmd(), binRemoveCmd(), binCleanupCmd(), binUseCmd(),
	)
	return c
}

func binListCmd() *cobra.Command {
	var typ string
	c := &cobra.Command{
		Use:  "list",
		RunE: func(c *cobra.Command, _ []string) error {
			m, err := binmgr.New(paths())
			if err != nil {
				return fail(c, err)
			}
			var data any
			if typ == "" {
				data = map[string]any{"servers": m.List("server"), "gadgets": m.List("gadget")}
			} else {
				data = m.List(typ)
			}
			emit(c, Envelope{OK: true, Data: data})
			return nil
		},
	}
	c.Flags().StringVar(&typ, "type", "", "server|gadget (空=两类合并)")
	return c
}

func binSourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:  "sources",
		RunE: func(c *cobra.Command, _ []string) error {
			m, err := binmgr.New(paths())
			if err != nil {
				return fail(c, err)
			}
			cl := dl.NewClient(m.S.Download)
			out := []map[string]any{}
			for _, v := range dl.Variants {
				vs, err := cl.ListVersions(c.Context(), v)
				if err != nil {
					return fail(c, fmt.Errorf("%s: %w", v, err))
				}
				out = append(out, map[string]any{"variant": v, "versions": vs})
			}
			emit(c, Envelope{OK: true, Data: out})
			return nil
		},
	}
}

func binDownloadCmd() *cobra.Command {
	var variant, typ, version string
	c := &cobra.Command{
		Use:  "download",
		RunE: func(c *cobra.Command, _ []string) error {
			if typ == "" {
				typ = "server"
			}
			m, err := binmgr.New(paths())
			if err != nil {
				return fail(c, err)
			}
			cl := dl.NewClient(m.S.Download)
			if version == "" {
				vs, err := cl.ListVersions(c.Context(), variant)
				if err != nil {
					return fail(c, err)
				}
				if len(vs) == 0 {
					return fail(c, fmt.Errorf("no releases with assets for %s", variant))
				}
				version = vs[0]
			}
			a, err := cl.FindAsset(c.Context(), variant, version, typ, core.DeviceArch())
			if err != nil {
				return fail(c, err)
			}
			b, err := dl.Install(c.Context(), m.P, m.S, m.M, variant, a)
			if err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: b})
			return nil
		},
	}
	c.Flags().StringVar(&variant, "variant", "official", "official|florida|undetected")
	c.Flags().StringVar(&typ, "type", "", "server|gadget")
	c.Flags().StringVar(&version, "version", "", "default: latest with assets")
	return c
}

func binImportCmd() *cobra.Command {
	var typ, file, version string
	c := &cobra.Command{
		Use:  "import",
		RunE: func(c *cobra.Command, _ []string) error {
			m, err := binmgr.New(paths())
			if err != nil {
				return fail(c, err)
			}
			b, err := m.Import(typ, file, version)
			if err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: b})
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

func binRemoveCmd() *cobra.Command {
	var typ, file string
	c := &cobra.Command{
		Use:  "remove",
		RunE: func(c *cobra.Command, _ []string) error {
			m, err := binmgr.New(paths())
			if err != nil {
				return fail(c, err)
			}
			if err := m.Remove(typ, file); err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: map[string]string{"removed": file}})
			return nil
		},
	}
	c.Flags().StringVar(&typ, "type", "", "server|gadget (required)")
	_ = c.MarkFlagRequired("type")
	c.Flags().StringVar(&file, "file", "", "file key (required)")
	_ = c.MarkFlagRequired("file")
	return c
}

func binCleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use:  "cleanup",
		RunE: func(c *cobra.Command, _ []string) error {
			m, err := binmgr.New(paths())
			if err != nil {
				return fail(c, err)
			}
			removed, err := m.Cleanup()
			if err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: map[string][]string{"removed": removed}})
			return nil
		},
	}
}

func binUseCmd() *cobra.Command {
	var typ, file string
	c := &cobra.Command{
		Use:  "use",
		RunE: func(c *cobra.Command, _ []string) error {
			m, err := binmgr.New(paths())
			if err != nil {
				return fail(c, err)
			}
			if err := m.SetActive(typ, file); err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: map[string]string{"active": file}})
			return nil
		},
	}
	c.Flags().StringVar(&typ, "type", "", "server|gadget (required)")
	_ = c.MarkFlagRequired("type")
	c.Flags().StringVar(&file, "file", "", "file key (required)")
	_ = c.MarkFlagRequired("file")
	return c
}
