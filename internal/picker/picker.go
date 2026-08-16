// Package picker offers a choice through fzf when it is installed.
//
// fzf is optional on purpose: trepo has to stay usable on a machine that does
// not have it, so every command that picks also has a non-interactive answer.
package picker

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// ErrCancelled is returned when the user dismissed the picker.
var ErrCancelled = errors.New("cancelled")

// ErrUnavailable is returned when fzf is not installed.
var ErrUnavailable = errors.New("fzf is not installed")

// Row is one choice: Display is what the user reads, Key is what the caller
// gets back.
type Row struct {
	Display []string
	Key     string
}

// Picker runs fzf.
type Picker struct {
	Prompt string
	Header string
	Multi  bool

	// Preview is a shell command shown alongside the list. "{}" in it is
	// replaced by the key of the highlighted row.
	Preview string
}

// Available reports whether a picker can be shown at all.
func Available() bool {
	_, err := exec.LookPath("fzf")
	return err == nil
}

// Pick shows the rows and returns the keys of the chosen ones.
//
// The whole row, key included, is handed to fzf and the answer is split here.
// fzf's own --accept-nth would be tidier but only exists from 0.57, and the
// versions shipped by long-term distributions are older than that.
func (p Picker) Pick(rows []Row) ([]string, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	if !Available() {
		return nil, ErrUnavailable
	}

	fields := len(rows[0].Display)
	args := []string{
		"--delimiter", "\t",
		"--with-nth", withNth(fields),
		"--prompt", p.Prompt,
	}
	if p.Header != "" {
		args = append(args, "--header", p.Header)
	}
	if p.Multi {
		args = append(args, "--multi")
	}
	if p.Preview != "" {
		args = append(args, "--preview", strings.ReplaceAll(p.Preview, "{}", keyPlaceholder(fields)))
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(render(rows))
	// fzf draws on the terminal, so its own stderr must reach it even when
	// trepo's stdout is being captured by a $(...) substitution.
	cmd.Stderr = os.Stderr
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// 1 is "no match", 130 is an interrupt; both mean nothing was
			// chosen, which is an answer rather than a failure.
			if code := exitErr.ExitCode(); code == 1 || code == 130 {
				return nil, ErrCancelled
			}
		}
		return nil, err
	}

	var keys []string
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		keys = append(keys, parts[len(parts)-1])
	}
	if len(keys) == 0 {
		return nil, ErrCancelled
	}
	return keys, nil
}

func render(rows []Row) string {
	var b strings.Builder
	for _, r := range rows {
		for _, col := range r.Display {
			b.WriteString(col)
			b.WriteByte('\t')
		}
		b.WriteString(r.Key)
		b.WriteByte('\n')
	}
	return b.String()
}

// withNth hides the key column, which is the last field.
func withNth(displayFields int) string {
	var parts []string
	for i := 1; i <= displayFields; i++ {
		parts = append(parts, itoa(i))
	}
	return strings.Join(parts, ",")
}

func keyPlaceholder(displayFields int) string { return "{" + itoa(displayFields+1) + "}" }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
