// Package sysinfo collects lightweight host/system status for the dashboard,
// reading /proc and /etc/os-release with the standard library only.
package sysinfo

import (
	"bufio"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Info is a snapshot of host and WireGuard status.
type Info struct {
	Hostname       string  `json:"hostname"`
	OS             string  `json:"os"`
	Kernel         string  `json:"kernel"`
	Arch           string  `json:"arch"`
	Uptime         int64   `json:"uptime"` // seconds
	Load1          float64 `json:"load1"`
	CPUCount       int     `json:"cpuCount"`
	MemTotal       uint64  `json:"memTotal"` // bytes
	MemUsed        uint64  `json:"memUsed"`  // bytes
	IPv4Forwarding bool    `json:"ipv4Forwarding"`
	WGVersion      string  `json:"wgVersion"`
	WGModuleLoaded bool    `json:"wgModuleLoaded"`
}

// Collect gathers the current system snapshot.
func Collect() Info {
	in := Info{
		Arch:     runtime.GOARCH,
		CPUCount: runtime.NumCPU(),
	}
	in.Hostname, _ = os.Hostname()
	in.OS = osPretty()
	in.Kernel = readTrim("/proc/sys/kernel/osrelease")
	in.Uptime = firstFloatAsInt("/proc/uptime")
	in.Load1 = firstFloat("/proc/loadavg")
	in.MemTotal, in.MemUsed = mem()
	in.IPv4Forwarding = readTrim("/proc/sys/net/ipv4/ip_forward") == "1"
	in.WGVersion = wgVersion()
	in.WGModuleLoaded = moduleLoaded()
	return in
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func osPretty() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return ""
}

func firstFloat(path string) float64 {
	fields := strings.Fields(readTrim(path))
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return v
}

func firstFloatAsInt(path string) int64 {
	return int64(firstFloat(path))
}

func mem() (total, used uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	var memTotal, memAvail uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			memTotal = parseKB(fields[1])
		case "MemAvailable:":
			memAvail = parseKB(fields[1])
		}
	}
	total = memTotal
	if memTotal >= memAvail {
		used = memTotal - memAvail
	}
	return total, used
}

func parseKB(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v * 1024
}

// EnableIPv4Forwarding turns on IP forwarding both immediately (via /proc) and
// persistently (a sysctl.d drop-in so it survives reboot). Requires root.
func EnableIPv4Forwarding() error {
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0o644); err != nil {
		return err
	}
	return os.WriteFile("/etc/sysctl.d/99-wirenest.conf",
		[]byte("# Managed by wirenest\nnet.ipv4.ip_forward = 1\n"), 0o644)
}

func moduleLoaded() bool {
	_, err := os.Stat("/sys/module/wireguard")
	return err == nil
}

// wgVersion extracts the version token from `wg --version`
// ("wireguard-tools v1.0.20210914 - ...").
func wgVersion() string {
	out, err := exec.Command("wg", "--version").Output()
	if err != nil {
		return ""
	}
	for _, tok := range strings.Fields(string(out)) {
		if len(tok) > 1 && tok[0] == 'v' {
			return strings.TrimPrefix(tok, "v")
		}
	}
	return ""
}
