// Command trepo manages repositories and their worktrees as one set of
// checkouts.
package main

import (
	"fmt"
	"os"

	"github.com/halkn/trepo/internal/cli"
)

func main() {
	// The working directory is what marks the checkout you are standing in,
	// and that mark is what stops trepo removing it. Carrying on without one
	// would drop a guard rather than report a problem.
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "trepo: cannot determine the working directory:", err)
		os.Exit(cli.ExitError)
	}
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, cli.Options{Cwd: cwd}))
}
