/*
 * 口令「多难猜」的估分 —— 画板 44 的强度条背后的算法。
 *
 * 为什么它不是校验器：这个项目对新口令的唯一硬性要求是**表单**上的 6 位下限
 * （ChangePassword.vue 的 `rules.newPassword`）。后端 `web/server.go` 的
 * `apiPassword` 只做「用户名 + 原口令」的身份核对，随后直接
 * `hashPassword(body.New)` 落库，长度和复杂度一个字都不查。
 * 所以这里算出来的分数只能当提示用，绝不能拿去拦提交 —— 前端偷偷加一道
 * 后端没有的门槛，等于让接口和界面对同一个口令给出两种结论。
 *
 * 也因此措辞只说「好猜 / 难猜」，不说「安全」：这套估分看的是离线暴力破解
 * 的搜索空间，它对撞库、键盘记录、口令复用一无所知，给不出「安全」这种结论。
 */

/** 强度条下面逐条列出的维度。`met` 为假只是「这一条没加分」，不是错误 */
export interface StrengthCheck {
  label: string;
  met: boolean;
}

export interface PasswordStrength {
  /** 0 = 空口令，1 = 很好猜 … 4 = 比较难猜 */
  level: 0 | 1 | 2 | 3 | 4;
  /** 强度条右侧的读数 */
  label: string;
  /** 进度条百分比，和 level 严格对应，免得条长和文字说两回事 */
  percent: number;
  tone: "info" | "dang" | "warn" | "ok";
  checks: StrengthCheck[];
}

/*
 * 键盘顺子：`123456`、`qwerty`、`asdfgh` 这类在物理键盘上连成一条的片段。
 * 单纯比字符码差（见 hasCodeRun）抓不到 `qwerty` —— q/w/e 的码值并不连续。
 */
const KEY_ROWS = ["1234567890", "qwertyuiop", "asdfghjkl", "zxcvbnm"];

/**
 * 常见到已经进了各家字典的口令片段。`adminadmin` 特别列进来：
 * 它是 `web/server.go` 的 `ensureAdminFromEnv` 在 `PT_ADMIN_PASS` 为空时
 * 写进库的默认口令，本项目的实例里真实存在。
 */
const COMMON_WORDS = [
  "password",
  "passwd",
  "adminadmin",
  "administrator",
  "admin",
  "root",
  "letmein",
  "iloveyou",
  "welcome",
  "abc123",
  "qwerty",
  "monkey",
  "dragon",
  "master",
  "login",
  "secret",
  "pttools",
  "pt-tools",
];

/** 连续 `len` 个字符的码值依次 +1 或 -1，例如 `abcd`、`4321` */
function hasCodeRun(s: string, len = 4): boolean {
  let up = 1;
  let down = 1;
  for (let i = 1; i < s.length; i++) {
    const delta = s.charCodeAt(i) - s.charCodeAt(i - 1);
    up = delta === 1 ? up + 1 : 1;
    down = delta === -1 ? down + 1 : 1;
    if (up >= len || down >= len) return true;
  }
  return false;
}

/** 任意 `len` 长的窗口正着或倒着出现在某一行键位里 */
function hasKeyboardRun(s: string, len = 4): boolean {
  const low = s.toLowerCase();
  for (let i = 0; i + len <= low.length; i++) {
    const seg = low.slice(i, i + len);
    const rev = [...seg].reverse().join("");
    if (KEY_ROWS.some((row) => row.includes(seg) || row.includes(rev))) return true;
  }
  return false;
}

/**
 * 常见的字符替换。`Passw0rd!` 之所以要还原成 `password!` 再去比字典，是因为
 * 这套替换本身就是字典攻击的标准预处理 —— 把 o 写成 0 并没有扩大搜索空间。
 */
const LEET: Record<string, string> = {
  "0": "o",
  "1": "i",
  "3": "e",
  "4": "a",
  "5": "s",
  "7": "t",
  $: "s",
  "@": "a",
  "!": "i",
};

function unleet(s: string): string {
  return s.replace(/[01345$@!7]/g, (c) => LEET[c] ?? c);
}

/** 整串由一个不超过 4 字符的片段重复而成，例如 `aaaaaa`、`abab`、`123123` */
function isRepeated(s: string): boolean {
  for (let n = 1; n <= 4 && n * 2 <= s.length; n++) {
    if (s.length % n === 0 && s.slice(0, n).repeat(s.length / n) === s) return true;
  }
  return false;
}

function countClasses(s: string): number {
  let n = 0;
  if (/[a-z]/.test(s)) n++;
  if (/[A-Z]/.test(s)) n++;
  if (/\d/.test(s)) n++;
  // 「符号」按排除法算：中日韩文字、空格、标点都落在这一类
  if (/[^a-zA-Z\d]/.test(s)) n++;
  return n;
}

/**
 * 长度档。给到 20 位是为了不冤枉长口令：`correct horse battery` 全是小写、
 * 只有两类字符，但 21 位的搜索空间已经超过一串 10 位的混合乱码。
 */
function lengthPoints(len: number): number {
  if (len >= 20) return 4;
  if (len >= 14) return 3;
  if (len >= 10) return 2;
  if (len >= 6) return 1;
  return 0;
}

function classPoints(classes: number): number {
  if (classes >= 4) return 3;
  if (classes >= 3) return 2;
  if (classes >= 2) return 1;
  return 0;
}

const LEVELS = [
  { label: "—", percent: 0, tone: "info" },
  { label: "很好猜", percent: 25, tone: "dang" },
  { label: "比较好猜", percent: 50, tone: "warn" },
  { label: "一般", percent: 75, tone: "info" },
  { label: "比较难猜", percent: 100, tone: "ok" },
] as const;

/**
 * 估一个口令有多难猜。`username` 传当前表单里的用户名，用来判断口令里是否
 * 嵌了它 —— 攻击者拿到用户名几乎是免费的，把它写进口令等于白送一段已知明文。
 */
export function scorePassword(password: string, username = ""): PasswordStrength {
  const pwd = password ?? "";
  const classes = countClasses(pwd);
  const name = (username ?? "").trim().toLowerCase();

  // 用户名短于 3 位时不判：`ab` 这种两字母子串在任何口令里都容易碰巧出现
  const hasUsername = name.length >= 3 && pwd.toLowerCase().includes(name);
  const plain = unleet(pwd.toLowerCase());
  const isWeakPattern =
    isRepeated(pwd) ||
    hasCodeRun(pwd) ||
    hasKeyboardRun(pwd) ||
    COMMON_WORDS.some((w) => plain.includes(w));

  const checks: StrengthCheck[] = [
    { label: "长度 14 位以上（长度比花样更能拖慢暴力破解）", met: pwd.length >= 14 },
    { label: "大写、小写、数字、符号里至少混了 3 类", met: classes >= 3 },
    { label: "不含你的用户名", met: !hasUsername },
    { label: "不是整串重复、连续键位或常见口令", met: !isWeakPattern },
  ];

  if (!pwd) {
    return { level: 0, ...LEVELS[0], checks };
  }

  let raw = lengthPoints(pwd.length) + classPoints(classes);
  if (hasUsername) raw -= 2;
  if (isWeakPattern) raw -= 2;
  raw = Math.min(7, Math.max(0, raw));

  const level = raw <= 1 ? 1 : raw <= 2 ? 2 : raw <= 4 ? 3 : 4;
  return { level, ...LEVELS[level], checks };
}
