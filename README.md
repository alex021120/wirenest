# WireNest

🪺 通过 Web 面板在 Linux 服务器上管理 WireGuard 异地组网。后端 Go（单静态二进制，前端 embed 进去），前端 Vue 3 + Naive UI，systemd 部署。

## 设计要点
- **单二进制零运行期依赖**：前端打包进 `internal/web/dist`，`go build` 后整个面板就是一个文件，丢到全新服务器即可运行（目标机只需 `wireguard-tools`）。
- **以 `.conf` 为准**：`/etc/wireguard/wg0.conf` 是配置的唯一真实来源，面板读写它并用 `wg syncconf` 热加载；面板自身状态（客户端私钥、登录凭据、设置）以 JSON 文件存在 `WIRENEST_DATA_DIR`（0600）。
- **UI**：企业级 Dashboard、扁平化、卡片式布局，主色柠檬黄。

## 目录结构
```
cmd/wirenest-panel/      程序入口
internal/
  api/                 HTTP handler 与路由
  auth/                登录 / 会话 / 鉴权中间件
  config/              面板自身配置（env 读取）
  web/                 go:embed 前端产物
web/                   Vue 3 + Vite + Naive UI 前端源码
deploy/                systemd unit 与安装脚本
```

## 本地开发
```bash
# 后端（:8000）
go run ./cmd/wirenest-panel

# 前端热更新（:5173，API 代理到 :8000）
cd web && npm install && npm run dev
```
浏览器打开 http://localhost:5173 ，默认账号 `admin` / `admin`。

## 构建单二进制
```bash
make build      # 先打包前端，再 embed 编译，产出 ./wirenest-panel
./wirenest-panel  # 默认监听 :8000
```

## 一键安装（推荐）

在全新的 Linux 服务器上（Debian/Ubuntu/CentOS/Alpine/Arch），以 root 执行一条命令：

```bash
curl -fsSL https://raw.githubusercontent.com/alex021120/wirenest/main/deploy/install.sh | sudo bash
```

运行后会**引导你输入**几项配置，直接回车即用默认值，其余一律用默认：

| 引导项 | 默认值 |
|--------|--------|
| 监听地址 | `0.0.0.0` |
| 面板端口 | `8000` |
| 管理员用户名 | `admin` |
| 管理员密码 | `admin`（输入时不回显；建议改掉） |

随后脚本自动：装 `wireguard-tools` → 下载预编译二进制 → 生成 `wg0` 接口与服务端密钥 → 开启 IPv4 转发 → 设置开机自启 → 以 systemd 运行面板（结束打印面板地址 / 账号 / 密码）。

> 前提：仓库已 `git tag v0.1.0 && git push --tags` 触发 GitHub Actions 构建并发布 Release（见 `.github/workflows/release.yml`），install.sh 从最新 Release 下载二进制。

装好后建议放到 HTTPS 反向代理（nginx/caddy）之后，并在防火墙放行面板端口。

## 管理命令 `wirenest`

一键安装会附带一个管理命令，直接运行调出菜单：

```bash
sudo wirenest
```

```
  1) 启动面板      4) 更新面板到最新版
  2) 停止面板      5) 更换运行端口
  3) 重启面板      6) 重置登录密码
  9) 卸载 WireNest 0) 退出
```

也支持子命令：`wirenest {start|stop|restart|update|status|uninstall}`。
- **重置登录密码**：忘记密码时找回（重写 systemd 环境变量并清除 `credentials.json`）。
- **卸载**：删除面板服务/二进制/管理命令、数据目录、wg0 配置并停用接口、IPv4 转发设置（需输入 `yes` 确认，不可恢复）。

## 手动部署
```bash
make build                       # 打包前端 + embed 编译，产出 ./wirenest-panel
sudo install -m755 ./wirenest-panel /usr/local/bin/wirenest-panel
sudo cp deploy/wirenest.service /etc/systemd/system/
# 改好 unit 里的 WIRENEST_ADMIN_PASS 等，然后：
sudo systemctl enable --now wirenest
```

## 配置（环境变量）
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `WIRENEST_ADDR` | `:8000` | 监听地址 |
| `WIRENEST_ADMIN_USER` | `admin` | 管理员用户名 |
| `WIRENEST_ADMIN_PASS` | `admin` | 管理员密码 |
| `WIRENEST_DATA_DIR` | `/var/lib/wirenest` | 面板状态目录（私钥/凭据/设置） |
| `WIRENEST_WG_CONF` | `/etc/wireguard/wg0.conf` | WireGuard 接口配置 |
| `WIRENEST_ENDPOINT` | （空） | 客户端连接的默认公网地址（也可在设置页改） |

## 里程碑
- [x] **M0 脚手架**：登录、Dashboard 布局、空卡片、embed、systemd
- [x] **M1 只读**：解析 wg0.conf + wgctrl 读握手/流量 + 总览/客户端表格 + 优雅降级
- [x] **M2 写入**：增删改客户端 + IPAM 自动分配 + 密钥生成 + 原子写 + `wg syncconf` 热加载 + 客户端配置生成（已在真实 wg0 + netns 客户端上端到端验证握手/流量）
- [x] **M3 体验**：查看/重新生成客户端配置 · 二维码扫码导入 · 设置页(网段/端口/MTU/公网地址) · 按客户端指定 IP · 改登录账号密码(bcrypt)
  - 定位：**仅异地组网**（site-to-site），客户端无 DNS、AllowedIPs 自动取组网网段
- [x] **运维增强**：接口启动/停止/重启 · 开机自启开关 · **宣告内网**（客户端背后 LAN 的 site-to-site 路由）· 总览实时网速与系统信息 · Endpoint 归属地 · 客户端上线提醒 · 一键安装脚本
- [ ] **M4 加固**：TLS/反代文档、登录限流、审计日志
