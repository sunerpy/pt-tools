# 本地二进制运行（Web 管理）

## 获取二进制

- 从 Release 下载适合平台的二进制并放入 `PATH`
- 或本地构建：

```bash
git clone https://github.com/sunerpy/pt-tools.git
cd pt-tools
go build -o pt-tools .
sudo mv pt-tools /usr/local/bin/pt-tools
```

## 启动 Web 管理界面

```bash
pt-tools
# 或指定地址与端口：
pt-tools web --host 0.0.0.0 --port 8080
```

首次运行进入 Web 页面完成初始化（数据库与全局设置）。

## 数据持久化路径

- 数据库：`~/.pt-tools/torrents.db`
- 下载目录：默认位于 `~/.pt-tools/downloads`（可在 Web 全局设置中修改）

如需预创建目录：

```bash
pt-tools config init
```