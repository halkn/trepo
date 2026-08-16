// Command trepo manages repositories and their worktrees as one set of
// checkouts.
package main

import (
	"os"

	"github.com/halkn/trepo/internal/cli"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, cli.Options{Cwd: cwd}))
}
