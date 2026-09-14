<!-- pt-tools-install-footer -->

---

> ⚠️ **这是预发版，不建议在生产环境使用。**
> 如遇问题请在 [GitHub Issues](https://github.com/sunerpy/pt-tools/issues) 反馈，
> 测试通过后将合入正式版。

## Installation (Prerelease)

### Using Docker

```bash
# 固定版本（预发版不会更新 :latest）
docker pull sunerpy/pt-tools:${TAG_NAME}
docker pull ghcr.io/sunerpy/pt-tools:${TAG_NAME}
```

> 预发版**不会**更新 `:latest` 标签。生产环境请继续使用正式版。

### From Binary

预发版**不提供** `releases/latest/download/...` 链接，请从本页 Assets 区下载：

| Platform | Architecture | File                             |
| -------- | ------------ | -------------------------------- |
| Linux    | x86_64       | `pt-tools-linux-amd64.tar.gz`    |
| Linux    | aarch64      | `pt-tools-linux-arm64.tar.gz`    |
| Windows  | x86_64       | `pt-tools-windows-amd64.exe.zip` |
| Windows  | aarch64      | `pt-tools-windows-arm64.exe.zip` |

> 不提供 macOS 产物。

### Install Script (pinned)

```bash
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/${TAG_NAME}/scripts/install.sh \
  | TOOL_VERSION=${TAG_NAME} sh
```

### Browser Extension

预发版仍产出 `pt-tools-helper.zip` 与 `pt-tools-helper.crx`，但**不会上传到
Edge Add-ons 商店**。其 manifest 版本为 `<M.m.p>.<N>`（N 为 RC 序号），
低于稳定版的 `<M.m.p>.65535`，因此安装预发版后仍可正常升级到稳定版。

### Verify Integrity

```bash
sha256sum -c checksums.txt --ignore-missing

gh attestation verify pt-tools-linux-amd64.tar.gz --repo sunerpy/pt-tools \
  --signer-workflow sunerpy/pt-tools/.github/workflows/release.yml \
  --deny-self-hosted-runners
```

<!-- /pt-tools-install-footer -->
