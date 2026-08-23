package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/halkn/trepo/internal/repo"
)

// Cache remembers what a provider last reported.
//
// It holds only what could be cloned, never what is here: nothing reads it to
// decide whether a repository exists locally, which is still answered by the
// filesystem alone. That is what keeps it a cache rather than a second record
// of trepo's state — losing the whole directory costs one network call.
type Cache struct {
	Dir string
	TTL time.Duration
}

type cached struct {
	Written time.Time     `json:"written"`
	Sources []repo.Source `json:"sources"`
}

// Load returns what the provider last reported, if that was recent enough.
//
// Anything unreadable, unparsable or old reads as a miss. A cache that cannot
// answer is not a failure — the caller asks the provider instead — so no error
// is reported for one.
func (c Cache) Load(provider string) ([]repo.Source, bool) {
	raw, err := os.ReadFile(c.path(provider))
	if err != nil {
		return nil, false
	}
	var entry cached
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, false
	}
	if time.Since(entry.Written) > c.TTL {
		return nil, false
	}
	return entry.Sources, true
}

// Save records what the provider reported. The write is best effort in the
// caller's hands: the answer is already correct without it.
func (c Cache) Save(provider string, sources []repo.Source) error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(cached{Written: time.Now(), Sources: sources})
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(provider), raw, 0o644)
}

func (c Cache) path(provider string) string {
	return filepath.Join(c.Dir, provider+".json")
}

// DefaultDir is where a cache belongs: reconstructible from the network, so it
// goes to the cache directory rather than to the data directory a worktree's
// uncommitted work would need.
func DefaultDir(home string) string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "trepo", "remote")
	}
	return filepath.Join(home, ".cache", "trepo", "remote")
}
