/**
 * 表格「点行进详情」用：点到行里的按钮、开关、链接、输入框、下拉这些控件时不算点行，
 * 它们有自己的动作；拖选文字（选中了内容）也不算，免得复制一个数字就跳走了。
 */
const INTERACTIVE = [
  "button",
  "a",
  "input",
  "textarea",
  "select",
  "label",
  "[role='button']",
  "[role='switch']",
  "[role='checkbox']",
  "[role='menuitem']",
  "[contenteditable='true']",
  ".el-switch",
  ".el-checkbox",
  ".el-radio",
  ".el-dropdown",
  ".el-select",
  ".el-input",
  ".el-input-number",
].join(", ");

export function isInteractiveClick(event: Event | undefined): boolean {
  const target = event?.target;
  if (target instanceof Element && target.closest(INTERACTIVE) !== null) {
    return true;
  }
  const selection = typeof window === "undefined" ? null : window.getSelection();
  return Boolean(selection && !selection.isCollapsed && selection.toString().trim() !== "");
}

/** 给 el-table 的 row-class-name：可点的行显示手形光标 */
export const ROW_LINK_CLASS = "pt-row-link";
