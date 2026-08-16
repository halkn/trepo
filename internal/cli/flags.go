package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/halkn/trepo/internal/checkout"
)

// parseFlags splits long options from positional arguments.
//
// Hand-rolled rather than the flag package because a query is a list of free
// words that may follow the options, which flag stops parsing at. valued names
// the options that take a value; every other option is a boolean and reads as
// "true".
func parseFlags(args []string, valued map[string]bool) (map[string]string, []string) {
	flags := map[string]string{}
	var rest []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "--") {
			rest = append(rest, arg)
			continue
		}

		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		switch {
		case hasValue:
			flags[name] = value
		case valued[name] && i+1 < len(args):
			i++
			flags[name] = args[i]
		default:
			flags[name] = "true"
		}
	}
	return flags, rest
}

// confirmer asks the user about a removal that could lose work.
//
// The question goes to /dev/tty rather than to stdin: by the time it is asked,
// stdin has usually been handed to fzf and is at EOF, and reading it would
// turn every interactive removal into a refusal. A machine with no terminal
// gets no confirmer at all, so those removals are refused with an explanation
// instead of being assumed.
func (a *app) confirmer() func(checkout.Checkout, checkout.Verdict) bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	reader := bufio.NewReader(tty)

	return func(c checkout.Checkout, v checkout.Verdict) bool {
		fmt.Fprintf(tty, "%s\n", c.Path)
		for _, reason := range v.Confirm {
			fmt.Fprintf(tty, "  - it %s\n", reason)
		}
		if v.BaseUnknown {
			fmt.Fprintln(tty, "  - there is no integration branch to compare against,"+
				" so whether its work is merged is unknown")
		}
		fmt.Fprint(tty, "remove it? [y/N] ")

		answer, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}
