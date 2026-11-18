/**
 * 复制文字。安全环境（HTTPS、localhost）用 Clipboard API；经局域网地址用 HTTP 打开时没有它，用隐藏的 textarea 加
 * execCommand("copy") 回退（同 TorrentSearch.vue 的做法）。复制成功返回 true，失败返回 false，不抛错。
 *
 * 回退用的 textarea 放进当前焦点所在的对话框或抽屉里：Element Plus 的弹层会把焦点拉回自己里面，放在 body 下选不中文字。
 */
export async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // 浏览器没给权限：走下面的回退
    }
  }
  const active = document.activeElement;
  const host = (active instanceof HTMLElement && active.closest("[role=dialog]")) || document.body;
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.left = "-9999px";
  area.style.top = "0";
  host.appendChild(area);
  try {
    area.focus();
    area.select();
    return document.execCommand("copy");
  } catch {
    return false;
  } finally {
    area.remove();
    if (active instanceof HTMLElement) active.focus();
  }
}
