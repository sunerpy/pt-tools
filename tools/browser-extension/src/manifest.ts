import pkg from "../package.json";

// 版本的唯一来源是 tools/browser-extension/package.json：
//   * release-please 通过 release-please-config.json 的 extra-files
//     （type: json, jsonpath: $.version）更新其 SemVer 值；
//   * 发布时 .github/workflows/release.yml 的 extension job 再把它改写为
//     浏览器可接受的四段数值版本（stable = <M.m.p>.65535，RC = <M.m.p>.<N>），
//     并在构建后断言 dist/manifest.json 与预期一致。
// 因此此处不得硬编码版本号，否则发布会在该断言处失败。
export const manifest = {
  manifest_version: 3,
  name: "__MSG_extensionName__",
  version: pkg.version,
  description: "__MSG_extensionDescription__",
  default_locale: "zh_CN",
  permissions: ["storage", "activeTab", "scripting", "notifications", "alarms"],
  optional_permissions: ["cookies", "tabs", "webNavigation"],
  optional_host_permissions: ["*://*/*"],
  background: {
    service_worker: "background.js",
    type: "module",
  },
  content_scripts: [
    {
      matches: ["*://*/*"],
      js: ["content.js"],
      run_at: "document_idle",
    },
  ],
  action: {
    default_popup: "src/popup/index.html",
    default_icon: {
      16: "icons/icon16.png",
      32: "icons/icon32.png",
      48: "icons/icon48.png",
      128: "icons/icon128.png",
    },
  },
  icons: {
    16: "icons/icon16.png",
    32: "icons/icon32.png",
    48: "icons/icon48.png",
    128: "icons/icon128.png",
  },
} as const;
