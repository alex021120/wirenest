#!/usr/bin/env bash
# One-command installer for wireguard-ui.
#
#   curl -fsSL https://raw.githubusercontent.com/<owner>/wireguard-ui/main/deploy/install.sh | sudo bash
#
# It downloads the prebuilt binary from the repo's latest GitHub Release, installs
# wireguard-tools, creates the wg0 interface (if absent), enables IP forwarding and
# boot autostart, and runs the panel as a systemd service with a random admin password.
#
# Override defaults via env, e.g.:
#   curl -fsSL .../install.sh | sudo WGUI_REPO=me/wireguard-ui WGUI_PORT=8000 bash
set -euo pipefail

REPO="${WGUI_REPO:-}"   # 你的 GitHub 仓库 owner/repo（也可用 WGUI_REPO 环境变量传入）
PANEL_PORT="${WGUI_PORT:-8000}"
WG_IFACE="${WGUI_WG_IFACE:-wg0}"
WG_SUBNET="${WGUI_WG_SUBNET:-10.7.0.1/24}"
WG_PORT="${WGUI_WG_PORT:-51820}"
ADMIN_USER="${WGUI_ADMIN_USER:-admin}"

DATA_DIR="/var/lib/wireguard-ui"
WG_CONF="/etc/wireguard/${WG_IFACE}.conf"
BIN_DST="/usr/local/bin/wireguard-ui"
UNIT="/etc/systemd/system/wireguard-ui.service"

say()  { printf '\033[1;33m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m错误:\033[0m %s\n' "$*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || die "请用 root 运行（sudo bash）"

case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "暂不支持的架构: $(uname -m)" ;;
esac

# --- dependencies (wireguard-tools + curl) ---
say "安装依赖 (wireguard-tools, curl)…"
if   command -v apt-get >/dev/null; then apt-get update -qq && apt-get install -y -qq wireguard-tools curl
elif command -v dnf     >/dev/null; then dnf install -y -q wireguard-tools curl
elif command -v yum     >/dev/null; then yum install -y -q wireguard-tools curl
elif command -v apk     >/dev/null; then apk add --no-cache wireguard-tools curl
elif command -v pacman  >/dev/null; then pacman -Sy --noconfirm wireguard-tools curl
else die "未识别的包管理器，请手动安装 wireguard-tools 后重试"
fi
command -v wg >/dev/null || die "wireguard-tools 安装失败（找不到 wg 命令）"

# --- obtain the binary: a local path arg (for pre-release testing) or download ---
LOCAL_BIN="${1:-}"
if [[ -n "$LOCAL_BIN" && -f "$LOCAL_BIN" ]]; then
  say "使用本地二进制: $LOCAL_BIN"
  install -m 0755 "$LOCAL_BIN" "$BIN_DST"
else
  [[ -n "$REPO" ]] || die "未设置仓库。请用 WGUI_REPO=owner/repo 传入，或先 make build 后用 'install.sh ./wireguard-ui' 安装本地二进制。"
  URL="https://github.com/${REPO}/releases/latest/download/wireguard-ui-linux-${ARCH}"
  say "下载二进制: $URL"
  tmp="$(mktemp)"
  curl -fL --retry 3 -o "$tmp" "$URL" \
    || die "下载失败。请确认 ${REPO} 已发布 Release，或用 WGUI_REPO=你的用户名/仓库名 覆盖。"
  install -m 0755 "$tmp" "$BIN_DST"
  rm -f "$tmp"
fi

# --- directories ---
mkdir -p /etc/wireguard "$DATA_DIR"
chmod 700 /etc/wireguard "$DATA_DIR"

# --- create the interface config on first install ---
if [[ ! -f "$WG_CONF" ]]; then
  say "生成接口配置 $WG_CONF (网段 $WG_SUBNET, 端口 $WG_PORT)…"
  sk="$(wg genkey)"
  umask 077
  cat > "$WG_CONF" <<EOF
[Interface]
Address = ${WG_SUBNET}
ListenPort = ${WG_PORT}
PrivateKey = ${sk}
EOF
else
  say "已存在 $WG_CONF，保留不动。"
fi

# --- IP forwarding (required for site-to-site) ---
say "开启 IPv4 转发…"
echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-wireguard-ui.conf
sysctl -q -p /etc/sysctl.d/99-wireguard-ui.conf || true

# --- bring the interface up + enable on boot ---
say "启用并启动 wg-quick@${WG_IFACE}…"
systemctl enable --now "wg-quick@${WG_IFACE}" 2>/dev/null || wg-quick up "$WG_IFACE" 2>/dev/null || true

# --- admin password: preserve on re-install, otherwise generate a random one ---
if [[ -f "$UNIT" ]] && grep -q 'WGUI_ADMIN_PASS=' "$UNIT"; then
  ADMIN_PASS="$(sed -n 's/^Environment=WGUI_ADMIN_PASS=//p' "$UNIT" | head -n1)"
  say "检测到已有安装，沿用现有管理员密码。"
else
  rnd="$(head -c 32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9')"
  ADMIN_PASS="${rnd:0:20}"
fi

# --- best-effort public IP for the client Endpoint ---
PUBIP="$(curl -fsS --max-time 6 https://api.ip.sb/ip 2>/dev/null \
       || curl -fsS --max-time 6 https://ifconfig.me/ip 2>/dev/null || true)"

# --- systemd unit (runs as root: the panel drives wg-quick/systemctl/sysctl) ---
say "写入 systemd 服务并启动面板…"
cat > "$UNIT" <<EOF
[Unit]
Description=WireNest - WireGuard management panel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=WGUI_ADDR=:${PANEL_PORT}
Environment=WGUI_DATA_DIR=${DATA_DIR}
Environment=WGUI_WG_CONF=${WG_CONF}
Environment=WGUI_ENDPOINT=${PUBIP}
Environment=WGUI_ADMIN_USER=${ADMIN_USER}
Environment=WGUI_ADMIN_PASS=${ADMIN_PASS}
ExecStart=${BIN_DST}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF
chmod 600 "$UNIT"   # the unit holds the admin password

systemctl daemon-reload
systemctl enable --now wireguard-ui

printf '\n\033[1;32m✓ 安装完成\033[0m\n'
printf '  面板地址 : http://%s:%s\n' "${PUBIP:-<服务器IP>}" "$PANEL_PORT"
printf '  用户名   : %s\n' "$ADMIN_USER"
printf '  密码     : %s\n' "$ADMIN_PASS"
printf '\n提示: 密码保存在 %s（仅 root 可读）。生产环境请放到 HTTPS 反代后并放行防火墙端口 %s。\n' "$UNIT" "$PANEL_PORT"
