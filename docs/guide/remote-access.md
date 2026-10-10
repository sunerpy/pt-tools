# 远程访问

远程访问让 pt-tools 的手机 App 在外面也能看种子、站点数据与订阅，并按你给的权限暂停、删除、推送种子或管理订阅。手机扫网页上的二维码完成配对，之后 App 经直连地址或 relay 连回这台 pt-tools。连接是端到端加密的：relay 只转发，看不到内容。

App 的用法见[手机 App](mobile-app.md)。远程访问默认关着。它和 [API 令牌](api-tokens.md)是两套东西：令牌给脚本与 MCP 客户端用，要自己保管明文；远程访问的设备凭配对时生成的密钥连接，不用填令牌，也不需要把 pt-tools 的端口开放到公网。

## 工作方式

App 有两条路连回 pt-tools，配对时两条都会写进二维码，App 先试直连，连不上再走 relay：

| 方式  | 什么时候用                                                                           |
| ----- | ------------------------------------------------------------------------------------ |
| 直连  | 手机与 pt-tools 在同一个局域网，或者 pt-tools 的地址从外面也能访问（反向代理）       |
| relay | 手机在外面、pt-tools 在家里的内网：pt-tools 主动连到 relay，App 也连 relay，由它转发 |

- 两种方式里跑的都是同一种加密会话（Noise 协议），在局域网的明文 HTTP 上也是加密的。
- 设备只能调用 [App API](../reference/app-api.md)：网页的设置、站点、下载器、通知通道、令牌管理等接口在加密通道里都不存在。
- 协议的细节见仓库里的[远程访问协议](https://github.com/sunerpy/pt-tools/blob/main/docs/design/remote-access.md)。

## 打开远程访问

在「系统 → 远程访问」：

1. 打开「远程访问」。
2. 「直连地址」填 App 访问这台 pt-tools 用的地址，例如 `http://192.168.1.10:8080`；放在反向代理之后时填代理的地址，可以带子路径，例如 `https://example.com/pt-tools`。不填的话 App 只经 relay 连。
3. 「relay 地址」每行填一个 `wss://` 地址，最多 4 个。构建时内置了托管 relay 的版本会显示「填入托管 relay」。只在局域网里用时可以不填。
4. 保存。

第一次打开时 pt-tools 生成这台主机的密钥（用 `secret.key` 里的 AES 密钥加密以后存进数据库）。右边的「状态」显示 hostId、在线的连接数与每个 relay 的连接情况；relay 断开时那一行写明原因，pt-tools 会自己按退避重连。

放在反向代理之后时，代理要转发 WebSocket（`Upgrade` 与 `Connection` 请求头），直连入口的路径是 `/remote/v1/stream`。

## 添加设备

1. 在「设备」里点「添加设备」。
2. 选权限：「完全控制」能查看，也能暂停、删除、推送种子，签到，管理订阅；「只读」只能查看。
3. 「这次的直连地址」默认是设置里的直连地址，没填时是浏览器地址栏里的地址，只写进这一次的二维码。
4. 点「生成二维码」，用 App 扫描。

- 二维码只能用一次，10 分钟内有效；关掉这个窗口时作废。一次只有一个二维码有效，重新生成以后旧的作废。
- 配对请求失败 5 次（配对密钥不对，或者 App 发来的内容不对），这个二维码作废。
- 二维码等于一把临时的钥匙：拿到它的人能在 10 分钟内配对成一台设备。不要发给别人，也不要截图发到聊天里。
- 配对成功以后窗口显示设备名，「设备」列表里多一台；所有启用的通知通道收到一条「新设备已配对」。

## 管理设备

「设备」列表显示每台设备的权限、是否在线（经直连还是 relay），以及最近一次连接的时间。

- 改名：只改显示的名字，设备的连接不受影响。
- 改权限：「改成只读」或「改成完全控制」。改了以后这台设备现在的连接马上断开，App 重连以后按新的权限。
- 撤销：这台设备马上断开，之后再也连不上；要用的话重新扫码配对。撤销了的设备留在列表里，可以「删除记录」。

## relay

relay 是一个只做转发的服务。pt-tools 连上 relay 时用主机密钥签名证明自己的 hostId，别的机器冒充不了这台主机；App 与 pt-tools 之间的内容 relay 解不开。

relay 能看到的是：这台主机的 hostId、App 与 pt-tools 的 IP、连接的时间与流量大小。介意这些的话，只用直连，或者用自己部署的 relay。

### 自己部署 relay

relay 有两种实现，协议与行为一样（同一套一致性测试），任选一种：

**pt-tools 自带的 relay**：放在有公网地址的 VPS 或 NAS 上。在国内访问 Cloudflare 慢或不稳定时，放在香港等近的地区的 VPS 上效果最好。

```bash
pt-tools relay serve --listen 0.0.0.0:8443 --public-url wss://relay.example.com
```

- `--public-url` 是 App 与 pt-tools 里填的地址，必须和实际访问的一致（pt-tools 按它签名）。
- 一般放在反向代理之后，由代理做 TLS（`wss://`），代理要转发 WebSocket；要按客户端 IP 限流时用 `--client-ip-header` 指定代理写的头。也可以用 `--tls-cert`、`--tls-key` 直接做 TLS。
- Docker：用同一个镜像，设 `PT_MODE=relay` 与 `PT_TOOLS_RELAY_PUBLIC_URL`，例如 `docker run -d -p 8443:8443 -e PT_MODE=relay -e PT_TOOLS_RELAY_PUBLIC_URL=wss://relay.example.com sunerpy/pt-tools`。
- 健康检查用 `GET /ready`：接新连接时回 200，排空、连接数满了、暂停服务时回 503。监控可以抓 `GET /metrics`（Prometheus 文本，只有连接数、拒绝数、关闭码、转发字节这些聚合的计数）。
- 重启或升级时（`docker stop` 发 SIGTERM），relay 先不接新连接，再以 1012 关掉所有连接；pt-tools 收到 1012 以后在 1–5 秒里重连，App 也会自动重连。前面有负载均衡按 `/ready` 摘流量时，用 `--drain-grace` 留几秒。
- 参数与限额见[命令行](../reference/cli.md#pt-tools-relay-serve)。

**Cloudflare 版**：部署在你自己的 Cloudflare 账号下（Workers 与 Durable Objects，免费计划可用）。在仓库的 `relay/cloudflare` 目录里：

```bash
pnpm install
pnpm exec wrangler login
pnpm exec wrangler deploy                              # 地址是 https://pt-tools-relay.<你的子域>.workers.dev
pnpm exec wrangler deploy --domain relay.example.com   # 或者用自己托管在 Cloudflare 上的域名
```

App 与 pt-tools 里填 `wss://` 开头的同一个地址。限额在 `wrangler.jsonc` 的 `vars` 里改：每台 pt-tools 同时的手机连接（默认 16）、每天的转发量（默认 2 GiB，免费计划的额度有限）、每个 IP 每分钟的新连接（默认 30）。

两种 relay 都提供 `GET /healthz`（进程在线）与 `GET /ready`（接不接新连接），可以用来确认服务在线。

## 轮换主机密钥

「状态」下面的「轮换主机密钥」会换一套主机密钥，并撤销所有设备：它们记着旧的主机公钥，已经连不上了，要重新扫码配对。只在怀疑密钥泄露时用，例如数据库与 `secret.key` 的备份外流。

## 审计与安全

- 设备做的写操作（暂停、删除、推送种子，签到，管理订阅）记在「ChatOps → 操作审计」里，通道是「远程设备」，触发用户是设备编号；因为权限不够被拒的也记。配对的结果也记，命令是 `remote:pair`。
- 关掉远程访问时所有设备立即断开，直连入口回 404，relay 连接断开；设备与主机密钥保留，重新打开以后照常能连。
- 直连入口每个 IP 每分钟最多 30 次握手。部署在反向代理之后时 pt-tools 看到的都是代理的地址，所有设备合用这个额度。
- 远程访问的设置与设备管理只认网页登录，API 令牌改不了它们。

## 常见问题

**App 提示设备没有配对或已撤销**：这台设备被撤销了，或者主机密钥轮换过。到网页上重新生成二维码配对。

**直连连不上**：确认手机能打开直连地址；放在反向代理之后时确认代理转发了 WebSocket。连不上时 App 会再试 relay。

**relay 一直显示断开**：看那一行的原因。「relay 认证没有通过」说明 relay 校验签名用的地址和你填的地址不一致（例如经过了别的转发），换成 relay 对外的正式地址。
