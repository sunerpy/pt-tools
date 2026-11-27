# 飞牛 fnOS 应用包（原生）

这里是 pt-tools 的飞牛 fnOS 原生应用包：发版时 `Release` workflow 的 `fnos-fpk` job 用 `build-fnos-fpk.sh` 把 Linux 二进制打成 `pt-tools-fnos-x86.fpk` 和 `pt-tools-fnos-arm.fpk`，挂在 Release 上。用户在飞牛「应用中心 → 手动安装」里上传 fpk 安装或升级，安装后的用法写在[安装文档](../../../docs/guide/install.md)里。

`deploy/fnos/apps/` 是另一份 Docker 版的定义，给第三方应用中心 fnos-apps 用，和这里无关；两者的 appname 都是 `pt-tools`，不要同时装。

## 文件

| 文件                                  | 作用                                                                       |
| ------------------------------------- | -------------------------------------------------------------------------- |
| `manifest.in`                         | manifest 模板，`@VERSION@`、`@PLATFORM@` 由构建替换，`checksum` 由构建追加 |
| `cmd/main`                            | 运行控制：`start`、`stop`、`status`                                        |
| `cmd/*_init`、`cmd/*_callback`        | 生命周期钩子（见下）                                                       |
| `config/privilege`、`config/resource` | 以应用用户 `pt-tools` 运行；不声明额外资源                                 |
| `wizard/install`、`wizard/uninstall`  | 安装时的提示；卸载时选保留还是清除数据                                     |
| `ui/config`                           | 桌面图标：新标签页打开 `http://<NAS>:8080/`                                |
| `build-fnos-fpk.sh`                   | 打包脚本，本地与发布流程共用                                               |
| `test-fnos-fpk.sh`                    | 测试脚本，CI 的 `fnOS Package` job 跑它                                    |
| `testdata/fake-pt-tools/`             | 测试用的假 pt-tools（go 工具链按 testdata 规则不编它）                     |

图标在打包时从 `deploy/fnos/apps/pt-tools/fnos/` 复制（1.0 的标志），这里不放第二份。

## 规范出处

- 飞牛开发者文档 <https://developer.fnnas.com/docs/>：core-concepts 下的 framework、manifest、environment-variables、privilege、resource、app-entry、wizard，cli 下的 fnpack、appcenter-cli。2026-11-23 按每天同步的镜像 [ckcoding/fnnas-docs](https://github.com/ckcoding/fnnas-docs) 提交 `55565f6`（2026-11-19）读取。
- 官方打包工具 fnpack 1.3.3（<https://static2.fnnas.com/fnpack/fnpack-1.3.3-linux-amd64>，SHA-256 `1464d6aa9b995ebecb263c5ec60f0f4a7cbf45e0ce887836755fd65abd647d1e`）。包的结构照它的产出，打包时不下载它。

## 在飞牛上实测的行为

2026-11-23 在 fnOS 1.2.1102（x86）上装了一个探针应用，每个钩子记录运行用户、环境变量和数据目录，结论：

- fpk 是 tar.gz：`app.tgz`（app/ 的内容加上 config/）、`cmd/`、`config/`、`wizard/`、`manifest`（末尾 `checksum` 是 app.tgz 的 MD5）、`ICON.PNG`、`ICON_256.PNG`。
- 钩子和 `main` 都以应用用户运行；工作目录 `/`；umask 0022；环境里没有 `HOME`、`PATH`、`LANG`、`TZ`。所以 `cmd/main` 自己设 `PATH`，并设 `HOME=$TRIM_PKGVAR`（pt-tools 的数据目录取 `$HOME/.pt-tools`）。
- `TRIM_PKGVAR` 是 `/vol<N>/@appdata/pt-tools`：install_init 时还不存在，install_callback 时已建好。`TRIM_SERVICE_PORT` 等于 manifest 的 `service_port`。
- 飞牛每 30 秒调一次 `main status`，start 之前先调 status。
- 升级顺序：旧版 `main stop`（`TRIM_APP_STATUS=UPGRADE`）→ 新版 upgrade_init → **旧版 uninstall_init、uninstall_callback（`TRIM_APP_STATUS=UPGRADE`，没有 `wizard_delete_data`）** → 新版 install_init、install_callback → upgrade_callback → 启动。`TRIM_PKGVAR` 的内容保留。
- 卸载：uninstall_init、uninstall_callback 的 `TRIM_APP_STATUS=UNINSTALL`，`wizard_delete_data` 是卸载向导里选的值。卸载后安装目录删除，`@appdata`、`@appconf`、`@apphome` 和应用用户都留着，飞牛自己不删数据。
- 手动安装没有飞牛签名的包时，应用中心提示「未经验证应用的安全提示……由 未知发布者 提供，未经飞牛验证」，点「同意」以后照常安装、升级。fnpack 打未签名包时提示只用于调试；飞牛目前没有公开申请开发者证书的渠道，所以发布的包不签名。
- `appcenter-cli install-fpk` 能装新应用，已经装了的应用不能用它升级或卸载，要用网页。
- 应用中心判断能不能升级时只比较版本号里的数字（`1.2.0-qa.2` 看成 1、2、0、2），逐位比，前面相同时数字多的大，字母不算。所以预览版 `X.Y.Z-rc.N` 装上以后不能直接升级到同版本号的正式版 `X.Y.Z`（被拒：「已安装相同或更高版本」），要卸载（保留数据）再装；正式版之间、正式版到下一版的预览版、预览版之间都能直接升级（2026-11-27 实测）。

## 设计

| 项       | 决定                                                                                                                                                                                                  |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 端口     | 固定 8080（`service_port=8080`、`checkport=true`）。`cmd/main` 用 `--host 0.0.0.0 --port "$TRIM_SERVICE_PORT"` 启动：`pt-tools web` 只认这两个参数，不读 `PT_HOST`、`PT_PORT`。要换端口用 Docker 安装 |
| 数据     | `$TRIM_PKGVAR/.pt-tools`（数据库、`secret.key`、pt-tools 自己的日志、备份）                                                                                                                           |
| 控制台   | `PT_TOOLS_LOG_CONSOLE=false` 关掉控制台日志；标准输出和标准错误追加到 `$TRIM_PKGVAR/logs/console.log`，超过 1 MiB 时复制成 `console.log.1` 再截断（start 与每次 status 检查），两份合计约 2 MiB 封顶  |
| 进程     | PID 文件 `$TRIM_PKGVAR/pt-tools.pid` 一行「PID 启动时间」（`/proc/<pid>/stat` 第 22 列）。发信号前核对启动时间和 `/proc/<pid>/exe`，不是这个应用的 pt-tools 就不发；stop 先 SIGTERM，30 秒后 SIGKILL  |
| 升级     | `PT_TOOLS_MANAGED_BY=fnos`：网页的版本面板不提供升级，提示到 Release 下载新版 fpk                                                                                                                     |
| 删数据   | 只在 `TRIM_APP_STATUS=UNINSTALL` 且 `wizard_delete_data=true` 时，而且 `TRIM_PKGVAR` 必须是 `/…/@appdata/pt-tools`、不是符号链接；只删目录里的内容                                                    |
| 其余钩子 | install__、upgrade_callback、config__ 空操作；upgrade_init、uninstall_init 调 `main stop`                                                                                                             |

`start` 先检查端口是否已被占用（应用中心的 checkport 之外再兜一次底），启动后最多等 15 秒看端口打开；进程退出时把这次的输出和日志位置写进 `TRIM_TEMP_LOGFILE`（飞牛显示给用户），退出 1。

## 本地构建与测试

```bash
# <版本> <x86|arm> <对应架构的 Linux 二进制> <输出目录>
bash deploy/fnos/native/build-fnos-fpk.sh 1.2.0 x86 /path/to/linux-amd64/pt-tools /tmp/fpk

# 结构、manifest、可重复构建与生命周期脚本（用假 pt-tools，不需要飞牛）
bash deploy/fnos/native/test-fnos-fpk.sh
```

`SOURCE_DATE_EPOCH` 没设时取最近一次提交的时间；同样的二进制和提交打出的包字节相同。装到飞牛上试：应用中心 → 手动安装 → 从电脑上传。
