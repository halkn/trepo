package remote

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/halkn/trepo/internal/repo"
)

// pageLimit is how many repositories gh is asked for. Its own default is 30,
// and an account truncated at 30 looks exactly like a small one, so the number
// is stated rather than inherited.
const pageLimit = "1000"

// GitHub lists repositories through the gh CLI.
//
// gh rather than the API directly: it already holds the token, refreshes it,
// knows the endpoint of an enterprise host and reports its own failures. The
// alternative is reimplementing all of that in the one place whose breakage is
// hardest to tell from an empty account.
type GitHub struct{ Run Runner }

func (GitHub) Name() string { return "github" }

// List reports the repositories of each owner, or of the authenticated account
// when no owner is named.
func (g GitHub) List(owners []string) ([]repo.Source, error) {
	if len(owners) == 0 {
		owners = []string{""}
	}

	var out []repo.Source
	for _, owner := range owners {
		args := []string{"repo", "list"}
		if owner != "" {
			args = append(args, owner)
		}
		args = append(args, "--limit", pageLimit, "--json", "nameWithOwner,sshUrl")

		stdout, err := g.Run.Run("", args...)
		if err != nil {
			return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
		}

		var reported []struct {
			NameWithOwner string `json:"nameWithOwner"`
			SSHURL        string `json:"sshUrl"`
		}
		if err := json.Unmarshal(stdout, &reported); err != nil {
			return nil, fmt.Errorf("cannot read what gh reported: %w", err)
		}

		for _, r := range reported {
			// The URL gh reports rather than one rebuilt from the name: it
			// carries the protocol and the host, which is what makes the clone
			// work against an enterprise instance.
			s, err := repo.Parse(r.SSHURL, "github.com")
			if err != nil {
				return nil, fmt.Errorf("cannot place %s: %w", r.NameWithOwner, err)
			}
			out = append(out, s)
		}
	}
	return out, nil
}

// Command is the Runner that spawns an external tool.
type Command struct{ Name string }

func (c Command) Run(dir string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(c.Name); err != nil {
		return nil, fmt.Errorf("%s is not installed, so remote repositories cannot be listed", c.Name)
	}

	cmd := exec.Command(c.Name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}
