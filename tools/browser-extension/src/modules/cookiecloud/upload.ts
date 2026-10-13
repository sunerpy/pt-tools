/*
 * 把 PT 站点的 Cookie 上传到 CookieCloud：读浏览器里的 Cookie → 取回服务端已有的数据并解密 → 合并 → 加密 → 上传。
 * 服务端已有的数据解不开（UUID 或密码不对）时停下，不覆盖。同一时间只有一次上传在跑。
 */
import { t } from "../../core/i18n";
import {
  type CookieCloudCrypto,
  CRYPTO_FIXED,
  DecryptError,
  decrypt,
  encrypt,
  passphrase,
} from "./crypto";
import { CookieCloudError, fetchPayload, parseServer, pushPayload } from "./client";
import {
  type CookieCloudData,
  collectSiteCookies,
  cookieDigest,
  mergeCookieData,
  parseCookieData,
  siteDomains,
} from "./payload";

/** CookieCloudConfig 是扩展里的 CookieCloud 设置（存在扩展的本地存储里）。 */
export interface CookieCloudConfig {
  /** enabled 为 true 时定时同步也上传到 CookieCloud */
  enabled: boolean;
  serverUrl: string;
  uuid: string;
  password: string;
  cryptoType: CookieCloudCrypto;
}

/** CookieCloudState 是上传的情况：上次成功的时间与指纹，上次的错误。 */
export interface CookieCloudState {
  lastUploadAt: string | null;
  /** lastDigest 与 lastTarget 是上次成功上传的 Cookie 指纹与目标（服务地址、UUID、加密方式与密码的指纹）：都没变时定时上传跳过 */
  lastDigest: string | null;
  lastTarget: string | null;
  domains: number;
  cookies: number;
  lastError: string | null;
  lastErrorAt: string | null;
}

export const DEFAULT_COOKIECLOUD_CONFIG: CookieCloudConfig = {
  enabled: false,
  serverUrl: "",
  uuid: "",
  password: "",
  cryptoType: CRYPTO_FIXED,
};

export const EMPTY_COOKIECLOUD_STATE: CookieCloudState = {
  lastUploadAt: null,
  lastDigest: null,
  lastTarget: null,
  domains: 0,
  cookies: 0,
  lastError: null,
  lastErrorAt: null,
};

export interface UploadResult {
  /** uploaded 为 false 时 Cookie 和上次上传的一样，这次没传 */
  uploaded: boolean;
  domains: number;
  cookies: number;
}

function assertConfigured(cfg: CookieCloudConfig): void {
  if (!cfg.serverUrl.trim() || !cfg.uuid.trim() || !cfg.password) {
    throw new CookieCloudError("not_configured", t("cc.err.notConfigured"));
  }
  parseServer(cfg.serverUrl);
}

/**
 * targetOf 是上传的目标：服务地址、UUID、加密方式，加上密码的指纹（换了密码要用新密码重新加密上传）。
 * 指纹是口令的 SHA-256，不是密码本身；密码本来就存在同一个扩展存储里。
 */
async function targetOf(cfg: CookieCloudConfig): Promise<string> {
  const key = await sha256Hex(passphrase(cfg.uuid, cfg.password));
  return [parseServer(cfg.serverUrl).toString(), cfg.uuid.trim(), cfg.cryptoType, key].join("|");
}

async function sha256Hex(text: string): Promise<string> {
  const hash = new Uint8Array(
    await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text)),
  );
  return Array.from(hash, (b) => b.toString(16).padStart(2, "0")).join("");
}

let running: Promise<unknown> = Promise.resolve();

/** serialize 让上传与测试一个接一个跑：定时同步与手动上传同时触发时，后一个等前一个做完。 */
function serialize<T>(fn: () => Promise<T>): Promise<T> {
  const next = running.then(fn, fn);
  running = next.catch(() => undefined);
  return next;
}

async function readExisting(cfg: CookieCloudConfig, key: string): Promise<CookieCloudData | null> {
  const existing = await fetchPayload(cfg.serverUrl, cfg.uuid);
  if (!existing) {
    return null;
  }
  let plain: string;
  try {
    plain = await decrypt(existing.encrypted, key, existing.crypto_type);
  } catch (error: unknown) {
    if (error instanceof DecryptError) {
      throw new CookieCloudError("decrypt", t("cc.err.decrypt"));
    }
    throw error;
  }
  const data = parseCookieData(plain);
  if (!data) {
    throw new CookieCloudError("decrypt", t("cc.err.notCookieCloud"));
  }
  return data;
}

/**
 * uploadToCookieCloud 上传一次。force 为 false 时（定时同步）Cookie 与目标都和上次成功上传的一样就不传。
 * overwrite 为 true 时（用户确认过）服务端已有的数据解不开也照样上传，整份换成这次的（改过密码时用）。
 * 返回结果与新的状态；失败时抛 CookieCloudError，调用方把错误记进状态。
 */
export function uploadToCookieCloud(
  cfg: CookieCloudConfig,
  state: CookieCloudState | null,
  opts: { force: boolean; overwrite?: boolean; now?: () => Date },
): Promise<{ result: UploadResult; state: CookieCloudState }> {
  return serialize(async () => {
    assertConfigured(cfg);
    const now = opts.now ?? (() => new Date());
    const ours = await collectSiteCookies();
    const domains = Object.keys(ours).length;
    const cookies = Object.values(ours).reduce((n, list) => n + list.length, 0);
    const digest = await cookieDigest(ours);
    const target = await targetOf(cfg);
    const prev = state ?? EMPTY_COOKIECLOUD_STATE;
    if (!opts.force && prev.lastDigest === digest && prev.lastTarget === target) {
      return { result: { uploaded: false, domains, cookies }, state: prev };
    }
    const key = passphrase(cfg.uuid, cfg.password);
    let existing: CookieCloudData | null;
    try {
      existing = await readExisting(cfg, key);
    } catch (error: unknown) {
      if (!(opts.overwrite && error instanceof CookieCloudError && error.code === "decrypt")) {
        throw error;
      }
      existing = null;
    }
    const merged = mergeCookieData(existing, ours, siteDomains(), now());
    const encrypted = await encrypt(JSON.stringify(merged), key, cfg.cryptoType);
    await pushPayload(cfg.serverUrl, cfg.uuid, encrypted, cfg.cryptoType);
    return {
      result: { uploaded: true, domains, cookies },
      state: {
        lastUploadAt: now().toISOString(),
        lastDigest: digest,
        lastTarget: target,
        domains,
        cookies,
        lastError: null,
        lastErrorAt: null,
      },
    };
  });
}

export interface CookieCloudTestResult {
  /** empty：服务端还没有这个 UUID 的数据（第一次上传时建）；ok：解得开 */
  status: "empty" | "ok";
  /** domains 是服务端数据里的域名数 */
  domains: number;
  updatedAt: string | null;
}

/** testCookieCloud 取回服务端的数据并解密，不写任何东西。解不开时抛 CookieCloudError。 */
export function testCookieCloud(cfg: CookieCloudConfig): Promise<CookieCloudTestResult> {
  return serialize(async () => {
    assertConfigured(cfg);
    const data = await readExisting(cfg, passphrase(cfg.uuid, cfg.password));
    if (!data) {
      return { status: "empty", domains: 0, updatedAt: null };
    }
    return {
      status: "ok",
      domains: Object.keys(data.cookie_data ?? {}).length,
      updatedAt: typeof data.update_time === "string" ? data.update_time : null,
    };
  });
}
