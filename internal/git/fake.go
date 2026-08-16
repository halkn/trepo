package git

import (
	"fmt"
	"strings"
)

// Response is what a Fake returns for one command.
type Response struct {
	Stdout string
	Err    error
}

// Call is one recorded invocation.
type Call struct {
	Dir  string
	Args []string
}

// Fake is a Runner that replays scripted responses, keyed by the command's
// arguments joined with spaces. It exists so that command construction can be
// tested without a repository; anything that depends on real git behaviour
// belongs in an integration test against a gittest fixture instead.
type Fake struct {
	Responses map[string]Response
	Calls     []Call
}

func (f *Fake) Run(dir string, args ...string) ([]byte, error) {
	f.Calls = append(f.Calls, Call{Dir: dir, Args: args})

	key := strings.Join(args, " ")
	resp, ok := f.Responses[key]
	if !ok {
		return nil, fmt.Errorf("git.Fake: no response scripted for %q", key)
	}
	if resp.Err != nil {
		return []byte(resp.Stdout), resp.Err
	}
	return []byte(resp.Stdout), nil
}
