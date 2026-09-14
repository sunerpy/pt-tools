<!-- pt-tools-install-footer -->

---

## Installation

### Using Docker (Recommended)

```bash
docker pull sunerpy/pt-tools:${TAG_NAME}
# 或从 GHCR 拉取（带 artifact attestation）
docker pull ghcr.io/sunerpy/pt-tools:${TAG_NAME}
```

### From Binary

| Platform | Architecture | File                             | Latest Link                                                                                             |
| -------- | ------------ | -------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Linux    | x86_64       | `pt-tools-linux-amd64.tar.gz`    | [Download](https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-linux-amd64.tar.gz)    |
| Linux    | aarch64      | `pt-tools-linux-arm64.tar.gz`    | [Download](https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-linux-arm64.tar.gz)    |
| Windows  | x86_64       | `pt-tools-windows-amd64.exe.zip` | [Download](https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-windows-amd64.exe.zip) |
| Windows  | aarch64      | `pt-tools-windows-arm64.exe.zip` | [Download](https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-windows-arm64.exe.zip) |

> 不提供 macOS 产物。

### Install Script

```bash
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/${TAG_NAME}/scripts/install.sh \
  | TOOL_VERSION=${TAG_NAME} sh
```

```powershell
$env:TOOL_VERSION = "${TAG_NAME}"
irm https://raw.githubusercontent.com/sunerpy/pt-tools/${TAG_NAME}/scripts/install.ps1 | iex
```

安装脚本会下载同一 tag 的 `checksums.txt`，校验 SHA-256 后才解压；摘要不符即中止。

### Browser Extension

| Format                        | File                  | Latest Link                                                                                  |
| ----------------------------- | --------------------- | -------------------------------------------------------------------------------------------- |
| Edge / Chrome (zip, unpacked) | `pt-tools-helper.zip` | [Download](https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-helper.zip) |
| Chrome (signed crx)           | `pt-tools-helper.crx` | [Download](https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-helper.crx) |

> 扩展 manifest 版本采用四段映射：稳定版为 `<M.m.p>.65535`，RC 为 `<M.m.p>.<N>`，
> 以保证浏览器可从任意 RC 单调升级到稳定版。

### Verify Integrity

```bash
# 校验和
sha256sum -c checksums.txt --ignore-missing

# 供应链证明（验证已下载的具体资产；每个 checksums.txt 条目均被证明）
gh attestation verify pt-tools-linux-amd64.tar.gz --repo sunerpy/pt-tools \
  --signer-workflow sunerpy/pt-tools/.github/workflows/release.yml \
  --deny-self-hosted-runners
```

### Docker Images

- `sunerpy/pt-tools:${TAG_NAME}`
- `sunerpy/pt-tools:latest`
- `ghcr.io/sunerpy/pt-tools:${TAG_NAME}`

<!-- /pt-tools-install-footer -->
