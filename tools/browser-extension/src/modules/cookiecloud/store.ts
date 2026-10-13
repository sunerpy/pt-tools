/*
 * CookieCloud 设置与上传状态的存储（chrome.storage.local，只由 background 写）。
 * 密码和 CookieCloud 扩展一样存在扩展的本地存储里；弹窗只拿得到「有没有设置」，拿不到密码本身。
 */
import { t } from "../../core/i18n";
import { STORAGE_KEYS } from "../../core/constants";
import { get, updateKey } from "../../core/storage";
import type { CookieCloudSettingsView } from "../../core/types";
import { CookieCloudError, parseServer } from "./client";
import { CRYPTO_FIXED, CRYPTO_LEGACY } from "./crypto";
import {
  type CookieCloudConfig,
  type CookieCloudState,
  DEFAULT_COOKIECLOUD_CONFIG,
  EMPTY_COOKIECLOUD_STATE,
} from "./upload";

/** MAX_FIELD 是服务地址与 UUID 的长度上限 */
const MAX_FIELD = 512;
/** MAX_PASSWORD 是密码的长度上限 */
const MAX_PASSWORD = 256;

export async function getCookieCloudConfig(): Promise<CookieCloudConfig> {
  const stored = await get<Partial<CookieCloudConfig>>(STORAGE_KEYS.cookieCloudConfig);
  return { ...DEFAULT_COOKIECLOUD_CONFIG, ...(stored ?? {}) };
}

export async function setCookieCloudConfig(cfg: CookieCloudConfig): Promise<void> {
  await updateKey<CookieCloudConfig>(STORAGE_KEYS.cookieCloudConfig, () => cfg);
}

export async function getCookieCloudState(): Promise<CookieCloudState | null> {
  return get<CookieCloudState>(STORAGE_KEYS.cookieCloudState);
}

export async function setCookieCloudState(state: CookieCloudState): Promise<void> {
  await updateKey<CookieCloudState>(STORAGE_KEYS.cookieCloudState, () => state);
}

/** cookieCloudView 是给弹窗看的设置与状态：不带密码。 */
export function cookieCloudView(
  cfg: CookieCloudConfig,
  state: CookieCloudState | null,
): CookieCloudSettingsView {
  const s = state ?? EMPTY_COOKIECLOUD_STATE;
  return {
    enabled: cfg.enabled,
    serverUrl: cfg.serverUrl,
    uuid: cfg.uuid,
    cryptoType: cfg.cryptoType,
    hasPassword: cfg.password !== "",
    lastUploadAt: s.lastUploadAt,
    domains: s.domains,
    cookies: s.cookies,
    lastError: s.lastError,
    lastErrorAt: s.lastErrorAt,
  };
}

/**
 * mergeCookieCloudInput 校验弹窗传来的设置，和现在的合成新设置：password 不传时保留原来的。
 * 打开上传时服务地址、UUID、密码都要有；服务地址要能解析。不合格时抛 CookieCloudError。
 */
export function mergeCookieCloudInput(raw: unknown, current: CookieCloudConfig): CookieCloudConfig {
  if (typeof raw !== "object" || raw === null) {
    throw new CookieCloudError("not_configured", t("cc.err.badSettings"));
  }
  const v = raw as Record<string, unknown>;
  const str = (x: unknown): string => (typeof x === "string" ? x.trim() : "");
  const serverUrl = str(v.serverUrl);
  const uuid = str(v.uuid);
  if (serverUrl.length > MAX_FIELD || uuid.length > MAX_FIELD) {
    throw new CookieCloudError("invalid_server", t("cc.err.tooLong"));
  }
  if (serverUrl) {
    parseServer(serverUrl);
  }
  let password = current.password;
  if (v.password !== undefined) {
    if (typeof v.password !== "string" || v.password.length > MAX_PASSWORD) {
      throw new CookieCloudError("not_configured", t("cc.err.badPassword"));
    }
    password = v.password;
  }
  const cryptoType = v.cryptoType === CRYPTO_LEGACY ? CRYPTO_LEGACY : CRYPTO_FIXED;
  const enabled = v.enabled === true;
  if (enabled && (!serverUrl || !uuid || !password)) {
    throw new CookieCloudError("not_configured", t("cc.err.notConfigured"));
  }
  return { enabled, serverUrl, uuid, password, cryptoType };
}
