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

## 构建时内置的值（可选）

| Secret               | 用途                                                                                                                             |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `PT_TOOLS_RELAY_URL` | 托管 relay 的地址（`wss://` 开头），写进二进制与镜像；没设时远程访问没有默认 relay                                               |
| `PT_TOOLS_TMDB_KEY`  | 内置的 TMDB API Key（v3 的 API Key 或 v4 的读取令牌），用户没填自己的 Key 时用；没设时用户要自己填。Key 随二进制分发，能被取出来 |

```bash
gh secret set PT_TOOLS_TMDB_KEY --repo sunerpy/pt-tools < tmdb.key
```

两个值设了时，`Release` workflow 检查 linux-amd64 归档里的 `pt-tools version` 显示 `configured`，否则 job 失败、draft 不发布。细节见 `docs/development.md` 的「默认 relay 地址」与「内置的 TMDB Key」。

## 国内镜像（阿里云 ACR，可选）

| Secret                     | 用途                                                 |
| -------------------------- | ---------------------------------------------------- |
| `ALIYUN_REGISTRY_USERNAME` | 阿里云容器镜像服务的登录用户名                       |
| `ALIYUN_REGISTRY_PASSWORD` | 容器镜像服务的固定密码（在控制台「访问凭证」里设置） |

| 变量          | 用途                                                                                    |
| ------------- | --------------------------------------------------------------------------------------- |
| `PUBLISH_ACR` | 只有精确字符串 `true` 时才推送到 ACR；缺失或其他值时不推                                |
| `ACR_IMAGE`   | 镜像地址，缺省为 `registry.cn-hangzhou.aliyuncs.com/sunerpy/pt-tools`，登录地址取第一段 |

开启前先在 ACR 个人版控制台建好命名空间 `sunerpy`（推送不会自动建），再设置两个 secret 和变量：

```bash
gh secret set ALIYUN_REGISTRY_USERNAME --repo sunerpy/pt-tools
gh secret set ALIYUN_REGISTRY_PASSWORD --repo sunerpy/pt-tools
gh variable set PUBLISH_ACR --repo sunerpy/pt-tools --body true
```

标签与 Docker Hub、GHCR 相同（版本号；稳定版另有 `latest` 等）。推送失败时镜像 job 失败，Release 保持 draft，与 Docker Hub 推送失败的处理一样。

## 文档站

| Secret              | 用途                                                                                                                           |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `FIRLAB_DOCS_TOKEN` | `publish-docs-site.yml` 把 `docs/` 同步到 `sunerpy/firlab` 时使用的 fine-grained PAT，只授权 `sunerpy/firlab` 的 Contents 读写 |

GitHub 没有创建 fine-grained PAT 的 API，只能在 **Settings → Developer settings → Personal access tokens → Fine-grained tokens** 中手工创建：Repository access 只选 `sunerpy/firlab`，Permissions 只给 Contents: Read and write，然后：

```bash
gh secret set FIRLAB_DOCS_TOKEN --repo sunerpy/pt-tools
```

缺少这个 secret 时，`publish-docs-site.yml` 在检出 firlab 一步失败，文档站保持上一次同步的内容；PR 上的 `docs-site.yml` 不读取任何 secret，不受影响。token 到期后重新创建并覆盖同名 secret。

## CRX 私钥一次性初始化

```bash
openssl genrsa -out crx-private.pem 2048
base64 -w 0 crx-private.pem > crx-private.pem.b64
```

把 `crx-private.pem.b64` 的内容保存为 `CRX_PRIVATE_KEY`，并把原始私钥存入受控离线保险库。私钥必须跨版本保持不变；丢失或更换后，既有 `.crx` 安装会被浏览器视为另一个扩展。

## 发布模型

`.github/workflows/release.yml` 在同一次 run 中创建 draft、构建并验证全部承诺产物，最后才公开 Release。不要手工推送 release tag，也不要恢复旧的 `push.tags`、`release:` 或 `workflow_run` 发布链。失败的 draft 通过 `workflow_dispatch` 指定现有 tag 重建。
