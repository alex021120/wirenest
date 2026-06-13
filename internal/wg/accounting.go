package wg

import (
	"context"
	"fmt"
	"time"
)

// accountInterval is how often the panel samples kernel transfer counters to
// accumulate per-client usage and enforce quotas/expiry. A client can at most
// overshoot its quota by one interval's worth of download before being cut off.
const accountInterval = 30 * time.Second

// StartAccounting runs the usage-accounting + enforcement loop until ctx is
// done. It samples once immediately so quotas take effect promptly on startup.
func (s *Service) StartAccounting(ctx context.Context) {
	if s.limits == nil {
		return
	}
	s.account()
	t := time.NewTicker(accountInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.account()
		}
	}
}

// account samples live counters, accumulates cumulative usage (surviving kernel
// counter resets across interface/host restarts), recomputes each client's
// blocked state, persists, and removes blocked peers from the live interface.
func (s *Service) account() {
	if s.limits == nil {
		return
	}
	dev, _ := ReadDevice(s.iface) // nil when the interface is down
	cfg, _, _, _ := s.load()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range cfg.Peers {
		var rawRx, rawTx int64
		present := false
		if dev != nil {
			if st, ok := dev.Peers[p.PublicKey]; ok {
				rawRx, rawTx, present = st.ReceiveBytes, st.TransmitBytes, true
			}
		}
		s.limits.Update(p.PublicKey, func(cl *ClientLimit, isNew bool) {
			if present {
				accumulate(cl, isNew, rawRx, rawTx)
			}
			cl.Blocked, cl.BlockReason = shouldBlock(cl, now)
		})
	}
	_ = s.limits.Save()
	s.removeBlockedLocked()
}

// accumulate folds a raw kernel sample into the cumulative totals. On first
// sight of a peer it adopts the current counters as the starting baseline (so
// existing traffic isn't suddenly re-counted from zero). A raw value below the
// last sample means the counter was reset (interface restart, or the peer was
// removed and re-added) — we then count the raw value itself as the new delta.
func accumulate(cl *ClientLimit, isNew bool, rawRx, rawTx int64) {
	if isNew {
		cl.RxTotal, cl.TxTotal = rawRx, rawTx
		cl.RxLast, cl.TxLast = rawRx, rawTx
		return
	}
	dRx := rawRx - cl.RxLast
	if dRx < 0 {
		dRx = rawRx
	}
	dTx := rawTx - cl.TxLast
	if dTx < 0 {
		dTx = rawTx
	}
	cl.RxTotal += dRx
	cl.TxTotal += dTx
	cl.RxLast, cl.TxLast = rawRx, rawTx
}

// shouldBlock reports whether a client should currently be cut off, and why.
// Expiry takes precedence over quota in the reported reason.
func shouldBlock(cl *ClientLimit, now time.Time) (bool, string) {
	if cl.ExpiresAt != nil && now.After(*cl.ExpiresAt) {
		return true, "expired"
	}
	if cl.DownloadLimit > 0 && cl.TxTotal >= cl.DownloadLimit {
		return true, "quota"
	}
	return false, ""
}

// removeBlockedLocked removes every currently-blocked peer from the live
// interface (idempotent). Assumes s.mu is held. No-op when the interface is down.
// Re-adding an unblocked peer happens via reload(), which re-syncs from the .conf.
func (s *Service) removeBlockedLocked() {
	if s.limits == nil {
		return
	}
	dev, err := ReadDevice(s.iface)
	if err != nil {
		return
	}
	for pub, cl := range s.limits.Snapshot() {
		if !cl.Blocked {
			continue
		}
		if _, present := dev.Peers[pub]; present {
			_, _ = run("wg", "set", s.iface, "peer", pub, "remove")
		}
	}
}

// SetClientLimit sets a client's download quota (bytes; 0 = unlimited) and
// expiry (nil = never). It recomputes the blocked state immediately and re-syncs
// the interface so the change takes effect at once (a now-blocked client is
// dropped; a now-unblocked one is restored).
func (s *Service) SetClientLimit(publicKey string, downloadLimit int64, expiresAt *time.Time) (*WriteResult, error) {
	if downloadLimit < 0 {
		return nil, fmt.Errorf("流量上限不能为负数")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadForWrite()
	if err != nil {
		return nil, err
	}
	if indexOfPeer(cfg, publicKey) < 0 {
		return nil, fmt.Errorf("找不到该客户端")
	}
	s.limits.Update(publicKey, func(cl *ClientLimit, _ bool) {
		cl.DownloadLimit = downloadLimit
		cl.ExpiresAt = expiresAt
		cl.Blocked, cl.BlockReason = shouldBlock(cl, time.Now())
	})
	if err := s.limits.Save(); err != nil {
		return nil, err
	}
	return s.reloadResult(), nil
}

// ResetClientUsage zeroes a client's cumulative usage (e.g. a new billing
// period), re-baselining against the current live counters so in-flight traffic
// isn't immediately re-counted, then re-syncs (restoring a quota-blocked client).
func (s *Service) ResetClientUsage(publicKey string) (*WriteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadForWrite()
	if err != nil {
		return nil, err
	}
	if indexOfPeer(cfg, publicKey) < 0 {
		return nil, fmt.Errorf("找不到该客户端")
	}
	var rawRx, rawTx int64
	if dev, err := ReadDevice(s.iface); err == nil {
		if st, ok := dev.Peers[publicKey]; ok {
			rawRx, rawTx = st.ReceiveBytes, st.TransmitBytes
		}
	}
	s.limits.Update(publicKey, func(cl *ClientLimit, _ bool) {
		cl.RxTotal, cl.TxTotal = 0, 0
		cl.RxLast, cl.TxLast = rawRx, rawTx
		cl.Blocked, cl.BlockReason = shouldBlock(cl, time.Now())
	})
	if err := s.limits.Save(); err != nil {
		return nil, err
	}
	return s.reloadResult(), nil
}
