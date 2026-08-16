package repo_test

import (
	"testing"

	"github.com/halkn/trepo/internal/repo"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		arg         string
		defaultHost string
		want        repo.Source
	}{
		{
			name:        "shorthand uses the default host and gets a url built for it",
			arg:         "owner/repo",
			defaultHost: "github.com",
			want: repo.Source{
				Repo:     repo.Repo{Host: "github.com", Owner: "owner", Name: "repo"},
				CloneURL: "https://github.com/owner/repo",
			},
		},
		{
			name:        "shorthand honours a different default host",
			arg:         "owner/repo",
			defaultHost: "gitlab.com",
			want: repo.Source{
				Repo:     repo.Repo{Host: "gitlab.com", Owner: "owner", Name: "repo"},
				CloneURL: "https://gitlab.com/owner/repo",
			},
		},
		{
			name: "https url is kept verbatim",
			arg:  "https://github.com/owner/repo.git",
			want: repo.Source{
				Repo:     repo.Repo{Host: "github.com", Owner: "owner", Name: "repo"},
				CloneURL: "https://github.com/owner/repo.git",
			},
		},
		{
			name: "trailing slash is trimmed from the path but not the url",
			arg:  "https://github.com/owner/repo/",
			want: repo.Source{
				Repo:     repo.Repo{Host: "github.com", Owner: "owner", Name: "repo"},
				CloneURL: "https://github.com/owner/repo/",
			},
		},
		{
			name: "scp style url is kept verbatim",
			arg:  "git@github.com:owner/repo.git",
			want: repo.Source{
				Repo:     repo.Repo{Host: "github.com", Owner: "owner", Name: "repo"},
				CloneURL: "git@github.com:owner/repo.git",
			},
		},
		{
			name: "ssh url with a port drops the port from the host only",
			arg:  "ssh://git@github.com:2222/owner/repo.git",
			want: repo.Source{
				Repo:     repo.Repo{Host: "github.com", Owner: "owner", Name: "repo"},
				CloneURL: "ssh://git@github.com:2222/owner/repo.git",
			},
		},
		{
			name: "azure https drops the _git segment",
			arg:  "https://dev.azure.com/org/proj/_git/repo",
			want: repo.Source{
				Repo:     repo.Repo{Host: "dev.azure.com", Owner: "org/proj", Name: "repo"},
				CloneURL: "https://dev.azure.com/org/proj/_git/repo",
			},
		},
		{
			name: "azure https with userinfo resolves to the same place",
			arg:  "https://org@dev.azure.com/org/proj/_git/repo",
			want: repo.Source{
				Repo:     repo.Repo{Host: "dev.azure.com", Owner: "org/proj", Name: "repo"},
				CloneURL: "https://org@dev.azure.com/org/proj/_git/repo",
			},
		},
		{
			name: "azure ssh resolves to the same place as its https form",
			arg:  "git@ssh.dev.azure.com:v3/org/proj/repo",
			want: repo.Source{
				Repo:     repo.Repo{Host: "dev.azure.com", Owner: "org/proj", Name: "repo"},
				CloneURL: "git@ssh.dev.azure.com:v3/org/proj/repo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := tt.defaultHost
			if host == "" {
				host = "github.com"
			}
			got, err := repo.Parse(tt.arg, host)
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", tt.arg, err)
			}
			if got != tt.want {
				t.Errorf("Parse(%q)\n got: %+v\nwant: %+v", tt.arg, got, tt.want)
			}
		})
	}
}

// Anything that names neither a host nor a shorthand must be refused rather
// than turned into some path under the root: guessing here would scatter
// checkouts in places the user never asked for.
func TestParseRejects(t *testing.T) {
	for _, arg := range []string{
		"",
		"   ",
		"notaurl",
		"owner/proj/repo",
		"owner/",
		"/repo",
		"https://github.com/owner",
		"https://github.com/",
	} {
		t.Run(arg, func(t *testing.T) {
			got, err := repo.Parse(arg, "github.com")
			if err == nil {
				t.Errorf("Parse(%q) = %+v, want an error", arg, got)
			}
		})
	}
}

func TestRepoSlug(t *testing.T) {
	r := repo.Repo{Host: "dev.azure.com", Owner: "org/proj", Name: "repo"}
	if got, want := r.Slug(), "org/proj/repo"; got != want {
		t.Errorf("Slug() = %q, want %q", got, want)
	}
}

func TestRepoRelPath(t *testing.T) {
	r := repo.Repo{Host: "github.com", Owner: "owner", Name: "repo"}
	if got, want := r.RelPath(), "github.com/owner/repo"; got != want {
		t.Errorf("RelPath() = %q, want %q", got, want)
	}
}
