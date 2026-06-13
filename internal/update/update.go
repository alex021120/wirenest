// Package update checks GitHub Releases for a newer version and performs an
// in-place self-update: download the latest binary for this arch, validate it,
// atomically replace the running executable, and re-exec into it.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const userAgent = "wirenest-selfupdate"

var (
	mu        sync.Mutex
	cachedTag string
	cachedAt  time.Time
	cacheTTL  = 10 * time.Minute
)

// Latest returns the latest release tag (e.g. "v0.1.2") for owner/repo. The
// result is cached briefly so the per-load version check doesn't hit the
// GitHub API (60 req/h unauthenticated) on every page view.
func Latest(ctx context.Context, repo string) (string, error) {
	mu.Lock()
	if cachedTag != "" && time.Since(cachedAt) < cacheTTL {
		t := cachedTag
		mu.Unlock()
		return t, nil
	}
	mu.Unlock()

	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}
	var out struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	if out.TagName == "" {
		return "", errors.New("未找到 Release")
	}
	mu.Lock()
	cachedTag, cachedAt = out.TagName, time.Now()
	mu.Unlock()
	return out.TagName, nil
}

// IsNewer reports whether release tag `latest` is strictly newer than `current`
// (semver vX.Y.Z). Returns false if either can't be parsed (e.g. "dev"), so a
// local/unreleased build is never prompted to "update".
func IsNewer(latest, current string) bool {
	l, ok1 := parseSemver(latest)
	c, ok2 := parseSemver(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parseSemver(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v = strings.SplitN(v, "-", 2)[0] // drop any pre-release suffix
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// SelfUpdate downloads the latest release binary for this OS/arch, validates it,
// atomically replaces the running executable and re-execs into the new version
// shortly after (so the HTTP handler can respond first). Only acts if the latest
// release is actually newer than `current`.
func SelfUpdate(ctx context.Context, repo, current string) (string, error) {
	tag, err := Latest(ctx, repo)
	if err != nil {
		return "", err
	}
	if !IsNewer(tag, current) {
		return "", errors.New("已经是最新版本")
	}

	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/wireguard-ui-linux-%s", repo, runtime.GOARCH)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("下载失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(exe), ".wgui-update-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	n, copyErr := io.Copy(tmp, resp.Body)
	tmp.Close()
	if copyErr != nil {
		return "", fmt.Errorf("写入失败：%w", copyErr)
	}
	if n < 1<<20 {
		return "", errors.New("下载的文件过小，疑似无效")
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", err
	}
	if err := validate(tmpName); err != nil {
		return "", err
	}

	// Atomic replace: on Linux the running process keeps the old (now unlinked)
	// inode until it re-execs, so swapping the file under it is safe.
	if err := os.Rename(tmpName, exe); err != nil {
		return "", fmt.Errorf("替换失败：%w", err)
	}

	// Re-exec into the new binary after a short grace period so the caller's
	// HTTP response goes out first. Keeps the same PID, so systemd (Type=simple)
	// doesn't notice; the listen socket is CLOEXEC so the new image re-binds.
	go func() {
		time.Sleep(800 * time.Millisecond)
		_ = syscall.Exec(exe, os.Args, os.Environ())
	}()
	return tag, nil
}

// validate sanity-checks a downloaded binary: ELF magic + it runs `-version`
// cleanly, so a corrupt/incompatible download never replaces a working binary.
func validate(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	magic := make([]byte, 4)
	_, _ = io.ReadFull(f, magic)
	f.Close()
	if !(magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F') {
		return errors.New("下载的文件不是有效的可执行文件")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, path, "-version").Run(); err != nil {
		return fmt.Errorf("新二进制自检失败：%w", err)
	}
	return nil
}
