package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Config holds the panel's own runtime configuration.
// Milestone 0: minimal, sourced from environment variables with sane defaults.
// Later milestones will move credentials into SQLite and add WireGuard paths.
type Config struct {
	// Addr is the listen address, e.g. ":8000".
	Addr string
	// AdminUser / AdminPass are the bootstrap credentials for the single admin.
	// TODO(milestone 4): replace with bcrypt-hashed credentials stored in SQLite.
	AdminUser string
	AdminPass string
	// DataDir is where the panel keeps its own state (SQLite, sessions...).
	DataDir string
	// WgConfPath is the WireGuard interface config treated as source of truth.
	WgConfPath string
	// EndpointHost is the public host (IP or DNS) clients dial. Used to render
	// the Endpoint line in generated client configs. Empty -> a placeholder.
	EndpointHost string
	// Repo is the GitHub "owner/repo" used to check for and download updates.
	Repo string
	// Version is the build version (e.g. "v0.1.2"), injected at build time and
	// set by main; "dev" for unversioned local builds.
	Version string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// env2 reads the new WIRENEST_* key, falling back to the legacy WGUI_* key, then
// the default. This keeps a panel that self-updated across the rename working:
// its systemd unit still has WGUI_* vars (port/data dir/credentials), which the
// new binary would otherwise ignore and reset to defaults.
func env2(newKey, legacyKey, def string) string {
	if v := os.Getenv(newKey); v != "" {
		return v
	}
	if v := os.Getenv(legacyKey); v != "" {
		return v
	}
	return def
}

// IfaceName derives the WireGuard interface name from the config filename,
// e.g. "/etc/wireguard/wg0.conf" -> "wg0".
func (c Config) IfaceName() string {
	base := filepath.Base(c.WgConfPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Load builds a Config from environment variables.
func Load() Config {
	return Config{
		Addr:         env2("WIRENEST_ADDR", "WGUI_ADDR", ":8000"),
		AdminUser:    env2("WIRENEST_ADMIN_USER", "WGUI_ADMIN_USER", "admin"),
		AdminPass:    env2("WIRENEST_ADMIN_PASS", "WGUI_ADMIN_PASS", "admin"),
		DataDir:      env2("WIRENEST_DATA_DIR", "WGUI_DATA_DIR", "/var/lib/wirenest"),
		WgConfPath:   env2("WIRENEST_WG_CONF", "WGUI_WG_CONF", "/etc/wireguard/wg0.conf"),
		EndpointHost: env2("WIRENEST_ENDPOINT", "WGUI_ENDPOINT", ""),
		Repo:         env2("WIRENEST_REPO", "WGUI_REPO", "alex021120/wirenest"),
	}
}
