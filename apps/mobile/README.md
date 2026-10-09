# pt-tools 手机 App

Flutter（3.47.7）写的 Android / iOS App：扫网页上的二维码配对，经直连或 relay 连回 pt-tools，调用 App API v1。用法见 [手机 App](../../docs/guide/mobile-app.md)，协议见 [远程访问协议](../../docs/design/remote-access.md)。

## 目录

| 路径                | 内容                                                                                                      |
| ------------------- | --------------------------------------------------------------------------------------------------------- |
| `lib/src/remote/`   | 远程访问协议：配对链接、Noise IK、隧道帧、会话、WebSocket 传输、连接管理与配对（只依赖 Dart，不依赖界面） |
| `lib/src/app/`      | 界面：路由、Riverpod 状态、各页面                                                                         |
| `lib/l10n/`         | 中英文案，由 `tool/strings.py` 生成，不要手改                                                             |
| `packages/ptt_api/` | App API 的客户端，由 `tool/gen_api.sh` 按 `docs/reference/app-api-v1.yaml` 生成，不要手改                 |
| `test/remote/`      | 协议测试：Noise 公开向量、本协议向量与 Go 录制的整次会话（读 `internal/remote/testdata`）、会话与连接     |
| `test/interop/`     | 跟 Go 的测试主机互通（直连与 relay）                                                                      |

## 常用命令

```bash
flutter pub get
flutter analyze
flutter test

# 跟 Go 的主机互通（仓库根目录起测试主机）
go run ./internal/remote/testhost -info /tmp/testhost.json &
PTT_TESTHOST=/tmp/testhost.json flutter test test/interop

# 改了 App API 的契约或文案以后
tool/gen_api.sh
python3 tool/strings.py && flutter gen-l10n && dart format lib/l10n

flutter build apk --debug
```

CI 在 `.github/workflows/mobile.yml`：生成代码与文案没有漂移、格式、静态检查、单测、互通、debug APK 与 iOS（不签名）构建。
