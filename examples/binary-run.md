# 本地二进制运行 pt-tools

## 功能特性

- RSS 自动订阅下载免费种子
- 多站点种子搜索（当前 66 个内置站点，见[支持站点列表](../docs/sites.md)）
- 用户信息统计和等级进度追踪
- 支持 qBittorrent 和 Transmission 下载器
- 过滤规则精细化筛选

## 获取二进制

### 方式一：下载预编译版本

前往 [Releases 页面](https://github.com/sunerpy/pt-tools/releases) 下载适合你系统的版本：

| 系统    | 架构  | 文件名                           |
| ------- | ----- | -------------------------------- |
| Linux   | amd64 | `pt-tools-linux-amd64.tar.gz`    |
| Linux   | arm64 | `pt-tools-linux-arm64.tar.gz`    |
| Windows | amd64 | `pt-tools-windows-amd64.exe.zip` |
| Windows | arm64 | `pt-tools-windows-arm64.exe.zip` |

下载归档与同一 Release 的 `checksums.txt` 后先校验再解压：

```bash
# Linux amd64 示例
sha256sum -c checksums.txt --ignore-missing
tar -xzf pt-tools-linux-amd64.tar.gz
chmod +x pt-tools
```

Release 安装说明还提供固定 tag 的 Linux / Windows 安装脚本；脚本会强制校验 SHA-256。

### 方式二：从源码构建

```bash
git clone https://github.com/sunerpy/pt-tools.git
cd pt-tools

# 需要 Go、Node.js 与 pnpm；精确版本见 docs/development.md
make build

# 构建产物
./dist/pt-tools version
```

## 启动服务

```bash
# 使用默认配置启动（监听 0.0.0.0:8080）
./pt-tools web

# 指定监听地址和端口
./pt-tools web --host 0.0.0.0 --port 8080

# 后台运行
nohup ./pt-tools web > pt-tools.log 2>&1 &
```

访问 `http://localhost:8080` 进入 Web 管理界面。

**初始登录账号**：`admin` / `adminadmin`。首次登录后请立即修改密码。

## 数据目录

所有数据存储在 `~/.pt-tools` 目录：

```
~/.pt-tools/
├── secret.key       # 凭证加密密钥，必须单独备份
├── torrents.db      # SQLite 数据库（配置、任务记录、用户信息缓存）
├── downloads/       # 临时种子文件
└── logs/            # 轮转日志
```

> [!IMPORTANT]
> `secret.key` 丢失后，数据库中的 Cookie 等凭证无法解密。请与数据库一起备份，并限制备份文件的访问权限。

如需预创建目录：

```bash
pt-tools config init
```

## 常用命令

```bash
# 启动 Web 服务
pt-tools web

# 初始化配置目录
pt-tools config init

# 查看版本
pt-tools version

# 查看帮助
pt-tools --help
```

## Web 界面功能

1. **用户信息仪表盘**：查看所有站点的上传量、下载量、魔力值、等级进度等
2. **种子搜索**：跨站点搜索种子，支持批量下载和推送到下载器
3. **下载器管理**：配置 qBittorrent/Transmission，设置下载目录
4. **站点管理**：配置站点认证信息和 RSS 订阅
5. **过滤规则**：对 RSS 进行精细化筛选
6. **任务列表**：查看所有下载任务记录

## 内置支持站点

当前内置适配 66 个站点，完整清单、认证方式与适配备注见[支持站点列表](../docs/sites.md)。如需支持其他站点，可使用 PT Tools Helper 采集脱敏页面数据后提交 Issue。

## 设置为系统服务（Linux）

创建 systemd 服务文件 `/etc/systemd/system/pt-tools.service`：

```ini
[Unit]
Description=PT-Tools Service
After=network.target

[Service]
Type=simple
User=your_username
WorkingDirectory=/home/your_username
ExecStart=/usr/local/bin/pt-tools web --host 0.0.0.0 --port 8080
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启用并启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable pt-tools
sudo systemctl start pt-tools

# 查看状态
sudo systemctl status pt-tools

# 查看日志
sudo journalctl -u pt-tools -f
```
