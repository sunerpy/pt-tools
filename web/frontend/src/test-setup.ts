/**
 * 只在测试里跑：把 happy-dom 的 localStorage / sessionStorage 补回全局。
 *
 * vitest 的 happy-dom 环境用 populateGlobal 把窗口对象上的键搬到 globalThis，但它会
 * 跳过「Node 自己已经有同名全局、又不在它那份白名单里」的键。Node 25 正好自带一个
 * 原生 localStorage，没有 --localstorage-file 时一读就抛 SecurityError，于是 window
 * 上那份 happy-dom 实现被彻底遮住，任何 localStorage 访问都成了报错。
 *
 * pinia 引的 @vue/devtools-kit 在模块求值阶段就要读 localStorage，比测试正文还早，
 * 所以补丁只能放在 setupFiles 里。
 */
function unusable(name: "localStorage" | "sessionStorage") {
  try {
    return globalThis[name] === undefined;
  } catch {
    // 原生实现抛 SecurityError 就走到这里
    return true;
  }
}

// node 环境的用例（例如 utils/format.test.ts）没有 DOM，也用不到 storage，直接跳过
if (typeof document !== "undefined") {
  for (const name of ["localStorage", "sessionStorage"] as const) {
    if (!unusable(name)) {
      continue;
    }
    Object.defineProperty(globalThis, name, {
      value: new Storage(),
      configurable: true,
      writable: true,
    });
  }
}

/*
 * happy-dom 的 MutationObserver、ResizeObserver 没有 Symbol.toStringTag：Vue 的 ref 把它们当成普通对象包成 Proxy，
 * 经 Proxy 调 observe 时读不到私有字段就抛错（浏览器里这两个类有 toStringTag，Vue 不会包）。
 * Element-Plus-X 的 BubbleList 把 observer 存在 ref 里，所以补上和浏览器一样的标签。
 */
if (typeof document !== "undefined") {
  for (const name of ["MutationObserver", "ResizeObserver", "IntersectionObserver"] as const) {
    const ctor = (globalThis as Record<string, unknown>)[name] as
      | { prototype?: object }
      | undefined;
    if (ctor?.prototype && !(Symbol.toStringTag in ctor.prototype)) {
      Object.defineProperty(ctor.prototype, Symbol.toStringTag, {
        value: name,
        configurable: true,
      });
    }
  }
}
