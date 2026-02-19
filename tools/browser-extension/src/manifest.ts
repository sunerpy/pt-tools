export const manifest = {
  manifest_version: 3,
  name: "__MSG_extensionName__",
  version: "0.1.0",
  description: "__MSG_extensionDescription__",
  default_locale: "zh_CN",
  permissions: ["storage", "activeTab", "scripting"],
  optional_permissions: ["cookies", "tabs"],
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
