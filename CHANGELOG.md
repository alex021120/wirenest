# 更新日志

本项目的版本变更记录。每个版本的发布说明也会同步显示在 GitHub Releases 页（来自对应 tag 的注释信息）。

## v0.1.4
- 面板 systemd 单元由 `wireguard-ui.service` 重命名为 `wirenest.service`；安装脚本自动迁移旧单元（沿用原密码后停用删除），`wirenest` 命令兼容旧单元名。
- Release 发布说明改为取自 tag 注释信息，从此每个版本都会说明改了什么。
- 二进制名（`/usr/local/bin/wireguard-ui`）与数据目录（`/var/lib/wireguard-ui`）保持不变。

## v0.1.3
- 新增 `wirenest` 管理菜单命令：启动 / 停止 / 重启 / 更新面板 / 更换运行端口 / 重置登录密码，也支持子命令。
- 一键安装脚本会一并安装 `wirenest` 命令。

## v0.1.2
- 版本显示：左上角 logo 旁显示当前版本。
- 检查更新：进面板时对比 GitHub 最新 Release，有新版弹窗提示。
- 一键自更新：下载校验新二进制后原子替换并 re-exec 重启（同 PID）。

## v0.1.1
- 性能优化：静态资源永久缓存（immutable）、gzip 压缩（JS 290KB→95KB）、访问日志降噪、隐藏标签页暂停轮询。

## v0.1.0
- 首个发布：WireGuard 异地组网管理面板（Go 单二进制 + Vue3/Naive UI）。
- 客户端增删改 + 二维码、宣告内网（site-to-site）、接口启停/重启、开机自启、实时网速与系统信息、Endpoint 归属地、客户端上线提醒、一键安装脚本。
