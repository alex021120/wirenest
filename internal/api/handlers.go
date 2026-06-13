package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"wireguard-ui/internal/auth"
	"wireguard-ui/internal/config"
	"wireguard-ui/internal/sysinfo"
	"wireguard-ui/internal/wg"
)

// Handlers bundles dependencies shared by the HTTP handlers.
type Handlers struct {
	cfg  config.Config
	auth *auth.Manager
	wg   *wg.Service
}

func NewHandlers(cfg config.Config, mgr *auth.Manager, svc *wg.Service) *Handlers {
	return &Handlers{cfg: cfg, auth: mgr, wg: svc}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Health is an unauthenticated liveness probe.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login validates credentials and issues a session cookie.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式不正确"})
		return
	}
	if !h.auth.Verify(req.Username, req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "用户名或密码错误"})
		return
	}
	h.auth.Issue(w, req.Username)
	writeJSON(w, http.StatusOK, map[string]string{"username": req.Username})
}

// Logout clears the current session.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	h.auth.Clear(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Me returns the current authenticated user.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": h.auth.Current(r)})
}

type changeCredsReq struct {
	CurrentPassword string `json:"currentPassword"`
	Username        string `json:"username"`
	NewPassword     string `json:"newPassword"`
}

// ChangeCredentials updates the admin username/password (verifying the current
// password first). The active session stays valid.
func (h *Handlers) ChangeCredentials(w http.ResponseWriter, r *http.Request) {
	var req changeCredsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式不正确"})
		return
	}
	if err := h.auth.ChangeCredentials(req.CurrentPassword, req.Username, req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// systemResp flattens host info with the WireGuard interface status.
type systemResp struct {
	sysinfo.Info
	Interface string `json:"interface"`
	WGRunning bool   `json:"wgRunning"`
}

// System returns host/system status for the dashboard: OS, kernel, uptime,
// memory, IPv4 forwarding and WireGuard status.
func (h *Handlers) System(w http.ResponseWriter, r *http.Request) {
	iface, running := h.wg.InterfaceStatus()
	writeJSON(w, http.StatusOK, systemResp{
		Info:      sysinfo.Collect(),
		Interface: iface,
		WGRunning: running,
	})
}

// InterfaceControl starts, stops, or restarts the WireGuard interface via
// wg-quick, so the operator can manage it from the panel (e.g. after a reboot).
func (h *Handlers) InterfaceControl(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效请求"})
		return
	}
	if err := h.wg.InterfaceAction(in.Action); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	iface, running := h.wg.InterfaceStatus()
	writeJSON(w, http.StatusOK, map[string]any{"interface": iface, "running": running})
}

// SetAutostart enables or disables wg-quick@<iface> at boot (systemd), so the
// interface comes back automatically after a server reboot.
func (h *Handlers) SetAutostart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效请求"})
		return
	}
	if err := h.wg.SetAutostart(in.Enabled); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"autostart": h.wg.AutostartEnabled()})
}

// EnableIPForward turns on IPv4 forwarding (immediate + persistent).
func (h *Handlers) EnableIPForward(w http.ResponseWriter, r *http.Request) {
	if err := sysinfo.EnableIPv4Forwarding(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "开启失败：" + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ipv4Forwarding": true})
}

// Overview returns dashboard summary stats merged from the .conf file and live
// kernel state.
func (h *Handlers) Overview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.wg.Overview())
}

// Clients returns the client list: static peers from .conf enriched with live
// handshake/transfer counters when the interface is up.
func (h *Handlers) Clients(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.wg.Clients())
}

type createClientReq struct {
	Name    string   `json:"name"`
	Address string   `json:"address"` // optional; empty -> auto-allocate
	Subnets []string `json:"subnets"` // optional announced LANs behind this client
}

// CreateClient allocates an address + key pair, appends the peer, writes the
// config atomically and hot-reloads. Returns the one-time client config.
func (h *Handlers) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req createClientReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式不正确"})
		return
	}
	res, err := h.wg.AddClient(req.Name, req.Address, req.Subnets)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// SetClientSubnets replaces the announced LAN subnets behind a client, so other
// clients can route to LANs sitting behind it (site-to-site).
func (h *Handlers) SetClientSubnets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicKey string   `json:"publicKey"`
		Subnets   []string `json:"subnets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式不正确"})
		return
	}
	res, err := h.wg.SetClientSubnets(req.PublicKey, req.Subnets)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type keyReq struct {
	PublicKey string `json:"publicKey"`
}

// DeleteClient removes a peer by public key. The key travels in the body rather
// than the path because WireGuard public keys (base64) can contain '/'.
func (h *Handlers) DeleteClient(w http.ResponseWriter, r *http.Request) {
	var req keyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PublicKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少客户端公钥"})
		return
	}
	res, err := h.wg.DeleteClient(req.PublicKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type renameReq struct {
	PublicKey string `json:"publicKey"`
	Name      string `json:"name"`
}

// Network returns the address pools and their next free host, for the
// "new client" dialog.
func (h *Handlers) Network(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"pools": h.wg.Pools()})
}

// DetectPublicIP asks an external service for this server's public IP, so the
// settings page can auto-fill the endpoint. It is the server (not the browser)
// that makes the request, which is what we want.
func (h *Handlers) DetectPublicIP(w http.ResponseWriter, r *http.Request) {
	ip, err := detectPublicIP(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "无法获取公网 IP：" + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ip": ip})
}

// detectPublicIP tries a few plain-text "what is my IP" services in order.
func detectPublicIP(ctx context.Context) (string, error) {
	services := []string{
		"https://api.ip.sb/ip",
		"https://ifconfig.me/ip",
		"https://api.ipify.org",
	}
	client := &http.Client{Timeout: 6 * time.Second}
	var lastErr error
	for _, u := range services {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "curl/8") // ask for the plain-text form
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		ip := strings.TrimSpace(string(body))
		if net.ParseIP(ip) != nil {
			return ip, nil
		}
		lastErr = fmt.Errorf("%s 返回了无效内容", u)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("所有探测源均失败")
	}
	return "", lastErr
}

// IPInfo proxies a geolocation lookup (country + city) for the given IP through
// the server. The client list's Endpoint column uses it for a hover tooltip;
// proxying server-side avoids browser CORS and mixed-content (HTTP→HTTPS) issues.
func (h *Handlers) IPInfo(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))
	if net.ParseIP(ip) == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的 IP"})
		return
	}
	country, city, err := cachedIPLocation(r.Context(), ip)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "查询失败：" + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"country": country, "city": city})
}

// ip9.com.cn is a free service capped at ~60 lookups/minute, so we cache results
// per IP and never hit it twice for the same address while a result is fresh.
// Successful results are stable, so they're cached for a day; failures get a
// short cool-off so a transient error can still retry soon.
const (
	ipCacheTTL    = 24 * time.Hour
	ipCacheErrTTL = time.Minute
)

type ipLoc struct {
	country, city string
	at            time.Time
	ok            bool
}

func (e ipLoc) fresh(now time.Time) bool {
	ttl := ipCacheErrTTL
	if e.ok {
		ttl = ipCacheTTL
	}
	return now.Sub(e.at) < ttl
}

var (
	ipCacheMu  sync.Mutex
	ipCache    = map[string]ipLoc{}
	ipInflight = map[string]chan struct{}{} // de-dupes concurrent lookups of one IP
)

var errIPLookupCached = fmt.Errorf("最近一次查询失败，请稍后再试")

// cachedIPLocation returns an IP's country/city, serving from cache when fresh
// and collapsing concurrent lookups of the same IP into a single upstream call.
func cachedIPLocation(ctx context.Context, ip string) (string, string, error) {
	for {
		now := time.Now()
		ipCacheMu.Lock()
		if e, ok := ipCache[ip]; ok && e.fresh(now) {
			ipCacheMu.Unlock()
			if !e.ok {
				return "", "", errIPLookupCached
			}
			return e.country, e.city, nil
		}
		if ch, busy := ipInflight[ip]; busy {
			// Another request is already fetching this IP; wait and re-check.
			ipCacheMu.Unlock()
			select {
			case <-ch:
				continue
			case <-ctx.Done():
				return "", "", ctx.Err()
			}
		}
		ch := make(chan struct{})
		ipInflight[ip] = ch
		ipCacheMu.Unlock()

		country, city, err := lookupIPLocation(ctx, ip)

		ipCacheMu.Lock()
		ipCache[ip] = ipLoc{country: country, city: city, at: time.Now(), ok: err == nil}
		delete(ipInflight, ip)
		close(ch)
		ipCacheMu.Unlock()
		return country, city, err
	}
}

// lookupIPLocation queries ip9.com.cn for an IP's country and city. The ip is
// already validated by the caller, so it's safe to interpolate into the URL.
func lookupIPLocation(ctx context.Context, ip string) (country, city string, err error) {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ip9.com.cn/get?ip="+ip, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "curl/8")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	var out struct {
		Ret  int `json:"ret"`
		Data struct {
			Country string `json:"country"`
			City    string `json:"city"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", fmt.Errorf("解析响应失败")
	}
	return out.Data.Country, out.Data.City, nil
}

// Settings returns the current interface + panel settings.
func (h *Handlers) Settings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.wg.GetSettings())
}

// UpdateSettings validates and applies a settings change.
func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in wg.SettingsInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式不正确"})
		return
	}
	res, err := h.wg.UpdateSettings(in)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ClientConfig rebuilds and returns the importable config for an existing peer.
func (h *Handlers) ClientConfig(w http.ResponseWriter, r *http.Request) {
	var req keyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PublicKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少客户端公钥"})
		return
	}
	res, err := h.wg.ClientConfig(req.PublicKey)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// RenameClient updates a peer's friendly name.
func (h *Handlers) RenameClient(w http.ResponseWriter, r *http.Request) {
	var req renameReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PublicKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少客户端公钥"})
		return
	}
	res, err := h.wg.RenameClient(req.PublicKey, req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
