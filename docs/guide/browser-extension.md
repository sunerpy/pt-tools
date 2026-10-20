# 浏览器扩展

本页介绍 PT Tools Helper 浏览器扩展的安装方法和几项功能：把站点 Cookie 同步到 pt-tools（可以定时同步，也可以上传到 CookieCloud），以及采集页面数据用于申请新增站点。

## 安装

| 方式                            | 说明                                                                                                                                                                                                                                    |
| ------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Edge 扩展商店（推荐 Edge 使用） | 在 Microsoft Edge 中打开 [Edge Add-ons](https://microsoftedge.microsoft.com/addons/detail/pt-tools-helper/pgicnjkmgenmjfhlclodbpbedjmojbea)，搜索「PT Tools Helper」。可以自动更新；每次发布约有一周的商店审核延迟。这个商店只面向 Edge |
| 手动安装（适用于 Chrome）       | 从[发布页面](https://github.com/sunerpy/pt-tools/releases)下载 `pt-tools-helper.zip` 并解压到固定目录，在 `chrome://extensions` 或 `edge://extensions` 中开启「开发者模式」，点击「加载已解压的扩展程序」。不会自动更新                 |
| 签名的 `.crx` 包                | 同一发布页面中的 `pt-tools-helper.crx`，用固定密钥签名，扩展 ID 在各版本间不变。Chrome 在 Windows 和 macOS 上不允许安装 Chrome 应用商店以外的 `.crx` 文件，在这两个系统上请用手动安装                                                   |
| Firefox                         | 在 `about:debugging#/runtime/this-firefox` 中「临时载入附加组件」，选择解压目录中的 `manifest.json`。浏览器重启后需要重新载入                                                                                                           |

首次点击扩展图标时，点击「授权并启用」授予所需权限。

## 连接到 pt-tools

点击扩展弹窗右上角的「设置」，填写 pt-tools 的地址（例如 `http://192.168.1.100:8080`），需要时填写 pt-tools 的用户名和密码，然后点击「连接测试」。地址必须能从当前浏览器访问。

弹窗顶部会显示连没连上 pt-tools；打开弹窗时扩展会先显示站点列表，同时去问一次 pt-tools，最多等 4 秒。

## 同步 Cookie

适用于 pt-tools 已经支持、且使用 Cookie 认证的站点。

弹窗的主页面是「我的站点」：浏览器里有这个站点的 Cookie、或者 pt-tools 里启用了这个站点的，都列在这里（其余内置站点点「显示全部」才列出来）。每一行写着浏览器里 Cookie 的状态，和 pt-tools 里这个站点的保号状态（还剩几天、快到期了）；要处理的站点排在前面。

- **单个站点**：在「我的站点」里点这一行的「同步」；当前标签页就是这个站点时，弹窗上方也有「同步 Cookie 到 pt-tools」。
- **全部**：点列表上方的「同步全部」，同步 Cookie 有效的所有站点。
- **自动**：在站点页面打开弹窗，打开「自动同步 Cookie」开关后，重新登录或 Cookie 更新时会自动同步。
- **保号**：点「打开站点」在后台标签页里打开这些站点，或者点某一行的「打开」。

使用 API Key 或 Passkey 的站点（例如 M-Team、Rousi Pro）不能通过扩展同步，请在 pt-tools 的站点设置中填写，见[获取 Cookie 与 API Key](get-cookie-apikey.md)。HDDolby 这类要 Cookie 和 API Key 两样的站点，同步 Cookie 时 pt-tools 先把 Cookie 存下；还没填 API Key 的话站点先不启用，扩展会提示你去 pt-tools 里补上 API Key。

### 定时同步

「设置」里的「定时同步」按间隔（10–1440 分钟，默认 60）自动同步一次，浏览器开着就会跑，不用打开站点页面：

- 默认只同步打开了「自动同步 Cookie」的站点；勾选「同步 pt-tools 里已启用的全部站点」后，pt-tools 里已经启用的 Cookie 站点都会同步。定时同步不会替你启用新站点。
- 只推 Cookie 有变化的站点：每推一次，pt-tools 都会刷新这个站点并检查一次登录状态，没变的不推。手动点「同步」总是推。
- 打开了下面的「CookieCloud」时，同一轮也上传到 CookieCloud。
- 要同步、但浏览器里没有可用 Cookie 的站点（没登录、Cookie 不全或过期了）不推，结果里按站点列出来。先在浏览器里登录这些站点，下一轮就会同步。
- 每一轮的结果写在设置下方；出了和上一轮不一样的问题（例如 pt-tools 的登录过期、某个站点推不上去、某个站点没有可用的 Cookie）时，扩展会发一条系统通知。
- 「立即同步一次」马上跑一轮，规则和定时的一样。有站点没同步上、没有可用的 Cookie，或者没连上 pt-tools 时，提示会写明有问题，详情看下方的结果。

### 同步到 CookieCloud

pt-tools 不在浏览器能直接访问的地方（例如浏览器在公司、pt-tools 在家里的 NAS），或者别的工具也要用这些 Cookie 时，可以让扩展把 Cookie 加密以后上传到你自己的 [CookieCloud](https://github.com/easychen/CookieCloud) 服务端，pt-tools 再从那里取回，见 [CookieCloud 导入](cookiecloud.md)。不用另装 CookieCloud 扩展。

1. 在「设置」的「CookieCloud」里填写服务地址、用户 KEY（UUID）和端对端加密密码，选加密方式（推荐 `aes-128-cbc-fixed`），点「保存」。第一次保存时浏览器会请你授权扩展访问这个服务地址。
2. 点「测试」：会提示能不能连上，以及服务端是没有数据、还是已有的数据解得开。
3. 点「立即上传」上传一次；勾选「定时同步时上传到 CookieCloud」后，定时同步每一轮都会上传（Cookie 没变化时不传）。
4. 在 pt-tools 的「站点 → CookieCloud 导入」里填同样的服务地址、UUID 和密码。

上传的是 pt-tools 支持的 PT 站点域名下的全部 Cookie，别的网站的不传。上传前扩展先取回服务端已有的数据、在本机解密，只换掉这些 PT 站点域名，别的域名和 local storage 原样保留，所以和 CookieCloud 扩展共用一个 UUID 也不会把它同步的数据冲掉。服务端已有的数据用填写的密码解不开时不上传；改过密码时，「立即上传」会问你要不要覆盖，覆盖以后其他设备也要改成新密码。

扩展不跟随服务端的重定向（地址里带着 UUID），请填写最终的服务地址。

## 采集页面数据

适用于 pt-tools 还不支持的站点，也可以用于向开发者报告已支持站点的解析问题。扩展会依次采集种子列表页、种子详情页和个人信息页，采集时自动移除 Passkey、Cookie、会话 ID、邮箱、IP 地址、API Key、Token 和邀请链接。完成后可以导出 ZIP，或直接打开预填好的 GitHub Issue。完整步骤见[请求新增站点](request-new-site.md)。

## 隐私

- 扩展只把 Cookie 发送到你在「设置」中填写的 pt-tools 地址，以及你填写的 CookieCloud 服务端（打开了的话）。发给 CookieCloud 的是用你的密码加密以后的数据，密码只在浏览器里使用，不会发出去。
- CookieCloud 的密码和 CookieCloud 扩展一样保存在浏览器的扩展存储中；扩展的弹窗里看不到它，只显示「已设置」。
- 填写的 pt-tools 用户名和密码保存在浏览器的扩展存储中。在共用电脑上使用时，请不要保存密码，并在用完后移除扩展。
- 导出的 ZIP 已经脱敏，提交前仍建议打开检查一遍。
