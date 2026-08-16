// Package git runs the git binary and parses what it prints.
//
// Everything above this package deals in typed results rather than in argument
// slices, so the exact invocations stay in one place and can be tested without
// a repository on disk.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes a git command in a directory and returns its stdout as git
// wrote it. Callers that need a trimmed string use Output; callers that only
// care whether git succeeded use OK.
type Runner interface {
	Run(dir string, args ...string) ([]byte, error)
}

// Error is a git command that exited non-zero.
type Error struct {
	Args     []string
	Stderr   string
	ExitCode int
	Err      error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("git %s: exit %d", strings.Join(e.Args, " "), e.ExitCode)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// Exec is the Runner that actually spawns git.
type Exec struct {
	// Env replaces the process environment when set. Tests use it to keep the
	// developer's own git configuration out of the fixtures.
	Env []string
}

func (e Exec) Run(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = e.Env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &Error{
			Args:     args,
			Stderr:   strings.TrimSpace(stderr.String()),
			ExitCode: cmd.ProcessState.ExitCode(),
			Err:      err,
		}
	}
	return stdout.Bytes(), nil
}

// Output runs a command and returns its stdout with surrounding whitespace
// removed, which is what almost every git query wants.
func Output(r Runner, dir string, args ...string) (string, error) {
	out, err := r.Run(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// OK reports whether a command succeeded. Use it for questions git answers
// with its exit status, such as whether a ref exists.
func OK(r Runner, dir string, args ...string) bool {
	_, err := r.Run(dir, args...)
	return err == nil
}
