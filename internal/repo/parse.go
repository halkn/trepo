// Package repo turns the many spellings of a repository into one location
// under the trepo root, and finds the repositories already there.
package repo

import (
	"fmt"
	"path"
	"strings"
)

// Repo identifies a repository by where it came from. Owner holds everything
// between the host and the repository name, so it is a single segment on
// GitHub and "org/proj" on Azure DevOps.
type Repo struct {
	Host  string
	Owner string
	Name  string
	Root  string // absolute path of the main checkout; empty until discovered
}

// Slug is the repository without its host, which is what identifies it in
// output: the host rarely tells two checkouts apart. A repository found
// outside the trepo root has no known owner, and is named by itself rather
// than by a slug with nothing in front of the slash.
func (r Repo) Slug() string {
	if r.Owner == "" {
		return r.Name
	}
	return r.Owner + "/" + r.Name
}

// RelPath is the location of the repository below the trepo root.
func (r Repo) RelPath() string { return path.Join(r.Host, r.Owner, r.Name) }

// Source is a repository together with the URL to clone it from.
type Source struct {
	Repo     Repo
	CloneURL string
}

// Parse resolves a repository argument: either an "owner/repo" shorthand or a
// clone URL in any of the forms git accepts.
//
// The URL is passed through untouched unless it had to be invented for the
// shorthand. Rebuilding it would discard the protocol and credentials the user
// chose, which are exactly what makes the clone work on their machine.
func Parse(arg, defaultHost string) (Source, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return Source{}, fmt.Errorf("empty repository argument")
	}

	host, p, cloneURL, err := split(arg, defaultHost)
	if err != nil {
		return Source{}, err
	}

	host, p = normalize(host, p)
	owner, name, err := splitOwnerName(p)
	if err != nil {
		return Source{}, fmt.Errorf("cannot derive a repository from %q: %w", arg, err)
	}
	if host == "" {
		return Source{}, fmt.Errorf("cannot derive a host from %q", arg)
	}

	return Source{
		Repo:     Repo{Host: host, Owner: owner, Name: name},
		CloneURL: cloneURL,
	}, nil
}

// split separates the host from the path for each URL shape git understands,
// and reports the URL to clone from.
func split(arg, defaultHost string) (host, p, cloneURL string, err error) {
	switch {
	case !strings.Contains(arg, ":") && strings.Count(arg, "/") == 1:
		// The bare "owner/repo" shorthand: the only case with no URL to reuse.
		return defaultHost, arg, "https://" + defaultHost + "/" + strings.Trim(arg, "/"), nil

	case strings.Contains(arg, "://"):
		rest := arg[strings.Index(arg, "://")+3:]
		host, p, _ = strings.Cut(rest, "/")
		return host, p, arg, nil

	case strings.Contains(arg, ":"):
		// scp-like syntax, e.g. git@github.com:owner/repo.git
		host, p, _ = strings.Cut(arg, ":")
		return host, p, arg, nil
	}

	return "", "", "", fmt.Errorf("cannot derive a host from %q: give owner/repo or a clone URL", arg)
}

// normalize folds the spellings of one repository onto a single host and path.
//
// Azure DevOps needs it twice over: the "_git" segment is an artifact of its
// URL scheme, and its SSH host and "v3" prefix would otherwise place the same
// repository somewhere else than the HTTPS URL does.
func normalize(host, p string) (string, string) {
	host = host[strings.LastIndex(host, "@")+1:]
	if h, _, found := strings.Cut(host, ":"); found {
		host = h // a port belongs to the URL, not to the location on disk
	}

	if host == "ssh.dev.azure.com" || host == "vs-ssh.visualstudio.com" {
		host = "dev.azure.com"
		p = strings.TrimPrefix(p, "v3/")
	}
	p = strings.ReplaceAll(p, "/_git/", "/")

	p = strings.Trim(p, "/")
	p = strings.TrimSuffix(p, ".git")
	return host, p
}

// splitOwnerName takes the last segment as the repository name and everything
// before it as the owner.
func splitOwnerName(p string) (owner, name string, err error) {
	owner, name, found := strings.Cut(p, "/")
	if !found || owner == "" || name == "" {
		return "", "", fmt.Errorf("path %q is not owner/name", p)
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		owner, name = owner+"/"+name[:i], name[i+1:]
	}
	if name == "" || strings.Contains(owner, "//") {
		return "", "", fmt.Errorf("path %q is not owner/name", p)
	}
	return owner, name, nil
}
