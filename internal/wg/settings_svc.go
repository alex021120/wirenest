package wg

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

// SettingsView is the combined settings shown on the settings page: the wg
// interface fields plus the panel-level fields.
type SettingsView struct {
	Address      []string `json:"address"`
	ListenPort   int      `json:"listenPort"`
	MTU          int      `json:"mtu"`
	EndpointHost string   `json:"endpointHost"`
	Configured   bool     `json:"configured"`
	Live         bool     `json:"live"`
	Autostart    bool     `json:"autostart"` // wg-quick@<iface> enabled for boot
}

// SettingsInput is an incoming settings update.
type SettingsInput struct {
	Address      []string `json:"address"`
	ListenPort   int      `json:"listenPort"`
	MTU          int      `json:"mtu"`
	EndpointHost string   `json:"endpointHost"`
}

// SettingsResult reports how the change was applied.
type SettingsResult struct {
	Applied    bool   `json:"applied"`              // took effect on the live interface
	Restarted  bool   `json:"restarted"`            // interface was restarted (vs hot reload)
	ApplyError string `json:"applyError,omitempty"` // saved but apply failed / iface down
}

// GetSettings returns the current settings.
func (s *Service) GetSettings() SettingsView {
	cfg, configured, _, live := s.load()
	st := s.settings.Snapshot()
	return SettingsView{
		Address:      cfg.Interface.Address,
		ListenPort:   cfg.Interface.ListenPort,
		MTU:          cfg.Interface.MTU,
		EndpointHost: st.EndpointHost,
		Configured:   configured,
		Live:         live,
		Autostart:    s.AutostartEnabled(),
	}
}

// AutostartEnabled reports whether wg-quick@<iface> is enabled to start on boot.
func (s *Service) AutostartEnabled() bool {
	// `systemctl is-enabled` prints the state to stdout and exits non-zero when
	// not enabled, so we read stdout regardless of exit status.
	out, _ := runOut("systemctl", "is-enabled", "wg-quick@"+s.iface)
	return out == "enabled"
}

// SetAutostart enables or disables wg-quick@<iface> at boot via systemd.
func (s *Service) SetAutostart(enable bool) error {
	verb := "disable"
	if enable {
		verb = "enable"
	}
	if out, err := run("systemctl", verb, "wg-quick@"+s.iface); err != nil {
		return fmt.Errorf("设置开机自启失败：%s", out)
	}
	return nil
}

// UpdateSettings validates and persists settings. Panel-level fields (endpoint,
// DNS) are always saved. Interface fields are written to wg0.conf and applied:
// a changed address/MTU restarts the interface; a port-only change hot-reloads.
func (s *Service) UpdateSettings(in SettingsInput) (*SettingsResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate interface inputs up front.
	if len(in.Address) == 0 {
		return nil, fmt.Errorf("至少需要一个组网网段")
	}
	for _, a := range in.Address {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(a)); err != nil {
			return nil, fmt.Errorf("无效网段: %s（应形如 10.7.0.1/24）", a)
		}
	}
	if in.ListenPort < 1 || in.ListenPort > 65535 {
		return nil, fmt.Errorf("监听端口需在 1-65535 之间")
	}
	if in.MTU != 0 && (in.MTU < 1280 || in.MTU > 1500) {
		return nil, fmt.Errorf("MTU 需在 1280-1500 之间（或留空用默认）")
	}

	// Panel-level settings are always safe to persist.
	if err := s.settings.Update(SettingsData{
		EndpointHost: strings.TrimSpace(in.EndpointHost),
	}); err != nil {
		return nil, err
	}

	cfg, err := s.loadForWrite()
	if err != nil {
		// Not configured yet: panel settings saved, nothing to apply.
		return &SettingsResult{Applied: false, ApplyError: err.Error()}, nil
	}

	addr := cleanList(in.Address)
	needRestart := !equalStrs(cfg.Interface.Address, addr) || cfg.Interface.MTU != in.MTU

	cfg.SetInterfaceKey("Address", strings.Join(addr, ", "))
	cfg.SetInterfaceKey("ListenPort", strconv.Itoa(in.ListenPort))
	if in.MTU > 0 {
		cfg.SetInterfaceKey("MTU", strconv.Itoa(in.MTU))
	} else {
		cfg.RemoveInterfaceKey("MTU")
	}
	if err := s.persist(cfg); err != nil {
		return nil, err
	}

	res := &SettingsResult{}
	var applyErr error
	if needRestart {
		res.Restarted = true
		res.Applied, applyErr = s.restartInterface()
	} else {
		res.Applied, applyErr = s.reload()
	}
	if applyErr != nil {
		res.ApplyError = applyErr.Error()
	}
	return res, nil
}

// restartInterface re-applies the whole config with `wg-quick down/up`, needed
// for changes wg syncconf can't apply live (address, MTU). It only acts if the
// interface is currently up; otherwise the change is just saved to disk.
func (s *Service) restartInterface() (bool, error) {
	if _, err := ReadDevice(s.iface); err != nil {
		return false, nil // not up; saved to disk, operator brings it up later
	}
	if out, err := run("wg-quick", "down", s.iface); err != nil {
		return false, fmt.Errorf("接口停止失败：%s", out)
	}
	if out, err := run("wg-quick", "up", s.iface); err != nil {
		return false, fmt.Errorf("接口启动失败：%s", out)
	}
	s.removeBlockedLocked() // wg-quick up re-adds all peers; re-drop blocked ones
	return true, nil
}

// InterfaceAction performs a lifecycle operation on the WireGuard interface via
// wg-quick: "up", "down", or "restart". Bringing it up (or restarting) needs the
// .conf to exist. Serialized against settings writes via the same mutex.
func (s *Service) InterfaceAction(action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, configured, _, live := s.load()
	switch action {
	case "up":
		if live {
			return fmt.Errorf("接口已在运行")
		}
		if !configured {
			return fmt.Errorf("找不到配置文件，请先在设置页配置接口")
		}
		if out, err := run("wg-quick", "up", s.iface); err != nil {
			return fmt.Errorf("启动失败：%s", out)
		}
		s.removeBlockedLocked()
	case "down":
		if !live {
			return fmt.Errorf("接口未在运行")
		}
		if out, err := run("wg-quick", "down", s.iface); err != nil {
			return fmt.Errorf("停止失败：%s", out)
		}
	case "restart":
		if !configured {
			return fmt.Errorf("找不到配置文件，请先在设置页配置接口")
		}
		if live {
			if out, err := run("wg-quick", "down", s.iface); err != nil {
				return fmt.Errorf("停止失败：%s", out)
			}
		}
		if out, err := run("wg-quick", "up", s.iface); err != nil {
			return fmt.Errorf("启动失败：%s", out)
		}
		s.removeBlockedLocked()
	default:
		return fmt.Errorf("未知操作：%s", action)
	}
	return nil
}

// run executes a command, returning trimmed stderr (or stdout) on failure.
func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return msg, err
	}
	return "", nil
}

// runOut runs a command and returns its trimmed stdout regardless of exit code
// (some tools like `systemctl is-enabled` write a useful answer then exit != 0).
func runOut(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
