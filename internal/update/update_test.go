package update

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.1.2", "v0.1.1", true},  // patch bump
		{"v0.2.0", "v0.1.9", true},  // minor bump beats higher patch
		{"v1.0.0", "v0.9.9", true},  // major bump
		{"v0.1.1", "v0.1.1", false}, // same
		{"v0.1.1", "v0.1.2", false}, // older -> no downgrade prompt
		{"v0.1.0", "v0.1.10", false},// 10 > 0, numeric not lexical
		{"v0.1.10", "v0.1.2", true}, // 10 > 2 numeric
		{"v0.1.1", "dev", false},    // unparseable current -> no prompt
		{"latest", "v0.1.1", false}, // unparseable latest -> no prompt
		{"", "v0.1.1", false},       // empty
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}
