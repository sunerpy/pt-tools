/**
 * App 的返回（系统返回键与顶栏的返回箭头）先问编辑页：最上面有没有开着的一层。
 * 有就关掉它、App 留在编辑页；什么都没开着，App 才离开编辑页（有改动时先问要不要放弃）。
 *
 * 一层是展开的下拉、抽屉或对话框（试搜索的操作单、试搜索面板）。关法和按 Esc 一样，交给 Element Plus：
 * - 下拉：Esc 发在它自己的输入框上，只收起下拉、不往外传；
 * - 抽屉与对话框：焦点陷阱在 document 上收 Esc，只有最上面没被暂停的那一层会关。
 */
declare global {
  interface Window {
    /** App 按返回时调用：关掉了一层返回 true */
    ptMobileBack?: () => boolean;
  }
}

function escape(target: EventTarget) {
  target.dispatchEvent(
    new KeyboardEvent("keydown", {
      key: "Escape",
      code: "Escape",
      bubbles: true,
      cancelable: true,
    }),
  );
}

/** 抽屉或对话框的遮罩：显示着、也不在关闭的动画里（关闭时 Vue 给它加 *-leave-active） */
function openOverlay(el: Element) {
  if ([...el.classList].some((c) => c.endsWith("-leave-active"))) return false;
  const view = el.ownerDocument.defaultView;
  return view ? view.getComputedStyle(el).display !== "none" : true;
}

export function closeTopLayer(doc: Document = document): boolean {
  // 展开着的下拉：el-select 的输入框、el-autocomplete 的外框带 aria-expanded="true"
  const open = doc.querySelectorAll('[role="combobox"][aria-expanded="true"]');
  const box = open[open.length - 1];
  const input = box?.matches("input, textarea") ? box : box?.querySelector("input, textarea");
  if (input) {
    escape(input);
    return true;
  }
  if ([...doc.querySelectorAll(".el-overlay")].some(openOverlay)) {
    escape(doc);
    return true;
  }
  return false;
}

export function installBack(win: Window = window) {
  win.ptMobileBack = () => closeTopLayer(win.document);
}
