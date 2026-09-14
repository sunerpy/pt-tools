# GitHub Actions 发布配置

在仓库 **Settings → Secrets and variables → Actions** 中配置以下值。`GITHUB_TOKEN` 由 GitHub 自动提供，不需要手工创建 PAT。

## 必需 Secrets

| Secret            | 用途                                                                                         |
| ----------------- | -------------------------------------------------------------------------------------------- |
| `CRX_PRIVATE_KEY` | Base64 编码的稳定 RSA 私钥，用于签名 `pt-tools-helper.crx`；缺失时 Release 保持 draft 并失败 |
| `DOCKER_USERNAME` | Docker Hub 用户名                                                                            |
| `DOCKER_PASSWORD` | Docker Hub access token，不要使用账户密码                                                    |

## 发布后副作用

| Secret               | 用途                          |
| -------------------- | ----------------------------- |
| `TELEGRAM_BOT_TOKEN` | Telegram 发布公告机器人 token |
| `TELEGRAM_CHAT_ID`   | 公告目标 chat ID              |
| `EDGE_PRODUCT_ID`    | Edge Add-ons 产品 ID          |
| `EDGE_CLIENT_ID`     | Edge Publish API 客户端 ID    |
| `EDGE_API_KEY`       | Edge Publish API 密钥         |

GitHub Release、二进制、扩展、校验和与容器镜像属于发布门禁的一部分；Telegram 公告和 Edge 商店提交在 Release 公开后运行，不会把已经验证的 Release 回滚。

仓库变量可在恢复或验收发布链时关闭对外副作用：

- `ANNOUNCE_TELEGRAM=false`：跳过 Telegram 公告。
- `PUBLISH_EDGE=false`：跳过 Edge 商店提交。

变量缺失或不是精确字符串 `false` 时保持默认启用。

## CRX 私钥一次性初始化

```bash
openssl genrsa -out crx-private.pem 2048
base64 -w 0 crx-private.pem > crx-private.pem.b64
```

把 `crx-private.pem.b64` 的内容保存为 `CRX_PRIVATE_KEY`，并把原始私钥存入受控离线保险库。私钥必须跨版本保持不变；丢失或更换后，既有 `.crx` 安装会被浏览器视为另一个扩展。

## 发布模型

`.github/workflows/release.yml` 在同一次 run 中创建 draft、构建并验证全部承诺产物，最后才公开 Release。不要手工推送 release tag，也不要恢复旧的 `push.tags`、`release:` 或 `workflow_run` 发布链。失败的 draft 通过 `workflow_dispatch` 指定现有 tag 重建。
