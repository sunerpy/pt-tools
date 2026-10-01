# 升级与备份

本页介绍如何升级 pt-tools、备份和恢复数据，以及清理日志与暂存文件。

## 升级

数据库结构在启动时自动迁移，升级不需要手动处理数据。升级前建议先按下文备份数据目录。

### Docker

```bash
docker compose pull
docker compose up -d
```

使用 `docker run` 时，先拉取新镜像，再用同样的参数和同一个数据目录重新创建容器：

```bash
docker pull sunerpy/pt-tools:latest
docker stop pt-tools && docker rm pt-tools
docker run -d --name pt-tools -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools -e TZ=Asia/Shanghai \
  sunerpy/pt-tools:latest
```

容器内无法使用 Web 界面中的升级功能，请始终通过更换镜像升级。

### Linux 与 Windows 二进制

Web 界面发现新版本时会在左上角版本号旁提示。点击后可以查看更新说明并开始升级：程序会下载新版本、替换当前的可执行文件，完成后提示「升级完成，请重启应用」。

- 用 systemd 运行时，执行 `sudo systemctl restart pt-tools`。
- 手动运行时，结束当前进程后重新运行 `pt-tools web`。

预发版本（rc 版本）升级前会再次确认，非必要建议等待正式版。也可以重新运行[安装](install.md)中的安装脚本来升级。

## 备份

数据目录（Docker 中是挂载到 `/app/.pt-tools` 的目录，二进制部署是运行用户的 `~/.pt-tools`）里需要备份的是：

| 文件                                            | 内容                                 |
| ----------------------------------------------- | ------------------------------------ |
| `torrents.db`（以及同名的 `-wal`、`-shm` 文件） | 全部配置、站点、订阅、规则和历史记录 |
| `secret.key`                                    | 加密站点 Cookie 和通知凭证的密钥     |

> [!IMPORTANT]
> 数据库和密钥要作为一个整体备份和恢复。只恢复数据库、没有原来的密钥时，已保存的 Cookie 和通知凭证都无法解密。

数据库运行时启用了 WAL 模式，最近的写入可能还在 `torrents.db-wal` 中。最稳妥的做法是停止 pt-tools 后再复制：

```bash
# Docker
docker compose stop
tar -czf pt-tools-backup.tar.gz -C ./data torrents.db secret.key
docker compose start

# 二进制（systemd）
sudo systemctl stop pt-tools
tar -czf pt-tools-backup.tar.gz -C ~/.pt-tools torrents.db secret.key
sudo systemctl start pt-tools
```

停止服务后 WAL 中的内容已经写回数据库文件，这时只需要复制 `torrents.db` 和 `secret.key`。

### 单独备份密钥

密钥也可以导出为一行 base64 文本，保存到密码管理器中：

```bash
pt-tools secret export > secret.b64
# Docker
docker exec pt-tools pt-tools secret export > secret.b64
```

输出内容就是密钥本身，请像保管密码一样保管它。

## 恢复

1. 停止 pt-tools。
2. 把备份的 `torrents.db` 和 `secret.key` 放回数据目录，覆盖现有文件。
3. 启动 pt-tools。

只有 base64 形式的密钥备份时，用 `secret import` 写回（已有密钥文件时需要加 `--force`）：

```bash
pt-tools secret import --force < secret.b64
```

也可以不写文件，通过环境变量 `PT_TOOLS_SECRET_KEY` 提供 base64 形式的密钥，它的优先级高于 `secret.key`。

## 清理日志与暂存文件

日志、暂存的 `.torrent` 文件和旧的配置备份会随时间累积。可以在「规则 → 自动清理」页面底部的「清理工作目录」中先预览、再清理，也可以使用命令行：

```bash
# 预览（默认只列出将删除的文件）
pt-tools clean
# 确认后执行；可以用 --category 限定为 logs、staging、backups 中的一部分
pt-tools clean --confirm
```

数据库和密钥属于受保护的文件，任何清理操作都不会删除它们。命令的完整参数见[命令行](../reference/cli.md)。

Docker 容器自身的标准输出日志不在数据目录中，限制方法见[配置说明](../configuration.md)中「日志管理」一节。
