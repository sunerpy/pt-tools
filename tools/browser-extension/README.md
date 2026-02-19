# PT Tools Helper 浏览器扩展

PT 站点数据采集助手，一键采集站点数据、同步 Cookie 到 pt-tools。

## 安装

### Chrome / Edge

1. 下载 `pt-tools-helper.zip` 并解压
2. 打开 `chrome://extensions`，开启「开发者模式」
3. 点击「加载已解压的扩展程序」，选择解压目录
4. 首次点击扩展图标时会请求权限，点击「授权并启用」

### Firefox

1. 打开 `about:debugging#/runtime/this-firefox`
2. 点击「临时载入附加组件」，选择解压目录中的 `manifest.json`

## 使用说明

### 功能一：同步 Cookie 到 pt-tools（内置站点）

适用于 pt-tools 已支持的站点（HDSky、SpringSunday、HDDolby、NovaHD 等 Cookie 鉴权站点）。

1. 在浏览器中登录 PT 站点
2. 点击扩展图标，会显示「✅ 站点名称」
3. 在「全局设置」中填入 pt-tools 地址并保存
4. 点击「🔄 同步 Cookie 到 pt-tools」
5. 或开启「自动同步 Cookie」，后续登录时自动同步

批量同步：在「全局设置」→「批量同步 Cookie」中勾选多个站点一键同步。

> M-Team（API Key）、Rousi Pro（Passkey）等非 Cookie 鉴权站点需在 pt-tools 中手动配置。

### 功能二：采集站点数据（请求新增站点支持）

适用于 pt-tools 尚未支持的站点。需要采集 **3 个页面**，扩展会自动脱敏敏感信息。

#### 采集步骤

1. **打开目标站点**，扩展会自动识别并显示「❓ 未知站点」
2. **采集种子列表页**：
   - 打开 `torrents.php` 或种子浏览页面（最好包含免费种子）
   - 点击扩展图标 → 点击「📸 采集种子列表页」
3. **采集种子详情页**：
   - 点击一个种子进入详情页（优先选择有免费标签的种子）
   - 点击扩展图标 → 点击「📸 采集种子详情页」
4. **采集个人信息页**：
   - 点击用户名进入个人资料页（`userdetails.php?id=xxx`）
   - 点击扩展图标 → 点击「📸 采集个人信息页」
5. 采集完成后（3/3），在「采集记录」区域选择：
   - 「📦 导出 ZIP」— 下载包含所有页面数据的压缩包
   - 「🐙 提交 Issue」— 自动下载 ZIP 并打开预填的 GitHub Issue 页面，将 ZIP 拖拽上传到 Issue 后提交

也可以将导出的 ZIP 直接发送到交流群：

- Telegram: https://t.me/+7YK2kmWIX0s1Nzdl
- QQ 群: 274984594

#### 自动脱敏

采集时自动移除以下敏感信息：

- Passkey、PHPSESSID、Cookie 值
- 邮箱地址、IP 地址
- API Key、Bearer Token、邀请链接

#### 已知站点也可采集

在已支持的站点页面上也可以点击「📸 采集当前页面数据」，用于向开发者报告解析问题。

## 开发

```bash
cd tools/browser-extension
pnpm install
pnpm build        # 构建到 dist/
pnpm run pack     # 构建 + 站点一致性检查 + 打包 zip
pnpm dev          # watch 模式
```

## 项目结构

```
src/
├── background/     # Service Worker：权限 API、消息编排
├── content/        # Content Script：PT 站点检测、页面采集
├── popup/          # Vue 3 弹窗 UI
├── core/           # 共享：类型、消息、存储、常量、PT 站点域名库
├── modules/
│   ├── collector/  # 页面检测、HTML 采集、自动脱敏
│   ├── sync/       # Cookie 读取、pt-tools API 客户端
│   └── export/     # ZIP 打包、GitHub Issue 创建
└── utils/          # URL/DOM 工具
```
