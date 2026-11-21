/** 后台标签页里记录页面请求的脚本：注册 ID、打包后的文件名、页面里挂记录的全局名 */
export const RECORDER_SCRIPT_ID = "pt-tools-helper-request-recorder";
export const RECORDER_FILE = "request-recorder.js";
export const RECORDER_GLOBAL = "__ptToolsHelperRecorder";

/*
 * 一键采集在后台标签页里记下的内容（渲染后的页面 rendered_*、请求记录 requests_*）的上限。
 *
 * 它们和其余页面一起存进 chrome.storage.local 里的采集会话。扩展没有 unlimitedStorage 权限，
 * chrome.storage.local 一共只有 10 MiB（QUOTA_BYTES = 10485760）。Chrome 按值写成 JSON 以后的 UTF-8 字节数计
 * （extensions/browser/api/storage/settings_storage_quota_enforcer.cc）：汉字记 3 字节，页面里的每个 <
 * 写成 \u003C 记 6 字节（base/json/string_escape.cc）。本仓库站点测试里的 70 段页面样本
 * （site/v2/definitions/*_fixture_test.go）平均 1.3 字节/字符，最多 1.43；请求记录是 JSON 文本
 * 再存成字符串，引号要转义两次，按 2 字节/字符估。
 *
 * 一次采集开三个标签页（列表页、详情页、个人页），各存一份渲染后的页面和一份请求记录：
 *   以前：3 × 3 Mi 字符的页面 + 3 × 60 条 × 128 KiB 的回应，最坏三十多 MiB，一次就能超出配额；
 *   现在：3 × (256 KiB + 128 KiB) = 1152 Ki 字符，按 2 字节/字符算最多约 2.25 MiB。
 * 必采的页面（首页、列表、详情、个人页、魔力值、规则、RSS、脚本等）一般一次 1–2 Mi 字符。两者合起来
 * 一般还在会话的总量上限（core/storage.ts 的 MAX_SESSIONS_BYTES，按 JSON 字符数算 4 Mi）以内，
 * 折成字节也离 10 MiB 很远，别的采集会话、站点快照与设置还有空间。真存不下时 saveSession 先去掉这两类页面再存
 * （core/storage.ts 的 OPTIONAL_PAGE_TYPES）。
 *
 * 渲染后的页面只为看页面脚本插进去的内容（优惠图标、展开的做种列表），开头 256 KiB 里已有页头和几十行；
 * 列表页一般 150–400 KB，超出的部分截掉。
 */

/** 每个页面最多记多少个请求 */
export const RECORDER_MAX_ENTRIES = 30;
/** 每个请求的回应正文最多留多长（字符）：页面脚本取的优惠状态、做种列表一般几 KB 到几十 KB */
export const RECORDER_MAX_BODY = 32 * 1024;
/** 每个请求的参数最多留多长（字符）：只要看参数名和格式 */
export const RECORDER_MAX_PARAMS = 4 * 1024;
/** 一个页面的请求记录（requests_*.json）最多多长（字符）；超出时从最后一条往前去掉回应正文和参数 */
export const REQUESTS_LOG_MAX_CHARS = 128 * 1024;
/** 渲染后的页面（rendered_*.html）最多留多长（字符） */
export const RENDERED_DOM_MAX_CHARS = 256 * 1024;
