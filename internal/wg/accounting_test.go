package wg

import (
	"testing"
	"time"
)

func TestAccumulate(t *testing.T) {
	cl := &ClientLimit{}

	// First sample adopts the live counters as the baseline (no retro-counting).
	accumulate(cl, true, 1000, 5000)
	if cl.RxTotal != 1000 || cl.TxTotal != 5000 {
		t.Fatalf("baseline: got rx=%d tx=%d, want 1000/5000", cl.RxTotal, cl.TxTotal)
	}

	// Normal growth adds the delta.
	accumulate(cl, false, 1500, 9000)
	if cl.RxTotal != 1500 || cl.TxTotal != 9000 {
		t.Fatalf("growth: got rx=%d tx=%d, want 1500/9000", cl.RxTotal, cl.TxTotal)
	}

	// Counter reset (interface restart): raw < last -> count raw as the delta,
	// so cumulative keeps growing instead of dropping to zero.
	accumulate(cl, false, 200, 300)
	if cl.RxTotal != 1700 || cl.TxTotal != 9300 {
		t.Fatalf("after reset: got rx=%d tx=%d, want 1700/9300", cl.RxTotal, cl.TxTotal)
	}
	if cl.RxLast != 200 || cl.TxLast != 300 {
		t.Fatalf("last not re-baselined: got %d/%d", cl.RxLast, cl.TxLast)
	}
}

func TestShouldBlock(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name       string
		cl         ClientLimit
		wantBlock  bool
		wantReason string
	}{
		{"no limits", ClientLimit{}, false, ""},
		{"under quota", ClientLimit{DownloadLimit: 1000, TxTotal: 999}, false, ""},
		{"at quota", ClientLimit{DownloadLimit: 1000, TxTotal: 1000}, true, "quota"},
		{"over quota", ClientLimit{DownloadLimit: 1000, TxTotal: 2000}, true, "quota"},
		{"unlimited ignores usage", ClientLimit{DownloadLimit: 0, TxTotal: 1 << 40}, false, ""},
		{"not expired", ClientLimit{ExpiresAt: &future}, false, ""},
		{"expired", ClientLimit{ExpiresAt: &past}, true, "expired"},
		{"expired wins over quota", ClientLimit{DownloadLimit: 1000, TxTotal: 2000, ExpiresAt: &past}, true, "expired"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, reason := shouldBlock(&c.cl, now)
			if b != c.wantBlock || reason != c.wantReason {
				t.Fatalf("got (%v,%q), want (%v,%q)", b, reason, c.wantBlock, c.wantReason)
			}
		})
	}
}
