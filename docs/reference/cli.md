# 命令行

本页列出 `pt-tools` 程序的命令和参数。日常使用只需要 `pt-tools web`，其余命令用于备份密钥、清理文件和排查问题。

在 Docker 中运行命令时，在前面加上 `docker exec pt-tools`，例如 `docker exec pt-tools pt-tools version`。

## pt-tools web

启动 Web 界面和后台任务。直接运行 `pt-tools` 不带子命令时，效果相同。

```bash
pt-tools web --host 0.0.0.0 --port 8080
```

| 参数                 | 说明                                                                                                                        | 默认值    |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------- | --------- |
| `--host`             | 监听地址                                                                                                                    | `0.0.0.0` |
| `--port`             | 监听端口                                                                                                                    | `8080`    |
| `--qbit-compat-addr` | [qB 兼容入口](../guide/qbit-compat.md)的监听地址（如 `0.0.0.0:8081`）；不填时看环境变量 `PT_QBIT_COMPAT_ADDR`，都没有就不开 | 不开      |

Docker 镜像通过环境变量 `PT_HOST`、`PT_PORT` 设置这两项，其余环境变量见[安装](../guide/install.md)和[配置说明](../configuration.md)。

## pt-tools mcp

把运行中的 pt-tools 的 [MCP](../guide/mcp.md) 工具接到标准输入输出，给 Claude Desktop 这类只支持 stdio 的 MCP 客户端用。它连上 `--url` 指定的 pt-tools，把那里的工具原样转发；不打开数据库，也不启动调度器，所以可以放在客户端那台机器上运行。

```bash
PT_TOOLS_MCP_TOKEN=ptt_1_xxxxxxxx pt-tools mcp --url http://192.168.1.10:8080
```

| 参数      | 说明                                                                                                               | 默认值                  |
| --------- | ------------------------------------------------------------------------------------------------------------------ | ----------------------- |
| `--url`   | pt-tools 的地址；只给主机与端口时补上 `/mcp`。不填时看环境变量 `PT_TOOLS_MCP_URL`                                  | `http://127.0.0.1:8080` |
| `--token` | 有「MCP 读取」或「MCP 操作」权限的 API 令牌。不填时看环境变量 `PT_TOOLS_MCP_TOKEN`；放在命令行里会出现在进程列表中 | 无                      |

连接成功时在标准错误输出一行提示，标准输出只用于 MCP 协议。

## pt-tools secret

导出或导入用于加密站点 Cookie 和通知凭证的密钥。

```bash
# 把密钥以 base64 文本输出到标准输出
pt-tools secret export

# 从标准输入读取 base64 密钥，写入 ~/.pt-tools/secret.key
pt-tools secret import < secret.b64
```

| 子命令   | 参数      | 说明                                                  |
| -------- | --------- | ----------------------------------------------------- |
| `export` |           | 输出内容就是密钥本身，请妥善保管                      |
| `import` | `--force` | 覆盖已有的 `secret.key`；不加时，文件已存在会拒绝写入 |

导入时会检查密钥长度必须是 32 字节，并以原子方式写入文件。备份与恢复的完整流程见[升级与备份](../guide/upgrade.md)。

## pt-tools clean

清理数据目录中的日志、暂存种子和旧备份。默认只预览，不删除任何文件。

```bash
pt-tools clean                         # 预览全部类别
pt-tools clean --category logs         # 只预览日志
pt-tools clean --confirm               # 执行删除
pt-tools clean --confirm --keep-backups 10
```

| 参数             | 说明                                                                                   | 默认值 |
| ---------------- | -------------------------------------------------------------------------------------- | ------ |
| `--confirm`      | 执行真实删除；不加时只预览                                                             | 关闭   |
| `--category`     | 限定类别，可以是 `logs`、`staging`、`backups` 中的一个或多个，用逗号分隔；留空表示全部 | 全部   |
| `--keep-backups` | `backups` 类别保留最近的份数                                                           | `5`    |
| `--dry-run`      | 预览模式。设为 `false` 但不加 `--confirm` 时，命令会拒绝执行                           | `true` |

三个类别的含义：

- `logs`：已轮转的日志备份。正在写入的日志文件不会被删除。
- `staging`：`downloads/` 下已经推送、已失效或超过保留期的 `.torrent` 文件。
- `backups`：自动生成的历史配置备份，只保留最近 N 份。

`torrents.db`、`secret.key` 和正在写入的日志在任何情况下都不会被删除。

## pt-tools version

输出版本号、构建时间和对应的 Git 提交，提交问题时请附上这段输出。

```bash
pt-tools version
```

## pt-tools completion

生成 Bash 或 Zsh 的命令补全脚本。

```bash
source <(pt-tools completion bash)
pt-tools completion zsh > "${fpath[1]}/_pt-tools"
```

## 较少使用的命令

下面的命令不在 `pt-tools --help` 中显示，只在特定情况下使用。

| 命令                       | 用途                                                                                                                        |
| -------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `pt-tools config init`     | 预先创建 `~/.pt-tools` 和 `downloads` 目录。`pt-tools web` 启动时也会自动创建                                               |
| `pt-tools db fix-timezone` | 修正早期版本把「免费结束时间」按 UTC 存储导致的 8 小时偏差。先用 `--dry-run` 预览，确认后再执行；没有遇到这个问题时不要运行 |

数据库结构由 `pt-tools web` 在启动时自动迁移，不需要手动初始化或升级数据库。
