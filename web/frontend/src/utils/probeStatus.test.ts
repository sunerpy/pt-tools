import { describe, expect, it } from "vitest";
import { isProbeSuccess, probeStatusLabel, probeStatusSeverity } from "./probeStatus";

describe("probeStatus", () => {
  it("labels the statuses added for the probe pre-check", () => {
    expect(probeStatusLabel("NOT_CONFIGURED")).toBe("未配置凭证 (NOT_CONFIGURED)");
    expect(probeStatusLabel("UNSUPPORTED")).toBe("暂不支持探测 (UNSUPPORTED)");
  });

  it("asks for action on missing credentials but keeps dynamic sites neutral", () => {
    expect(probeStatusSeverity("NOT_CONFIGURED")).toBe("warning");
    expect(probeStatusSeverity("UNSUPPORTED")).toBe("info");
    expect(isProbeSuccess("NOT_CONFIGURED")).toBe(false);
    expect(isProbeSuccess("UNSUPPORTED")).toBe(false);
  });

  it("keeps unknown values in the error bucket", () => {
    expect(probeStatusSeverity("SOMETHING_NEW")).toBe("error");
    expect(probeStatusSeverity(undefined)).toBe("error");
  });
});
