
# pt-tools

`pt-tools` 是一个专为私人追踪器 (PT) 站点设计的命令行工具，旨在帮助用户自动化处理与 PT 站点相关的任务，例如筛选免费种子、与 PT 站点 API 交互，以及管理数据库操作。

## 功能特性

- **灵活的运行模式**：支持单次执行和持续监控模式。
- **Shell 补全支持**：提供 Bash 和 Zsh 的补全功能。
- **可定制的配置**：轻松初始化和自定义配置文件。
- **数据库管理**：支持数据库初始化、备份和查询等操作。

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
   mv pt-tools /usr/local/bin/
   ```

---

## 使用说明

### 基本命令结构

```plaintext
pt-tools [命令] [选项]
```

### 可用命令

| 命令          | 描述                     |
|---------------|--------------------------|
| `completion`  | 生成 Bash 和 Zsh 的补全脚本 |
| `config`      | 管理配置文件             |
| `db`          | 执行数据库相关操作       |
| `run`         | 以单次或持续模式运行工具  |
| `task`        | 管理计划任务（开发中）    |
| `help`        | 显示帮助信息             |

### 全局选项

| 选项                | 描述                                              |
|---------------------|---------------------------------------------------|
| `-c, --config`      | 配置文件路径（默认: `$HOME/.pt-tools/config.toml`） |
| `-h, --help`        | 显示命令的帮助信息                                |
| `-t, --toggle`      | 切换某些功能（占位符）                            |

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

| 选项               | 描述                             |
|--------------------|----------------------------------|
| `-m, --mode`       | 运行模式：`single` 或 `persistent`（默认: `single`） |
| `-h, --help`       | 显示 `run` 命令的帮助信息        |

---

### `completion`

生成用于 Bash 和 Zsh 的 Shell 补全脚本。

**示例**：

- 对于 Bash：
  ```bash
  pt-tools completion bash
  source <(pt-tools completion bash)
  ```

- 对于 Zsh：
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

    * 如果命令行参数、环境变量和配置文件中均未提供某项配置，且不存在`$HOME/.pt-tools/config.toml`， `pt-tools` 将创建初始化的默认配置文件。

---

---

### 注意事项

> 1. 环境变量优先级高于配置文件：
>
>     * 即使显式指定了 `--config`，环境变量中设置的值仍然会覆盖配置文件中的值。
>     * 如果不希望环境变量覆盖配置文件，请确保未设置对应的环境变量。
> 2. 命令行标志始终优先于环境变量和配置文件，用于动态覆盖配置项。

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

欢迎贡献代码！请通过 [GitHub 仓库](https://github.com/your-repo/pt-tools) 提交问题或拉取请求。

---

## 许可证

本项目基于 [MIT 许可证](LICENSE) 进行许可。
