package wg

import (
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
)

// onlineWindow: a peer is considered online if its last handshake is within
// this window. WireGuard has no "disconnect" event — online is inferred purely
// from handshake recency, so a peer lingers "online" until its last handshake
// ages out of this window. Active peers rehandshake about every 120s, so the
// window must stay above that (else a live peer flaps offline between
// handshakes); 150s gives ~30s of margin while flipping offline sooner than
// the old 3-minute window.
const onlineWindow = 150 * time.Second

// LiveStat is per-peer kernel state, keyed by public key.
type LiveStat struct {
	LastHandshake time.Time
	ReceiveBytes  int64
	TransmitBytes int64
	Endpoint      string
	Online        bool
}

// DeviceState is the live state of one WireGuard interface.
type DeviceState struct {
	ListenPort int
	Peers      map[string]LiveStat // public key -> stats
}

// ReadDevice queries the kernel for the named interface (e.g. "wg0") via
// netlink. It returns an error if WireGuard is unavailable or the interface
// does not exist; callers treat that as "not live" rather than fatal.
func ReadDevice(name string) (*DeviceState, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	dev, err := client.Device(name)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	state := &DeviceState{
		ListenPort: dev.ListenPort,
		Peers:      make(map[string]LiveStat, len(dev.Peers)),
	}
	for _, p := range dev.Peers {
		var endpoint string
		if p.Endpoint != nil {
			endpoint = p.Endpoint.String()
		}
		online := !p.LastHandshakeTime.IsZero() &&
			now.Sub(p.LastHandshakeTime) <= onlineWindow
		state.Peers[p.PublicKey.String()] = LiveStat{
			LastHandshake: p.LastHandshakeTime,
			ReceiveBytes:  p.ReceiveBytes,
			TransmitBytes: p.TransmitBytes,
			Endpoint:      endpoint,
			Online:        online,
		}
	}
	return state, nil
}
