# pt-tools

`pt-tools` 是一个专为PT站点设计的命令行工具，旨在通过rss订阅链接帮助用户自动化处理与 PT 站点相关的任务，例如筛选免费种子、与 PT 站点 API 交互，以及管理数据库操作。

## 功能特性

* **灵活的运行模式**：支持单次执行和持续监控模式。
* **Shell 补全支持**：提供 Bash 和 Zsh 的补全功能。
* **可定制的配置**：轻松初始化和自定义配置文件。
* **数据库管理**：支持数据库初始化、备份和查询等操作。

---

## 安装方法

### 从源码构建

1. 克隆代码库：

    ```bash
    git clone https://github.com/sunerpy/pt-tools.git
    cd pt-tools
    ```
2. 构建二进制文件：

    ```bash
    go build -o pt-tools .
    ```
3. 将二进制文件移动到系统 `PATH`：

    ```bash
    mv pt-tools /usr/local/bin/pt-tools
    ```

### 一键部署

提供了一个自动化脚本，帮助用户快速下载和安装` pt-tools`。

```json
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/download.sh | bash
```

### 下载最新 Release

1. 前往 [pt-tools Release 页面](https://github.com/sunerpy/pt-tools/releases)，下载适合你系统的最新版本二进制文件。
2. 解压下载的压缩包：

    ```bash
    tar -xvzf pt-tools-linux-amd64.tar.gz
    ```

    或：

    ```bash
    unzip pt-tools-windows-amd64.exe.zip
    ```
3. 将解压后的二进制文件移动到系统 `PATH`：

    ```bash
    mv pt-tools /usr/local/bin/
    ```
4. 验证安装：

    ```bash
    pt-tools version
    ```

---

## 快速开始

### 初始化配置

1. 初始化默认配置文件：

    ```bash
    pt-tools config init
    ```

    运行此命令后，将生成默认配置文件，路径为 `$HOME/.pt-tools/config.toml`。
2. 打开配置文件修改选项：

    ```bash
    vim $HOME/.pt-tools/config.toml
    ```

    或使用其他文本编辑器。根据实际需求调整配置选项。

### 单次运行

使用以下命令进行一次性任务运行：

```bash
pt-tools run
```

### 持续运行

如果需要持续执行任务，可以使用 `-m persistent` 模式：

```bash
pt-tools run -m persistent
```

---

通过以上简单步骤，你可以快速配置并运行 `pt-tools`。

---

## 使用说明

### 基本命令结构

```plaintext
pt-tools [命令] [选项]
```

### 可用命令

| 命令 | 描述                        |
| ------ | ----------------------------- |
| `completion`     | 生成 Bash 和 Zsh 的补全脚本 |
| `config`     | 管理配置文件                |
| `db`     | 执行数据库相关操作          |
| `run`     | 以单次或持续模式运行工具    |
| `task`     | 管理计划任务（开发中）      |
| `help`     | 显示帮助信息                |

### 全局选项

| 选项 | 描述                   |
| ------ | ------------------------ |
| `-c, --config`     | 配置文件路径（默认:`$HOME/.pt-tools/config.toml`）  |
| `-h, --help`     | 显示命令的帮助信息     |
| `-t, --toggle`     | 切换某些功能（占位符） |

---

## 命令详解

### `run`

`run` 命令允许用户以不同的模式运行工具。

**示例**：

1. 单次运行模式：

    ```bash
    pt-tools run --mode=single
    ```

    工具运行一次后退出。
2. 持续运行模式：

    ```bash
    pt-tools run --mode=persistent
    ```

    工具持续运行，按设定的时间间隔重复执行任务。

**用法**：

```plaintext
pt-tools run [选项]
```

| 选项 | 描述                     |
| ------ | -------------------------- |
| `-m, --mode`     | 运行模式：`single` 或 `persistent`（默认: `single`） |
| `-h, --help`     | 显示 `run` 命令的帮助信息     |

---

### `completion`

生成用于 Bash 和 Zsh 的 Shell 补全脚本。

**示例**：

* 对于 Bash：

  ```bash
  pt-tools completion bash
  source <(pt-tools completion bash)
  ```
* 对于 Zsh：

  ```bash
  pt-tools completion zsh
  source <(pt-tools completion zsh)
  ```

**用法**：

```plaintext
pt-tools completion [bash|zsh]
```

---

### `config`

管理工具的配置文件。

**示例**：

```bash
pt-tools config init
```

在 `$HOME/.pt-tools/config.toml` 路径下初始化默认配置文件。

---

### `db`

执行数据库相关操作，例如初始化、备份或查询。

**示例**：

```bash
pt-tools db init
```

---

## 配置说明

`pt-tools` 的配置优先级从高到低如下：

1. 环境变量

    * 环境变量的优先级高于配置文件。即使使用了 `--config` 指定配置文件，环境变量的值仍然会覆盖文件中的相应配置项。
    * 环境变量的名称需与配置键名一致（不区分大小写）。
    * 示例：

      ```bash
      export DEFAULT_INTERVAL=10
      ```
2. 指定的配置文件

    * 如果使用 `-c` 或者 `--config` 显式指定配置文件路径，`pt-tools` 将读取该文件，并使用文件中的值。
    * 示例：

      ```bash
      pt-tools --config /path/to/config.toml
      ```
3. 默认的配置文件

    * 如果未显式指定 `--config`，工具会在默认路径（如 `$HOME/.pt-tools/config.toml`）查找配置文件，并使用文件中的值。
4. 程序内置默认值

    * 如果命令行参数、环境变量和配置文件中均未提供某项配置，且不存在 `$HOME/.pt-tools/config.toml`， `pt-tools` 将创建初始化的默认配置文件。

---

---

### 注意事项

> 1. 环境变量优先级高于配置文件：
>
>     * 即使显式指定了 `--config`，环境变量中设置的值仍然会覆盖配置文件中的值。
>     * 如果不希望环境变量覆盖配置文件，请确保未设置对应的环境变量。
> 2. 命令行标志始终优先于环境变量和配置文件，用于动态覆盖配置项。

### 配置说明

#### 全局配置 `[global]`

| 配置项 | 类型   | 默认值 | 描述                               |
| -------- | -------- | -------- | ------------------------------------ |
| `default_interval`       | 字符串 | `"5m"`       | 默认的任务间隔时间，格式支持`"5m"`、`"10m"`等。 |
| `default_enabled`       | 布尔值 | `true`       | 默认是否启用所有站点任务。         |
| `download_dir`       | 字符串 | `"downloads"`       | 种子文件下载的默认目录。           |
| `download_limit_enabled`       | 布尔值 | `true`       | 是否启用下载速度限制。             |
| `download_speed_limit`       | 整数   | `20`       | 下载速度限制，单位为 MB/s。        |

#### qBittorrent 配置 `[qbit]`

| 配置项 | 类型   | 默认值 | 描述                             |
| -------- | -------- | -------- | ---------------------------------- |
| `enabled`       | 布尔值 | `true`       | 是否启用 qBittorrent 客户端。    |
| `url`       | 字符串 | `"http://xxx.xxx.xxx:8080"`       | qBittorrent Web UI 的 URL 地址。 |
| `user`       | 字符串 | `"admin"`       | qBittorrent 登录用户名。         |
| `password`       | 字符串 | `"adminadmin"`       | qBittorrent 登录密码。           |

#### 站点配置 `[sites]`

每个站点通过 `[sites.<站点名>]` 配置，支持多个站点。

##### 站点通用配置

| 配置项 | 类型   | 默认值         | 描述                   |
| -------- | -------- | ---------------- | ------------------------ |
| `name`       | 字符串 | 必填           | 站点名称，用于标识。   |
| `enabled`       | 布尔值 | `false`               | 是否启用该站点的任务。 |
| `auth_method`       | 字符串 | `"api_key"`或`"cookie"`             | 认证方式，可选`"api_key"`或`"cookie"`。     |
| `api_key`       | 字符串 | 必填（如果`auth_method`为`"api_key"`） | API 密钥。             |
| `api_url`       | 字符串 | 必填（如果`auth_method`为`"api_key"`） | API 地址。             |
| `cookie`       | 字符串 | 必填（如果`auth_method`为`"cookie"`） | Cookie 值。            |

##### RSS 配置

每个站点的 RSS 配置通过 `[[sites.<站点名>.rss]]` 定义，支持多个 RSS 配置。

| 配置项 | 类型   | 默认值 | 描述                                         |
| -------- | -------- | -------- | ---------------------------------------------- |
| `name`       | 字符串 | 必填   | RSS 订阅的名称。                             |
| `url`       | 字符串 | 必填   | RSS 订阅链接地址。                           |
| `category`       | 字符串 | 必填   | RSS 订阅分类，用于标记种子类型（例如：`"Tv"`、`"Mv"`）。 |
| `tag`       | 字符串 | 必填   | 为任务添加的标记，用于区分不同任务来源。     |
| `interval_minutes`       | 整数   | 必填   | 任务执行间隔时间，单位为分钟。               |
| `download_sub_path`       | 字符串 | 必填   | 下载的种子文件存储的子目录路径（相对于`download_dir`）。   |

---

### 示例

#### 全局配置

```toml
[global]
default_interval = "5m"
default_enabled = true
download_dir = "downloads"
download_limit_enabled = true
download_speed_limit = 20
```

#### qBittorrent 配置

```toml
[qbit]
enabled = true
url = "http://127.0.0.1:8080"
user = "admin"
password = "adminadmin"
```

#### 站点配置

```toml
[sites.mteam]
name = "MTeam"
enabled = true
auth_method = "api_key"
api_key = "your_api_key"
api_url = "https://api.m-team.xxx/api"

[[sites.mteam.rss]]
name = "RSS1"
url = "https://rss.m-team.xxx/api/rss/xxx"
category = "Tv"
tag = "MT"
interval_minutes = 15
download_sub_path = "mteam/tvs"
```

---

## 高级使用

### 启用 Shell 补全

#### Bash

1. 生成 Bash 补全脚本：

    ```bash
    pt-tools completion bash > /etc/bash_completion.d/pt-tools
    ```
2. 重新加载 Shell 或直接加载补全脚本：

    ```bash
    source /etc/bash_completion.d/pt-tools
    ```

#### Zsh

1. 确保已启用补全功能：

    ```bash
    echo "autoload -U compinit; compinit" >> ~/.zshrc
    ```
2. 生成 Zsh 补全脚本：

    ```bash
    pt-tools completion zsh > "${fpath[1]}/_pt-tools"
    ```
3. 启动新 Shell 会话。

---

## 贡献

欢迎贡献代码！请通过 [GitHub 仓库](https://github.com/sunerpy/pt-tools) 提交问题或拉取请求。

---

## 许可证

本项目基于 [MIT 许可证](https://github.com/sunerpy/pt-tools?tab=MIT-1-ov-file) 进行许可。