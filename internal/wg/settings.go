package wg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// SettingsData is panel-level configuration that is NOT part of wg semantics:
// currently just the public endpoint clients dial. Kept out of wg0.conf so it
// never pollutes the interface section.
type SettingsData struct {
	EndpointHost string `json:"endpointHost"`
}

// Settings is a persistent store for SettingsData (JSON, written atomically).
type Settings struct {
	path string
	mu   sync.Mutex
	data SettingsData
}

// OpenSettings loads settings from path; if absent, it starts with the given
// default endpoint (typically from the WIRENEST_ENDPOINT env var).
func OpenSettings(path, defaultEndpoint string) (*Settings, error) {
	s := &Settings{path: path, data: SettingsData{EndpointHost: defaultEndpoint}}
	raw, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(raw, &s.data)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// Snapshot returns a copy of the current settings.
func (s *Settings) Snapshot() SettingsData {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data
}

// Update replaces and persists the settings.
func (s *Settings) Update(d SettingsData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = d
	return s.save()
}

// save assumes the caller holds s.mu.
func (s *Settings) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}
