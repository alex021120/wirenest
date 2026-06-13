package wg

import (
	"strings"
	"testing"
)

// Covers all three friendly-name placements:
//   - phone:  `# Name` before the first [Peer] (section start)
//   - laptop: `# Name` before a [Peer] while the previous peer is still open
//             (the realistic layout that regressed once)
//   - tablet: `# Name` inside [Peer], right after the header
const sample = `[Interface]
Address = 10.7.0.1/24
ListenPort = 51820
PrivateKey = aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789aBcDeF0=
DNS = 1.1.1.1, 8.8.8.8
MTU = 1420
PostUp = iptables -A FORWARD -i %i -j ACCEPT

# Name = phone
[Peer]
PublicKey = PUBKEY_PHONE_000000000000000000000000000000=
AllowedIPs = 10.7.0.2/32
PersistentKeepalive = 25

# Name = laptop
[Peer]
PublicKey = PUBKEY_LAPTOP_00000000000000000000000000000=
AllowedIPs = 10.7.0.3/32, fd00::3/128
Endpoint = 203.0.113.5:51820

[Peer]
# Name = tablet
PublicKey = PUBKEY_TABLET_00000000000000000000000000000=
AllowedIPs = 10.7.0.4/32
`

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got := cfg.Interface.ListenPort; got != 51820 {
		t.Errorf("ListenPort = %d, want 51820", got)
	}
	if got := cfg.Interface.MTU; got != 1420 {
		t.Errorf("MTU = %d, want 1420", got)
	}
	if len(cfg.Interface.Address) != 1 || cfg.Interface.Address[0] != "10.7.0.1/24" {
		t.Errorf("Address = %v", cfg.Interface.Address)
	}
	if len(cfg.Interface.DNS) != 2 {
		t.Errorf("DNS = %v, want 2 entries", cfg.Interface.DNS)
	}

	if len(cfg.Peers) != 3 {
		t.Fatalf("got %d peers, want 3", len(cfg.Peers))
	}

	// Name from a comment before the first [Peer].
	if cfg.Peers[0].Name != "phone" {
		t.Errorf("peer0 name = %q, want phone", cfg.Peers[0].Name)
	}
	if cfg.Peers[0].PersistentKeepalive != 25 {
		t.Errorf("peer0 keepalive = %d", cfg.Peers[0].PersistentKeepalive)
	}

	// Name from a comment before [Peer] while the previous peer is open
	// (the realistic layout); multiple AllowedIPs; endpoint.
	if cfg.Peers[1].Name != "laptop" {
		t.Errorf("peer1 name = %q, want laptop", cfg.Peers[1].Name)
	}
	if len(cfg.Peers[1].AllowedIPs) != 2 {
		t.Errorf("peer1 AllowedIPs = %v, want 2", cfg.Peers[1].AllowedIPs)
	}
	if cfg.Peers[1].Endpoint != "203.0.113.5:51820" {
		t.Errorf("peer1 endpoint = %q", cfg.Peers[1].Endpoint)
	}

	// Name from a comment inside [Peer], right after the header.
	if cfg.Peers[2].Name != "tablet" {
		t.Errorf("peer2 name = %q, want tablet", cfg.Peers[2].Name)
	}
}

// TestSetInterfaceKey verifies surgical edits replace the target key, dedupe,
// append when missing, and leave unrelated lines (PrivateKey, PostUp) intact.
func TestSetInterfaceKey(t *testing.T) {
	const src = `[Interface]
Address = 10.7.0.1/24
ListenPort = 51820
PrivateKey = aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789aBcDeF0=
PostUp = iptables -A FORWARD -i %i -j ACCEPT
`
	cfg, err := ParseConfig(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	cfg.SetInterfaceKey("ListenPort", "51999") // replace existing
	cfg.SetInterfaceKey("Address", "10.9.0.1/24")
	cfg.SetInterfaceKey("MTU", "1380") // append (was absent)

	out := string(cfg.Serialize())
	for _, must := range []string{
		"ListenPort = 51999",
		"Address = 10.9.0.1/24",
		"MTU = 1380",
		"PrivateKey = aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789aBcDeF0=",
		"PostUp = iptables -A FORWARD -i %i -j ACCEPT",
	} {
		if !strings.Contains(out, must) {
			t.Errorf("missing %q in:\n%s", must, out)
		}
	}
	if strings.Contains(out, "51820") {
		t.Errorf("old ListenPort not replaced:\n%s", out)
	}

	cfg.RemoveInterfaceKey("MTU")
	if strings.Contains(string(cfg.Serialize()), "MTU") {
		t.Errorf("MTU not removed")
	}
}

// TestSerializeLossless ensures the bits we don't model survive a write:
// interface PostUp/PrivateKey kept verbatim, peer PresharedKey preserved.
func TestSerializeLossless(t *testing.T) {
	const src = `[Interface]
Address = 10.7.0.1/24
ListenPort = 51820
PrivateKey = aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789aBcDeF0=
PostUp = iptables -A FORWARD -i %i -j ACCEPT

# Name = phone
[Peer]
PublicKey = PUBKEY_PHONE_000000000000000000000000000000=
PresharedKey = PSKPSKPSKPSKPSKPSKPSKPSKPSKPSKPSKPSKPSKPSK0=
AllowedIPs = 10.7.0.2/32
`
	cfg, err := ParseConfig(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	out := string(cfg.Serialize())
	for _, must := range []string{
		"PrivateKey = aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789aBcDeF0=",
		"PostUp = iptables -A FORWARD -i %i -j ACCEPT",
		"PresharedKey = PSKPSKPSKPSKPSKPSKPSKPSKPSKPSKPSKPSKPSKPSK0=",
		"# Name = phone",
	} {
		if !strings.Contains(out, must) {
			t.Errorf("serialized output missing %q\n--- got ---\n%s", must, out)
		}
	}

	// Re-parsing the output must yield the same peer set.
	cfg2, err := ParseConfig(strings.NewReader(out))
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if len(cfg2.Peers) != 1 || cfg2.Peers[0].PublicKey != cfg.Peers[0].PublicKey {
		t.Errorf("round-trip changed peers: %+v", cfg2.Peers)
	}
}
