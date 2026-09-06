package cmd

import (
	"github.com/spf13/cobra"

	"utsusemi/ctl/internal/core"
	"utsusemi/ctl/internal/srv"
)

func newServerCmd() *cobra.Command {
	c := &cobra.Command{Use: "server", Short: "frida-server lifecycle"}
	c.AddCommand(serverRunCmd("start"), serverRunCmd("stop"), serverRunCmd("restart"), serverStatusCmd(), serverSetCmd())
	return c
}

func serverRunCmd(action string) *cobra.Command {
	return &cobra.Command{
		Use: action,
		RunE: func(c *cobra.Command, _ []string) error {
			m := srv.New(paths())
			var (
				st  srv.Status
				err error
			)
			switch action {
			case "start":
				st, err = m.Start()
			case "stop":
				st, err = m.Stop()
			case "restart":
				if _, err = m.Stop(); err == nil {
					st, err = m.Start()
				}
			}
			if err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: st})
			return nil
		},
	}
}

func serverStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use: "status",
		RunE: func(c *cobra.Command, _ []string) error {
			emit(c, Envelope{OK: true, Data: srv.New(paths()).Status()})
			return nil
		},
	}
}

func serverSetCmd() *cobra.Command {
	var active, autostart, crash string
	c := &cobra.Command{
		Use:  "set",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := core.LoadSettings(paths())
			if err != nil {
				return fail(c, err)
			}
			if c.Flags().Changed("active") {
				s.Server.Active = active
			}
			if c.Flags().Changed("autostart") {
				s.Server.Autostart = autostart == "true" || autostart == "1"
			}
			if c.Flags().Changed("restart-on-crash") {
				s.Server.RestartOnCrash = crash == "true" || crash == "1"
			}
			if err := core.SaveSettings(paths(), s); err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: s.Server})
			return nil
		},
	}
	c.Flags().StringVar(&active, "active", "", "active server file key")
	c.Flags().StringVar(&autostart, "autostart", "", "true|false")
	c.Flags().StringVar(&crash, "restart-on-crash", "", "true|false")
	return c
}
