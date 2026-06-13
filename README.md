# WireNest 🪺

> 一个轻量的 WireGuard 异地组网 Web 管理面板 —— 单二进制、一条命令装好、一个命令管理。

WireNest 让你在 Linux 服务器上用网页管理 WireGuard：增删客户端、扫码导入、宣告内网（让各地局域网互通）、查看实时状态，并能在面板内一键升级。后端 Go 编译成**单个静态二进制**（前端已打包进去），目标机只需 `wireguard-tools`，零额外依赖。

## ✨ 功能

- **客户端管理**：增删改、自动分配 IP、生成配置 + **二维码扫码导入**
- **宣告内网（site-to-site）**：把客户端背后的局域网（如 `192.168.1.0/24`）路由给其它客户端
- **接口控制**：在面板里启动 / 停止 / 重启 WireGuard、一键开启 IPv4 转发与开机自启
- **实时状态**：在线客户端、上下行网速、握手时间、Endpoint 归属地、系统信息
- **一键自更新**：有新版时左上角版本号变色提示，点一下即可升级
- **企业级 Dashboard UI**：扁平化、卡片式，主色柠檬黄

## 🚀 一键安装

在全新的 Linux 服务器上（Debian / Ubuntu / CentOS / Alpine / Arch），以 root 执行一条命令：

```bash
curl -fsSL https://raw.githubusercontent.com/alex021120/wirenest/main/deploy/install.sh | sudo bash
```

安装时会**引导你输入**几项配置（直接回车用默认值），其余全自动：

| 引导项 | 默认值 |
|--------|--------|
| 监听地址 | `0.0.0.0` |
| 面板端口 | `8000` |
| 管理员用户名 | `admin` |
| 管理员密码 | `admin`（建议改掉） |

脚本会自动装好依赖、下载二进制、创建 `wg0` 接口、开启 IPv4 转发与开机自启，并以 systemd 运行面板。装完打印**面板地址 / 账号 / 密码**。

> 装好后浏览器访问 `http://<服务器IP>:8000`。生产环境建议放到 HTTPS 反向代理（nginx / caddy）之后，并在防火墙放行面板端口。

## 🛠 管理命令 `wirenest`

安装会附带一个管理命令，直接运行调出菜单：

```bash
sudo wirenest
```

```
  1) 启动面板        4) 更新面板到最新版
  2) 停止面板        5) 更换运行端口
  3) 重启面板        6) 重置登录密码
  9) 卸载 WireNest   0) 退出
```

也支持子命令：`wirenest {start|stop|restart|update|status|uninstall}`。

- **更新**：拉取最新 Release 升级面板（面板内点版本号也可）。
- **重置登录密码**：忘记密码时从服务器侧找回。
- **卸载**：删除面板、数据、wg0 配置并停用接口（需输入 `yes` 确认，不可恢复）。

## 从源码构建

需要 Go 1.22+ 与 Node 18+：

```bash
make build            # 打包前端 + embed 编译，产出 ./wirenest-panel
./wirenest-panel      # 默认监听 :8000，账号 admin/admin
```

本地前端热更新：`cd web && npm install && npm run dev`（开 http://localhost:5173 ）。

## 配置（环境变量）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `WIRENEST_ADDR` | `:8000` | 面板监听地址 |
| `WIRENEST_ADMIN_USER` | `admin` | 管理员用户名 |
| `WIRENEST_ADMIN_PASS` | `admin` | 管理员密码 |
| `WIRENEST_DATA_DIR` | `/var/lib/wirenest` | 面板状态目录（私钥 / 凭据 / 设置） |
| `WIRENEST_WG_CONF` | `/etc/wireguard/wg0.conf` | WireGuard 接口配置 |
| `WIRENEST_ENDPOINT` | （空） | 客户端连接的默认公网地址 |

更新历史见 [CHANGELOG.md](CHANGELOG.md)。
