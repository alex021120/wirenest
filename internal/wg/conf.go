// Package wg reads and writes WireGuard configuration and reads live kernel state.
//
// The .conf file is the source of truth for static configuration (interface
// settings, peers, friendly names). Live counters (handshake, transfer) come
// from the kernel via wgctrl and are merged on top for display only.
//
// Write safety: the [Interface] section is preserved verbatim, and any peer
// lines the panel does not model (PresharedKey, unknown keys, foreign comments)
// are carried through unchanged, so round-tripping a hand-edited file is lossless.
package wg

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
)

// Interface holds the parsed [Interface] section. Raw is the verbatim body
// (every non-header line) used to re-serialize the section without loss.
type Interface struct {
	Address    []string `json:"address"`
	ListenPort int      `json:"listenPort"`
	DNS        []string `json:"dns"`
	MTU        int      `json:"mtu"`
	PrivateKey string   `json:"-"` // never exposed via the API
	Raw        []string `json:"-"` // verbatim [Interface] body lines
}

// Peer is one [Peer] section. Name comes from a `# Name = ...` comment.
// Extra preserves any lines the panel does not model (e.g. PresharedKey).
type Peer struct {
	Name                string   `json:"name"`
	PublicKey           string   `json:"publicKey"`
	AllowedIPs          []string `json:"allowedIPs"`
	Endpoint            string   `json:"endpoint"`
	PersistentKeepalive int      `json:"persistentKeepalive"`
	Extra               []string `json:"-"`
}

// Config is a parsed wg .conf file.
type Config struct {
	Interface Interface `json:"interface"`
	Peers     []Peer    `json:"peers"`
}

// LoadConfig parses the config at path. A missing file returns (nil, os.ErrNotExist)
// so callers can distinguish "not configured yet" from real parse errors.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseConfig(f)
}

const (
	secNone = iota
	secInterface
	secPeer
)

// ParseConfig parses wg-quick style INI. It is tolerant: unknown keys are
// preserved (not dropped), sections are case-insensitive, and `Key = Value` may
// use any spacing.
func ParseConfig(r io.Reader) (*Config, error) {
	cfg := &Config{}
	section := secNone
	// pendingName carries a `# Name = X` comment seen just before a [Peer].
	pendingName := ""

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		// Section headers.
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			switch strings.ToLower(line) {
			case "[interface]":
				section = secInterface
			case "[peer]":
				section = secPeer
				cfg.Peers = append(cfg.Peers, Peer{Name: pendingName})
				pendingName = ""
			default:
				section = secNone
			}
			continue
		}

		// The [Interface] body is kept verbatim and also parsed for the few
		// fields we read (address pool, listen port, DNS, private key).
		if section == secInterface {
			// A `# Name = X` comment here actually labels the upcoming first
			// [Peer] (it lexically falls in the interface section because that
			// section runs until the next header). Pull it out so it doesn't
			// pollute the interface body.
			if name, ok := parseNameComment(line); ok {
				pendingName = name
				continue
			}
			cfg.Interface.Raw = append(cfg.Interface.Raw, line)
			if !strings.HasPrefix(line, "#") {
				if k, v, ok := splitKV(line); ok {
					applyInterface(&cfg.Interface, k, v)
				}
			}
			continue
		}

		// Comments outside [Interface]: either a friendly name or, inside a
		// peer, a foreign comment we preserve.
		if strings.HasPrefix(line, "#") {
			if name, ok := parseNameComment(line); ok {
				if section == secPeer && !peerStarted(cfg.Peers[len(cfg.Peers)-1]) {
					cfg.Peers[len(cfg.Peers)-1].Name = name
				} else {
					pendingName = name
				}
			} else if section == secPeer {
				p := &cfg.Peers[len(cfg.Peers)-1]
				p.Extra = append(p.Extra, line)
			}
			continue
		}

		if section == secPeer {
			p := &cfg.Peers[len(cfg.Peers)-1]
			k, v, ok := splitKV(line)
			if !ok || !applyPeer(p, k, v) {
				// Unknown/unparseable line: keep it so writes stay lossless.
				p.Extra = append(p.Extra, line)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// peerStarted reports whether any config key has been read for this peer yet.
// Used to decide whether a `# Name` comment names this peer or the next one.
func peerStarted(p Peer) bool {
	return p.PublicKey != "" || len(p.AllowedIPs) > 0 ||
		p.Endpoint != "" || p.PersistentKeepalive != 0 || len(p.Extra) > 0
}

// parseNameComment recognises `# Name = phone` (case-insensitive key).
func parseNameComment(line string) (string, bool) {
	body := strings.TrimSpace(strings.TrimLeft(line, "#"))
	key, val, ok := splitKV(body)
	if ok && strings.EqualFold(key, "name") && val != "" {
		return val, true
	}
	return "", false
}

func splitKV(line string) (key, val string, ok bool) {
	i := strings.IndexByte(line, '=')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

func splitList(val string) []string {
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func applyInterface(iface *Interface, key, val string) {
	switch strings.ToLower(key) {
	case "address":
		iface.Address = splitList(val)
	case "listenport":
		iface.ListenPort, _ = strconv.Atoi(val)
	case "dns":
		iface.DNS = splitList(val)
	case "mtu":
		iface.MTU, _ = strconv.Atoi(val)
	case "privatekey":
		iface.PrivateKey = val
	}
}

// applyPeer sets a known peer field and reports whether the key was recognised.
func applyPeer(peer *Peer, key, val string) bool {
	switch strings.ToLower(key) {
	case "publickey":
		peer.PublicKey = val
	case "allowedips":
		peer.AllowedIPs = splitList(val)
	case "endpoint":
		peer.Endpoint = val
	case "persistentkeepalive":
		peer.PersistentKeepalive, _ = strconv.Atoi(val)
	default:
		return false
	}
	return true
}

// lineKey returns the lower-cased key of an "Key = Value" line, or "" for
// comments / non-kv lines.
func lineKey(line string) string {
	if strings.HasPrefix(line, "#") {
		return ""
	}
	if k, _, ok := splitKV(line); ok {
		return strings.ToLower(k)
	}
	return ""
}

// SetInterfaceKey replaces the first "Key = ..." line in the [Interface] body
// (case-insensitive), removing any later duplicates, or appends it if absent.
// All other lines (PrivateKey, PostUp, ...) are left untouched.
func (c *Config) SetInterfaceKey(key, value string) {
	want := strings.ToLower(key)
	line := key + " = " + value
	replaced := false
	out := c.Interface.Raw[:0]
	for _, l := range c.Interface.Raw {
		if lineKey(l) == want {
			if !replaced {
				out = append(out, line)
				replaced = true
			}
			continue // drop duplicates
		}
		out = append(out, l)
	}
	if !replaced {
		out = append(out, line)
	}
	c.Interface.Raw = out
}

// RemoveInterfaceKey deletes all "Key = ..." lines from the [Interface] body.
func (c *Config) RemoveInterfaceKey(key string) {
	want := strings.ToLower(key)
	out := c.Interface.Raw[:0]
	for _, l := range c.Interface.Raw {
		if lineKey(l) == want {
			continue
		}
		out = append(out, l)
	}
	c.Interface.Raw = out
}

// Serialize renders the config back to wg-quick .conf text. The [Interface]
// body is emitted verbatim; peers are rendered from their modeled fields plus
// any preserved Extra lines.
func (c *Config) Serialize() []byte {
	var b strings.Builder
	b.WriteString("[Interface]\n")
	for _, l := range c.Interface.Raw {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	for _, p := range c.Peers {
		b.WriteByte('\n')
		if p.Name != "" {
			b.WriteString("# Name = ")
			b.WriteString(p.Name)
			b.WriteByte('\n')
		}
		b.WriteString("[Peer]\n")
		if p.PublicKey != "" {
			b.WriteString("PublicKey = ")
			b.WriteString(p.PublicKey)
			b.WriteByte('\n')
		}
		if len(p.AllowedIPs) > 0 {
			b.WriteString("AllowedIPs = ")
			b.WriteString(strings.Join(p.AllowedIPs, ", "))
			b.WriteByte('\n')
		}
		if p.Endpoint != "" {
			b.WriteString("Endpoint = ")
			b.WriteString(p.Endpoint)
			b.WriteByte('\n')
		}
		if p.PersistentKeepalive != 0 {
			b.WriteString("PersistentKeepalive = ")
			b.WriteString(strconv.Itoa(p.PersistentKeepalive))
			b.WriteByte('\n')
		}
		for _, e := range p.Extra {
			b.WriteString(e)
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}
