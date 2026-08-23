package remote_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/halkn/trepo/internal/remote"
	"github.com/halkn/trepo/internal/repo"
)

func TestCacheRoundTrip(t *testing.T) {
	c := remote.Cache{Dir: t.TempDir(), TTL: time.Hour}
	want := []repo.Source{src("halkn/api"), src("git@github.com:halkn/web.git")}

	if err := c.Save("github", want); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Load("github")
	if !ok {
		t.Fatal("Load() found nothing just after Save()")
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sources, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Repo != want[i].Repo || got[i].CloneURL != want[i].CloneURL {
			t.Errorf("source %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A stale answer is worse than no answer here: the point of the list is what
// could be cloned now, and a repository created since the last run is exactly
// what the user is looking for.
func TestCacheExpires(t *testing.T) {
	c := remote.Cache{Dir: t.TempDir(), TTL: time.Hour}
	if err := c.Save("github", []repo.Source{src("halkn/api")}); err != nil {
		t.Fatal(err)
	}

	expired := remote.Cache{Dir: c.Dir, TTL: -time.Second}
	if _, ok := expired.Load("github"); ok {
		t.Error("Load() served an entry past its age")
	}
}

func TestCacheMissIsNotAnError(t *testing.T) {
	c := remote.Cache{Dir: filepath.Join(t.TempDir(), "not-created-yet"), TTL: time.Hour}
	if _, ok := c.Load("github"); ok {
		t.Error("Load() found something in an empty cache")
	}
}

// The cache is a convenience, not a record. If it cannot be written the answer
// is still correct, so a read-only cache directory must not fail the command.
func TestCacheLoadIgnoresRubbish(t *testing.T) {
	dir := t.TempDir()
	c := remote.Cache{Dir: dir, TTL: time.Hour}
	if err := c.Save("github", []repo.Source{src("halkn/api")}); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "github.json"), "not json"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Load("github"); ok {
		t.Error("Load() accepted a file it could not parse")
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
