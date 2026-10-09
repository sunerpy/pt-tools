# 数据与安全

本页说明 pt-tools 在哪里保存数据、哪些内容会加密、会主动连接哪些地址，以及部署时需要注意的安全事项。

## 数据保存在哪里

所有数据都在数据目录中：Docker 部署是挂载到 `/app/.pt-tools` 的目录，二进制部署是运行用户的 `~/.pt-tools`。pt-tools 不使用任何外部数据库或托管服务。

| 文件或目录    | 内容                                                      |
| ------------- | --------------------------------------------------------- |
| `torrents.db` | SQLite 数据库：配置、站点、订阅、规则、任务和用户数据缓存 |
| `secret.key`  | 加密密钥                                                  |
| `downloads/`  | 暂存的 `.torrent` 文件                                    |
| `logs/`       | 运行日志                                                  |

## 哪些内容会加密

以下内容在写入数据库之前，用 `secret.key` 中的密钥以 AES-256-GCM 加密：

- 站点 Cookie；
- 通知通道的凭证，例如 Telegram Bot Token 和 OneBot Access Token；
- CloakBrowser 的访问令牌；
- [远程访问](../guide/remote-access.md)的主机密钥。

密钥在首次启动时自动生成；也可以通过环境变量 `PT_TOOLS_SECRET_KEY` 提供 base64 形式的密钥，它的优先级高于 `secret.key` 文件。

此前的版本会在数据库的 `site_settings` 表里另存一份 Cookie 明文。升级后第一次启动时，pt-tools 会先给只有明文的 Cookie 补上密文，再清掉这份明文；这一步不另写备份。更早的版本升级时在 `~/.pt-tools/backups/` 留下的 `site_settings_v8_to_v9_*.json` 里也有 Cookie 明文，确认升级后一切正常，可以删除这些文件。

> [!WARNING]
> 站点的 **API Key 和 Passkey 目前以明文保存在数据库中**。数据目录和它的备份都应当只允许运行 pt-tools 的用户读取，不要放在共享目录或公开的网盘中。

通知通道列表接口不返回凭证；只有在已登录的通道详情编辑页中，凭证才会解密显示，用于修改。

## 账号与登录

- pt-tools 只有一个管理员账号。密码以加盐并迭代 10 万次的 SHA-256 摘要保存，数据库中没有明文密码。
- 登录后的会话保存在进程内存中，通过名为 `session` 的 Cookie（HttpOnly、SameSite=Lax）识别。重启 pt-tools 后需要重新登录。
- 30 天没有任何请求的会话自动失效；同时最多保留 256 个会话，超出时淘汰最久没用的那个。修改密码后，除发起修改的这个会话外，其他浏览器和设备上的会话全部失效。
- 同一 IP 在 15 分钟内登录失败 10 次（含用户名不存在）后，这个 IP 暂停登录 15 分钟。部署在反向代理之后时，pt-tools 看到的都是代理的地址，失败次数会合并计算。
- 除登录页、静态资源和健康检查接口 `/api/ping` 外，所有页面和接口都要求已登录。`/api/ping` 只返回运行状态和版本号。
- [App API](app-api.md)（`/api/app/v1/`）、[qB 兼容入口](../guide/qbit-compat.md)与 [MCP](../guide/mcp.md)（`/mcp`）另外接受 [API 令牌](../guide/api-tokens.md)。令牌按新建时选的权限调用它们，访问不了其他接口，也不能管理令牌；库里只存令牌的 SHA-256 摘要。`/mcp` 只认令牌，网页登录的会话用不了。
- 经[远程访问](../guide/remote-access.md)配对的设备在加密隧道里调用 App API，权限是配对时选的「完全控制」或「只读」，访问不了其他接口；撤销或改权限以后它的连接立即断开。远程访问的设置与设备管理只认网页登录。
- 初始账号见[安装](../guide/install.md)；首次登录后请立即修改密码。忘记密码时可以用 `PT_ADMIN_RESET` 重置，见[配置说明](../configuration.md)。

## 会主动连接哪些地址

| 对象                     | 用途                                                       |
| ------------------------ | ---------------------------------------------------------- |
| 你添加的 PT 站点         | 拉取 RSS、搜索、读取用户数据、探测登录状态                 |
| 你配置的下载器           | 推送种子、读取任务和剩余空间                               |
| 你启用的通知通道         | 发送通知；Telegram 使用长轮询接收命令                      |
| GitHub（api.github.com） | 检查新版本；二进制部署升级时下载新版本                     |
| CloakBrowser Manager     | 仅在你填好端点、token 和 Profile ID 后，作为登录探测的后备 |
| 你填的 relay             | 仅在打开远程访问后：主动连接，转发手机 App 的加密连接      |

[CloakBrowser 后备](../guide/site-login-monitoring.md#cloakbrowser-后备)会把站点 Cookie 交给你部署的 CloakBrowser，由它打开站点页面。

设置了 `HTTP_PROXY` 或 `HTTPS_PROXY` 时，这些请求按代理设置发出；`ALL_PROXY` 只对站点访问和下载器连接生效。pt-tools 不包含统计或遥测，不向任何第三方发送使用数据。

## 会监听哪些端口

- Web 界面和接口：默认 `8080`。[MCP](../guide/mcp.md) 的 `/mcp` 也在这个端口上；打开[远程访问](../guide/remote-access.md)以后，直连入口 `/remote/v1/stream` 也在这里（关着时回 404）。
- [qB 兼容入口](../guide/qbit-compat.md)：给了 `--qbit-compat-addr` 或 `PT_QBIT_COMPAT_ADDR` 时另外监听一个端口，用有「qB 兼容」权限的 API 令牌登录；该端口只在内网开放。
- QQ OneBot 通道：启用后另外监听一个端口，供 NapCat 以反向 WebSocket 连接，例如 `0.0.0.0:6701` 的 `/onebot/v11/ws`。监听地址不是本机地址（127.0.0.1、localhost）时必须设置 Access Token，否则通道不会启动；该端口只在内网开放。
- 自建的 relay（[`pt-tools relay serve`](cli.md#pt-tools-relay-serve)）是单独的进程，监听 `--listen` 指定的端口；它要对公网开放，转发的内容是端到端加密的，自己不保存任何数据。

## 部署建议

- 不要把 Web 端口直接暴露在公网上。需要远程访问时，放在启用 HTTPS 的反向代理之后，或通过 VPN 访问。会话 Cookie 没有 Secure 标记，它和 API 令牌在纯 HTTP 下都可能被同一网络中的他人截获。
- 备份时把 `torrents.db` 和 `secret.key` 作为一个整体保存，并加密备份文件，见[升级与备份](../guide/upgrade.md)。
- ChatOps 只执行用绑定码完成绑定的账号发来的命令，QQ 和 Telegram 通道还会先按通道里的名单过滤发送者。目前每个完成绑定的账号都拥有管理员权限，可以暂停、删除种子和管理订阅，所以绑定码只发给你信任的账号。每条命令都记入操作审计，审计中的 token、passkey 等参数在写入前脱敏。

## 远程访问的信任模型

- 手机 App 与 pt-tools 之间的内容用 Noise 协议端到端加密，直连与经 relay 都一样；在局域网的明文 HTTP 上也是加密的。
- relay 看得到这台主机的 hostId、App 与 pt-tools 的 IP、连接的时间与流量大小，看不到内容，也冒充不了这台主机。
- 配对二维码 10 分钟内有效、只能用一次，拿到它的人能配对成一台设备，权限是生成时选的。数据库与 `secret.key` 外流时，在「系统 → 远程访问」轮换主机密钥，所有设备都要重新配对。

## 自动化访问的风险

pt-tools 使用你自己的 Cookie、API Key 或 Passkey 访问站点。部分站点可能把频繁的自动访问视为违规。启用 RSS、登录探测等功能前，请确认站点规则允许，并合理设置检查间隔。pt-tools 不保证自动化访问不会导致警告、降级或封禁。

## 报告安全问题

发现安全问题时，请不要在公开的 Issue 中贴出利用细节或任何凭证。可以先在 [GitHub Issues](https://github.com/sunerpy/pt-tools/issues) 中说明问题类型，或通过 [Telegram 群](https://t.me/+7YK2kmWIX0s1Nzdl) 联系维护者。
