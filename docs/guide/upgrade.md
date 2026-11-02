# 升级与备份

本页介绍如何升级 pt-tools、备份和恢复数据，以及清理日志与暂存文件。

## 升级

数据库结构在启动时自动迁移，升级不需要手动处理数据。换成新版本以后第一次启动时，pt-tools 会在迁移数据库以前先自动备份一次（见下文[备份](#备份)），所以升级前不用再手动备份；想多留一份，也可以先在网页上点一次「立即备份」。

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

Web 界面发现新版本时会在左上角版本号旁提示。点击后可以查看更新说明并开始升级：程序会下载新版本，按同一版本的 `checksums.txt` 核对 SHA-256，一致后才替换当前的可执行文件，完成后提示「升级完成，请重启应用」。校验不一致或该版本没有 `checksums.txt` 时，升级失败，当前程序不变。

- 用 systemd 运行时，执行 `sudo systemctl restart pt-tools`。
- 手动运行时，结束当前进程后重新运行 `pt-tools web`。

预发版本（rc 版本）升级前会再次确认，非必要建议等待正式版。也可以重新运行[安装](install.md)中的安装脚本来升级。

## 备份

一份备份是一个 `pt-tools-backup-<时间>-<原因>.tar.gz` 文件，里面是数据库的快照（全部配置、站点、订阅、规则和历史记录）和一个说明文件，在「带上密钥」打开时还有 `secret.key`（加密站点 Cookie 与通知凭证的密钥）。快照在服务运行时也能做，不用停 pt-tools。

备份在这几种时候做：

| 原因       | 什么时候                                                                               | 保留                          |
| ---------- | -------------------------------------------------------------------------------------- | ----------------------------- |
| 定时备份   | 每天在设定的时刻（默认 04:30）做一次；那时 pt-tools 没开着的话，下次启动以后补做当天的 | 最新 7 份（可以改成 1–60 份） |
| 升级前备份 | 换成新版本以后第一次启动时，在迁移数据库以前                                           | 最新 3 份                     |
| 手动备份   | 在网页上点「立即备份」                                                                 | 不自动删                      |
| 命令行备份 | 执行 `pt-tools db backup`                                                              | 不自动删                      |

设置在「全局设置 → 备份」里：

- **每天定时备份**：默认打开。定时备份没做成时会发一条「备份失败」通知（发给所有启用的通知通道，每天最多一条），原因也显示在「备份与恢复」面板上。升级前备份不受这个开关影响。
- **备份时刻**、**保留份数**：保留份数只算定时备份。
- **备份目录**：留空时是数据目录下的 `backups`。要填绝对路径，保存时会试着写一个文件，确认 pt-tools 有权限写。放到另一块盘或网络存储上，数据盘坏了备份也还在。Docker 部署时这个目录要挂载进容器。
- **带上密钥**：默认打开。带上密钥的备份换了机器、或者原来的密钥丢了也能直接恢复，但拿到它的人就能解开里面保存的 Cookie 与通知凭证，要和数据库一样妥善保管。关掉以后备份里只有数据库，恢复时要有做备份时的密钥。

同一页下面的「备份与恢复」面板列出备份目录里的备份，可以下载、恢复、删除，也可以「立即备份」。

> [!IMPORTANT]
> 数据库和密钥要作为一个整体备份和恢复。只恢复数据库、没有原来的密钥时，已保存的 Cookie 和通知凭证都无法解密。

### 命令行备份

```bash
# 放进备份设置里的目录（默认数据目录下的 backups）
pt-tools db backup
# 放到别的目录，或者写成指定的文件（已有同名文件时不覆盖）
pt-tools db backup --output /mnt/backup/
# 不带密钥
pt-tools db backup --no-secret

# Docker
docker exec pt-tools pt-tools db backup
```

服务在运行时也能用这个命令：它只读打开数据库，不改动任何数据。带不带密钥默认按备份设置。

也可以停掉 pt-tools 后自己复制数据目录里的 `torrents.db` 与 `secret.key`：停止服务以后，`torrents.db-wal` 里的内容已经写回数据库文件。

### 单独备份密钥

密钥也可以导出为一行 base64 文本，保存到密码管理器中：

```bash
pt-tools secret export > secret.b64
# Docker
docker exec pt-tools pt-tools secret export > secret.b64
```

输出内容就是密钥本身，请像保管密码一样保管它。备份关掉了「带上密钥」时，恢复要用到它。

## 恢复

恢复分两步：先校验一份备份并放进待恢复，**重启 pt-tools 以后**才换上。重启以前可以取消，现在的数据不会被改动。

- **网页**：在「全局设置 → 备份与恢复」里点一份备份的「恢复」，或者「上传备份恢复」一个备份文件（最大 2 GiB）。校验通过以后面板顶上显示「已放好待恢复的备份」，重启 pt-tools 生效，重启以前可以点「取消恢复」。
- **命令行**：

  ```bash
  pt-tools db restore --input /mnt/backup/pt-tools-backup-20261102-043000-scheduled.tar.gz
  pt-tools db restore --cancel   # 取消尚未生效的恢复

  # Docker：备份文件要在容器里看得到，例如放进数据目录
  docker exec pt-tools pt-tools db restore --input /app/.pt-tools/backups/pt-tools-backup-20261102-043000-scheduled.tar.gz
  docker restart pt-tools
  ```

重启时，pt-tools 在打开数据库以前把现在的 `torrents.db`（要换密钥时还有 `secret.key`）挪进数据目录下的 `backups/pre-restore-<时间>/`，换上备份里的文件，再用当前生效的密钥解一遍备份里保存的凭证。任何一步出错，都把原来的文件放回原位、按原来的数据启动，面板上写明没换上的原因。确认恢复没有问题以后，`pre-restore-<时间>` 文件夹可以删掉。

密钥按这几条处理，校验不通过的备份不会放进待恢复：

- 用环境变量 `PT_TOOLS_SECRET_KEY` 提供密钥时，恢复以后仍用它，不动 `secret.key`。环境变量的密钥解不开备份里保存的凭证时拒绝恢复。
- 备份带了密钥：恢复以后用备份里的密钥。原来的 `secret.key` 在放好待恢复以后丢了也不要紧，重启时会换上备份里的这把。
- 备份没带密钥：现在的密钥解得开备份里保存的凭证（或者备份里没有加密保存的凭证）才允许恢复，`secret.key` 不变。解不开时，先用 `pt-tools secret import` 导入做备份时的密钥再恢复。

备份是更新版本的 pt-tools 做的（数据库版本更高）时不能恢复，先升级 pt-tools。恢复一份旧版本的备份时，换上以后照常自动迁移数据库结构（迁移以前同样会先做一份升级前备份）。

### 不用恢复功能

要回到旧版本（例如从预览版回到 0.48.0），旧版本里没有恢复功能：停掉 pt-tools，解开升级前备份（`pt-tools-backup-<时间>-pre-upgrade.tar.gz`），把里面的 `torrents.db` 与 `secret.key` 放回数据目录（同时删掉数据目录里的 `torrents.db-wal`、`torrents.db-shm`），再启动旧版本。

只有 base64 形式的密钥备份时，用 `secret import` 写回（已有密钥文件时需要加 `--force`）：

```bash
pt-tools secret import --force < secret.b64
```

也可以不写文件，通过环境变量 `PT_TOOLS_SECRET_KEY` 提供 base64 形式的密钥，它的优先级高于 `secret.key`。

启动时 pt-tools 会检查密钥，以下两种情况拒绝启动，日志里写明原因：

- `secret.key` 存在但读不出或格式不对（应为 64 位十六进制）。pt-tools 不会覆盖它，请换回正确的文件，或用 `secret import` 导入备份。
- `secret.key` 不存在，数据库里却已有加密保存的 Cookie 或通知凭证，通常是只恢复了数据库或数据目录没挂载对。把原来的 `secret.key` 放回去再启动即可。确认原密钥已经丢失时，设置环境变量 `PT_TOOLS_ACCEPT_NEW_SECRET_KEY=1` 启动一次，pt-tools 会启用新密钥，之后重新填写站点 Cookie 和通知凭证。

## 清理日志与暂存文件

日志、暂存的 `.torrent` 文件和旧的配置备份会随时间累积。可以在「规则 → 自动清理」页面底部的「清理工作目录」中先预览、再清理，也可以使用命令行：

```bash
# 预览（默认只列出将删除的文件）
pt-tools clean
# 确认后执行；可以用 --category 限定为 logs、staging、backups 中的一部分
pt-tools clean --confirm
```

数据库和密钥属于受保护的文件，任何清理操作都不会删除它们。`backups` 类别只删迁移时留下的旧配置备份（`.json` 文件），备份文件（`.tar.gz`）与 `pre-restore-<时间>` 文件夹不会被它删除。命令的完整参数见[命令行](../reference/cli.md)。

Docker 容器自身的标准输出日志不在数据目录中，限制方法见[配置说明](../configuration.md)中「日志管理」一节。
