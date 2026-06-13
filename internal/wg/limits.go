package wg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ClientLimit holds the per-client traffic quota / expiry config plus the
// cumulative usage counters the panel maintains itself. WireGuard's kernel
// transfer counters reset to zero whenever the interface restarts (or the host
// reboots), so a quota can't be enforced off them directly — we sample them
// periodically and accumulate the deltas here, persisting across restarts.
//
// Direction is from the client's perspective:
//   - RxTotal — cumulative bytes the client UPLOADED (server received from peer)
//   - TxTotal — cumulative bytes the client DOWNLOADED (server transmitted to peer)
//
// The quota (DownloadLimit) caps TxTotal, since the request is to limit the
// client's download.
type ClientLimit struct {
	DownloadLimit int64      `json:"downloadLimit"`       // bytes; 0 = unlimited
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"` // nil = never expires

	RxTotal int64 `json:"rxTotal"` // cumulative client upload (bytes)
	TxTotal int64 `json:"txTotal"` // cumulative client download (bytes)
	RxLast  int64 `json:"rxLast"`  // last raw kernel ReceiveBytes sampled
	TxLast  int64 `json:"txLast"`  // last raw kernel TransmitBytes sampled

	Blocked     bool   `json:"blocked"`               // currently enforced (peer removed from kernel)
	BlockReason string `json:"blockReason,omitempty"` // "quota" | "expired"
}

// Limits is a persistent JSON store of per-public-key ClientLimit, written
// atomically (0600). It only holds in-memory mutations until Save() is called,
// so the periodic accounting loop can batch a tick's worth of updates into one
// write. Save() is a no-op when nothing actually changed.
type Limits struct {
	path     string
	mu       sync.Mutex
	m        map[string]*ClientLimit
	lastSave []byte // marshaled snapshot of the last successful Save (dedupe)
}

// OpenLimits loads the store from path, starting empty if it does not exist.
func OpenLimits(path string) (*Limits, error) {
	l := &Limits{path: path, m: map[string]*ClientLimit{}}
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &l.m)
		l.lastSave = data
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return l, nil
}

// Update mutates (creating if needed) the entry for pub via fn, which receives
// the entry and whether it was just created. In-memory only; call Save to persist.
func (l *Limits) Update(pub string, fn func(cl *ClientLimit, isNew bool)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cl, ok := l.m[pub]
	if !ok {
		cl = &ClientLimit{}
		l.m[pub] = cl
	}
	fn(cl, !ok)
}

// Get returns a copy of the entry for pub, if present.
func (l *Limits) Get(pub string) (ClientLimit, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cl, ok := l.m[pub]
	if !ok {
		return ClientLimit{}, false
	}
	return *cl, true
}

// Snapshot returns a copy of all entries, keyed by public key.
func (l *Limits) Snapshot() map[string]ClientLimit {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]ClientLimit, len(l.m))
	for k, v := range l.m {
		out[k] = *v
	}
	return out
}

// Delete removes the entry for pub (e.g. when its client is deleted) and persists.
func (l *Limits) Delete(pub string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.m[pub]; ok {
		delete(l.m, pub)
		_ = l.saveLocked()
	}
}

// Save persists the store if it changed since the last successful write.
func (l *Limits) Save() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.saveLocked()
}

// saveLocked assumes l.mu is held. It skips the write when the content is
// byte-identical to the last save, so an idle panel doesn't churn the disk.
func (l *Limits) saveLocked() error {
	data, err := json.MarshalIndent(l.m, "", "  ")
	if err != nil {
		return err
	}
	if l.lastSave != nil && string(data) == string(l.lastSave) {
		return nil
	}
	dir := filepath.Dir(l.path)
	tmp, err := os.CreateTemp(dir, ".limits-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, l.path); err != nil {
		return err
	}
	l.lastSave = data
	return nil
}
