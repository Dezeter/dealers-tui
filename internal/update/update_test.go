package update

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"0.1.0", "0.1.1", true},
		{"v1.2.3", "v1.2.4", true},
		{"v0.1.0", "v0.1.0", false},     // equal
		{"v0.2.0", "v0.1.0", false},     // older latest
		{"v1.0.0", "v1.0", false},       // v1.0 == v1.0.0
		{"v1", "v1.0.1", true},          // short current
		{"dev", "v0.1.0", false},        // unstamped dev build → no nag
		{"v0.1.0", "nightly", false},    // unparseable tag → fail safe
		{"", "v1.0.0", false},           // empty current
		{"v0.1.0", "v0.2.0-rc1", true},  // pre-release suffix stripped
		{"v0.2.0-rc1", "v0.2.0", false}, // same base version, not newer
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestLatestRejectsBadRepo(t *testing.T) {
	for _, repo := range []string{"", "noslash", "a/b/c", "/"} {
		if _, err := Latest(t.Context(), repo); err == nil {
			t.Errorf("Latest(%q) expected error, got nil", repo)
		}
	}
}
