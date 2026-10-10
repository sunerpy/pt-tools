# 远程访问协议 v1

本文是手机 App 与 pt-tools 之间远程访问协议的规范（路线图 M15），Go 主机端在 `internal/remote`，App 按本文实现。各层的大小在 v1 里冻结，改动要升协议版本。

测试向量在 `internal/remote/testdata/`：`vectors.json` 是本协议的向量（hostId、配对链接、隧道帧、relay 外层帧、relay 认证、一次完整会话的线上字节），`noise_ik_vectors.json` 是 Noise 公开测试向量里 `Noise_IK_25519_ChaChaPoly_BLAKE2s` 的两组（cacophony 与 snow）。两端都要逐字节通过。

用法见[远程访问](../guide/remote-access.md)。

## 总览

```text
App ──直连 WebSocket──────────────────────────────▶ pt-tools /remote/v1/stream
App ──WebSocket──▶ relay /v1/client/<hostId> ═外层帧═ relay /v1/host/<hostId> ◀── pt-tools 主动连接
```

- 两条路上跑的都是同一种 Noise 会话：App 是发起方，pt-tools 是响应方。relay 只转发 Noise 消息，看不到明文。
- 握手以后，每条 Noise 传输消息装一个隧道帧；HTTP 请求与回应拆成隧道帧，一条会话里按请求编号复用。
- 配对过的设备只能调用 App API v1（`/api/app/v1/*`），权限是配对时选的 `app:read`，或 `app:read app:write`。

## 身份

| 名字            | 是什么                                                                                      |
| --------------- | ------------------------------------------------------------------------------------------- |
| 主机签名密钥    | Ed25519。只用来向 relay 证明这台主机拥有 hostId                                             |
| hostId          | `lower(base32(sha256(Ed25519 公钥)))` 的前 26 个字符，标准字母表、无填充，只有 `a–z`、`2–7` |
| 主机 Noise 密钥 | X25519 静态密钥；公钥写在配对链接里，App 记下来（之后握手时校验主机）                       |
| 设备密钥        | X25519 静态密钥，App 在每次配对时新生成，私钥只存在手机的安全存储里                         |

两把主机密钥在第一次打开远程访问时生成，用 pt-tools 的 AES 密钥加密以后存库。轮换主机密钥会撤销所有设备。

文本里的二进制值（链接里的公钥、配对密钥，接口里的公钥）一律是 base64url，无填充，只接受规范写法（最后一个字符的填充位为 0）。

## 配对链接

```text
pttools://pair?v=1&h=<hostId>&k=<主机 X25519 公钥>&s=<配对密钥>&r=<relay 地址，逗号分隔>&d=<直连地址>
```

- `k`、`s` 是 32 字节的 base64url。`r` 与 `d` 可选，至少要有一个；值按查询串转义，参数按上面的顺序写。
- `r` 里每个地址是 `ws://` 或 `wss://`，可以带路径前缀，最多 4 个；`d` 是 `http://` 或 `https://`，可以带反向代理的子路径。两者都没有用户信息、查询串、片段与百分号转义，末尾没有 `/`。
- `v` 不是 `1` 时 App 提示升级；不认识的参数忽略；`v`、`h`、`k`、`s`、`r`、`d` 重复出现时整个链接作废。

## 传输

- 直连：`<d>/remote/v1/stream`（`http` 换成 `ws`，`https` 换成 `wss`）。每条 WebSocket 二进制消息恰好是一条 Noise 消息；文本消息、超过 65535 字节的消息都会让主机关掉连接。
- 经 relay：App 连 `<relay>/v1/client/<hostId>`，同样每条二进制消息是一条 Noise 消息。relay 与主机之间的格式见下面的「relay」。
- 远程访问关着时直连入口回 404。每个 IP 每分钟最多 30 次直连握手，多的回 429。浏览器发来的跨源 WebSocket（`Origin` 与 `Host` 不一致）被拒绝；App 不带 `Origin`。

## Noise 握手

- 协议：`Noise_IK_25519_ChaChaPoly_BLAKE2s`。
- prologue：ASCII `pt-tools-remote-v1` 后接 hostId 的 26 个 ASCII 字符。两边不一致时握手失败。
- 第一条消息（App → 主机）的内容是 JSON `{"v":1,"client":"<App 名字与版本>"}`；第二条消息（主机 → App）是 `{"v":1,"mode":"device"|"pairing","host":"<pt-tools 版本>"}`。JSON 都不超过 1024 字节。
- 主机解不开第一条消息（App 记的主机公钥不对，或者 hostId 不对）时直接断开。
- 主机看过设备公钥以后决定会话种类：

  | 设备公钥                 | 结果                                                  |
  | ------------------------ | ----------------------------------------------------- |
  | 在设备表里，没有撤销     | `mode=device`，权限从库里读出来固定在这条会话上       |
  | 不在表里，现在有配对窗口 | `mode=pairing`，只能调用 `POST /remote/v1/pair`       |
  | 不在表里，没有配对窗口   | 第二条消息带 `{"v":1,"error":"not_paired"}`，随后断开 |

  其他错误：`unsupported_version`（第一条消息的 `v` 不是 1）、`busy`（会话太多，稍后再连）。带 `error` 的第二条消息照样完成握手，App 据此提示用户，而不是只看到连接断开。

- 每条握手消息最多等 10 秒。

## 大小（冻结）

| 层                 | 上限                                                           |
| ------------------ | -------------------------------------------------------------- |
| Noise 消息         | 65535 字节（含 16 字节认证标签）                               |
| Noise 传输消息明文 | 65519 字节                                                     |
| 隧道帧             | `[type u8][req_id u32 大端][payload]`，payload 最多 65514 字节 |
| relay 外层帧       | `[type u8][stream u32 大端][一条 Noise 消息]`，最多 65540 字节 |
| 一个请求体         | 16 MiB；一条会话里没处理完的请求体合计也不超过 16 MiB          |
| 同时进行的请求     | 每条会话 16 个（从 REQ_HEAD 算到回应发完）                     |

超过的帧与消息被拒绝；超过的请求体回 413，同时进行的请求超过 16 个回 429（会话不断）。

## 隧道帧

| 类型        | 值     | 方向       | 编号 | 内容                                           |
| ----------- | ------ | ---------- | ---- | ---------------------------------------------- |
| `REQ_HEAD`  | `0x01` | App → 主机 | ≠ 0  | JSON `{"method","path","headers"}`，最多 8 KiB |
| `REQ_BODY`  | `0x02` | App → 主机 | ≠ 0  | 请求体的一段，不能为空                         |
| `REQ_END`   | `0x03` | App → 主机 | ≠ 0  | 空                                             |
| `CANCEL`    | `0x04` | 双向       | ≠ 0  | 可选的 UTF-8 原因，最多 256 字节               |
| `RESP_HEAD` | `0x11` | 主机 → App | ≠ 0  | JSON `{"status","headers"}`                    |
| `RESP_BODY` | `0x12` | 主机 → App | ≠ 0  | 回应体的一段，不能为空                         |
| `RESP_END`  | `0x13` | 主机 → App | ≠ 0  | 空                                             |
| `PING`      | `0x20` | 双向       | 0    | 最多 64 字节，对方用 `PONG` 原样回             |
| `PONG`      | `0x21` | 双向       | 0    | 同上                                           |
| `GOAWAY`    | `0x30` | 主机 → App | 0    | JSON `{"reason"}`，主机随后断开                |

### 请求

- 请求编号由 App 选，不为 0。一个编号在 App 发完 `REQ_END`（或 `CANCEL`）并且收到 `RESP_END`（或主机的 `CANCEL`）以后才能再用。主机收到的 `REQ_HEAD` 用了还在进行的编号时，按违反协议处理。
- `method` 只能是 `GET`、`HEAD`、`POST`、`PUT`、`PATCH`、`DELETE`，别的回 405。
- `path` 以 `/` 开头，可以带查询串，最多 2048 字节，没有空白、控制字符、非 ASCII 字符与 `#`。`?` 之前的部分必须规整：没有百分号转义，没有 `.`、`..`、空段与末尾的 `/`，字符限于 RFC 3986 的 pchar。不规整回 400。
- `headers` 最多 16 个，名字最多 64 字节，值最多 1024 字节，不能有 CR、LF、NUL。主机只转发 `Accept`、`Accept-Language`、`Content-Type`、`If-Modified-Since`、`If-None-Match`，其余（包括 `Cookie` 与 `Authorization`）一律丢掉。
- 设备会话只能调用 `/api/app/v1/` 下的路径，配对会话只能 `POST /remote/v1/pair`，别的回 403。
- 主机收齐 `REQ_END` 以后才处理请求。App 发 `CANCEL` 以后主机不再给这个编号发帧（已经在路上的照样会到，App 丢掉即可）。
- 主机回了 413 的请求，之后的 `REQ_BODY` 丢掉，等 `REQ_END` 或 `CANCEL`。不认识的编号上的 `REQ_BODY`、`REQ_END`、`CANCEL` 一律忽略。

### 回应

- `RESP_HEAD` 之后是零个或多个 `RESP_BODY`，最后是 `RESP_END`。`HEAD` 请求没有 `RESP_BODY`。
- 主机只写回 `Allow`、`Cache-Control`、`Content-Disposition`、`Content-Type`、`Etag`、`Last-Modified`、`Retry-After` 这几个回应头。
- 回应头发出以后处理出错时，主机发 `CANCEL`（原因 `internal error`），App 把这个回应当成不完整的。回应头发出之前出错时回 500。
- 隧道自己拒绝的请求（上面的 400、403、405、413、429）回 JSON `{"error","message"}`，和 App API 的错误格式一样。

### 会话

- 主机 90 秒没收到任何帧就断开；App 在前台时至少每 30 秒发一次 `PING`，90 秒没收到主机的任何帧（`PONG` 也算）就当连接已经断了，重新连接。
- App 收到 `GOAWAY` 以后不再发请求，自己结束会话：经 relay 时主机随后的关闭不一定传得到 App（例如主机正在断开 relay 连接）。
- 违反协议（帧格式不对、请求编号冲突、App 发了 `RESP_*` 或 `GOAWAY`）时主机发 `GOAWAY protocol_error` 并断开。
- `GOAWAY` 的原因：

  | 原因             | 什么时候                                       | App 怎么做                 |
  | ---------------- | ---------------------------------------------- | -------------------------- |
  | `revoked`        | 这台设备被撤销了                               | 删掉这台主机，提示重新配对 |
  | `scope_changed`  | 这台设备的权限改了                             | 立即重连，按新权限显示     |
  | `disabled`       | 远程访问关掉了                                 | 提示远程访问已关闭         |
  | `key_rotated`    | 主机密钥换了，所有设备都撤销了                 | 提示重新配对               |
  | `paired`         | 配对完成                                       | 用正常会话重连             |
  | `pairing_closed` | 配对窗口关了（过期、取消、换了新的、输错太多） | 提示到网页上重新生成二维码 |
  | `shutdown`       | pt-tools 正在退出                              | 稍后重连                   |
  | `protocol_error` | 对端违反了协议                                 | 记日志，重连               |

- 撤销设备、改它的权限、关掉远程访问、轮换主机密钥都会立即关掉相关的会话；握手与登记之间发生的这些变化，主机在登记以后再核对一次。
- 一台主机同时最多 64 条会话、32 个在握手的连接、4 条配对会话；超出时握手回 `busy`，或者连接以 4429 关掉。

## 配对

1. 网页上「系统 → 远程访问 → 添加设备」开一个配对窗口：32 字节的配对密钥，10 分钟内有效，只能用一次，失败 5 次（密钥、请求体或设备名不对）作废。一次只有一个窗口，新开的会替换旧的，旧窗口里连上的配对会话跟着断开。
2. App 扫码拿到链接，生成新的设备密钥，握手得到 `mode=pairing` 的会话。
3. App 发 `POST /remote/v1/pair`，请求体 `{"secret":"<base64url>","name":"<设备名>"}`，最多 4 KiB（超过的在收包时就回 413），不认识的字段与 JSON 之后多出的内容都被拒绝。设备名最多 64 个字符，不能有控制字符，空名字记成「未命名设备」。
4. 主机用常量时间比较配对密钥。核对与写设备表在同一把锁里完成：取消窗口、换新窗口要么在它之前（这次配对失败），要么在它之后（设备已经配对）。

   | 回应                               | 意思                                                                                                      |
   | ---------------------------------- | --------------------------------------------------------------------------------------------------------- |
   | 200 `{"device":{…}}`               | 配对成功，随后 `GOAWAY paired`                                                                            |
   | 400 `invalid_body`、`invalid_name` | 请求体或设备名不对，和输错密钥一样计入失败次数                                                            |
   | 401 `invalid_secret`               | 密钥不对（格式不对也算）；一个窗口里第 5 次失败时窗口作废，这条与别的配对会话随后 `GOAWAY pairing_closed` |
   | 409 `rejected`                     | 设备数到了上限（50 台），或者这把公钥已经配对过                                                           |
   | 410 `pairing_closed`               | 窗口过期、用过、取消或作废，随后 `GOAWAY pairing_closed`                                                  |
   | 413 `body_too_large`               | 请求体超过 4 KiB                                                                                          |

5. 配对成功以后主机写设备表（名字、公钥、权限），发「新设备已配对」通知，App 重连，之后就是 `mode=device` 的会话。每次配对请求的结果都记操作审计（通道「远程设备」，命令 `remote:pair`，结果 `success`、`denied:secret`、`denied:invalid_body`、`denied:invalid_name`、`denied:pairing_closed`、`denied:rejected` 或 `error:save_device`；配对成功以前设备编号是 0）。

## relay

relay 是一个只做转发的服务。主机主动连接，App 经它连到主机；两段 WebSocket 都由 relay 维持，内容是端到端加密的 Noise 消息。

### 主机连接

1. 主机连 `<relay>/v1/host/<hostId>`。
2. relay 发 `CHALLENGE`（32 字节随机数）。
3. 主机回 `AUTH`：Ed25519 公钥（32 字节）加签名（64 字节）。签名的内容是 ASCII `pt-tools-relay-v1` ‖ hostId ‖ 随机数 ‖ relay 的 origin。
4. relay 校验公钥推导出的 hostId 与路径里的一致、签名正确，回 `READY`；不对时以 4401 关闭。同一个 hostId 的新连接认证通过以后替换旧连接（旧的以 4409 关闭）。

relay 的 origin 是 `scheme://主机[:端口]`：小写，去掉默认端口（`ws` 的 80、`wss` 的 443），IPv6 地址加方括号。主机按它连上的地址签，relay 按自己对外的地址校验，所以别的 relay 拿到签名也冒充不了这台主机。

### 外层帧

| 类型        | 值     | 方向         | 流  | 内容                                         |
| ----------- | ------ | ------------ | --- | -------------------------------------------- |
| `CHALLENGE` | `0x01` | relay → 主机 | 0   | 32 字节随机数                                |
| `AUTH`      | `0x02` | 主机 → relay | 0   | Ed25519 公钥加签名，共 96 字节               |
| `READY`     | `0x03` | relay → 主机 | 0   | 空                                           |
| `OPEN`      | `0x10` | relay → 主机 | ≥ 1 | 空：有一个新的客户端连接                     |
| `DATA`      | `0x11` | 双向         | ≥ 1 | 一条 Noise 消息，1 到 65535 字节             |
| `CLOSE`     | `0x12` | 双向         | ≥ 1 | 空，或 u16 关闭码（大端）加最多 123 字节原因 |
| `ACCEPT`    | `0x13` | 主机 → relay | ≥ 1 | 空：这个流的握手通过了，relay 从这里起计量   |

- 流编号由 relay 分配。relay 把客户端发来的每条二进制消息装进 `DATA` 转给主机，把主机发来的 `DATA` 的内容原样发给客户端；任一端可以 `CLOSE`，relay 随后关掉客户端那条 WebSocket。主机的 `CLOSE` 排在它之前发来的 `DATA` 后面：relay 发完这些消息再关（主机常常发完 `GOAWAY` 马上关流）。
- 握手通过（会话种类是 `device` 或 `pairing`）以后、发第二条握手消息以前，主机在这个流上发 `ACCEPT`；拒绝的握手（`not_paired`、`busy` 等）不发。直连没有这一步。
- `ACCEPT` 以前每个方向只能发一条不超过 4096 字节的消息（客户端的握手第一条、主机拒绝握手的回话），多发或者太大时 relay 以 4400 关掉这个流。这两条在 `ACCEPT` 时计入每天的转发量；没有 `ACCEPT` 就关掉的流不计，未认证、未配对的客户端耗不掉主机的额度。
- 某个客户端一直不读时，relay 照常读主机连接，同一台主机别的流不受影响；跟不上的客户端以 4429 关掉。Go 版给每个客户端排队最多 64 条消息，排满即关；Cloudflare 版由运行时缓冲（Workers 的 WebSocket 没有发送完成的通知），发不出去时关。
- 主机的连接断开（或者被同一个 hostId 的新连接替换）时，relay 以 4404 关掉这台主机的所有客户端连接：主机那边的会话已经随连接结束，留着客户端连接只会让 App 等到空闲超时。
- 一条 relay 连接上主机最多同时接 16 个流，再来的 `OPEN` 直接回 `CLOSE` 4429。某个流排队的消息超过 64 条时主机关掉那个流，不拖住别的流。
- 保活：主机每 30 秒发一条文本消息 `ping`，relay 回文本 `pong`（Cloudflare 的自动应答不用唤醒 Durable Object）。主机 90 秒没收到 relay 的任何消息就断开重连。
- 断开以后主机按指数退避重连：1 秒起，每次翻倍，最长 60 秒，加 ±20% 的抖动；连上满 60 秒以后下次从 1 秒重新开始。relay 以 1012 关（重启）时退避回到 1 秒，在 1–5 秒里随机等一会儿再连，很多主机同时断开时分散开；以 1013 关（太忙）时至少等 10–20 秒；升级前回 503（满了、排空中）时至少等它给的 `Retry-After`（最多听 5 分钟），再加最多一半的抖动。

### 关闭码

| 码     | 意思                               |
| ------ | ---------------------------------- |
| `4400` | 违反协议                           |
| `4401` | 主机认证没有通过                   |
| `4404` | 主机不在线（客户端连接）           |
| `4409` | 同一个 hostId 的新连接替换了旧连接 |
| `4429` | 超出限额                           |
| `4503` | relay 暂停服务                     |
| `1012` | relay 重启（停服务前关掉所有连接） |

客户端发文本消息以 4400 关闭，发超过 65535 字节的消息以 1009 关闭。暂停服务、hostId 不对、限流这几种情况，relay 先完成 WebSocket 握手再用对应的关闭码关掉，客户端能看到原因。连接数到了上限、relay 正在排空时，relay 在升级之前回 HTTP 503（带 `Retry-After`），不占连接。

### 限额（冻结）

| 项目                 | 托管版（Cloudflare）默认 | 自建版（Go）默认 | 超额处理                                                                     |
| -------------------- | ------------------------ | ---------------- | ---------------------------------------------------------------------------- |
| 每主机并发客户端流   | 16                       | 16               | 新连接以 4429 关闭                                                           |
| 每主机每天转发量     | 2 GiB                    | 不限（0）        | 关掉这台主机的全部客户端流，并以 4429 拒绝新的，直到 00:00 UTC；主机连接保持 |
| 每 IP 每分钟新建连接 | 30                       | 30               | 以 4429 拒绝                                                                 |
| 暂停服务             | `RELAY_DISABLED=true`    | `--disabled`     | 一律 4503（Go 版连接数满时在升级前回 503）                                   |

每 IP 的计数里 IPv6 按 /64 合计（一台机器通常拿得到整个 /64），映射的 IPv4（`::ffff:a.b.c.d`）按 IPv4 计。每天转发量是两个方向 payload 字节的合计（只算主机发过 `ACCEPT` 的流）。Cloudflare 版用 `vars`（`MAX_STREAMS_PER_HOST`、`DAILY_BYTES_PER_HOST`、`MAX_CONN_PER_IP_PER_MIN`、`RELAY_DISABLED`）覆盖默认值，Go 版用同名的命令行参数。两者都有 `GET /healthz`，回 `{"ok":true,"version":"…","disabled":false}`。

### 运维

参考了 [paseo-relay](https://github.com/getpaseo/paseo-relay) 的做法：

- `GET /ready`（两种实现都有，在一致性测试里）：接新连接时回 200 `{"status":"ready"}`；不接时回 503 `{"status":"unready","reason":"…"}`，原因按新连接实际遇到的顺序给一个：`draining`（排空中）、`full`（连接数到了上限）、`disabled`（暂停服务）。负载均衡与健康检查按它摘流量；`/healthz` 只说明进程活着。
- 同时连接数上限（Go 版，`--max-connections`，默认 10000，负数不限）：主机（包括还在认证的）与客户端都算。满了的时候新连接在升级之前回 503 `{"error":"full"}`，已有的连接不受影响。Cloudflare 版由平台扩容，没有这一项。
- 排空与停止（Go 版）：收到 SIGTERM 以后先不接新连接（`/ready` 与新连接回 503 `draining`），等 `--drain-grace`（默认 0），再以 1012 关掉所有主机（包括还在认证、正在登记的）与客户端连接，等关闭帧发出去（最多 1 秒）以后退出。在 Docker 里，`docker stop` 的等待时间要比 `--drain-grace` 多几秒。
- `GET /metrics`（Go 版，`--metrics=false` 关掉）：Prometheus 文本格式，只有聚合的计数，没有 hostId 与 IP：`pt_relay_ready`、`pt_relay_draining`、`pt_relay_connections`、`pt_relay_connection_limit`、`pt_relay_host_connections`、`pt_relay_hosts`、`pt_relay_clients`、`pt_relay_rejected_total{reason}`（`full`、`draining`、`rate_limited`、`disabled`）、`pt_relay_closed_total{code}`（relay 发起的关闭，按关闭码计，在开始关闭时计数，对端不一定还收得到）、`pt_relay_bytes_total{direction}`（`to_host`、`to_client`）、`pt_relay_auth_failures_total`，以及 `go_goroutines`、`go_memstats_heap_inuse_bytes`。

### 实现

- Go 版：`internal/remote/relayserver`，命令 `pt-tools relay serve`（见[命令行](../reference/cli.md#pt-tools-relay-serve)）。
- Cloudflare 版：`relay/cloudflare`，每个 hostId 一个 Durable Object，用 WebSocket Hibernation API 持有连接，主机的 `ping` 由自动应答回 `pong`。每天的用量先预留再用：存储里写「已经用的加一块」（1 MiB，或上限的 1/64），Durable Object 被逐出时读回预留数，只会多算不会少算，还在内存里时一秒后写回准确数。每个 IP 一个 Durable Object 计数，计数写在存储里。relay 的 origin 取 `PUBLIC_URL`，没有设置时按请求的地址推导。
- 一致性测试：`internal/remote/relaytest`（Go 黑盒）。`go test` 对 Go 版跑；`relay/cloudflare/scripts/conformance.sh` 起三个 `wrangler dev` 实例（不同限额）对 Cloudflare 版跑同一套，并用真正的主机端经它走一遍配对与会话。

## 信任模型

- relay 看得到 hostId、两端的 IP、连接时间与流量大小，看不到请求与回应的内容，也冒充不了主机（没有主机的 X25519 私钥完成不了握手）。
- 拿到配对链接的人可以在 10 分钟内配对成一台设备，权限是生成链接时选的。链接只在生成它的网页上出现一次，网页关掉弹窗时窗口作废。
- 设备只能调用 App API v1：现有的 `/api/*`、令牌管理、`/mcp` 在隧道里都不存在；设备做的写操作按 App API 的规则记操作审计，通道是「远程设备」。
- 主机密钥与设备公钥都在 pt-tools 的数据库里，数据库与 `secret.key` 外流时应当轮换主机密钥。
