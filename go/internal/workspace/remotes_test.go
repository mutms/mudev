package workspace

import (
	"reflect"
	"testing"
)

func TestOrderRemotes(t *testing.T) {
	cases := []struct {
		name    string
		remotes map[string]string
		want    []string
	}{
		{
			// Every remote is fetched, origin first and the rest alphabetically.
			name: "origin leads",
			remotes: map[string]string{
				"origin":       "git@github.com:mutms/patches.git",
				"moodle":       "https://github.com/moodle/moodle.git",
				"someothergit": "https://example.org/x.git",
			},
			want: []string{"origin", "moodle", "someothergit"},
		},
		{
			name:    "only origin",
			remotes: map[string]string{"origin": "https://github.com/mutms/moodle-tool_mulib.git"},
			want:    []string{"origin"},
		},
		{
			name:    "no origin",
			remotes: map[string]string{"zeta": "z", "alpha": "a"},
			want:    []string{"alpha", "zeta"},
		},
		{
			name:    "no remotes at all",
			remotes: map[string]string{},
			want:    []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := orderRemotes(c.remotes)

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("orderRemotes = %v, want %v", got, c.want)
			}
		})
	}
}

func TestOrderRemotesIsStable(t *testing.T) {
	remotes := map[string]string{"origin": "o", "zeta": "z", "alpha": "a", "mirror": "b"}

	// Go randomises map iteration, so the same inputs must still produce the
	// same order every time or a run is not reproducible.
	first := orderRemotes(remotes)

	for range 20 {
		if got := orderRemotes(remotes); !reflect.DeepEqual(got, first) {
			t.Fatalf("orderRemotes is not stable: %v then %v", first, got)
		}
	}

	if !reflect.DeepEqual(first, []string{"origin", "alpha", "mirror", "zeta"}) {
		t.Errorf("orderRemotes = %v", first)
	}
}
