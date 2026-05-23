# PT Tools Helper 浏览器扩展 - 隐私策略

**最后更新：2026-05-23**

## 简介

PT Tools Helper 是 [pt-tools](https://github.com/sunerpy/pt-tools) 的配套浏览器扩展，用于将 PT 站点 Cookie 同步到用户自部署的 pt-tools 实例，并在用户主动触发时采集站点页面用于新站点适配请求。

本扩展**不会**将任何用户数据上传到任何中心化服务器、第三方分析平台或除「用户自己配置的 pt-tools 实例」之外的任何 endpoint。

## 数据收集与传输

### 我们读取什么

| 数据                                                                           | 何时读取                                                                                | 何时传输                                                                                       | 传输到哪里                                     |
| ------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| PT 站点 Cookie（如 `c_secure_uid`、`c_secure_pass`、`SPRINGID`、`session` 等） | 用户点击「同步 Cookie」或「批量同步」按钮时，或在已开启「自动同步」的站点页面被检测到时 | 仅 HTTP POST 到用户在「全局设置」中配置的 `pt-tools URL`（默认 `http://localhost:8080`）       | 用户自部署的 pt-tools 后端                     |
| API Key、Passkey                                                               | 同上                                                                                    | 同上                                                                                           | 同上                                           |
| PT 站点页面 HTML（torrents 列表/详情/用户信息页）                              | **仅在用户主动点击「📸 采集」按钮时**                                                   | 仅作为本地下载的 ZIP 文件，或在用户点击「🐙 提交 Issue」时附加到用户跳转后的 GitHub Issue 表单 | 用户本地磁盘 + 用户主动选择上传的 GitHub Issue |
| 当前打开的 Tab URL                                                             | 用户点击扩展图标时（用于检测当前是否处于支持的 PT 站点）                                | 不外发                                                                                         | 仅用于扩展弹窗 UI                              |
| 用户配置（pt-tools URL、用户名、自动同步开关、采集会话）                       | 设置保存时                                                                              | 不外发                                                                                         | 浏览器本地 `chrome.storage.local`              |

### 我们不收集什么

- 不收集广告标识符 / 设备指纹 / 浏览历史 / 键盘记录
- 不上传 telemetry / 崩溃报告 / 使用统计到任何中心化服务
- 不与任何第三方共享数据
- 不出售数据
- 不用于贷款 / 信用评估
- 不用于本扩展「单一用途」之外的任何场景

## 自动脱敏

在「📸 采集」流程中，扩展会在数据落盘前自动移除以下敏感字段：

- Passkey、PHPSESSID、`c_secure_*` 等 Cookie 值
- 邮箱地址（替换为 `user@example.com`）
- IP 地址（替换为 `127.0.0.1`）
- API Key、Bearer Token
- 邀请链接

具体规则见源码 [`tools/browser-extension/src/core/constants.ts`](https://github.com/sunerpy/pt-tools/blob/main/tools/browser-extension/src/core/constants.ts) 的 `SANITIZE_RULES` 数组。

## 权限说明

| 权限                        | 用途                                                                                                  |
| --------------------------- | ----------------------------------------------------------------------------------------------------- |
| `storage`                   | 保存用户的 pt-tools URL、自动同步开关、最近采集会话等本地配置                                         |
| `activeTab`                 | 用户点击扩展图标后，仅对**当前**标签页临时启用页面访问，用于检测站点身份                              |
| `scripting`                 | 用户点击「📸 采集」时，注入采集脚本到当前 PT 站点页面以读取并脱敏页面 HTML                            |
| `cookies`（可选权限）       | 用户在「全局设置」中授权后，读取 PT 站点 Cookie 用于同步到自部署的 pt-tools                           |
| `tabs`（可选权限）          | 检测当前活动标签页所属的 PT 站点                                                                      |
| `*://*/*`（可选 host 权限） | 用户在弹窗中点「🔓 授权并启用」时申请；仅用于 `chrome.cookies.getAll({domain})` 跨 PT 站点读取 cookie |

`cookies` / `tabs` / `host_permissions` 均为**可选权限**（`optional_permissions` / `optional_host_permissions`），在用户首次点击扩展图标并主动授权前不会被启用。

## 远程代码

本扩展**不**使用任何远程代码（远程 `<script>`、远程模块、`eval()` 字符串）。所有 JS / Wasm 均打包在 ZIP 内，可在 [GitHub Releases](https://github.com/sunerpy/pt-tools/releases) 验证源码与构建产物对应关系。

## 开源审计

本扩展全量开源，源码托管于 <https://github.com/sunerpy/pt-tools>，路径 [`tools/browser-extension/`](https://github.com/sunerpy/pt-tools/tree/main/tools/browser-extension)。任何用户均可审计代码、自行构建、对照 Edge 商店包的内容。

## 联系方式

- 提交 Issue：<https://github.com/sunerpy/pt-tools/issues>
- 交流群：[Telegram](https://t.me/+7YK2kmWIX0s1Nzdl) / QQ 274984594

## 变更历史

- 2026-05-23：首次发布（对应扩展 v0.2.1）
