// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from "vitest";

import { isInteractiveClick } from "./rowClick";

function clickOn(el: Element): Event {
  const ev = new MouseEvent("click", { bubbles: true });
  Object.defineProperty(ev, "target", { value: el });
  return ev;
}

afterEach(() => {
  document.body.innerHTML = "";
  window.getSelection()?.removeAllRanges();
});

describe("isInteractiveClick", () => {
  it("点到行里的控件不算点行", () => {
    document.body.innerHTML = `<table><tr><td><span id="txt">hdsky</span></td>
      <td><button id="btn"><i id="ico"></i></button></td>
      <td><div class="el-switch"><span id="sw"></span></div></td>
      <td><a id="lnk" href="#">x</a></td></tr></table>`;
    const q = (id: string) => document.getElementById(id)!;
    expect(isInteractiveClick(clickOn(q("txt")))).toBe(false);
    expect(isInteractiveClick(clickOn(q("ico")))).toBe(true);
    expect(isInteractiveClick(clickOn(q("sw")))).toBe(true);
    expect(isInteractiveClick(clickOn(q("lnk")))).toBe(true);
    expect(isInteractiveClick(undefined)).toBe(false);
  });

  it("选中了文字（拖选复制）不算点行", () => {
    document.body.innerHTML = `<p id="p">12.3 TiB</p>`;
    const range = document.createRange();
    range.selectNodeContents(document.getElementById("p")!);
    window.getSelection()!.addRange(range);
    expect(isInteractiveClick(clickOn(document.getElementById("p")!))).toBe(true);
  });
});
