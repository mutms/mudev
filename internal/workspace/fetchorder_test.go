package workspace

import (
	"reflect"
	"testing"
)

func TestOrderRemotes(t *testing.T) {
	dev := map[string]string{
		"origin":       "git@github.com:mutms/patches.git",
		"backup":       "git@forgejo.lan:mutms/patches.git",
		"upstream":     "https://github.com/moodle/moodle.git",
		"someothergit": "https://example.org/x.git",
	}

	cases := []struct {
		name    string
		remotes map[string]string
		order   []string
		want    []string
	}{
		{
			// The point of the setting: ask the near mirror before github, so
			// origin has only the difference left to send.
			name:    "recipe order leads",
			remotes: dev,
			order:   []string{"backup", "origin"},
			want:    []string{"backup", "origin", "someothergit", "upstream"},
		},
		{
			// Unlisted remotes are still fetched — all remotes, always — with
			// origin at the head of the remainder.
			name:    "no order given",
			remotes: dev,
			order:   nil,
			want:    []string{"origin", "backup", "someothergit", "upstream"},
		},
		{
			// A catalogue plugin has only origin, so a workspace-wide order
			// naming mirrors it does not have costs it nothing.
			name:    "names the checkout does not have are skipped",
			remotes: map[string]string{"origin": "https://github.com/mutms/moodle-tool_mulib.git"},
			order:   []string{"backup", "origin"},
			want:    []string{"origin"},
		},
		{
			name:    "a repeated name is fetched once",
			remotes: map[string]string{"origin": "o", "backup": "b"},
			order:   []string{"backup", "backup", "origin"},
			want:    []string{"backup", "origin"},
		},
		{
			name:    "no remotes at all",
			remotes: map[string]string{},
			order:   []string{"backup"},
			want:    []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := orderRemotes(c.remotes, c.order)

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("orderRemotes = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOrderRemotesIsStable(t *testing.T) {
	remotes := map[string]string{"origin": "o", "zeta": "z", "alpha": "a", "backup": "b"}

	// Go randomises map iteration, so the same inputs must still produce the
	// same order every time or a run is not reproducible.
	first := orderRemotes(remotes, []string{"backup"})

	for range 20 {
		if got := orderRemotes(remotes, []string{"backup"}); !reflect.DeepEqual(got, first) {
			t.Fatalf("orderRemotes is not stable: %v then %v", first, got)
		}
	}

	if !reflect.DeepEqual(first, []string{"backup", "origin", "alpha", "zeta"}) {
		t.Errorf("orderRemotes = %v", first)
	}
}
