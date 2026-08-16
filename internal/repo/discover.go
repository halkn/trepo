package repo

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover lists the repositories below root, sorted by their location.
//
// Only the two layouts trepo itself creates are searched: host/owner/repo and
// the host/org/project/repo that Azure DevOps needs. Walking to an arbitrary
// depth instead would pull in every vendored dependency that ships a .git,
// which is never what the user meant by "my repositories".
func Discover(root string) ([]Repo, error) {
	var found []Repo

	hosts, err := subdirs(root)
	if err != nil {
		return nil, err
	}
	for _, host := range hosts {
		owners, err := subdirs(filepath.Join(root, host))
		if err != nil {
			return nil, err
		}
		for _, owner := range owners {
			names, err := subdirs(filepath.Join(root, host, owner))
			if err != nil {
				return nil, err
			}
			for _, name := range names {
				dir := filepath.Join(root, host, owner, name)
				if isRepo(dir) {
					found = append(found, Repo{Host: host, Owner: owner, Name: name, Root: dir})
					continue
				}
				// Not a checkout, so it may be an Azure DevOps project holding
				// repositories one level further down.
				deeper, err := subdirs(dir)
				if err != nil {
					return nil, err
				}
				for _, leaf := range deeper {
					leafDir := filepath.Join(dir, leaf)
					if isRepo(leafDir) {
						found = append(found, Repo{
							Host:  host,
							Owner: owner + "/" + name,
							Name:  leaf,
							Root:  leafDir,
						})
					}
				}
			}
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].Root < found[j].Root })
	return found, nil
}

// resolveSymlinks follows what exists of a path, leaving the rest as given.
func resolveSymlinks(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	dir, leaf := filepath.Split(filepath.Clean(p))
	if dir == "" || filepath.Clean(dir) == filepath.Clean(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(resolveSymlinks(filepath.Clean(dir)), leaf)
}

func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// subdirs lists the directory names in dir. A missing directory is an empty
// result rather than an error: an unused root is a normal state, not a fault.
func subdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		} else if e.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(dir, e.Name())); err == nil && info.IsDir() {
				names = append(names, e.Name())
			}
		}
	}
	return names, nil
}

// FromRoot describes the repository checked out at root.
//
// When root sits below the trepo root its location carries the host and owner,
// which is how a repository found by path gets the same identity as one found
// by discovery. Anywhere else only the directory name is known, and claiming
// more than that would put a made-up host in the output.
func FromRoot(root, trepoRoot string) Repo {
	root = filepath.Clean(root)
	r := Repo{Name: filepath.Base(root), Root: root}

	// git reports paths with their symlinks resolved and a configured root
	// usually keeps them, so the two are only comparable once both are.
	rel, err := filepath.Rel(resolveSymlinks(trepoRoot), resolveSymlinks(root))
	if err != nil || strings.HasPrefix(rel, "..") {
		return r
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 3 {
		return r
	}
	r.Host = parts[0]
	r.Owner = strings.Join(parts[1:len(parts)-1], "/")
	r.Name = parts[len(parts)-1]
	return r
}
