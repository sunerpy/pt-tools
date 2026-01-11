/**
 * 格式化字节大小为人类可读格式
 * @param bytes 字节数
 * @returns 格式化后的字符串，如 "1.5 GB"
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  if (bytes < 0) return '-' + formatBytes(-bytes)

  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  const index = Math.min(i, sizes.length - 1)

  return parseFloat((bytes / Math.pow(k, index)).toFixed(2)) + ' ' + sizes[index]
}

/**
 * 格式化大数字为可读格式（支持中文单位）
 * @param num 数字
 * @returns 格式化后的字符串，如 "1.5万" 或 "2.3亿"
 */
export function formatNumber(num: number): string {
  if (num === 0) return '0'
  if (num < 0) return '-' + formatNumber(-num)
  if (num < 1000) return num.toFixed(0)
  if (num < 10000) return (num / 1000).toFixed(2) + 'K'
  if (num < 100000000) return (num / 10000).toFixed(2) + '万'
  return (num / 100000000).toFixed(2) + '亿'
}

/**
 * 格式化比率
 * @param ratio 比率值
 * @returns 格式化后的字符串
 */
export function formatRatio(ratio: number): string {
  if (ratio === Infinity || ratio > 999999) return '∞'
  if (ratio < 0) return '-'
  return ratio.toFixed(2)
}

/**
 * 格式化 Unix 时间戳为本地时间字符串
 * @param timestamp Unix 时间戳（秒）
 * @returns 格式化后的日期时间字符串
 */
export function formatTime(timestamp: number): string {
  if (!timestamp || timestamp <= 0) return '-'
  return new Date(timestamp * 1000).toLocaleString('zh-CN')
}

/**
 * 格式化 Unix 时间戳为日期字符串（不含时间）
 * @param timestamp Unix 时间戳（秒）
 * @returns 格式化后的日期字符串
 */
export function formatDate(timestamp: number): string {
  if (!timestamp || timestamp <= 0) return '-'
  return new Date(timestamp * 1000).toLocaleDateString('zh-CN')
}

/**
 * 格式化 Unix 时间戳为相对时间描述
 * @param timestamp Unix 时间戳（秒）
 * @returns 相对时间描述，如 "5分钟前", "2小时前", "3天前"
 */
export function formatTimeAgo(timestamp: number): string {
  if (!timestamp || timestamp <= 0) return '-'

  const now = Date.now() / 1000
  const diff = now - timestamp

  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`
  if (diff < 2592000) return `${Math.floor(diff / 86400)}天前`
  if (diff < 31536000) return `${Math.floor(diff / 2592000)}月前`
  return `${Math.floor(diff / 31536000)}年前`
}

/**
 * 格式化注册时长
 * @param timestamp 注册时间戳（秒）
 * @returns 时长描述，如 "3年2月"
 */
export function formatJoinDuration(timestamp: number): string {
  if (!timestamp || timestamp <= 0) return '-'

  const now = Date.now() / 1000
  const diff = now - timestamp

  const years = Math.floor(diff / 31536000)
  const months = Math.floor((diff % 31536000) / 2592000)
  const days = Math.floor((diff % 2592000) / 86400)

  if (years > 0) {
    return months > 0 ? `${years}年${months}月` : `${years}年`
  }
  if (months > 0) {
    return days > 0 ? `${months}月${days}天` : `${months}月`
  }
  return `${days}天`
}

/**
 * 解析 ISO 8601 duration 格式为中文描述
 * @param duration ISO 8601 duration 字符串，如 "P5W", "P10W", "P1Y", "P1M", "P7D"
 * @returns 中文描述，如 "5周", "10周", "1年", "1月", "7天"
 */
export function parseISODuration(duration: string): string {
  if (!duration) return '-'

  const match = duration.match(/P(\d+)([YMWD])/i)
  if (!match || match.length < 3) return duration

  const num = match[1]!
  const unit = match[2]!
  const units: Record<string, string> = {
    Y: '年',
    M: '月',
    W: '周',
    D: '天'
  }

  return `${num}${units[unit.toUpperCase()] || unit}`
}

/**
 * 解析 ISO 8601 duration 为天数
 * @param duration ISO 8601 duration 字符串
 * @returns 天数
 */
export function parseISODurationToDays(duration: string): number {
  if (!duration) return 0

  const match = duration.match(/P(\d+)([YMWD])/i)
  if (!match || match.length < 3) return 0

  const num = parseInt(match[1]!, 10)
  const unit = match[2]!

  switch (unit.toUpperCase()) {
    case 'Y':
      return num * 365
    case 'M':
      return num * 30
    case 'W':
      return num * 7
    case 'D':
      return num
    default:
      return 0
  }
}

/**
 * 预定义的站点颜色映射
 */
const siteColors: Record<string, string> = {
  hdsky: '#1890ff',
  mteam: '#52c41a',
  springsunday: '#fa8c16',
  hddolby: '#722ed1',
  audiences: '#eb2f96',
  pterclub: '#13c2c2',
  ourbits: '#f5222d',
  chdbits: '#faad14',
  ttg: '#2f54eb'
}

/**
 * 获取站点头像背景颜色
 * @param siteName 站点名称
 * @returns 颜色值（十六进制或 HSL）
 */
export function getAvatarColor(siteName: string): string {
  const lower = siteName.toLowerCase()

  // 检查预定义颜色
  if (siteColors[lower]) {
    return siteColors[lower]
  }

  // 根据名称哈希生成一致的颜色
  let hash = 0
  for (let i = 0; i < siteName.length; i++) {
    hash = siteName.charCodeAt(i) + ((hash << 5) - hash)
  }

  const hue = Math.abs(hash) % 360
  return `hsl(${hue}, 70%, 50%)`
}

/**
 * 获取比率对应的类型（用于 Element Plus tag）
 * @param ratio 比率值
 * @returns 'success' | 'warning' | 'danger' | 'info'
 */
export function getRatioType(ratio: number): 'success' | 'warning' | 'danger' | 'info' {
  if (ratio >= 2) return 'success'
  if (ratio >= 1) return 'info'
  if (ratio >= 0.5) return 'warning'
  return 'danger'
}

/**
 * 计算注册天数
 * @param joinTimestamp 注册时间戳（秒）
 * @returns 注册天数
 */
export function calculateDaysSinceJoin(joinTimestamp: number): number {
  if (!joinTimestamp || joinTimestamp <= 0) return 0
  const now = Date.now() / 1000
  return Math.floor((now - joinTimestamp) / 86400)
}

/**
 * 检查是否满足注册时间要求
 * @param joinTimestamp 注册时间戳（秒）
 * @param requiredDuration ISO 8601 duration 字符串
 * @returns 是否满足
 */
export function checkIntervalRequirement(joinTimestamp: number, requiredDuration: string): boolean {
  if (!requiredDuration) return true
  if (!joinTimestamp || joinTimestamp <= 0) return false

  const requiredDays = parseISODurationToDays(requiredDuration)
  const actualDays = calculateDaysSinceJoin(joinTimestamp)

  return actualDays >= requiredDays
}

/**
 * 站点魔力值名称映射
 */
const siteBonusNames: Record<string, string> = {
  mteam: '魔力',
  hdsky: '魔力',
  springsunday: '茉莉',
  hddolby: '鲸币'
  // 默认使用"魔力"
}

/**
 * 获取站点魔力值的名称
 * @param siteId 站点ID
 * @returns 魔力值名称，如"魔力"、"茉莉"、"鲸币"
 */
export function getSiteBonusName(siteId: string): string {
  const lower = siteId.toLowerCase()
  return siteBonusNames[lower] || '魔力'
}

/**
 * 站点做种积分名称映射（只有部分站点有做种积分）
 */
const siteSeedingBonusNames: Record<string, string> = {
  springsunday: '做种积分',
  hddolby: '保种积分'
}

/**
 * 获取站点做种积分的名称
 * @param siteId 站点ID
 * @returns 做种积分名称，如果站点没有做种积分则返回 null
 */
export function getSiteSeedingBonusName(siteId: string): string | null {
  const lower = siteId.toLowerCase()
  return siteSeedingBonusNames[lower] || null
}

/**
 * 检查站点是否有做种积分
 * @param siteId 站点ID
 * @returns 是否有做种积分
 */
export function hasSiteSeedingBonus(siteId: string): boolean {
  const lower = siteId.toLowerCase()
  return lower in siteSeedingBonusNames
}
