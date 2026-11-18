/**
 * 监听地址是否只对本机开放，与后端 internal/notify/adapter/qq 的 isLoopbackListenAddr 同规则：
 * 127.x.x.x、::1、localhost 算本机；0.0.0.0、[::]、主机名为空（":6701"）都会监听所有网卡，不算。
 * QQ 通道监听在非本机地址时后端要求必须设置 Access Token，表单在保存前用它提示。
 */
export function isLoopbackListenAddr(addr: string): boolean {
  const m = addr.trim().match(/^(?:\[([^\]]+)\]|([^:[\]]*)):\d+$/);
  if (!m) return false;
  const host = (m[1] ?? m[2] ?? "").toLowerCase();
  if (host === "localhost" || host === "::1") return true;
  return /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(host);
}
