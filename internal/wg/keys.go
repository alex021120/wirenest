package wg

import (
	"fmt"
	"strings"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// KeyPair is a freshly generated WireGuard key pair. The private key is shown to
// the user exactly once (in the generated client config) and never stored by the
// server, which keeps only the public key in wg0.conf.
type KeyPair struct {
	Private string
	Public  string
}

// GenerateKeyPair creates a new Curve25519 key pair.
func GenerateKeyPair() (KeyPair, error) {
	k, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return KeyPair{}, err
	}
	return KeyPair{Private: k.String(), Public: k.PublicKey().String()}, nil
}

// PublicFromPrivate derives the public key for a base64 private key.
func PublicFromPrivate(priv string) (string, error) {
	k, err := wgtypes.ParseKey(priv)
	if err != nil {
		return "", err
	}
	return k.PublicKey().String(), nil
}

// ClientConfigParams describes everything needed to render a client .conf.
// This panel targets site-to-site networking, so there is no DNS and AllowedIPs
// defaults to the VPN subnet(s) rather than a full tunnel.
type ClientConfigParams struct {
	ClientPrivateKey string
	ClientAddress    string // e.g. "10.7.0.2/32"
	ServerPublicKey  string
	Endpoint         string // "host:port"; may be a placeholder if host unknown
	AllowedIPs       string // routes the client sends through the tunnel (VPN subnet)
}

// BuildClientConfig renders a ready-to-import client configuration.
func BuildClientConfig(p ClientConfigParams) string {
	allowed := p.AllowedIPs
	if allowed == "" {
		allowed = "0.0.0.0/0" // fallback if the subnet can't be derived
	}
	var b strings.Builder
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", p.ClientPrivateKey)
	fmt.Fprintf(&b, "Address = %s\n", p.ClientAddress)
	b.WriteString("\n[Peer]\n")
	fmt.Fprintf(&b, "PublicKey = %s\n", p.ServerPublicKey)
	fmt.Fprintf(&b, "Endpoint = %s\n", p.Endpoint)
	fmt.Fprintf(&b, "AllowedIPs = %s\n", allowed)
	b.WriteString("PersistentKeepalive = 25\n")
	return b.String()
}
