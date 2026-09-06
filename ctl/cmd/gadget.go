package cmd

import (
	"github.com/spf13/cobra"

	"utsusemi/ctl/internal/gadgetcfg"
)

func newGadgetCmd() *cobra.Command {
	c := &cobra.Command{Use: "gadget", Short: "gadget injection rules"}
	c.AddCommand(gadgetAppsCmd(), gadgetRulesCmd(), gadgetSetCmd(), gadgetApplyCmd())
	return c
}

func gadgetAppsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "apps",
		RunE: func(c *cobra.Command, _ []string) error {
			apps, err := gadgetcfg.ListApps(nil)
			if err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: apps})
			return nil
		},
	}
}

func gadgetRulesCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rules",
		RunE: func(c *cobra.Command, _ []string) error {
			r, err := gadgetcfg.LoadRules(paths())
			if err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: r})
			return nil
		},
	}
}

func gadgetSetCmd() *cobra.Command {
	var app, delay string
	var enable, gating bool
	c := &cobra.Command{
		Use:  "set",
		RunE: func(c *cobra.Command, _ []string) error {
			r, err := gadgetcfg.LoadRules(paths())
			if err != nil {
				return fail(c, err)
			}
			rule := gadgetcfg.Rule{}
			idx := -1
			for i, x := range r.Rules {
				if x.AppName == app {
					rule, idx = x, i
					break
				}
			}
			if c.Flags().Changed("enable") {
				rule.Enabled = enable
			}
			if c.Flags().Changed("delay") {
				v, err := parseInt64(delay)
				if err != nil {
					return fail(c, err)
				}
				rule.StartUpDelayMS = v
			}
			if c.Flags().Changed("child-gating") {
				rule.ChildGatingEnabled = gating
				if rule.ChildGatingMode == "" {
					rule.ChildGatingMode = "freeze"
				}
			}
			rule.AppName = app
			if idx >= 0 {
				r.Rules[idx] = rule
			} else {
				r.Rules = append(r.Rules, rule)
			}
			if err := r.Save(paths()); err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: rule})
			return nil
		},
	}
	c.Flags().StringVar(&app, "app", "", "package name (required)")
	_ = c.MarkFlagRequired("app")
	c.Flags().BoolVar(&enable, "enable", false, "启用该规则")
	c.Flags().StringVar(&delay, "delay", "", "startup delay ms")
	c.Flags().BoolVar(&gating, "child-gating", false, "启用子进程拦截")
	return c
}

func gadgetApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use: "apply",
		RunE: func(c *cobra.Command, _ []string) error {
			if err := gadgetcfg.Apply(paths()); err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: map[string]string{"applied": paths().GadgetConfig()}})
			return nil
		},
	}
}
