import { describe, expect, it } from "vitest";
import {
  calculateDaysSinceJoin,
  checkIntervalRequirement,
  formatBytes,
  formatDate,
  formatNumber,
  formatRatio,
  formatTime,
  getAvatarColor,
  getRatioType,
  parseISODuration,
  parseISODurationToDays,
} from "./format";

describe("formatBytes", () => {
  it("should format 0 bytes", () => {
    expect(formatBytes(0)).toBe("0 B");
  });

  it("should format bytes", () => {
    expect(formatBytes(500)).toBe("500 B");
  });

  it("should format kilobytes", () => {
    expect(formatBytes(1024)).toBe("1 KB");
    expect(formatBytes(1536)).toBe("1.5 KB");
  });

  it("should format megabytes", () => {
    expect(formatBytes(1024 * 1024)).toBe("1 MB");
    expect(formatBytes(1.5 * 1024 * 1024)).toBe("1.5 MB");
  });

  it("should format gigabytes", () => {
    expect(formatBytes(1024 * 1024 * 1024)).toBe("1 GB");
    expect(formatBytes(200 * 1024 * 1024 * 1024)).toBe("200 GB");
  });

  it("should format terabytes", () => {
    expect(formatBytes(1024 * 1024 * 1024 * 1024)).toBe("1 TB");
    expect(formatBytes(2.5 * 1024 * 1024 * 1024 * 1024)).toBe("2.5 TB");
  });

  it("should handle negative values", () => {
    expect(formatBytes(-1024)).toBe("-1 KB");
  });
});

describe("formatNumber", () => {
  it("should format 0", () => {
    expect(formatNumber(0)).toBe("0");
  });

  it("should format small numbers", () => {
    expect(formatNumber(500)).toBe("500");
    expect(formatNumber(999)).toBe("999");
  });

  it("should format thousands", () => {
    expect(formatNumber(1000)).toBe("1.00K");
    expect(formatNumber(5500)).toBe("5.50K");
  });

  it("should format 万 (10000s)", () => {
    expect(formatNumber(10000)).toBe("1.00万");
    expect(formatNumber(600000)).toBe("60.00万");
  });

  it("should format 亿 (100000000s)", () => {
    expect(formatNumber(100000000)).toBe("1.00亿");
    expect(formatNumber(250000000)).toBe("2.50亿");
  });
});

describe("formatRatio", () => {
  it("should format normal ratios", () => {
    expect(formatRatio(1.5)).toBe("1.50");
    expect(formatRatio(2.0)).toBe("2.00");
    expect(formatRatio(0.5)).toBe("0.50");
  });

  it("should format infinity", () => {
    expect(formatRatio(Infinity)).toBe("∞");
    expect(formatRatio(1000000)).toBe("∞");
  });

  it("should handle negative values", () => {
    expect(formatRatio(-1)).toBe("-");
  });
});

describe("formatTime", () => {
  it("should return - for invalid timestamps", () => {
    expect(formatTime(0)).toBe("-");
    expect(formatTime(-1)).toBe("-");
  });

  it("should format valid timestamps", () => {
    const timestamp = 1704067200; // 2024-01-01 00:00:00 UTC
    const result = formatTime(timestamp);
    expect(result).toContain("2024");
  });
});

describe("formatDate", () => {
  it("should return - for invalid timestamps", () => {
    expect(formatDate(0)).toBe("-");
    expect(formatDate(-1)).toBe("-");
  });

  it("should format valid timestamps", () => {
    const timestamp = 1704067200; // 2024-01-01 00:00:00 UTC
    const result = formatDate(timestamp);
    expect(result).toContain("2024");
  });
});

describe("parseISODuration", () => {
  it("should parse weeks", () => {
    expect(parseISODuration("P5W")).toBe("5周");
    expect(parseISODuration("P10W")).toBe("10周");
  });

  it("should parse years", () => {
    expect(parseISODuration("P1Y")).toBe("1年");
    expect(parseISODuration("P2Y")).toBe("2年");
  });

  it("should parse months", () => {
    expect(parseISODuration("P1M")).toBe("1月");
    expect(parseISODuration("P6M")).toBe("6月");
  });

  it("should parse days", () => {
    expect(parseISODuration("P7D")).toBe("7天");
    expect(parseISODuration("P30D")).toBe("30天");
  });

  it("should handle empty or invalid input", () => {
    expect(parseISODuration("")).toBe("-");
    expect(parseISODuration("invalid")).toBe("invalid");
  });

  it("should be case insensitive", () => {
    expect(parseISODuration("p5w")).toBe("5周");
    expect(parseISODuration("P5w")).toBe("5周");
  });
});

describe("parseISODurationToDays", () => {
  it("should convert weeks to days", () => {
    expect(parseISODurationToDays("P5W")).toBe(35);
    expect(parseISODurationToDays("P10W")).toBe(70);
  });

  it("should convert years to days", () => {
    expect(parseISODurationToDays("P1Y")).toBe(365);
    expect(parseISODurationToDays("P2Y")).toBe(730);
  });

  it("should convert months to days", () => {
    expect(parseISODurationToDays("P1M")).toBe(30);
    expect(parseISODurationToDays("P6M")).toBe(180);
  });

  it("should return days directly", () => {
    expect(parseISODurationToDays("P7D")).toBe(7);
    expect(parseISODurationToDays("P30D")).toBe(30);
  });

  it("should return 0 for invalid input", () => {
    expect(parseISODurationToDays("")).toBe(0);
    expect(parseISODurationToDays("invalid")).toBe(0);
  });
});

describe("getAvatarColor", () => {
  it("should return predefined colors for known sites", () => {
    expect(getAvatarColor("hdsky")).toBe("#1890ff");
    expect(getAvatarColor("mteam")).toBe("#52c41a");
    expect(getAvatarColor("springsunday")).toBe("#fa8c16");
    expect(getAvatarColor("hddolby")).toBe("#722ed1");
  });

  it("should be case insensitive for known sites", () => {
    expect(getAvatarColor("HDSky")).toBe("#1890ff");
    expect(getAvatarColor("MTEAM")).toBe("#52c41a");
  });

  it("should generate consistent colors for unknown sites", () => {
    const color1 = getAvatarColor("unknownsite");
    const color2 = getAvatarColor("unknownsite");
    expect(color1).toBe(color2);
  });

  it("should generate different colors for different sites", () => {
    const color1 = getAvatarColor("site1");
    const color2 = getAvatarColor("site2");
    expect(color1).not.toBe(color2);
  });

  it("should return HSL format for unknown sites", () => {
    const color = getAvatarColor("randomsite");
    expect(color).toMatch(/^hsl\(\d+, 70%, 50%\)$/);
  });
});

describe("getRatioType", () => {
  it("should return success for ratio >= 2", () => {
    expect(getRatioType(2)).toBe("success");
    expect(getRatioType(5)).toBe("success");
  });

  it("should return info for ratio >= 1", () => {
    expect(getRatioType(1)).toBe("info");
    expect(getRatioType(1.5)).toBe("info");
  });

  it("should return warning for ratio >= 0.5", () => {
    expect(getRatioType(0.5)).toBe("warning");
    expect(getRatioType(0.8)).toBe("warning");
  });

  it("should return danger for ratio < 0.5", () => {
    expect(getRatioType(0.3)).toBe("danger");
    expect(getRatioType(0)).toBe("danger");
  });
});

describe("calculateDaysSinceJoin", () => {
  it("should return 0 for invalid timestamps", () => {
    expect(calculateDaysSinceJoin(0)).toBe(0);
    expect(calculateDaysSinceJoin(-1)).toBe(0);
  });

  it("should calculate days correctly", () => {
    const now = Math.floor(Date.now() / 1000);
    const thirtyDaysAgo = now - 30 * 86400;
    const days = calculateDaysSinceJoin(thirtyDaysAgo);
    expect(days).toBeGreaterThanOrEqual(29);
    expect(days).toBeLessThanOrEqual(31);
  });
});

describe("checkIntervalRequirement", () => {
  it("should return true for empty requirement", () => {
    expect(checkIntervalRequirement(1000000, "")).toBe(true);
  });

  it("should return false for invalid join timestamp", () => {
    expect(checkIntervalRequirement(0, "P5W")).toBe(false);
    expect(checkIntervalRequirement(-1, "P5W")).toBe(false);
  });

  it("should check interval correctly", () => {
    const now = Math.floor(Date.now() / 1000);
    const sixWeeksAgo = now - 6 * 7 * 86400;
    const twoWeeksAgo = now - 2 * 7 * 86400;

    // 6 weeks ago should satisfy P5W requirement
    expect(checkIntervalRequirement(sixWeeksAgo, "P5W")).toBe(true);

    // 2 weeks ago should not satisfy P5W requirement
    expect(checkIntervalRequirement(twoWeeksAgo, "P5W")).toBe(false);
  });
});
