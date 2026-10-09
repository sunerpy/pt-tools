// Package remote 是远程访问（路线图 M15）的协议与主机端：手机 App 扫码配对以后，经直连 WebSocket 或 relay
// 调用这台 pt-tools 的 App API v1。线格式（v1，大小冻结）写在 docs/design/remote-access.md，
// 测试向量在 testdata/，Go 与 App（Dart）两端共用。
//
// 分层：
//   - 主机密钥（identity.go）：Ed25519 向 relay 证明身份，hostId 由它推导；X25519 是 Noise 的静态密钥。
//   - 加密（noise.go）：Noise_IK_25519_ChaChaPoly_BLAKE2s，设备是发起方，prologue 绑定协议版本与 hostId。
//   - 隧道帧（frame.go）：每条 Noise 传输消息的明文是一个帧，一条会话里按请求编号复用。
//   - relay 外层帧（outer.go）：主机与 relay 之间的连接里按流编号转发 Noise 消息，relay 看不到明文。
//   - 配对（pairing.go）：一次性配对密钥，只在配对窗口里接受不认识的设备公钥。
//   - 主机（host.go、session.go、relay.go）：直连入口、relay 客户端、会话登记与分发；设备会话里只放行 /api/app/v1/*。
//   - 设备端（client.go）：Go 实现，给测试、验收与互通测试用。
package remote
