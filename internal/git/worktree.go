package git

import (
	"bytes"
	"fmt"
	"strings"
)

// zeroOID is what git reports for HEAD when the branch has no commit yet.
const zeroOID = "0000000000000000000000000000000000000000"

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path     string
	Head     string
	Branch   string // without refs/heads/; empty when detached or unborn
	Main     bool   // the checkout that git lists first
	Bare     bool
	Detached bool
	Locked   bool
	LockedBy string
	Prunable bool
	Unborn   bool // branch exists in name only, because HEAD has no commit
}

// ListWorktrees reports the checkouts of the repository at dir, main first.
func ListWorktrees(r Runner, dir string) ([]Worktree, error) {
	out, err := r.Run(dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return ParseWorktreeList(out)
}

// ParseWorktreeList reads the NUL-separated porcelain form.
//
// The NUL form rather than the line form because paths and branch names are
// emitted raw: a newline in either splits a line-based record in two and
// shifts every field after it.
func ParseWorktreeList(out []byte) ([]Worktree, error) {
	var (
		list    []Worktree
		current Worktree
		open    bool
	)

	flush := func() {
		if open {
			list = append(list, current)
			current, open = Worktree{}, false
		}
	}

	for _, attr := range bytes.Split(out, []byte{0}) {
		if len(attr) == 0 {
			flush() // the empty attribute is the record separator
			continue
		}

		key, value, _ := strings.Cut(string(attr), " ")
		switch key {
		case "worktree":
			flush()
			current = Worktree{Path: value, Main: len(list) == 0}
			open = true
		case "HEAD":
			current.Head = value
			current.Unborn = value == zeroOID
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			current.Bare = true
		case "detached":
			current.Detached = true
		case "locked":
			current.Locked = true
			current.LockedBy = value
		case "prunable":
			current.Prunable = true
		default:
			return nil, fmt.Errorf("unknown worktree attribute %q", key)
		}
	}
	flush()

	// An unborn branch is a name with nothing behind it; reporting it as a real
	// branch would make callers look for a commit that is not there.
	for i := range list {
		if list[i].Unborn {
			list[i].Branch = ""
		}
	}
	return list, nil
}
