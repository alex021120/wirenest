package wg

import (
	"errors"
	"io/fs"
	"sync"
	"time"
)

// Service reads the .conf source of truth and merges live kernel stats, and
// (since Milestone 2) performs serialized writes with hot reload.
type Service struct {
	confPath string
	iface    string
	secrets  *Secrets
	settings *Settings
	limits   *Limits

	mu sync.Mutex // serializes read-modify-write of the .conf
}

func NewService(confPath, iface string, secrets *Secrets, settings *Settings, limits *Limits) *Service {
	return &Service{confPath: confPath, iface: iface, secrets: secrets, settings: settings, limits: limits}
}

// ClientView is one peer as shown in the UI: static config + live stats.
//
// Byte totals are the panel's persistent cumulative counters (they survive
// interface/host restarts), reported from the client's perspective:
// UploadTotal = client upload, DownloadTotal = client download. The quota
// (DownloadLimit) caps DownloadTotal.
type ClientView struct {
	Name          string     `json:"name"`
	PublicKey     string     `json:"publicKey"`
	AllowedIPs    []string   `json:"allowedIPs"`
	Subnets       []string   `json:"subnets"` // announced LANs behind this client
	Endpoint      string     `json:"endpoint"`
	LastHandshake *time.Time `json:"lastHandshake"`
	ReceiveBytes  int64      `json:"rxBytes"` // live (instantaneous) upload, for speed calc
	TransmitBytes int64      `json:"txBytes"` // live (instantaneous) download, for speed calc
	UploadTotal   int64      `json:"uploadTotal"`   // cumulative client upload
	DownloadTotal int64      `json:"downloadTotal"` // cumulative client download (quota-counted)
	DownloadLimit int64      `json:"downloadLimit"` // bytes; 0 = unlimited
	ExpiresAt     *time.Time `json:"expiresAt"`     // nil = never expires
	Blocked       bool       `json:"blocked"`
	BlockReason   string     `json:"blockReason"` // "quota" | "expired"
	Online        bool       `json:"online"`
}

// OverviewView is the dashboard summary.
type OverviewView struct {
	Interface    string    `json:"interface"`
	Configured   bool      `json:"configured"`
	Live         bool      `json:"live"`
	Address      []string  `json:"address"`
	ListenPort   int       `json:"listenPort"`
	ClientsTotal int       `json:"clientsTotal"`
	Online       int       `json:"online"`
	RxBytes      int64     `json:"rxBytes"`
	TxBytes      int64     `json:"txBytes"`
	LastUpdated  time.Time `json:"lastUpdated"`
}

// ClientsView is the client list plus state flags.
type ClientsView struct {
	Clients    []ClientView `json:"clients"`
	Configured bool         `json:"configured"`
	Live       bool         `json:"live"`
}

// load reads config (may be absent) and live device state (may be unavailable),
// returning flags rather than failing so the UI can render partial data.
func (s *Service) load() (cfg *Config, configured bool, dev *DeviceState, live bool) {
	c, err := LoadConfig(s.confPath)
	if err == nil {
		cfg, configured = c, true
	} else if !errors.Is(err, fs.ErrNotExist) {
		// A real parse/permission error: log-worthy, but still degrade to "not
		// configured" so the panel stays up. (Logged by the caller if needed.)
		cfg = &Config{}
	} else {
		cfg = &Config{}
	}

	if d, err := ReadDevice(s.iface); err == nil {
		dev, live = d, true
	}
	return cfg, configured, dev, live
}

// InterfaceStatus reports the interface name and whether it is currently up.
func (s *Service) InterfaceStatus() (iface string, running bool) {
	_, _, _, live := s.load()
	return s.iface, live
}

func (s *Service) Overview() OverviewView {
	cfg, configured, dev, live := s.load()
	ov := OverviewView{
		Interface:    s.iface,
		Configured:   configured,
		Live:         live,
		Address:      cfg.Interface.Address,
		ListenPort:   cfg.Interface.ListenPort,
		ClientsTotal: len(cfg.Peers),
		LastUpdated:  time.Now().UTC(),
	}
	if live && dev.ListenPort != 0 {
		ov.ListenPort = dev.ListenPort
	}
	for _, p := range cfg.Peers {
		if !live {
			continue
		}
		if st, ok := dev.Peers[p.PublicKey]; ok {
			ov.RxBytes += st.ReceiveBytes
			ov.TxBytes += st.TransmitBytes
			if st.Online {
				ov.Online++
			}
		}
	}
	return ov
}

func (s *Service) Clients() ClientsView {
	cfg, configured, dev, live := s.load()
	var usage map[string]ClientLimit
	if s.limits != nil {
		usage = s.limits.Snapshot()
	}
	out := ClientsView{
		Clients:    make([]ClientView, 0, len(cfg.Peers)),
		Configured: configured,
		Live:       live,
	}
	for _, p := range cfg.Peers {
		addr, announced := splitPeerRoutes(cfg, p)
		allowed := p.AllowedIPs
		if addr != "" {
			allowed = []string{addr} // keep the IP column to just the tunnel address
		}
		cv := ClientView{
			Name:       p.Name,
			PublicKey:  p.PublicKey,
			AllowedIPs: allowed,
			Subnets:    announced,
			Endpoint:   p.Endpoint,
		}
		if u, ok := usage[p.PublicKey]; ok {
			cv.UploadTotal = u.RxTotal
			cv.DownloadTotal = u.TxTotal
			cv.DownloadLimit = u.DownloadLimit
			cv.ExpiresAt = u.ExpiresAt
			cv.Blocked = u.Blocked
			cv.BlockReason = u.BlockReason
		}
		if live {
			if st, ok := dev.Peers[p.PublicKey]; ok {
				cv.ReceiveBytes = st.ReceiveBytes
				cv.TransmitBytes = st.TransmitBytes
				cv.Online = st.Online
				if st.Endpoint != "" {
					cv.Endpoint = st.Endpoint
				}
				if !st.LastHandshake.IsZero() {
					t := st.LastHandshake
					cv.LastHandshake = &t
				}
			}
		}
		out.Clients = append(out.Clients, cv)
	}
	return out
}
