import { installRequestRecorder } from "./request-recorder";

// 由后台在一键采集时用 chrome.scripting.registerContentScripts 注册（world: MAIN、runAt: document_start），
// 只在采集打开的那几个后台标签页期间注册着，采完就注销。
installRequestRecorder(window);
