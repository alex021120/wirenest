#!/usr/bin/env bash
# One-command installer for WireNest (WireGuard 管理面板).
#
#   curl -fsSL https://raw.githubusercontent.com/alex021120/wirenest/main/deploy/install.sh | sudo bash
#
# 运行后会**引导你输入**监听地址 / 面板端口 / 管理员用户名 / 管理员密码（直接回车用默认值），
# 其余目录与配置一律采用默认。无需在命令行附加任何参数。
#
# 它会装 wireguard-tools、下载预编译二进制、创建 wg0 接口、开启 IP 转发与开机自启，
# 并以 systemd 运行面板。
set -euo pipefail

# --- 仓库（下载二进制用）。可用 WGUI_REPO 环境变量覆盖。 ---
REPO="${WGUI_REPO:-alex021120/wirenest}"

# --- 这些保持默认，不询问 ---
WG_IFACE="wg0"
WG_SUBNET="10.7.0.1/24"
WG_PORT="51820"
DATA_DIR="/var/lib/wireguard-ui"
WG_CONF="/etc/wireguard/${WG_IFACE}.conf"
BIN_DST="/usr/local/bin/wireguard-ui"
UNIT="/etc/systemd/system/wireguard-ui.service"

say()  { printf '\033[1;33m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m错误:\033[0m %s\n' "$*" >&2; exit 1; }

# 探测是否有可用的控制终端：即便被 `curl | bash` 管道执行也能读键盘输入。
# 注意不能只用 `[[ -r /dev/tty ]]`——无控制终端时它可能为真但 open() 仍失败（ENXIO）；
# 这里真去打开一次来判定，没有终端（如 CI）时静默采用默认值。
USE_TTY=0
if (exec 3<>/dev/tty) 2>/dev/null; then USE_TTY=1; fi

# 引导输入（回车=默认）。
ask() {        # ask <提示> <默认> -> 输出最终值
  local prompt="$1" def="$2" in=""
  if (( USE_TTY )); then
    printf '%s [%s]: ' "$prompt" "$def" > /dev/tty
    read -r in < /dev/tty || true
  fi
  printf '%s' "${in:-$def}"
}
ask_secret() { # ask_secret <提示(含默认说明)> <默认> -> 输出最终值（输入不回显，且不显示默认值本身）
  local prompt="$1" def="$2" in=""
  if (( USE_TTY )); then
    printf '%s: ' "$prompt" > /dev/tty
    read -rs in < /dev/tty || true
    printf '\n' > /dev/tty
  fi
  printf '%s' "${in:-$def}"
}

[[ $EUID -eq 0 ]] || die "请用 root 运行（sudo bash）"

case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "暂不支持的架构: $(uname -m)" ;;
esac

# --- 引导输入（回车=默认） ---
printf '\033[1m请配置面板（直接回车采用默认值）：\033[0m\n'
BIND_ADDR="$(ask  '监听地址'     '0.0.0.0')"
PANEL_PORT="$(ask '面板端口'     '8000')"
[[ "$PANEL_PORT" =~ ^[0-9]+$ ]] && (( PANEL_PORT >= 1 && PANEL_PORT <= 65535 )) || { PANEL_PORT=8000; say "端口无效，已用默认 8000。"; }
ADMIN_USER="$(ask '管理员用户名' 'admin')"
# 重装时默认保持当前密码（回车不会把已设密码重置成 admin）；首装默认 admin。
EXISTING_PASS=""
[[ -f "$UNIT" ]] && EXISTING_PASS="$(sed -n 's/^Environment=WGUI_ADMIN_PASS=//p' "$UNIT" | head -n1)"
if [[ -n "$EXISTING_PASS" ]]; then
  ADMIN_PASS="$(ask_secret '管理员密码（回车=保持当前）' "$EXISTING_PASS")"
else
  ADMIN_PASS="$(ask_secret '管理员密码（回车=admin）' 'admin')"
fi
WGUI_ADDR="${BIND_ADDR}:${PANEL_PORT}"

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

# --- 获取二进制：WGUI_LOCAL_BIN 指向本地文件（发布前自测用），否则从 Release 下载 ---
if [[ -n "${WGUI_LOCAL_BIN:-}" && -f "${WGUI_LOCAL_BIN}" ]]; then
  say "使用本地二进制: ${WGUI_LOCAL_BIN}"
  install -m 0755 "${WGUI_LOCAL_BIN}" "$BIN_DST"
else
  [[ -n "$REPO" ]] || die "未设置仓库。请在脚本顶部把 REPO 改成你的 owner/repo，或用 WGUI_REPO=owner/repo 传入。"
  URL="https://github.com/${REPO}/releases/latest/download/wireguard-ui-linux-${ARCH}"
  say "下载二进制: $URL"
  tmp="$(mktemp)"
  curl -fL --retry 3 -o "$tmp" "$URL" \
    || die "下载失败。请确认 ${REPO} 已发布 Release。"
  install -m 0755 "$tmp" "$BIN_DST"
  rm -f "$tmp"
fi

# --- management CLI: `wirenest` menu (start/stop/update/port/password) ---
say "安装管理命令 wirenest…"
if curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/deploy/wirenest" -o /usr/local/bin/wirenest 2>/dev/null \
   && head -1 /usr/local/bin/wirenest | grep -q '^#!'; then
  chmod 0755 /usr/local/bin/wirenest
else
  rm -f /usr/local/bin/wirenest
  say "（wirenest 下载失败，跳过；不影响面板运行）"
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
Environment=WGUI_ADDR=${WGUI_ADDR}
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
printf '  监听     : %s\n' "$WGUI_ADDR"
printf '  用户名   : %s\n' "$ADMIN_USER"
printf '  密码     : %s\n' "$ADMIN_PASS"
[[ "$ADMIN_PASS" == "admin" ]] && printf '\033[1;31m  ⚠️ 你使用了默认弱密码 admin，请尽快在面板「设置」里修改。\033[0m\n'
if [[ -x /usr/local/bin/wirenest ]]; then
  printf '\n管理命令: 运行 \033[1mwirenest\033[0m 调出菜单（启动/停止/更新/改端口/重置密码）。\n'
fi
printf '\n提示: 配置（含密码）在 %s（仅 root 可读）。生产环境请放到 HTTPS 反代后并放行防火墙端口 %s。\n' "$UNIT" "$PANEL_PORT"
