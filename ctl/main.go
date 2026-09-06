package main

import (
	"os"

	"utsusemi/ctl/cmd"
)

func main() { os.Exit(cmd.Execute(os.Args[1:], os.Stdout, os.Stderr)) }
