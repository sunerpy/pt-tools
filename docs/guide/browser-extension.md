# 浏览器扩展

本页介绍 PT Tools Helper 浏览器扩展的安装方法和两项功能：把站点 Cookie 同步到 pt-tools，以及采集页面数据用于申请新增站点。

## 安装

| 方式                  | 说明                                                                                                                                                                                                                                         |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Edge 扩展商店（推荐） | 在 [Edge Add-ons](https://microsoftedge.microsoft.com/addons/detail/pt-tools-helper/pgicnjkmgenmjfhlclodbpbedjmojbea) 中搜索「PT Tools Helper」。可以自动更新；每次发布约有一周的商店审核延迟。Chrome 等 Chromium 内核浏览器也可以从这里安装 |
| 手动安装              | 从[发布页面](https://github.com/sunerpy/pt-tools/releases)下载 `pt-tools-helper.zip` 并解压到固定目录，在 `chrome://extensions` 或 `edge://extensions` 中开启「开发者模式」，点击「加载已解压的扩展程序」。不会自动更新                      |
| Chrome 签名包         | 同一发布页面中的 `pt-tools-helper.crx`                                                                                                                                                                                                       |
| Firefox               | 在 `about:debugging#/runtime/this-firefox` 中「临时载入附加组件」，选择解压目录中的 `manifest.json`。浏览器重启后需要重新载入                                                                                                                |

首次点击扩展图标时，点击「授权并启用」授予所需权限。

## 连接到 pt-tools

在扩展底部的「全局设置」中填写 pt-tools 的地址（例如 `http://192.168.1.100:8080`），需要时填写 pt-tools 的用户名和密码，然后点击「连接测试」。地址必须能从当前浏览器访问。

## 同步 Cookie

适用于 pt-tools 已经支持、且使用 Cookie 认证的站点。

- **单个站点**：在浏览器中登录站点，点击扩展图标，确认显示站点名称且 Cookie 状态为「有效」，点击「同步 Cookie 到 pt-tools」。
- **批量**：在「全局设置」的「批量同步 Cookie」中勾选多个站点，点击「同步已选」。
- **自动**：打开「自动同步 Cookie」开关后，重新登录或 Cookie 更新时会自动同步。

使用 API Key 或 Passkey 的站点（例如 M-Team、Rousi Pro）不能通过扩展同步，请在 pt-tools 的站点设置中填写，见[获取 Cookie 与 API Key](get-cookie-apikey.md)。

## 采集页面数据

适用于 pt-tools 还不支持的站点，也可以用于向开发者报告已支持站点的解析问题。扩展会依次采集种子列表页、种子详情页和个人信息页，采集时自动移除 Passkey、Cookie、会话 ID、邮箱、IP 地址、API Key、Token 和邀请链接。完成后可以导出 ZIP，或直接打开预填好的 GitHub Issue。完整步骤见[请求新增站点](request-new-site.md)。

## 隐私

- 扩展只把 Cookie 发送到你在「全局设置」中填写的 pt-tools 地址。
- 填写的 pt-tools 用户名和密码保存在浏览器的扩展存储中。在共用电脑上使用时，请不要保存密码，并在用完后移除扩展。
- 导出的 ZIP 已经脱敏，提交前仍建议打开检查一遍。
