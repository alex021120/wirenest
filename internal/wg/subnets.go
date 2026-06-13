package wg

import (
	"fmt"
	"net"
	"strings"
)

// A peer's AllowedIPs in wg0.conf is its tunnel address (a /32 in the VPN
// subnet) followed by any "announced" LAN subnets sitting behind that client.
// These helpers split and recombine those, and compute what each client should
// route into the tunnel so peers can reach LANs behind one another.

// splitPeerRoutes separates a peer's AllowedIPs into its tunnel address (the
// host /32 inside a VPN pool) and its announced subnets (the LANs behind it).
func splitPeerRoutes(cfg *Config, p Peer) (addr string, announced []string) {
	pools := ipv4Pools(cfg)
	for _, a := range p.AllowedIPs {
		if addr == "" && isTunnelAddr(a, pools) {
			addr = a
			continue
		}
		announced = append(announced, a)
	}
	return addr, announced
}

// isTunnelAddr reports whether a CIDR/host is a single host inside a VPN pool
// (i.e. the peer's own tunnel address) rather than an announced subnet.
func isTunnelAddr(cidr string, pools []*net.IPNet) bool {
	if strings.Contains(cidr, "/") {
		if _, ipnet, err := net.ParseCIDR(cidr); err == nil {
			if ones, bits := ipnet.Mask.Size(); ones != bits {
				return false // a real subnet, not a /32 address
			}
		}
	}
	ip := parseHost(cidr)
	if ip == nil {
		return false
	}
	for _, n := range pools {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientAllowedIPs is the AllowedIPs a client routes into the tunnel: the VPN
// subnet(s) plus every *other* peer's announced LAN subnets — so it can reach
// LANs behind other clients without routing its own LAN back through the tunnel.
func clientAllowedIPs(cfg *Config, excludePub string) string {
	var parts []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			parts = append(parts, s)
		}
	}
	for _, n := range ipv4Pools(cfg) {
		add(n.String())
	}
	for _, p := range cfg.Peers {
		if p.PublicKey == excludePub {
			continue
		}
		if _, announced := splitPeerRoutes(cfg, p); len(announced) > 0 {
			for _, a := range announced {
				add(a)
			}
		}
	}
	return strings.Join(parts, ", ")
}

// normalizeSubnets validates and canonicalizes announced subnets to network
// form (e.g. "192.168.1.5/24" -> "192.168.1.0/24"), de-duplicating and
// rejecting the VPN subnet itself (already routed) and non-IPv4 input.
func normalizeSubnets(cfg *Config, subnets []string) ([]string, error) {
	pools := ipv4Pools(cfg)
	out := []string{}
	seen := map[string]bool{}
	for _, raw := range subnets {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		ip, ipnet, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, fmt.Errorf("无效网段: %s（应形如 192.168.1.0/24）", raw)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("仅支持 IPv4 网段: %s", raw)
		}
		norm := ipnet.String()
		for _, pn := range pools {
			if pn.String() == norm {
				return nil, fmt.Errorf("%s 是组网网段本身，无需宣告", norm)
			}
		}
		if !seen[norm] {
			seen[norm] = true
			out = append(out, norm)
		}
	}
	return out, nil
}

// SetClientSubnets replaces a peer's announced LAN subnets (keeping its tunnel
// address), persists, and hot-reloads. Other clients' generated configs will
// then route those subnets into the tunnel.
func (s *Service) SetClientSubnets(publicKey string, subnets []string) (*WriteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.loadForWrite()
	if err != nil {
		return nil, err
	}
	idx := indexOfPeer(cfg, publicKey)
	if idx < 0 {
		return nil, fmt.Errorf("找不到该客户端")
	}
	clean, err := normalizeSubnets(cfg, subnets)
	if err != nil {
		return nil, err
	}
	addr, _ := splitPeerRoutes(cfg, cfg.Peers[idx])

	allowed := make([]string, 0, 1+len(clean))
	if addr != "" {
		allowed = append(allowed, addr)
	}
	allowed = append(allowed, clean...)
	cfg.Peers[idx].AllowedIPs = allowed

	if err := s.persist(cfg); err != nil {
		return nil, err
	}
	return s.reloadResult(), nil
}
