package repo_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/halkn/trepo/internal/repo"
)

// mkRepo fakes a checkout: discovery only looks for .git, so a directory is
// enough and keeps the test independent of git itself.
func mkRepo(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, rel, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func slugs(rs []repo.Repo) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Host + "/" + r.Slug()
	}
	return out
}

func TestDiscoverFindsBothLayouts(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, "github.com/halkn/trepo")
	mkRepo(t, root, "github.com/halkn/dotfiles")
	mkRepo(t, root, "dev.azure.com/org/proj/service")

	got, err := repo.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"dev.azure.com/org/proj/service",
		"github.com/halkn/dotfiles",
		"github.com/halkn/trepo",
	}
	if !reflect.DeepEqual(slugs(got), want) {
		t.Errorf("Discover()\n got: %v\nwant: %v", slugs(got), want)
	}
}

func TestDiscoverRecordsAbsoluteRoot(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, "github.com/halkn/trepo")

	got, err := repo.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "github.com/halkn/trepo")
	if got[0].Root != want {
		t.Errorf("Root = %q, want %q", got[0].Root, want)
	}
}

// The fixed depths are what keeps vendored checkouts out of the list: a
// node_modules dependency with its own .git is not a repository the user is
// working on.
func TestDiscoverIgnoresNestedRepositories(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, "github.com/halkn/trepo")
	mkRepo(t, root, "github.com/halkn/trepo/node_modules/dep")

	got, err := repo.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("Discover() = %v, want only the top-level repository", slugs(got))
	}
}

func TestDiscoverOnMissingRootIsEmpty(t *testing.T) {
	got, err := repo.Discover(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("Discover() on a missing root failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Discover() = %v, want empty", slugs(got))
	}
}
