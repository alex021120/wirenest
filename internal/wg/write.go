package wg

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// confPerm keeps the config private (contains the server private key).
const confPerm = 0o600

// AddResult is returned after creating a client. ConfigText / PrivateKey are
// shown to the user once and never persisted server-side.
type AddResult struct {
	Name        string `json:"name"`
	PublicKey   string `json:"publicKey"`
	Address     string `json:"address"`
	PrivateKey  string `json:"privateKey"`
	ConfigText  string `json:"configText"`
	QRCode      string `json:"qrCode"` // data: URL PNG
	Reloaded    bool   `json:"reloaded"`
	ReloadError string `json:"reloadError,omitempty"`
}

// WriteResult is returned by mutating ops that don't create a client.
type WriteResult struct {
	Reloaded    bool   `json:"reloaded"`
	ReloadError string `json:"reloadError,omitempty"`
}

var errNotConfigured = errors.New("尚未配置 WireGuard 接口，请先在设置页创建")

// AddClient appends a peer and hot-reloads. If address is non-empty it is used
// (validated against the pools); otherwise the next free IP is allocated. Any
// announced subnets (LANs behind this client) are added to its AllowedIPs.
func (s *Service) AddClient(name, address string, subnets []string) (*AddResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadForWrite()
	if err != nil {
		return nil, err
	}

	var addr string
	if strings.TrimSpace(address) == "" {
		addr, err = AllocateIPv4(cfg)
	} else {
		addr, err = ValidateClientAddress(cfg, strings.TrimSpace(address))
	}
	if err != nil {
		return nil, err
	}
	announced, err := normalizeSubnets(cfg, subnets)
	if err != nil {
		return nil, err
	}
	kp, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	cfg.Peers = append(cfg.Peers, Peer{
		Name:       strings.TrimSpace(name),
		PublicKey:  kp.Public,
		AllowedIPs: append([]string{addr}, announced...),
	})
	if err := s.persist(cfg); err != nil {
		return nil, err
	}
	// Remember the private key so the config can be re-viewed later.
	// Best-effort: a failure here doesn't undo the (successful) client creation.
	_ = s.secrets.Set(kp.Public, kp.Private)
	reloaded, reloadErr := s.reload()

	serverPub, err := PublicFromPrivate(cfg.Interface.PrivateKey)
	if err != nil {
		// Config written and peer added; we just can't render a client config.
		serverPub = "<SERVER_PUBLIC_KEY_UNAVAILABLE>"
	}
	cfgText := BuildClientConfig(ClientConfigParams{
		ClientPrivateKey: kp.Private,
		ClientAddress:    addr,
		ServerPublicKey:  serverPub,
		Endpoint:         s.endpoint(cfg),
		AllowedIPs:       clientAllowedIPs(cfg, kp.Public),
	})
	qr, _ := QRDataURL(cfgText)

	res := &AddResult{
		Name:       strings.TrimSpace(name),
		PublicKey:  kp.Public,
		Address:    addr,
		PrivateKey: kp.Private,
		ConfigText: cfgText,
		QRCode:     qr,
		Reloaded:   reloaded,
	}
	if reloadErr != nil {
		res.ReloadError = reloadErr.Error()
	}
	return res, nil
}

// DeleteClient removes the peer with the given public key.
func (s *Service) DeleteClient(publicKey string) (*WriteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadForWrite()
	if err != nil {
		return nil, err
	}
	idx := indexOfPeer(cfg, publicKey)
	if idx < 0 {
		return nil, fmt.Errorf("找不到该客户端（公钥 %q）", publicKey)
	}
	cfg.Peers = append(cfg.Peers[:idx], cfg.Peers[idx+1:]...)
	if err := s.persist(cfg); err != nil {
		return nil, err
	}
	_ = s.secrets.Delete(publicKey) // best-effort cleanup
	return s.reloadResult(), nil
}

// RenameClient sets a new friendly name for the peer.
func (s *Service) RenameClient(publicKey, name string) (*WriteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadForWrite()
	if err != nil {
		return nil, err
	}
	idx := indexOfPeer(cfg, publicKey)
	if idx < 0 {
		return nil, fmt.Errorf("找不到该客户端（公钥 %q）", publicKey)
	}
	cfg.Peers[idx].Name = strings.TrimSpace(name)
	if err := s.persist(cfg); err != nil {
		return nil, err
	}
	return s.reloadResult(), nil
}

// ConfigView is a client's importable config, rebuilt on demand.
type ConfigView struct {
	Name          string `json:"name"`
	Address       string `json:"address"`
	ConfigText    string `json:"configText"`
	QRCode        string `json:"qrCode"` // data: URL PNG; empty when the key isn't stored
	HasPrivateKey bool   `json:"hasPrivateKey"`
}

// ClientConfig rebuilds the importable config for an existing peer using its
// stored private key. If the key is not stored (peer added before storage
// existed, or added by hand), the config is returned with a private-key
// placeholder and HasPrivateKey=false.
func (s *Service) ClientConfig(publicKey string) (*ConfigView, error) {
	cfg, err := s.loadForWrite()
	if err != nil {
		return nil, err
	}
	idx := indexOfPeer(cfg, publicKey)
	if idx < 0 {
		return nil, fmt.Errorf("找不到该客户端（公钥 %q）", publicKey)
	}
	peer := cfg.Peers[idx]

	addr, _ := splitPeerRoutes(cfg, peer)
	serverPub, err := PublicFromPrivate(cfg.Interface.PrivateKey)
	if err != nil {
		serverPub = "<SERVER_PUBLIC_KEY_UNAVAILABLE>"
	}
	priv, has := s.secrets.Get(publicKey)
	if !has {
		priv = "<在此粘贴你首次保存的客户端私钥>"
	}
	text := BuildClientConfig(ClientConfigParams{
		ClientPrivateKey: priv,
		ClientAddress:    addr,
		ServerPublicKey:  serverPub,
		Endpoint:         s.endpoint(cfg),
		AllowedIPs:       clientAllowedIPs(cfg, publicKey),
	})
	// Only generate a scannable QR when the config is actually importable
	// (i.e. we have the real private key, not a placeholder).
	qr := ""
	if has {
		qr, _ = QRDataURL(text)
	}
	return &ConfigView{
		Name:          peer.Name,
		Address:       addr,
		ConfigText:    text,
		QRCode:        qr,
		HasPrivateKey: has,
	}, nil
}

// loadForWrite loads the config, turning "missing file" into a clear error
// (we never create the interface itself; that's an operator action).
func (s *Service) loadForWrite() (*Config, error) {
	cfg, err := LoadConfig(s.confPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errNotConfigured
		}
		return nil, err
	}
	return cfg, nil
}

func indexOfPeer(cfg *Config, publicKey string) int {
	for i := range cfg.Peers {
		if cfg.Peers[i].PublicKey == publicKey {
			return i
		}
	}
	return -1
}

// persist atomically replaces the config file: write a temp file in the same
// directory, fsync, then rename over the target (atomic on the same filesystem).
func (s *Service) persist(cfg *Config) error {
	data := cfg.Serialize()
	dir := filepath.Dir(s.confPath)
	tmp, err := os.CreateTemp(dir, ".wg-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if the rename succeeded

	if err := tmp.Chmod(confPerm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.confPath)
}

// reload applies the on-disk config to the running interface without dropping
// existing peers' sessions: `wg syncconf <iface> <(wg-quick strip <iface>)`.
// If the interface is not up, this returns an error and the caller treats the
// change as "saved but not yet live".
func (s *Service) reload() (bool, error) {
	strip := exec.Command("wg-quick", "strip", s.iface)
	stripped, err := strip.Output()
	if err != nil {
		return false, fmt.Errorf("读取配置失败（wg-quick strip）：%w", cmdErr(err))
	}
	sync := exec.Command("wg", "syncconf", s.iface, "/dev/stdin")
	sync.Stdin = bytes.NewReader(stripped)
	var stderr bytes.Buffer
	sync.Stderr = &stderr
	if err := sync.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return false, fmt.Errorf("应用配置失败（wg syncconf）：%s", msg)
	}
	return true, nil
}

func (s *Service) reloadResult() *WriteResult {
	reloaded, err := s.reload()
	r := &WriteResult{Reloaded: reloaded}
	if err != nil {
		r.ReloadError = err.Error()
	}
	return r
}

// endpoint builds the host:port clients should dial.
func (s *Service) endpoint(cfg *Config) string {
	host := s.settings.Snapshot().EndpointHost
	if host == "" {
		host = "<SERVER_PUBLIC_IP>" // operator must fill this in via settings
	}
	return host + ":" + strconv.Itoa(cfg.Interface.ListenPort)
}

// cmdErr surfaces stderr from an *exec.ExitError when available.
func cmdErr(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			return errors.New(msg)
		}
	}
	return err
}
