import { describe, expect, it } from "vitest";
import { ref } from "vue";
import type { SiteLoginState } from "@/api";
import { useLoginState } from "./useLoginState";

function state(partial: Partial<SiteLoginState>): SiteLoginState {
  return {
    site_name: "hdsky",
    enabled: true,
    consecutive_probe_failures: 0,
    ban_threshold_days: 30,
    remind_before_days: 10,
    reminder_cron: "0 10,22 * * *",
    notification_channel_ids: [],
    last_reminder_tier: "none",
    days_remaining: 20,
    tier: "none",
    probe_mode: "auto",
    ...partial,
  };
}

describe("useLoginState probe details", () => {
  it("names the source of the activity judgement", () => {
    const states = ref<Record<string, SiteLoginState>>({
      a: state({ effective_source: "last_access" }),
      b: state({ effective_source: "cookie_last_login" }),
      c: state({ effective_source: "last_visit" }),
      d: state({ effective_source: "none" }),
    });
    const { activeSourceLabel } = useLoginState(states);
    expect(activeSourceLabel("a")).toBe("站点最近访问");
    expect(activeSourceLabel("b")).toBe("站点最近登录");
    expect(activeSourceLabel("c")).toBe("浏览器访问");
    expect(activeSourceLabel("d")).toBe("");
    expect(activeSourceLabel("missing")).toBe("");
  });

  it("exposes schedule, failure streak and stale access", () => {
    const states = ref<Record<string, SiteLoginState>>({
      a: state({
        next_probe_at: 1_700_000_000,
        first_failure_at: 1_699_000_000,
        access_stale_since: 1,
      }),
      b: state({}),
    });
    const { nextProbeAt, failingSince, accessStale } = useLoginState(states);
    expect(nextProbeAt("a")).toBe(1_700_000_000);
    expect(failingSince("a")).toBe(1_699_000_000);
    expect(accessStale("a")).toBe(true);
    expect(nextProbeAt("b")).toBe(0);
    expect(failingSince("b")).toBe(0);
    expect(accessStale("b")).toBe(false);
  });
});

describe("useLoginState probe note", () => {
  it("explains a probe that did not succeed, including the neutral states", () => {
    const states = ref<Record<string, SiteLoginState>>({
      ok: state({ last_probe_status: "OK" }),
      never: state({}),
      missing: state({ last_probe_status: "NOT_CONFIGURED" }),
      dynamic: state({ last_probe_status: "UNSUPPORTED" }),
      expired: state({ last_probe_status: "SESSION_EXPIRED" }),
    });
    const { probeNote } = useLoginState(states);
    expect(probeNote("ok")).toBe("");
    expect(probeNote("never")).toBe("");
    expect(probeNote("absent")).toBe("");
    expect(probeNote("missing")).toBe("探测结果：未配置凭证 (NOT_CONFIGURED)");
    expect(probeNote("dynamic")).toBe("探测结果：暂不支持探测 (UNSUPPORTED)");
    expect(probeNote("expired")).toBe("探测结果：会话已过期 (SESSION_EXPIRED)");
  });
});
