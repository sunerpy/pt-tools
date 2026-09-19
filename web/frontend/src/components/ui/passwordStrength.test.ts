/*
 * 强度估分的钉子。
 *
 * 最要紧的一条是最后那组：这套算法**不是**校验器，后端 `apiPassword` 对新口令
 * 的长度和复杂度一个字都不查，前端唯一的硬门槛是表单上的 6 位下限。所以
 * 「算出 0 分」和「不许提交」必须是两件事 —— 组件不 emit、不返回任何
 * 可当成 disabled 的布尔值，这里把这个约定钉住。
 */
import { describe, expect, it } from "vitest";

import { scorePassword } from "./passwordStrength";

describe("scorePassword", () => {
  it("空口令是 level 0，条长为 0，不给任何判断", () => {
    const s = scorePassword("");
    expect(s.level).toBe(0);
    expect(s.percent).toBe(0);
    expect(s.label).toBe("—");
  });

  it.each([
    ["123456", "键盘顺子"],
    ["abcdef", "字母顺子"],
    ["aaaaaa", "整串重复"],
    ["123123", "短片段重复"],
    ["password", "字典词"],
    ["adminadmin", "本项目 PT_ADMIN_PASS 为空时写进库的默认口令"],
  ])("%s 判为很好猜（%s）", (pwd) => {
    expect(scorePassword(pwd).level).toBe(1);
  });

  it("0 换 o 这类替换骗不过字典：Passw0rd! 仍按常见口令扣分", () => {
    // 四类字符全齐、9 位，不还原替换的话会被算到「一般」
    expect(scorePassword("Passw0rd!").level).toBe(2);
  });

  it("口令里嵌了用户名要扣分", () => {
    const pwd = "Sunerpy#2026x";
    expect(scorePassword(pwd, "").level).toBeGreaterThan(scorePassword(pwd, "sunerpy").level);
  });

  it("用户名短于 3 位不参与判断，避免两字母子串误伤", () => {
    const pwd = "Qm#7vLx9tKp2";
    expect(scorePassword(pwd, "qm")).toEqual(scorePassword(pwd, ""));
  });

  it("长口令即使只有小写也不冤枉", () => {
    // 21 位、两类字符（小写 + 空格），搜索空间胜过一串 10 位乱码
    expect(scorePassword("correct horse battery").level).toBe(4);
  });

  it("够长又混了多类字符才给到最高档", () => {
    expect(scorePassword("Qm#7vLx9tKp2Rw5").level).toBe(4);
    // 同样的花样但只有 8 位，档位要落下来
    expect(scorePassword("Qm#7vLx9").level).toBeLessThan(4);
  });

  it("level 与条长、色调严格对应，免得读数和条长说两回事", () => {
    const tones: Record<number, string> = { 0: "info", 1: "dang", 2: "warn", 3: "info", 4: "ok" };
    for (const pwd of ["", "123456", "hunter22", "Qm#7vLx9", "Qm#7vLx9tKp2Rw5"]) {
      const s = scorePassword(pwd);
      expect(s.percent).toBe(s.level * 25);
      expect(s.tone).toBe(tones[s.level]);
    }
  });

  it("四条维度恒定输出，空口令也要列全", () => {
    for (const pwd of ["", "x", "Qm#7vLx9tKp2Rw5"]) {
      expect(scorePassword(pwd).checks).toHaveLength(4);
    }
  });

  it("不返回任何可当成提交门槛的字段：强度不拦提交", () => {
    // 比表单下限（6 位）更短、评分最低的口令，依然只是「分低」
    const s = scorePassword("12345");
    expect(s.level).toBe(1);
    expect(Object.keys(s).sort()).toEqual(["checks", "label", "level", "percent", "tone"]);
  });
});
