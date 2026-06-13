package wg

import (
	"errors"
	"fmt"
	"net"
)

// Pool describes an IPv4 address pool derived from the interface's Address,
// for the "new client" dialog: the subnet and the next free host in it.
type Pool struct {
	CIDR     string `json:"cidr"`     // server address form, e.g. "10.7.0.1/24"
	Network  string `json:"network"`  // "10.7.0.0/24"
	NextFree string `json:"nextFree"` // "10.7.0.4" or "" if exhausted
}

// usedAddrs collects every host address already taken: the server's own IPs
// plus every peer's AllowedIPs.
func usedAddrs(cfg *Config) map[string]bool {
	used := map[string]bool{}
	for _, a := range cfg.Interface.Address {
		if ip := parseHost(a); ip != nil {
			used[ip.String()] = true
		}
	}
	for _, p := range cfg.Peers {
		for _, a := range p.AllowedIPs {
			if ip := parseHost(a); ip != nil {
				used[ip.String()] = true
			}
		}
	}
	return used
}

// ipv4Pools returns the interface's IPv4 (network, serverIP) pairs.
func ipv4Pools(cfg *Config) []*net.IPNet {
	var nets []*net.IPNet
	for _, a := range cfg.Interface.Address {
		ip, n, err := net.ParseCIDR(a)
		if err != nil || ip.To4() == nil {
			continue
		}
		nets = append(nets, n)
	}
	return nets
}

// nextFreeIn finds the first unused host in network (skipping network/broadcast).
func nextFreeIn(network *net.IPNet, used map[string]bool) (string, bool) {
	bcast := broadcast(network)
	for ip := incIP(cloneIP(network.IP.To4())); network.Contains(ip); ip = incIP(ip) {
		if ip.Equal(network.IP.To4()) || ip.Equal(bcast) {
			continue
		}
		if !used[ip.String()] {
			return ip.String(), true
		}
	}
	return "", false
}

// Pools returns the address pools with their next free host, for the UI.
func (s *Service) Pools() []Pool {
	cfg, _, _, _ := s.load()
	used := usedAddrs(cfg)
	out := []Pool{}
	for i, a := range cfg.Interface.Address {
		ip, n, err := net.ParseCIDR(a)
		if err != nil || ip.To4() == nil {
			continue
		}
		next, _ := nextFreeIn(n, used)
		out = append(out, Pool{CIDR: a, Network: n.String(), NextFree: next})
		_ = i
	}
	return out
}

// AllocateIPv4 returns the next free /32 host in the first IPv4 pool.
func AllocateIPv4(cfg *Config) (string, error) {
	pools := ipv4Pools(cfg)
	if len(pools) == 0 {
		return "", errors.New("接口没有可分配的 IPv4 网段")
	}
	used := usedAddrs(cfg)
	if next, ok := nextFreeIn(pools[0], used); ok {
		return next + "/32", nil
	}
	return "", errors.New("地址池已用尽，没有可分配的 IP")
}

// ValidateClientAddress checks an operator-supplied address (bare IP or CIDR),
// ensuring it sits inside one of the interface pools and is not already used.
// It returns the normalized "ip/32" form.
func ValidateClientAddress(cfg *Config, addr string) (string, error) {
	ip := parseHost(addr)
	if ip == nil {
		return "", fmt.Errorf("无效 IP 地址: %s", addr)
	}
	pools := ipv4Pools(cfg)
	if len(pools) == 0 {
		return "", errors.New("接口没有可用的 IPv4 网段")
	}
	inPool := false
	for _, n := range pools {
		if n.Contains(ip) && !ip.Equal(n.IP.To4()) && !ip.Equal(broadcast(n)) {
			inPool = true
			break
		}
	}
	if !inPool {
		return "", fmt.Errorf("%s 不在任何组网网段内", ip)
	}
	if usedAddrs(cfg)[ip.String()] {
		return "", fmt.Errorf("%s 已被占用", ip)
	}
	return ip.String() + "/32", nil
}

// parseHost extracts the host IP from "10.0.0.2/32" or a bare "10.0.0.2".
func parseHost(s string) net.IP {
	if ip, _, err := net.ParseCIDR(s); err == nil {
		return ip.To4()
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip.To4()
	}
	return nil
}

func cloneIP(ip net.IP) net.IP {
	out := make(net.IP, len(ip))
	copy(out, ip)
	return out
}

func incIP(ip net.IP) net.IP {
	out := cloneIP(ip)
	for i := len(out) - 1; i >= 0; i-- {
		out[i]++
		if out[i] != 0 {
			break
		}
	}
	return out
}

func broadcast(n *net.IPNet) net.IP {
	ip := n.IP.To4()
	if ip == nil {
		return nil
	}
	out := cloneIP(ip)
	for i := range out {
		out[i] |= ^n.Mask[i]
	}
	return out
}
