package wg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Secrets is a small persistent store mapping a peer's public key to the client
// private key the panel generated for it. The .conf file remains the source of
// truth for peers; this only lets the panel re-display a usable client config
// after creation. Stored as JSON, 0600, written atomically.
type Secrets struct {
	path string
	mu   sync.Mutex
	m    map[string]string
}

// OpenSecrets loads the store from path, starting empty if it does not exist.
func OpenSecrets(path string) (*Secrets, error) {
	s := &Secrets{path: path, m: map[string]string{}}
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &s.m)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// Get returns the stored private key for a public key, if present.
func (s *Secrets) Get(publicKey string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[publicKey]
	return v, ok
}

// Set stores (and persists) the private key for a public key.
func (s *Secrets) Set(publicKey, privateKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[publicKey] = privateKey
	return s.save()
}

// Delete removes (and persists) the entry for a public key.
func (s *Secrets) Delete(publicKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, publicKey)
	return s.save()
}

// save assumes the caller holds s.mu.
func (s *Secrets) save() error {
	data, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".secrets-*.tmp")
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
	return os.Rename(tmpName, s.path)
}
