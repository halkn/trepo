package cli

import (
	"errors"
	"fmt"
	"strings"
)

// spec says which options a command takes: the keys are the accepted names and
// the value reports whether the option carries a value of its own.
type spec map[string]bool

// errHelp reports that the arguments asked for the usage text rather than for
// the command itself.
var errHelp = errors.New("help requested")

// parseFlags splits options from positional arguments.
//
// Hand-rolled rather than the flag package because a query is a list of free
// words that may follow the options, which flag stops parsing at.
//
// Anything that looks like an option and is outside the spec is an error rather
// than an unused entry in the map or a query term. The same parser feeds rm,
// where reading "--dryrun" as nothing would turn a rehearsal into a deletion,
// and reading "-f" as a query word would search for it instead of forcing. A
// query that genuinely starts with a dash comes after "--".
func parseFlags(args []string, accepted spec) (map[string]string, []string, error) {
	flags := map[string]string{}
	var rest []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if arg == "-h" || arg == "--help" {
			return nil, nil, errHelp
		}
		if !strings.HasPrefix(arg, "--") {
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return nil, nil, fmt.Errorf("unknown option %s", arg)
			}
			rest = append(rest, arg)
			continue
		}

		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		takesValue, known := accepted[name]
		if !known {
			return nil, nil, fmt.Errorf("unknown option --%s", name)
		}

		switch {
		case hasValue && !takesValue:
			// Reading the value as truth would make --dry-run=0 delete, since
			// every reader of a switch compares against "true".
			return nil, nil, fmt.Errorf("--%s takes no value", name)
		case hasValue:
			flags[name] = value
		case !takesValue:
			flags[name] = "true"
		case i+1 < len(args):
			i++
			flags[name] = args[i]
		default:
			return nil, nil, fmt.Errorf("--%s needs a value", name)
		}
	}
	return flags, rest, nil
}

// parse is parseFlags with the two ways it can end a run already handled: a
// request for the usage text, which every command answers on stdout, and a bad
// option, which no command can go on from. ok reports whether the caller may
// continue; when it is false, code is what the process exits with.
func (a *app) parse(args []string, accepted spec) (flags map[string]string, rest []string, code int, ok bool) {
	flags, rest, err := parseFlags(args, accepted)
	switch {
	case errors.Is(err, errHelp):
		fmt.Fprintln(a.stdout, usage)
		return nil, nil, ExitOK, false
	case err != nil:
		return nil, nil, fail(a.stderr, err), false
	}
	return flags, rest, ExitOK, true
}
