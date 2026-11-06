#!/usr/bin/env node
/*
 * 画板一致性验收 —— 拿真实渲染去比 Penpot 画板的构成，不一致就退非零。
 *
 * 为什么需要它：之前的验收脚本只在「横向溢出」或「出现未解析组件」时失败，几何数字
 * 只是打印出来给人看。于是少一整组卡片、分栏宽度错、坐标错都能拿到绿灯 ——
 * 一次 review 直接把这一点判成「验收不能支持已经按稿完成的结论」，判得对。
 *
 * 这个文件就是**画板事实的可执行副本**：
 *   EXPECT      每条路由该有哪些带、卡片层有哪些**栏宽**、至少几张卡（数字全部来自
 *               Penpot 读回，记在 docs/design/webui-board-spec.md）
 *   ALLOWED_GAPS 明确记下来的偏离，每条都得写原因。不在这张表里的缺失一律失败。
 *
 * 它校验的是带与栏的几何，不是逐张卡的存在：同一栏里少一张卡这种事量不出来，
 * 那一层靠 docs/design/webui-board-spec.md 的偏离表记录，别把它当成「全都对上了」。
 *
 * 用法（需要一个已经跑起来的 pt-tools 服务，前端 dist 必须是当前源码构建出来的）：
 *   node scripts/board-check.mjs http://127.0.0.1:8080 [--port 19400] [--route /tasks]
 *
 * 退出码：0 = 与画板一致，且没有等 owner 拍板的未落地设计项；2 = 结构一致但还有 N 项
 * 画板画了没落地、等 owner 逐项接受；1 = 有未登记差异或登记多余。
 *
 * 依赖：只用 Chrome 的 CDP，不装任何浏览器自动化包。Chrome 路径可用 PT_CHROME 覆盖。
 */
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { emptyStubScript, stubScript } from "./board-fixtures.mjs";

// ---------------------------------------------------------------- 画板事实

/**
 * 带式表格页的构成：head 64 → toolbar 40 → grid → （selband 44）→ foot 34 → 卡片。
 * 卡片宽度按画板的栏宽写，单位 px（1440 画布下主区 1112 宽、卡片区 1080 宽）。
 */
const BAND = "band";
/** 卡片页：head 之后直接进卡片层，没有表格带 */
const CARD = "card";

const EXPECT = {
  "/userinfo": {
    board: "10 用户统计",
    kind: BAND,
    ownHead: "kpi", // 画板用 KPI 带替代 head
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 1080], // p-up / p-dist / p-watch
    minCards: 3, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["上传构成", "等级分布", "需要关注"],
    /*
     * 画板 10 的 bar-64：seg（全部站点/正常/异常/保号预警）+ q + chip-0「周期: 本周」
     * + chip-1「排序: 分享率」+ 右端三枚图标钮。周期那枚由每日快照支撑（summary / trends）。
     */
    needsSeg: true,
    controls: ["全部站点", "正常", "异常", "保号预警", "排序", "列设置", "导出", "周期"],
    /* 画板 10 的行高与十列（docs/design/webui-board-spec.md §6）；差异见 ALLOWED_GAPS */
    gridRowHeight: 34,
    /* 画板 10 的十列。「判定活跃」曾经漏在这张表外 —— 期望表自己漏列，检查就永远发现不了 */
    gridColumns: [
      "站点",
      "等级",
      "数据量",
      "分享率",
      "做种",
      "做种体积",
      "积分",
      "时魔",
      "判定活跃",
      "更新",
    ],
    probes: [
      {
        /*
         * 「发现新版本」通知（duration 0、不会自己消失）不能压在底部状态条上 ——
         * 状态条右端就是版本按钮，看到提示后要点的正是那一格。假数据里一直有 v0.48.0，每次进页面都会弹。
         */
        desc: "「发现新版本」通知不压底部状态条",
        want: "verdict=ok",
        js: `(async () => {
          const t0 = performance.now();
          let n = null;
          while (performance.now() - t0 < 5000) {
            n = [...document.querySelectorAll('.el-notification')]
              .find((e) => (e.textContent ?? '').includes('发现新版本') && e.getBoundingClientRect().height > 0);
            if (n) break;
            await new Promise((r) => setTimeout(r, 50));
          }
          if (!n) return 'no-notification';
          const bar = document.querySelector('.pt-status');
          if (!bar) return 'no-status-bar';
          const nb = n.getBoundingClientRect().bottom;
          const bt = bar.getBoundingClientRect().top;
          return 'verdict=' + (nb <= bt ? 'ok' : 'MISMATCH') + ' 通知下沿 ' + Math.round(nb) + ' / 状态条上沿 ' + Math.round(bt);
        })()`,
      },
      {
        /*
         * 用户原话「右侧操作列背景透明，底层文字内容会透出来」：固定列是 sticky 单元格、背景 inherit 自行，
         * 选中行的底之前是 12% 混透明。点选一行、横滚一段，再量每个固定单元格的背景 alpha。
         */
        desc: "选中行时固定操作列的背景不透明（横滚时底下的字不透出来）",
        want: "verdict=ok",
        js: `(async () => {
          const alpha = (c) => {
            const m = c.match(/rgba?\\(([^)]+)\\)/);
            if (m) { const p = m[1].split(',').map((x) => x.trim()); return p.length > 3 ? Number(p[3]) : 1; }
            const k = c.match(/\\/\\s*([0-9.]+)\\s*\\)/);
            return k ? Number(k[1]) : 1;
          };
          const rows = [...document.querySelectorAll('.dash .pt-grid .el-table__body tr.el-table__row')];
          if (rows.length < 2) return 'no-rows';
          rows[1].querySelector('td:nth-child(3)').click();
          const wrap = document.querySelector('.dash .pt-grid .el-table__body-wrapper .el-scrollbar__wrap');
          if (wrap) { wrap.scrollLeft = 200; wrap.dispatchEvent(new Event('scroll')); }
          await new Promise((r) => setTimeout(r, 300));
          const cells = [...document.querySelectorAll('.dash .pt-grid .el-table__body td.el-table-fixed-column--right')];
          if (!cells.length) return 'no-fixed-column';
          const low = cells.map((c) => alpha(getComputedStyle(c).backgroundColor)).filter((a) => a < 0.999);
          const selected = !!document.querySelector('.dash .pt-grid tr.current-row');
          return 'verdict=' + (selected && low.length === 0 ? 'ok' : 'MISMATCH') + ' 选中行 ' + selected + '，半透明格 ' + low.length + ' / ' + cells.length;
        })()`,
      },
      {
        /*
         * 站点表把手（用户原话「支持下拉 / 上拉拖拽方式动态调整面板高度」）。页面 JS 合成的指针事件没有活动指针，
         * 这里走键盘：↓ 一步变高 24、↑ 一步变矮 24，Enter 回到自适应（高度回到原值、localStorage 里的记录清掉）。
         * ↓ 必须查：假数据 8 个站点，表是贴合内容的高度 —— 手动高度只写 max-height 时它只是上限，
         * 往下拉纹丝不动（评审抓到的缺陷），只查 ↑ 是查不出来的。
         */
        desc: "站点表把手：键盘 ↓ 变高、↑ 变矮，Enter 恢复自适应",
        want: "verdict=ok",
        js: `(async () => {
          const until = async (fn, ms) => {
            const t0 = performance.now();
            while (performance.now() - t0 < ms) { if (fn()) return true; await new Promise((r) => setTimeout(r, 50)); }
            return fn();
          };
          const table = document.querySelector('.dash .pt-grid');
          const handle = document.querySelector('[data-testid=pt-resize-handle]');
          if (!table || !handle) return 'no-handle';
          const KEY = 'pt-userinfo-sites-height-v1';
          localStorage.removeItem(KEY);
          const h0 = table.offsetHeight;
          handle.focus();
          const press = async (key, until_) => {
            handle.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }));
            await until(until_, 2000);
            return table.offsetHeight;
          };
          const hDown = await press('ArrowDown', () => table.offsetHeight !== h0);
          const storedDown = localStorage.getItem(KEY);
          const hBack1 = await press('Enter', () => table.offsetHeight === h0);
          const hUp = await press('ArrowUp', () => table.offsetHeight !== h0);
          const storedUp = localStorage.getItem(KEY);
          const hBack2 = await press('Enter', () => table.offsetHeight === h0);
          const cleared = localStorage.getItem(KEY) === null;
          const ok = hDown - h0 === 24 && storedDown === String(hDown) && hBack1 === h0 &&
            h0 - hUp === 24 && storedUp === String(hUp) && hBack2 === h0 && cleared &&
            handle.getAttribute('role') === 'separator';
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 高度 ' + h0 + ' ↓' + hDown + ' ⏎' + hBack1 +
            ' ↑' + hUp + ' ⏎' + hBack2 + '，存 ' + storedDown + ' / ' + storedUp + '，清掉 ' + cleared;
        })()`,
      },
      {
        /*
         * el-popover 自己不响应 Esc；App 的 Esc 先收「偏好与账户」浮层。窄屏抽屉里之前是反过来的：
         * Esc 收掉抽屉、浮层悬在原处。这里在钉住的导航列上验「Esc 收浮层」这条接线。
         */
        desc: "「偏好与账户」浮层按 Esc 收起",
        want: "verdict=ok",
        js: `(async () => {
          const until = async (fn, ms) => {
            const t0 = performance.now();
            while (performance.now() - t0 < ms) { if (fn()) return true; await new Promise((r) => setTimeout(r, 50)); }
            return fn();
          };
          const shown = () => [...document.querySelectorAll('.pt-prefs-popper')]
            .some((p) => getComputedStyle(p).display !== 'none' && p.getBoundingClientRect().height > 0);
          const who = document.querySelector('.pt-nav__account-who');
          if (!who) return 'no-account-row';
          who.click();
          if (!(await until(shown, 3000))) return 'popover-not-opened';
          window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', code: 'Escape', bubbles: true }));
          const closed = await until(() => !shown(), 2000);
          return 'verdict=' + (closed ? 'ok' : 'MISMATCH') + (closed ? '' : '（按 Esc 后浮层仍开着）');
        })()`,
      },
    ],
  },
  "/userinfo/export": {
    board: "11 导出分享图",
    kind: CARD,
    cards: [700, 364], // p-prev / p-set
    minCards: 2, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["预览", "导出设置"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/sites": {
    board: "12 站点列表",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"],
    cards: [364, 340, 344, 1080], // p-health / p-rss / p-ev / p-auth
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["探测健康", "RSS 订阅", "保号提醒", "认证方式"],
    /*
     * 画板 12 的 bar-64 上除了分段与搜索，还有两枚筛选 chip（认证 / 探测），
     * 画板 30 的移动稿上还有一排带计数的状态 chip（正常 / 异常 / 已禁用）。
     * 这一条此前**完全没查**：EXPECT 里既没有 controls 也没有 needsSeg，
     * 于是「工具栏少了三组筛选」在验收里看不出来。
     */
    needsSeg: true,
    controls: [
      "已启用",
      "全部",
      "正常",
      "异常",
      "已禁用",
      "认证",
      "探测",
      /* 画板 bar-64 右端那两枚 28×28 图标钮 */
      "列设置",
      "导出",
    ],
    /* 画板 12 的表头（th-6/7/8「自动 / 手动 / 禁用」是探测模式那一列里的三段控件，不是列） */
    gridColumns: ["#", "站点", "状态", "认证", "RSS", "探测", "操作"],
    probes: [
      {
        /*
         * 这条断言原先写的是 `after === 0`，而说明写的是「行数应当变成该档的计数」——
         * 两者不是一回事，于是它把一个假筛选定义成了成功：计数从全部站点算、
         * 筛却在「已启用」里筛，点非零的「已禁用 N」必定 0 行，断言照样通过。
         * 现在断言的是 chip 上那个数字本身。
         */
        desc: "状态 chip 真的在筛（点「已禁用 N」之后表格行数应当正好是 N）",
        want: "verdict=ok",
        js: `(async () => {
          const rows = () => document.querySelectorAll('.pt-band--grid tbody tr').length;
          const chip = document.querySelector('[data-testid=site-status-chip-off]');
          if (!chip) return 'no-chip';
          const label = (chip.textContent ?? '').trim();
          const want = Number((label.match(/(\\d+)/) ?? [])[1] ?? -1);
          const before = rows();
          chip.click();
          await new Promise((r) => setTimeout(r, 600));
          const after = rows();
          chip.click();
          await new Promise((r) => setTimeout(r, 400));
          const restored = rows();
          /* want 必须 > 0，否则这条探针是空的：0 行等于 0 行，什么都没证明 */
          const ok = want > 0 && after === want && restored === before;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') +
            ' chip=' + label + ' 期望 ' + want + ' 行数 ' + before + '→' + after + '→' + restored;
        })()`,
      },
    ],
  },
  "/supported-sites": {
    board: "14 已支持站点",
    kind: CARD,
    bands: ["toolbar"], // bar-64，之后直接是卡
    cards: [548, 516, 1080], // g0..g3 两栏（数量随站点数变）+ p-cap 通栏
    minCards: 5, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["内置能力"],
    /* 画板 14 的 bar-64：seg（架构）+ q + chip-0「已添加: 全部」+ 右端两枚视图钮 */
    needsSeg: true,
    controls: ["搜索", "已添加", "NexusPHP", "卡片视图", "紧凑列表"],
  },
  "/search": {
    board: "15 种子搜索",
    kind: BAND,
    ownHead: "head88", // search-head 88：查询框 + 筛选 chip
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 1080], // p-sites / p-alt / p-hist
    minCards: 3, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["站点命中", "没返回的站点", "搜索历史", "已保存的搜索"],
    /*
     * 铺一条 v1 格式的「保存搜索」：它的 category 存的是站点原始分类名，
     * 读取时要被迁移成桶 ID。这条分支只有老用户才会走到，必须显式铺出来。
     */
    seed: `localStorage.setItem(
      'pt-tools-search-saved-v1',
      JSON.stringify([{ name: 'Dune', keyword: 'Dune', sites: [], sortBy: 'seeders',
        orderDesc: true, category: '电影/HD', freeOnly: false }]),
    )`,
    needsSeg: true, // 画板 bar-88 的分类分段
    /*
     * 分类分段器点下去到底有没有用。画板画了这五个档位，而它的作用只有点了才看得出来：
     * 假数据里每三条有一条是 Gazelle 形状（没有分类、只有题材标签），这类行必须**不**
     * 出现在具体档位下，并且被筛掉的条数要写在表格上方。
     */
    probes: [
      /*
       * 「筛选」浮层（媒体中心 M2a，按解析信息筛）：勾「1080p」以后表格只剩那几行，
       * 行数等于选项上写的数。排在分类那条探针前面，在没动过的状态上点，点完再取消。
       */
      {
        desc: "筛选浮层：勾 1080p，表格行数变成选项上的数，取消后回到原数",
        want: "verdict=ok",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const rows = () => document.querySelectorAll('.el-table__body tbody tr').length;
          const btn = document.querySelector('[data-testid="search-facets-btn"]');
          if (!btn) return 'no-facets-btn';
          btn.click();
          await wait(500);
          const opt = document.querySelector('[data-testid="es-facet-resolution-1080p"]');
          if (!opt) return 'no-option';
          const n = Number((opt.querySelector('.es-facets__n')?.textContent ?? '').trim());
          const before = rows();
          opt.click();
          await wait(500);
          const after = rows();
          const label = (btn.textContent ?? '').replace(/\s+/g, ' ').trim();
          opt.click();
          await wait(500);
          const back = rows();
          btn.click();
          await wait(300);
          const ok = n > 0 && after === n && after < before && back === before && label === '筛选 · 1';
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 选项 ' + n + ' 行 ' + before + '→' + after + '→' + back + ' 按钮 ' + label;
        })()`,
      },
      {
        desc: "分类 + 仅免费 + 一并显示：说明里的条数与表格真会多出来的行数一致",
        want: "verdict=ok",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const click = (pred) => {
            const el = [...document.querySelectorAll('button, .el-segmented__item, .pt-seg__item, .el-checkbox')]
              .find(pred);
            if (!el) return false;
            el.click();
            return true;
          };
          /* 先开「仅免费」，再点「音乐」—— 两个筛选叠着才能测出计数用错集合 */
          if (!click((b) => (b.textContent ?? '').trim().startsWith('仅免费'))) return 'no-free';
          await wait(400);
          if (!click((b) => (b.textContent ?? '').trim() === '音乐')) return 'no-seg';
          await wait(600);
          const noteText = (document.querySelector('.pt-note.nocat')?.textContent ?? '');
          const said = Number((noteText.match(/(\\d+)\\s*条/) ?? [])[1] ?? -1);
          const before = document.querySelectorAll('.el-table__body tbody tr').length;
          if (!click((b) => (b.textContent ?? '').trim() === '一并显示')) return 'no-toggle:' + noteText;
          await wait(600);
          const after = document.querySelectorAll('.el-table__body tbody tr').length;
          const delta = after - before;
          /*
           * 哨兵必须互不包含：原来返回「一致 / 不一致」而断言用 includes('一致')——
           * 失败串「不一致」里也含「一致」，于是这条探针**失败也会通过**。
           * 这正是这份脚本一直在抓的那类假绿，自己却踩了一次。
           */
          return 'verdict=' + (said >= 0 && delta === said ? 'ok' : 'MISMATCH') +
            ' 说明 ' + said + ' 条 / 表格 ' + before + '→' + after;
        })()`,
      },
    ],
    /* 画板 bar-88 的五个固定档位 + search-head 的 go/ha-0/ha-1 + chip-1 */
    /* 画板 bar-88：五档分类 seg + chip-0「做种数 ↓」（排序）+ chip-1「仅免费」+ 右端两枚图标钮 */
    controls: [
      "搜索",
      "保存搜索",
      "最近搜索",
      "仅免费",
      "电影",
      "剧集",
      "动漫",
      "音乐",
      "排序",
      "列设置",
      /* 按解析信息筛的浮层（媒体中心 M2a，画板 06 的筛选组）；画板 15 没画 */
      "筛选",
    ],
    /* 画板 15 的表头 */
    gridColumns: ["站点", "标题", "大小", "优惠", "上传", "下载", "完成", "发布时间", "操作"],
    /*
     * 搜索页的结果、页脚带与分析卡都要先搜一次才有。这里直接写进输入框再点按钮，
     * 不动内部状态 —— 走的是用户真实路径，页面自己的 loading / 六态照常参与。
     */
    prepare: `(async () => {
      const input = document.querySelector('.pt-band--head input');
      if (!input) return 'no-input';
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setter.call(input, 'Dune');
      input.dispatchEvent(new Event('input', { bubbles: true }));
      await new Promise((r) => setTimeout(r, 200));
      const go = [...document.querySelectorAll('.pt-band--head button')]
        .find((b) => b.textContent.includes('搜索'));
      if (!go) return 'no-button';
      go.click();
      await new Promise((r) => setTimeout(r, 1500));
      return 'searched';
    })()`,
  },
  "/tasks": {
    board: "16 任务列表",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 1080], // p-thr / p-site / p-warn
    minCards: 3, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["最近 7 天吞吐", "按站点分布", "需要关注的任务"],
    /*
     * 画板 16 的 bar-64：seg + q + chip-0「站点: 全部」+ chip-1「优惠: Free」+ 右端三枚图标钮
     * （列设置 / 导出 / 刷新）。画板的表头是
     * 站点 / 优惠 / 标题 / 大小 / 进度 / 免费结束 / 最后检查 / 状态。
     */
    /* 画板这一页有分段器（五档下载器状态）；落地没有，偏离见 ALLOWED_GAPS */
    needsSeg: true,
    controls: ["已下载", "已推送", "已过期", "站点", "优惠", "列设置", "导出"],
    gridColumns: ["站点", "优惠", "标题", "大小", "进度", "免费结束", "最后检查", "状态"],
  },
  "/paused": {
    board: "17 暂停任务",
    kind: CARD,
    cards: [1080, 1080], // c-paused / c-archive
    minCards: 2, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["暂停中", "历史归档"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/downloader-hub": {
    board: "18 下载器 Web UI",
    kind: CARD,
    bands: ["toolbar"],
    cards: [276, 276, 276, 788, 386, 386], // 左列三卡 / 右上 p-grid / 右下 p-rate + p-note
    minCards: 6, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["传输", "状态", "筛选", "任务列表", "速率", "这一页的口径"],
    /*
     * 画板 18 的 bar-64：带计数的状态 seg + q「搜索标题、分类、标签…」+ chip「视图」
     * + 右端三枚图标钮。状态筛选落在左栏那张「状态」卡里（画板也把状态画在那张卡上），
     * 工具栏里不再摆第二份（同一屏两份同样的入口正是用户退回过的事），
     * 所以这里只查那几档状态的标签在不在，不要求它们长在工具栏带里。
     */
    controls: ["搜索标题、分类、标签", "列", "导出"],
    /*
     * 画板 18 的状态筛选画在「状态」那张卡里，不在 bar-64 上。
     * 这里只写读回规格里确实有的词（sub 示例：「12 下载中 · 19 做种中 · 6 已暂停」）——
     * 那张卡每一档的确切措辞不在读回规格里，所以「暂停」取的是公共子串，
     * 「暂停」与「已暂停」都能对上，不拿我猜的措辞去当画板事实。
     */
    pageControls: ["全部", "下载中", "做种中", "暂停"],
    /*
     * 画板 38（弹窗与菜单）与 39（任务详情抽屉）的规格。它们只有点开才存在，
     * 所以用探针量：列 popover 260、添加种子弹窗 620、任务详情抽屉 56%（1440 上 ≈806）。
     */
    probes: [
      {
        desc: "画板 38 的列 popover（260 宽）",
        want: "宽 260",
        js: `(async () => {
          const btn = document.querySelector('.hub__ico[aria-label=显示列]');
          if (!btn) return 'no-col-btn';
          btn.click();
          await new Promise((r) => setTimeout(r, 500));
          const pop = [...document.querySelectorAll('.el-popper')]
            .filter((el) => getComputedStyle(el).display !== 'none')
            .find((el) => (el.textContent ?? '').includes('列'));
          if (!pop) return 'no-popper';
          const w = Math.round(pop.getBoundingClientRect().width);
          document.body.click();
          return '宽 ' + w;
        })()`,
      },
      {
        desc: "画板 38 的行右键菜单（180 宽）",
        want: "宽 180",
        js: `(async () => {
          const row = document.querySelector('.hub__grid tbody tr, .vt-row');
          if (!row) return 'no-row';
          const r = row.getBoundingClientRect();
          row.dispatchEvent(new MouseEvent('contextmenu', {
            bubbles: true,
            clientX: Math.round(r.left + 40),
            clientY: Math.round(r.top + 8),
          }));
          await new Promise((res) => setTimeout(res, 600));
          const menu = document.querySelector('.table-context-menu');
          if (!menu) return 'no-ctx-menu';
          const w = Math.round(menu.getBoundingClientRect().width);
          document.body.click();
          return '宽 ' + w;
        })()`,
      },
      {
        desc: "画板 38 的添加种子弹窗（620 宽，卡头 42）",
        want: "宽 620|添加种子到下载器",
        js: `(async () => {
          const btn = [...document.querySelectorAll('button')]
            .find((b) => (b.textContent ?? '').includes('添加种子'));
          if (!btn) return 'no-add-btn';
          btn.click();
          await new Promise((r) => setTimeout(r, 600));
          const dlg = [...document.querySelectorAll('.el-dialog')]
            .find((el) => (el.textContent ?? '').includes('添加种子到下载器'));
          if (!dlg) return 'no-dialog';
          const w = Math.round(dlg.getBoundingClientRect().width);
          const title = (dlg.querySelector('.el-dialog__title')?.textContent ?? '').trim();
          const esc = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true });
          document.dispatchEvent(esc);
          await new Promise((r) => setTimeout(r, 400));
          return '宽 ' + w + ' | ' + title;
        })()`,
      },
      {
        desc: "画板 39 的任务详情：抽屉 56% 视口 / 内联 772（主区内、与视口无关），且详情请求打在真实 task_id 上",
        want: "verdict=ok",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          /*
           * 记下详情请求真正打到哪个地址。
           * 只量宽度、或只在抽屉里找一个固定字符串，都挡不住「地址拼错、假数据又照着同样的
           * 错地址登记」这种两头一起错 —— 那是评审连着两轮指出的同一处假绿。
           * 所以这里拦一层 fetch，把实际 URL 拿出来比。
           */
          const seen = [];
          const realFetch = window.fetch;
          window.fetch = (input, init) => {
            const raw = typeof input === 'string' ? input : input.url;
            if (raw.includes('/api/downloader-torrents/')) seen.push(raw);
            return realFetch(input, init);
          };
          /*
           * 行高、详情位置、数据范围收进了工具栏的「视图」面板（以前是四组没有组名的分段平铺在带上）。
           * 面板第一次点开才渲染，所以找不到档位时先点开它再找。
           */
          const pickMode = async (label) => {
            const find = () => [...document.querySelectorAll('.el-segmented__item')]
              .find((b) => (b.textContent ?? '').trim() === label);
            let el = find();
            if (!el) {
              document.querySelector('[data-testid=hub-view-btn]')?.click();
              await wait(400);
              el = find();
            }
            if (!el) return false;
            el.click();
            return true;
          };
          const openRow = async () => {
            const btn = [...document.querySelectorAll('button')]
              .find((b) => (b.textContent ?? '').trim() === '详情');
            if (!btn) return false;
            btn.click();
            await wait(900);
            return true;
          };
          const esc = () => document.dispatchEvent(
            new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
          );
          if (!(await pickMode('侧边抽屉'))) { window.fetch = realFetch; return 'no-mode-seg'; }
          await wait(400);
          if (!(await openRow())) { window.fetch = realFetch; return 'no-detail-btn'; }
          const drawer = document.querySelector('.hub__drawer');
          const dw = drawer ? Math.round(drawer.getBoundingClientRect().width) : 0;
          const dtext = drawer ? (drawer.textContent ?? '').replace(/\\s+/g, ' ') : '';
          esc();
          await wait(500);
          if (!(await pickMode('表格下方'))) { window.fetch = realFetch; return 'no-mode-seg-2'; }
          await wait(400);
          if (!(await openRow())) { window.fetch = realFetch; return 'no-detail-btn-2'; }
          const inline = document.querySelector('.hub__detail');
          const iw = inline ? Math.round(inline.getBoundingClientRect().width) : 0;
          window.fetch = realFetch;

          /* 详情地址：必须是 /{downloader_id}/{task_id}，且 task_id 不能是 undefined */
          const detailUrls = seen.filter((u) => /\\/api\\/downloader-torrents\\/\\d+\\//.test(u));
          const lastUrl = detailUrls[detailUrls.length - 1] ?? '';
          const taskId = lastUrl.split('/').pop() ?? '';
          const pathOk = taskId !== '' && taskId !== 'undefined' && !taskId.includes('?');
          /* 抽屉里显示的身份要和被点的那一行对得上（标题取自同一份数据） */
          const identityOk = dtext.length > 0 && taskId !== '' && dtext.includes('Hub.Task.');
          const ok =
            Math.abs(dw - Math.round(window.innerWidth * 0.56)) <= 4 && Math.abs(iw - 772) <= 12 && pathOk && identityOk;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') +
            ' 抽屉 ' + dw + '（' + Math.round(window.innerWidth * 0.56) + '） 内联 ' + iw + '（772） 详情地址 ' +
            (lastUrl || '没有发出详情请求') + ' 身份=' + (identityOk ? '对上' : dtext.slice(0, 50));
        })()`,
      },
    ],
  },
  "/downloaders": {
    board: "19 下载器设置",
    kind: CARD,
    cards: [1080, 548, 516, 1080, 1080], // p-dl / p-dir / p-safe / p-bind / p-log
    minCards: 5, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["下载器", "下载目录", "磁盘保护", "站点绑定", "连通性检查"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/filter-rules": {
    board: "20 过滤规则",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"], // bar-64 落地为空带，见 ALLOWED_GAPS
    cards: [548, 516, 1080, 1080], // p-order / p-test / p-hit / p-hint
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["匹配顺序", "试跑", "命中统计", "规则怎么生效"],
    needsSeg: true, // 画板 bar-64 的「全部 / 启用 / 禁用」分段
    /* 画板 20 的 bar-64：seg + q + chip-0「类型: 全部」+ chip-1「仅免费: 全部」+ 右端三枚图标钮 */
    controls: ["全部", "启用", "禁用", "类型", "仅免费", "列设置", "导出"],
    /* 画板 20 的「试跑这条」画在 p-test 那张卡里，不在 bar-64 上 */
    pageControls: ["试跑这条"],
    /*
     * 列设置里的每一项都必须真的能关掉那一列。
     *
     * 这条探针来自一个真实缺陷：「匹配范围」在列设置里列着、勾选状态也存进了
     * localStorage，而表头那一列压根没接 ruleColShown('scope') —— 勾掉它什么都不会发生。
     * 光看工具栏有没有「列设置」这枚钮是查不出无效控件的。
     */
    probes: [
      {
        desc: "列设置里勾掉「匹配范围」，表头那一列要真的消失（无效控件查不出来）",
        want: "verdict=ok",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const heads = () => [...document.querySelectorAll('.pt-band--grid th .cell')]
            .map((el) => (el.textContent ?? '').trim());
          const before = heads();
          if (!before.includes('匹配范围')) return 'no-column:' + before.join('/');
          document.querySelector('[data-testid=rules-cols-btn]')?.click();
          await wait(500);
          const row = [...document.querySelectorAll('.rules-cols__row')]
            .find((el) => (el.textContent ?? '').includes('匹配范围'));
          if (!row) return 'no-row';
          row.querySelector('.el-checkbox')?.click();
          await wait(600);
          const after = heads();
          const ok = !after.includes('匹配范围') && after.length === before.length - 1;
          /* 勾回去：这个偏好存 localStorage，留着会影响后面几趟（移动 / 空态）对同一页的量测 */
          row.querySelector('.el-checkbox')?.click();
          await wait(400);
          const restored = heads();
          return 'verdict=' + (ok && restored.includes('匹配范围') ? 'ok' : 'MISMATCH') +
            ' 列数 ' + before.length + '→' + after.length + '→' + restored.length;
        })()`,
      },
    ],
    /* 画板 20 的表头（落地多一列「启用」，是功能列，按 owner 原则保留） */
    gridColumns: [
      "序号",
      "名称",
      "匹配模式",
      "类型",
      "匹配范围",
      "优先级",
      "仅免费",
      "大小范围",
      "操作",
    ],
  },
  "/cleanup": {
    board: "21 自动删种",
    kind: CARD,
    cards: [1080], // p-main
    minCards: 1, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["自动删种"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
    /*
     * 画板 41（全局外壳）的两个浮层挂在外壳上，哪条路由都能验 —— 选这一页是因为它最轻
     * （一张卡，没有表格与轮询），点开浮层不会和页面自己的请求抢时间。
     *   user-pop  280 宽：头像 + admin + 「管理员 · 单用户模式」+ 偏好（外观 / 配色 / 日志级别）
     *   sched-pop 300 宽：「调度器」+ 状态胶囊 + 最后同步与间隔 + 停止 / 启动
     */
    probes: [
      {
        desc: "头像浮层 = 画板 41 的 user-pop（280 宽，含身份与三组偏好）",
        want: "宽 280|单用户模式|配色|日志级别",
        js: `(async () => {
          /* 钉住时 rail 淡出、入口是 nav 账号行的头像；收起时才是 rail 头像。点看得见的那个 */
          const av = [...document.querySelectorAll('.pt-nav__account-who, .pt-rail__avatar')]
            .find((el) => getComputedStyle(el).visibility !== 'hidden');
          if (!av) return 'no-avatar';
          av.click();
          await new Promise((r) => setTimeout(r, 500));
          /* rail 与 nav 各挂一个同类浮层（persistent），没打开的那个宽 0；取铺开了的那个 */
          const pop = [...document.querySelectorAll('.pt-prefs-popper')]
            .find((el) => el.getBoundingClientRect().width > 0);
          if (!pop) return 'no-popper';
          const w = Math.round(pop.getBoundingClientRect().width);
          const text = (pop.textContent ?? '').replace(/\\s+/g, ' ');
          document.body.click();
          return '宽 ' + w + ' | ' + text;
        })()`,
      },
      {
        desc: "版本检查浮层 = 画板 42（420 宽，有更新时列出 release）",
        want: "宽 420|v0.48.0",
        js: `(async () => {
          const host = document.querySelector('.pt-status__version');
          if (!host) return 'no-version-host';
          const btn = host.querySelector('button') ?? host.firstElementChild ?? host;
          btn.click();
          await new Promise((r) => setTimeout(r, 700));
          const pops = [...document.querySelectorAll('.el-popper')]
            .filter((el) => getComputedStyle(el).display !== 'none');
          const pop = pops.find((el) => (el.textContent ?? '').includes('v0.4')) ?? pops[0];
          if (!pop) return 'no-popper';
          const w = Math.round(pop.getBoundingClientRect().width);
          const text = (pop.textContent ?? '').replace(/\\s+/g, ' ').slice(0, 160);
          document.body.click();
          return '宽 ' + w + ' | ' + text;
        })()`,
      },
      {
        desc: "状态栏 RSS 任务浮层 = 画板 41 的 sched-pop（300 宽：状态与原因、作用范围、自动启动开关、停止 / 启动）",
        /* 2026-10「调度器」改名「RSS 任务」：状态取 /api/control/status，「最后同步 · 间隔」那行换成了状态说明 */
        want: "宽 300|RSS 任务|刷流、辅种|程序启动时自动运行|停止|启动",
        js: `(async () => {
          const cell = document.querySelector('.pt-status__cell--btn');
          if (!cell) return 'no-cell';
          cell.click();
          await new Promise((r) => setTimeout(r, 500));
          const menu = document.querySelector('.pt-status__menu');
          if (!menu) return 'no-menu';
          const pop = menu.closest('.el-popper') ?? menu.parentElement;
          const w = Math.round(pop.getBoundingClientRect().width);
          const text = (menu.textContent ?? '').replace(/\\s+/g, ' ');
          document.body.click();
          return '宽 ' + w + ' | ' + text;
        })()`,
      },
    ],
  },
  "/chatops/notifications": {
    board: "22 消息通知",
    kind: CARD,
    bands: ["toolbar"], // 画板 22 在通道卡之前有一条 bar-64
    cards: [548, 516, 548, 516, 1080], // nt0..nt3 两栏 + p-policy/p-stat + p-recent
    minCards: 7, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["投递策略", "最近投递统计", "最近的通知"],
    /* 画板 22 的 bar-64：seg（全部/已连接/异常/已停用）+ chip「类型」+ 两枚视图钮 */
    needsSeg: true,
    /* 「已连接」按画板列进来，落地没有这一档 —— 让它红，原因在 ALLOWED_GAPS 里 */
    /* 画板 22 的四档分段：全部 / 已连接 / 异常 / 已停用 —— 现在四档都落地了 */
    controls: ["全部", "已连接", "异常", "已停用", "类型", "卡片视图", "紧凑列表"],
    /*
     * 分段的四档必须**真的按运行态筛**，而不只是画在那里。
     * 假数据四个通道各占一档（running / connected / disabled / error），
     * 所以「已连接」应当剩 1 张、「异常」应当剩 1 张，且胶囊上的字要对得上 ——
     * 上一轮补 runtime_state 时假数据里压根没有这个字段，那两档筛的是一批空状态。
     */
    probes: [
      {
        desc: "运行态分段真的在筛（已连接 / 异常各剩一张，且胶囊文字对得上）",
        want: "verdict=ok",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const cards = () => [...document.querySelectorAll('.ch-card')];
          const seg = document.querySelector('[data-testid=channels-status-seg]');
          if (!seg) return 'no-seg';
          const pick = (label) => {
            const el = [...seg.querySelectorAll('.el-segmented__item')]
              .find((b) => (b.textContent ?? '').trim() === label);
            if (el) el.click();
            return Boolean(el);
          };
          const all = cards().length;
          if (!pick('已连接')) return 'no-connected-seg';
          await wait(500);
          const conn = cards();
          const connPill = conn.map((c) =>
            (c.querySelector('.pt-pill')?.textContent ?? '').trim());
          if (!pick('异常')) return 'no-error-seg';
          await wait(500);
          const err = cards();
          const errPill = err.map((c) =>
            (c.querySelector('.pt-pill')?.textContent ?? '').trim());
          pick('全部');
          await wait(400);
          const back = cards().length;
          const ok = all === 4 && conn.length === 1 && err.length === 1 && back === all &&
            connPill.every((t) => t.includes('已连接')) && errPill.every((t) => t.includes('异常'));
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') +
            ' 全部 ' + all + ' / 已连接 ' + conn.length + '(' + connPill.join(',') + ')' +
            ' / 异常 ' + err.length + '(' + errPill.join(',') + ') / 回到 ' + back;
        })()`,
      },
    ],
  },
  "/chatops/notifications/1": {
    board: "23 + 37 通道详情",
    kind: CARD,
    detail: true, // head 88 带面包屑
    /* 画板 23 的 hero / c-basic / c-test / p-msg，外加画板 37 的 p-sec 700 与 p-map 364 */
    cards: [1080, 1080, 1080, 700, 700, 364],
    minCards: 6,
    titles: [
      "hero",
      "基本信息",
      "凭证与连接",
      "连通性测试",
      "操作提示文案",
      /* 画板 37 的两张说明卡 */
      "密钥与安全",
      "channel_type 映射",
    ],
    /*
     * 这一页的数据得真的落到表单里。假数据里通道详情曾与列表撞前缀，详情页拿到一个数组，
     * 画出来是「未命名通道 / ID -」—— 卡都在、标题都对，检查照样全绿。
     */
    /* 画板 23 的三枚钮都在卡里（c-basic / c-test 底部），这一页没有 bar-64 */
    pageControls: ["保存基本信息", "保存凭证", "发送测试消息"],
    probes: [
      {
        desc: "通道详情的数据落进了表单（不是空壳）",
        want: "主 Telegram",
        js: `(() => {
          const heroEl = document.querySelector('[data-card]');
          const hero = (heroEl?.textContent ?? '').replace(/\\s+/g, ' ').trim();
          const name = document.querySelector('.pt-cards input')?.value ?? '';
          return hero + ' | 名称输入框=' + name;
        })()`,
      },
    ],
  },
  "/chatops/bindings": {
    board: "24 ChatOps 绑定",
    kind: CARD,
    /*
     * 2026-10-29 加了「通道」一栏（机器人状态、通知发给谁、名单里谁绑定了）放在最前面；
     * 「待绑定」只在有没用掉的绑定码时出现（假数据里有一个码，所以这里有它）。偏离见 ALLOWED_GAPS。
     */
    cards: [612, 612, 612], // p-pending / p-active / p-note（落地 1080，偏离见 ALLOWED_GAPS）
    minCards: 3, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["通道", "待绑定", "已绑定", "绑定是怎么走的"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
    probes: [
      {
        /*
         * 卡宽偏离画板（612 → 1080）的理由就是这一条：两张表的列放得下、不再横滚。
         * 偏离登记之后栏宽检查不再管这一页，所以要有一条正面断言钉住落地值，
         * 否则哪天卡又窄回去、表又横滚，检查照样全绿。空表也有列宽，假数据没有绑定照样量得到。
         */
        desc: "绑定页两张表在 1080 的卡里不横滚",
        want: "verdict=ok",
        js: `(() => {
          const widths = [...document.querySelectorAll('.bindings-page > section.pt-panel')].map((c) => c.offsetWidth);
          const tables = [...document.querySelectorAll('.bindings-page .el-table')];
          if (tables.length < 2) return 'no-tables:' + tables.length;
          const scroll = tables.map((t) => {
            const w = t.querySelector('.el-table__body-wrapper .el-scrollbar__wrap');
            return w ? w.scrollWidth - w.clientWidth : -1;
          });
          const ok = scroll.every((d) => d >= 0 && d <= 1) && widths.length >= 3 && widths.every((w) => Math.abs(w - 1080) <= 1);
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 卡宽 ' + widths.join('/') + ' 表横滚 ' + scroll.join('/');
        })()`,
      },
    ],
  },
  "/chatops/audit": {
    board: "25 操作审计",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 1080, 1080], // p-cmd / p-ch / p-fail / p-keep
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["命令分布", "渠道分布", "失败与被拒", "保留与清理"],
    /* 画板 25 的 bar-64：seg（全部/成功/被拒绝/出错）+ q + chip 通道 + chip 时间 + 三枚图标钮 */
    needsSeg: true,
    controls: ["全部", "成功", "被拒绝", "出错", "筛选命令、触发用户", "通道", "时间", "导出"],
    /* 画板 25 的表头 */
    gridColumns: ["时间", "通道", "触发用户", "命令", "结果", "延迟"],
    /*
     * 结果分段必须真的**少掉行**，而不只是换个颜色。
     *
     * 这条探针的第一版是假绿，评审判得对：说明写「按前缀筛」，而当时的假数据不认 result
     * 参数（同一路径永远回同一坨 JSON），所以能断言的只剩胶囊颜色 —— 筛没筛压根测不到，
     * `denied.length >= 0` 还恒为真。现在假数据按服务端语义认 result / channel_type
     * （见 board-fixtures.mjs 的 QUERY_AWARE），行数因此真的会变，可以拿来断言。
     *
     * 库里存的是 denied:not_bound / error:lookup_binding 这种带原因后缀的串，
     * 只有 success 是裸值 —— 分段发的是裸 denied，按等值筛会得零条。
     */
    probes: [
      {
        /*
         * 时间 chip 真的在筛。前端曾经发 start_time / end_time，而后端（web/api_chatops.go）
         * 只读 since / until —— 「最近 1 小时」点得动、页头也写着已筛选，返回的却是全量。
         * 假数据的前 4 条落在最近 1 小时里（board-fixtures.mjs 的 AUDIT_NOW），并按 since / until 过滤；
         * 参数名一错，行数就不会变，这条会红。
         */
        desc: "时间 chip「最近 1 小时」真的按 since / until 筛（行数变少且都在一小时内，选回不限恢复）",
        want: "verdict=ok",
        js: `(async () => {
          const until = async (cond, ms) => {
            const t0 = Date.now();
            while (Date.now() - t0 < ms) { if (cond()) return true; await new Promise((r) => setTimeout(r, 100)); }
            return false;
          };
          const rows = () => [...document.querySelectorAll('.pt-band--grid tbody tr')];
          const chip = document.querySelector('[data-testid=audit-time-chip]');
          if (!chip) return 'no-time-chip';
          const choose = async (label) => {
            (chip.querySelector('.el-select__wrapper') ?? chip).click();
            const ok = await until(() => [...document.querySelectorAll('.el-select-dropdown__item')]
              .some((o) => (o.textContent ?? '').trim() === label && o.offsetParent !== null), 3000);
            if (!ok) return false;
            [...document.querySelectorAll('.el-select-dropdown__item')]
              .find((o) => (o.textContent ?? '').trim() === label && o.offsetParent !== null).click();
            return true;
          };
          const all = rows().length;
          if (all < 2) return 'no-sample:' + all;
          if (!(await choose('最近 1 小时'))) return 'no-1h-option';
          await until(() => rows().length !== all, 3000);
          const recent = rows();
          /* 每行的时间单元格格式是 YYYY/MM/DD HH:mm:ss（本地时区），按本地时间解析 */
          const stamps = recent.map((tr) => {
            const m = (tr.textContent ?? '').match(/(\\d{4})\\/(\\d{2})\\/(\\d{2}) (\\d{2}):(\\d{2}):(\\d{2})/);
            return m ? new Date(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +m[6]).getTime() : NaN;
          });
          const inHour = stamps.every((t) => Number.isFinite(t) && Date.now() - t <= 3600_000 + 60_000);
          if (!(await choose('时间: 不限'))) return 'no-any-option';
          await until(() => rows().length === all, 3000);
          const back = rows().length;
          const ok = recent.length > 0 && recent.length < all && inHour && back === all;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 全部 ' + all + ' / 最近 1 小时 ' + recent.length +
            (inHour ? '' : '（有超出一小时的行）') + ' / 回到 ' + back;
        })()`,
      },
      {
        desc: "结果分段真的按前缀筛（行数要变成该档的条数，而不只是胶囊换色）",
        want: "verdict=ok",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const seg = document.querySelector('[data-testid=audit-result-seg]');
          if (!seg) return 'no-seg';
          const pick = (label) => {
            const el = [...seg.querySelectorAll('.el-segmented__item')]
              .find((b) => (b.textContent ?? '').trim() === label);
            if (el) el.click();
            return Boolean(el);
          };
          const rows = () => [...document.querySelectorAll('.pt-band--grid tbody tr')]
            .map((tr) => (tr.textContent ?? ''));
          const all = rows();
          /*
           * 期望值从页面自己算：全部档里有几行的结果胶囊写着「被拒」/「出错」，筛完就该剩几行。
           * 原来按行文本里的 denied / error 字样算（注意：这段在模板字符串里，不能写反引号）——
           * 后来胶囊改成短标签（画板 25 的胶囊里是 Success / Denied 这种一词标签，
           * 原始值 denied:not_bound 在 96 宽的列里会被切断），那个判据就失效了，
           * 这条探针当场报了 no-sample。按显示的标签算才稳。
           */
          const wantDenied = all.filter((t) => t.includes('被拒')).length;
          const wantError = all.filter((t) => t.includes('出错')).length;
          if (wantDenied === 0 || wantError === 0) return 'no-sample:' + all.length;
          if (!pick('被拒绝')) return 'no-denied-seg';
          await wait(900);
          const denied = rows();
          if (!pick('出错')) return 'no-error-seg';
          await wait(900);
          const err = rows();
          if (!pick('全部')) return 'no-all-seg';
          await wait(900);
          const back = rows();
          /* 颜色也一起验：denied:* 要 warn、error:* 要 dang（按整串查会全落灰色） */
          const pills = [...document.querySelectorAll('.pt-band--grid tbody .pt-pill')];
          const dPill = pills.find((p) => (p.textContent ?? '').includes('被拒'));
          const ePill = pills.find((p) => (p.textContent ?? '').includes('出错'));
          const colorOk = Boolean(dPill && dPill.className.includes('warn')) &&
            Boolean(ePill && ePill.className.includes('dang'));
          const ok = denied.length === wantDenied && denied.every((t) => t.includes('被拒')) &&
            err.length === wantError && err.every((t) => t.includes('出错')) &&
            back.length === all.length && denied.length < all.length && colorOk;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') +
            ' 全部 ' + all.length + ' / 被拒绝 ' + denied.length + '(期望 ' + wantDenied + ')' +
            ' / 出错 ' + err.length + '(期望 ' + wantError + ') / 回到 ' + back.length +
            ' 颜色 ' + (colorOk ? 'ok' : 'bad');
        })()`,
      },
    ],
  },
  "/chatops/rss-notifications": {
    board: "26 RSS 通知日志",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 548, 516, 1080], // p-res / p-idem / p-site / p-quiet / p-retry
    minCards: 5, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["推送结果分布", "幂等与去重", "待重试与失败", "按站点分布", "安静时段"],
    /* 画板 26 的 bar-64：seg（结果五档）+ q「筛选站点、种子 ID…」+ chip 通道 + 三枚图标钮 */
    needsSeg: true,
    controls: ["筛选站点、种子 ID", "已发送", "失败", "被限流", "全部通道", "更多筛选", "导出"],
    /* 画板 26 的表头 */
    gridColumns: ["时间", "站点", "种子 ID", "类型", "通道", "结果", "尝试", "操作"],
  },
  "/global": {
    board: "27 系统设置",
    kind: CARD,
    /*
     * p-cfg（画板另有一条 warn 提示，那是状态不是常驻卡）+ 备份与恢复。
     * 「备份与恢复」是 1.0 媒体中心 M0 加的卡，画板 27 没画；与 p-cfg 同宽，排在它下面。
     */
    cards: [1080, 1080],
    minCards: 2, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["全局配置", "备份与恢复"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/cloak-config": {
    board: "28 CloakBrowser",
    kind: CARD,
    cards: [1080, 700, 364, 1080], // intro / p-cfg / p-res / p-life
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["intro", "Manager 连接", "探测结果", "页面状态"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/logs": {
    board: "29 运行日志",
    kind: CARD,
    bands: ["toolbar"],
    cards: [300, 300, 300, 764], // 左列 p-files/p-lv/p-arc / 右 p-tail
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["日志文件", "级别筛选", "轮转归档", "运行日志"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
    /* 画板 29 的 bar-64：seg（全部/INFO/WARN/ERROR/DEBUG）+ q + 跟随 + 刷新 */
    needsSeg: true,
    controls: [
      "搜索日志内容",
      "跟随尾部",
      "自动刷新",
      "INFO",
      "WARN",
      "ERROR",
      "DEBUG",
      "最近 1 小时",
    ],
    /*
     * 正文搜索必须**真的画出命中的行**，而且是在滚到底的状态下搜。
     *
     * 说清这条探针证明了什么：一次评审判「滚到底再搜会渲染成空白」——
     * 代码层面 startIndex 确实没钳位，但在真实浏览器里内容变矮时 scrollTop 会被
     * 浏览器自己钳住并因此**真的派发一次 scroll 事件**，onLogScroll 随即把起点纠回来。
     * 我按带缺陷的构建实测过（1200 行、scrollTop=24965、命中 1 行）：照样画得出那一行。
     * 所以这条探针守的是「搜完能看见结果」这个行为，不是那个没能复现的空白态。
     */
    probes: [
      {
        desc: "先滚到底再搜索：命中的行要真的画出来（虚拟滚动起点不能停在旧位置）",
        want: "verdict=ok",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const target = document.querySelector('.log-container');
          if (!target) return 'no-scroller';
          target.scrollTop = target.scrollHeight;
          target.dispatchEvent(new Event('scroll'));
          await wait(400);
          const input = document.querySelector('[data-testid=logs-search] input')
            ?? document.querySelector('input[data-testid=logs-search]')
            ?? document.querySelector('.log-q input');
          if (!input) return 'no-search';
          /*
           * 关键词必须**只命中很少几行**。第一版取的是行里第一个英文单词，
           * JSON 行里那就是 "level" —— 每一行都有，命中集合不缩小，缺陷根本没被触发，
           * 于是这条探针在带缺陷的构建上照样绿。这里取一行末尾那一小段（含行号），
           * 并在命中数过多时直接判失败，不让它再退化成一条空探针。
           */
          const sample = document.querySelector('.log-line');
          const text = (sample?.textContent ?? '').trim();
          if (text.length < 12) return 'no-sample';
          const word = text.slice(-10);
          input.value = word;
          input.dispatchEvent(new Event('input', { bubbles: true }));
          await wait(700);
          const shown = document.querySelectorAll('.log-line').length;
          const pre = document.querySelector('.log-content');
          const hits = pre ? Math.round(parseFloat(pre.style.height || '0') / 24) : -1;
          /* hits 少、shown 大于 0：命中集合确实缩小了，而且缩小之后照样画得出来 */
          const ok = hits > 0 && hits <= 5 && shown > 0;
          /* 清空搜索框：探针共用同一个页面状态，留着它下一条探针量的就是「搜索 ∩ 时间窗」 */
          input.value = '';
          input.dispatchEvent(new Event('input', { bubbles: true }));
          await wait(400);
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') +
            ' 搜「' + word + '」命中 ' + hits + ' 行、画出 ' + shown + ' 行';
        })()`,
      },
      {
        /*
         * 「最近 1 小时」要真的少掉几行。假数据一半在一小时内、一半在三小时前，
         * 所以总行数必须严格变小 —— 只断言「点得动」测不出它有没有在筛。
         */
        desc: "「最近 1 小时」真的按行里的时间戳筛（总行数要变小，且 chip 不是停用态）",
        want: "verdict=ok",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const total = () => {
            const pre = document.querySelector('.log-content');
            return pre ? Math.round(parseFloat(pre.style.height || '0')) : -1;
          };
          const cb = document.querySelector('[data-testid=logs-last-hour]');
          if (!cb) return 'no-chip';
          if (cb.classList.contains('is-disabled')) return 'chip-disabled（这批日志解析不出时间戳）';
          const before = total();
          cb.querySelector('input')?.click();
          await wait(700);
          const after = total();
          cb.querySelector('input')?.click();
          await wait(500);
          const restored = total();
          const ok = before > 0 && after > 0 && after < before && restored === before;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') +
            ' 内容高 ' + before + '→' + after + '→' + restored;
        })()`,
      },
      {
        /*
         * 左栏多选两个级别后，工具栏的级别分段不能再亮着「全部」，点「全部」要真的清空。
         * el-segmented 的切换挂在原生 radio 的 change 上，点一个已经选中的 radio 不触发 change ——
         * 多选时若仍回 ""，「全部」亮着、列表却按两个级别筛着，点它也毫无反应。
         */
        desc: "左栏多选两个级别后分段不亮「全部」，点「全部」清空筛选",
        want: "verdict=ok",
        js: `(async () => {
          const until = async (fn, ms) => {
            const t0 = performance.now();
            while (performance.now() - t0 < ms) { if (fn()) return true; await new Promise((r) => setTimeout(r, 50)); }
            return fn();
          };
          const total = () => {
            const pre = document.querySelector('.log-content');
            return pre ? Math.round(parseFloat(pre.style.height || '0')) : -1;
          };
          const btns = [...document.querySelectorAll('.lv__btn')];
          const count = (b) => Number((b.querySelector('.lv__n')?.textContent ?? '0').trim()) || 0;
          const all = btns.reduce((s, b) => s + count(b), 0);
          /* 挑两档有行、合起来又不是全部的级别，这样「筛了」与「没筛」的行数一定不同 */
          const picks = btns.filter((b) => count(b) > 0).sort((a, b) => count(a) - count(b)).slice(0, 2);
          if (picks.length < 2 || count(picks[0]) + count(picks[1]) >= all) return 'no-sample';
          const before = total();
          for (const b of picks) b.click();
          await until(() => total() < before, 3000);
          const mixed = total();
          const seg = document.querySelector('[data-testid=logs-level-seg]');
          const lit = seg?.querySelector('.el-segmented__item.is-selected')?.textContent?.trim() ?? '';
          const allItem = [...(seg?.querySelectorAll('.el-segmented__item') ?? [])]
            .find((i) => (i.textContent ?? '').trim() === '全部');
          if (!allItem) return 'no-all-item';
          allItem.click();
          await until(() => total() === before, 3000);
          const back = total();
          const pressed = document.querySelectorAll('.lv__btn[aria-pressed="true"]').length;
          const ok = mixed < before && lit === '' && back === before && pressed === 0;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 内容高 ' + before + '→' + mixed + '→' + back +
            (lit ? '（多选时分段亮着「' + lit + '」）' : '') + (pressed ? '（点「全部」后仍有 ' + pressed + ' 档勾着）' : '');
        })()`,
      },
    ],
  },
  "/password": {
    board: "44 修改密码",
    kind: CARD,
    cards: [520, 344], // p-acct 居中 + p-rules/p-msg/p-note 三栏
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["账号信息", "口令规则", "改完会发生什么", "忘记密码怎么办"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/sites/M-Team": {
    board: "13 站点详情",
    kind: CARD,
    detail: true,
    tabs: true, // tabs 带 40 高
    tabLabels: ["概览", "RSS 订阅", "任务", "推送记录", "过滤规则", "凭据"], // 画板 tabs 的六项
    cards: [700, 700, 700, 364, 364, 364, 364], // 左列 p-rss/p-push/p-tasks · 右列 p-cred/p-keep/p-stat/p-danger
    minCards: 7, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["RSS 订阅", "最近推送", "任务", "站点凭据", "保号规则", "站点统计", "危险操作"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
    /*
     * 画板 13 的激活分区是「RSS 订阅」，左栏那三张卡就是这个分区下的内容。
     * 页面默认停在「概览」，所以先点一下分区带 —— 不点就只能量到右栏。
     */
    prepare: `(async () => {
      const tab = [...document.querySelectorAll('.pt-band__tab')]
        .find((b) => b.textContent.includes('RSS'));
      if (!tab) return 'no-tab';
      tab.click();
      await new Promise((r) => setTimeout(r, 400));
      return 'switched';
    })()`,
  },
};

/**
 * 明确记下来的偏离 —— 键是 `<路由> <检查项>`，值是原因。
 * 加一条就等于向 owner 承认「这里没按画板做」，所以原因必须写清是**为什么做不了**，
 * 不是「暂时不想做」。想让某处通过检查，先实现它，不是往这张表里加一行。
 */
const RAIL_GAP_REASON =
  "画板 01/D.rail 画了八个快捷入口 + 运行日志入口，落地在导航列钉住时把它们全藏起来" +
  "（现在是整条 rail 淡出：侧栏是一块，钉住时只剩 264 宽的 nav —— 用户原话" +
  "「展开时也应该是一体的而不是一个黑色侧栏 + 一个白色侧栏导航」）。" +
  "这是用户在真实浏览器验收里退回过的：原话「有些重复了吧」—— 导航列已经列出同样的十几个入口，" +
  "rail 再摆一遍就是同一屏里两份导航。用户当时定的三条硬约束（主题入口必须可见、" +
  "rail 快捷入口不能与导航列同时出现、导航列必须可收起）优先于画板，" +
  "所以这条不按画板改回去。导航列收起时这些入口照旧出现。";

/*
 * **有意裁剪**的白名单：按类名显式列出，每条写明为什么它被裁不算丢内容。
 *
 * 为什么用白名单而不是「聪明的判据」：上一版我为了消掉这两个误报，把条件收成
 * 「必须有一个子元素比容器还宽，或者自己是纯文本」—— 一次评审当场指出这**削弱了断言**：
 * 一行 nowrap 的 flex 里两个各 60 宽的子项塞进 100 宽的容器（100<120），
 * 两个子项都不单独超宽，于是整类「多个兄弟合计超宽」和「文本与元素混排」的溢出都漏掉了，
 * 而这些在收紧之前是会红的。判得对：为了两个误报放走一整类真问题不是等价交换。
 *
 * 所以判据回到宽的那一版，误报按名字一个个排除 —— 名单短、每条有理由、新出现的裁切
 * 不在名单里就会红。
 */
const CLIP_ALLOW_ENTRIES = [
  [
    "el-table__header-wrapper",
    "Element 把表头与表体拆成两个 wrapper，表头那个恒被裁 —— 它的滚动由表体的滚动条同步驱动，" +
      "而那个滚动条是它的兄弟不是祖先，祖先检查看不到。内容没丢。",
  ],
  [
    "poster",
    "导出分享图的装饰光晕是 `.poster::before`：absolute、200% 宽的径向渐变，" +
      "靠 overflow:hidden 裁掉正是它的实现方式。伪元素枚举不到，所以只能按名字排除。",
  ],
];
const CLIP_ALLOW_NAMES = CLIP_ALLOW_ENTRIES.map(([n]) => n);

/*
 * 「被裁掉的横向溢出」检测器 —— 注入进两份 MEASURE，也用于下面那个自检。
 *
 * 抽成一个函数的原因：这条断言被我弱化过一次（为了消两个误报，把条件收成「必须有子元素
 * 超宽」，于是「多个兄弟合计超宽」整类漏掉）。检测器一旦是个独立函数，就能拿合成 DOM
 * 反向验它 —— 见 CLIP_SELFTEST。
 *
 * 判为裁切：scrollWidth 明显超过 clientWidth，且
 *   · 自己与所有祖先都不能横向滚（能滚就是有意的可滚区域）；
 *   · 没有省略号、也没有多行夹断（那两种是有意的截断，画板自己也这么截长标题）；
 *   · 类名不在有意裁剪白名单里。
 */
const CLIP_FN = `
  const CLIP_ALLOW = new Set(${JSON.stringify(CLIP_ALLOW_NAMES)});
  const clipIn = (roots) => {
    const out = [];
    for (const root of roots) {
      for (const el of root.querySelectorAll('*')) {
        if (el.scrollWidth - el.clientWidth <= 2 || el.clientWidth <= 0) continue;
        let scrollable = false;
        for (let a = el; a && a !== document.body; a = a.parentElement) {
          const ox = getComputedStyle(a).overflowX;
          if (ox === 'auto' || ox === 'scroll') { scrollable = true; break; }
        }
        if (scrollable) continue;
        const cs = getComputedStyle(el);
        if (cs.textOverflow === 'ellipsis') continue;
        if (cs.webkitLineClamp && cs.webkitLineClamp !== 'none') continue;
        if ([...el.classList].some((c) => CLIP_ALLOW.has(c))) continue;
        out.push((el.className || el.tagName).toString().slice(0, 40) +
          ' ' + el.clientWidth + '<' + el.scrollWidth);
      }
    }
    return [...new Set(out)];
  };
`;

/*
 * 检测器的**反向自检**：往页面里塞几个合成的溢出样本，看 clipIn 是不是都抓得到，
 * 再塞两个「有意截断」的样本，看它是不是都放过。跑完立刻把沙盒删掉。
 *
 * 为什么需要它：这条断言被我弱化过一次 —— 为了消掉两个误报，我把条件收成
 * 「必须有一个子元素比容器还宽」，于是「多个兄弟合计超宽」「文本与元素混排」整类漏掉，
 * 而 board-check 照旧退 0。一次评审当场判这是 assertion weakening。
 * 判据现在回到宽的那一版 + 具名白名单，而这个自检就是拦住下一次弱化的那道闸：
 * 谁再把判据收窄，这里会立刻红。
 */
const CLIP_SELFTEST = `${CLIP_FN}
(() => {
  const box = document.createElement('div');
  box.id = 'pt-clip-selftest';
  box.style.cssText = 'position:fixed;left:-9999px;top:0;width:400px';
  box.innerHTML = [
    /* ① 单个子元素超宽（表格塞进窄 wrapper 就是这种） */
    '<div class="t-one pt-panel" style="width:100px;overflow:hidden">' +
      '<div style="width:300px;height:8px"></div></div>',
    /* ② 多个兄弟合计超宽 —— 每个都不单独超宽，正是被弱化的那一版漏掉的形状 */
    '<div class="t-sum pt-panel" style="width:100px;overflow:hidden;display:flex;flex-wrap:nowrap">' +
      '<div style="flex:0 0 60px;height:8px"></div><div style="flex:0 0 60px;height:8px"></div></div>',
    /* ③ 文本与元素混排：一个子元素 + 一段长文本，合起来装不下 */
    '<div class="t-mix pt-panel" style="width:100px;overflow:hidden;white-space:nowrap">' +
      '<span style="display:inline-block;width:60px"></span>' +
      'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789</div>',
    /* ④ 纯文本装不下 */
    '<div class="t-text pt-panel" style="width:100px;overflow:hidden;white-space:nowrap">' +
      'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789</div>',
    /* ⑤ 带省略号的截断 —— 有意的，必须放过 */
    '<div class="t-ell pt-panel" style="width:100px;overflow:hidden;white-space:nowrap;' +
      'text-overflow:ellipsis">ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789</div>',
    /* ⑥ 自己能横向滚 —— 有意的，必须放过 */
    '<div class="t-scroll pt-panel" style="width:100px;overflow-x:auto;white-space:nowrap">' +
      'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789</div>',
  ].join('');
  document.body.appendChild(box);
  const hits = clipIn(box.querySelectorAll('.pt-panel')).join(' | ');
  /* clipIn 从 root 的**后代**里找，所以把每个样本自己也当一次 root */
  const own = clipIn([box]).join(' | ');
  const all = hits + ' | ' + own;
  box.remove();
  const caught = (k) => all.includes(k);
  const want = ['t-one', 't-sum', 't-mix', 't-text'];
  const reject = ['t-ell', 't-scroll'];
  const missed = want.filter((k) => !caught(k));
  const wrong = reject.filter((k) => caught(k));
  return { ok: missed.length === 0 && wrong.length === 0, missed, wrong, all: all.slice(0, 200) };
})()`;

/*
 * 「画板画了、落地没有」这一类偏离的前缀。
 *
 * 分出这个标记的原因：偏离表里其实混着三类东西 —— 技术上做不了的、owner 拍板保留的、
 * 以及画板画了而确实没做的。三类混在一起打印，最后一类就被前两类稀释成了背景噪音，
 * 而收尾那句仍然是「与画板一致」。一次评审正是判「设计缺项仍被验收放行」，判得对：
 * 少了这个标记，脚本退 0 就等于替我说了一句没根据的话。
 */
const UNIMPL = "画板画了、落地没有：";

/*
 * 「画板画了、落地没有，而且 owner 已经逐项看过并接受」。
 *
 * 与 UNIMPL 的区别只在一件事：范围已经定了。所以这一类不再让脚本退 2 ——
 * 退 2 的意思是「等 owner 拍板」，拍完了就不该继续拦着。但它们照样每次都打印出来，
 * 接受不等于消失。
 *
 * 逐项记在这里，而不是给一个「全盘接受」的命令行开关：以后再冒出新的未落地项时，
 * 它不在这张表里，验收会照旧退 2 —— 开关做不到这一点，它会把新项一起蒙过去。
 */
const OWNER_OK = "画板画了、落地没有（owner 已逐项接受）：";

/** 逐项接受的出处，附在每条原因末尾 —— 免得以后有人把它当成还没人看过的待办 */
const OWNER_DECISION =
  " owner 2026-09-20 的决定：这三项都按偏离接受、不做（原话「都按偏离接受，不做」，问的是「还剩三项画板画了但没落地的设计项，每一项都需要新增后端能力，要我实现哪些」）。";

const ALLOWED_GAPS = {
  /* 外壳级偏离：每条桌面路由都会量到，逐条登记（键必须逐条写，键里不能有通配） */
  ...Object.fromEntries(
    [
      "/userinfo",
      "/userinfo/export",
      "/sites",
      "/supported-sites",
      "/search",
      "/tasks",
      "/paused",
      "/downloader-hub",
      "/downloaders",
      "/filter-rules",
      "/cleanup",
      "/chatops/notifications",
      "/chatops/notifications/1",
      "/chatops/bindings",
      "/chatops/audit",
      "/chatops/rss-notifications",
      "/global",
      "/cloak-config",
      "/logs",
      "/password",
      "/sites/M-Team",
    ].map((r) => [`${r} rail.items`, RAIL_GAP_REASON]),
  ),

  /*
   * 用户统计表：owner 已拍板 —— **用画板的密集网格，但保留头像、未读角标，以及
   * 「真实数据 / 入站 / 剩余天数 / 操作」四个改版前就有的功能列**。
   * 所以行高已经压回画板的 34（不再登记偏离），只剩「列比画板多四列」这一条，
   * 而它现在是 owner 的决定，不是待决事项。
   */
  /*
   * /sites 的列集合：画板 12 是 `# / 站点 / 状态 / 认证方式 / RSS 订阅 / 探测模式 / 操作`，
   * 落地多出「判定活跃 / 剩余天数 / 站点活跃 / 启用」四列 —— 与 /userinfo 那四列同一性质
   * （保号相关的功能列），按 owner 定下的原则（用画板的样式，保留现有功能）保留。
   * 这四列现在可以在「列设置」里自己关掉（画板 bar-64 右端的 bi-columns-3 就是它）。
   */
  /*
   * 画板 16 的 seg 是五档下载器状态（全部/下载中/做种中/等待中/已暂停），而这一页的记录是
   * RSS 流水：已下载 / 已推送 / 已过期，在 apiTasks 里是逐个 AND 的可叠加标记。
   * 两者不是同一套词汇 —— 那五档属于下载器控制台（画板 18），那一页确实有它们。
   */
  "/tasks controls.seg":
    OWNER_OK +
    "画板 16 的分段是五档下载器状态（全部/下载中/做种中/等待中/已暂停）；这一页是 RSS 流水，" +
    "只有已下载 / 已推送 / 已过期三个标记，而且它们是可叠加的（apiTasks 里逐个 AND）——" +
    "「已下载 + 已推送」= 推成功了的，收成互斥分段会把这个组合删掉（注意：这里的组合在 " +
    "apiTasks 里逐个 AND、是真的在工作的，与审计页那个假组合不同）。下载器状态在画板 18 那一页；" +
    "要按画板做就得把下载器实时状态按 hash 关联进列表页，等于每次翻页多一次下载器往返。" +
    OWNER_DECISION,

  /*
   * 画板 24 的三张卡都是 612 宽（右边那片画的是弹窗规格）。落地改成 1080：「已绑定」那张表有
   * 渠道 / 渠道用户 ID / 备注 / 回复语言 / 管理员 / 最后活跃 / 操作七列，列宽合计 946，
   * 「待绑定」五列合计 840 —— 在 612 的卡里只能横滚，右端几列看不到，列头「回复语言」
   * 还被固定的操作列盖住一半。收窄到 612 放得下就得删列。落地值由本页的探针钉住。
   */
  ...Object.fromEntries(
    ["cards.612", "cards.612#2", "cards.612#3"].map((k) => [
      `/chatops/bindings ${k}`,
      "画板 24 的卡 612 宽；两张表列宽合计 840 / 946，612 里只能横滚、右端的列看不到，" +
        "落地用 1080（宽屏也停在 1080）。2026-09-30 用户要求逐页审计美化时改的。",
    ]),
  ),

  "/filter-rules grid.columns":
    "画板 20 九列；落地多一列「启用」——规则的开关本来就在表里改（画板把开关画进了操作列的省略菜单），" +
    "把它收起来会让「这条规则现在生效吗」看不出来。按 owner 的原则保留功能列。",

  "/sites grid.columns":
    "画板 12 七列；落地十一列 —— 多出「判定活跃 / 剩余天数 / 站点活跃 / 启用」。" +
    "按 owner 的原则保留（用画板的样式设计，但保留现有功能），并且已经做成可在列设置里关掉。",

  "/userinfo grid.columns":
    "画板十列；落地默认十一列 —— 多出「操作」。「真实数据 / 入站 / 剩余天数」三列 " +
    "**owner 当初决定保留**（原话：用画板的样式设计，但要保留现有的头像、未读角标、" +
    "真实数据、入站、剩余天数、操作）；2026-09-30 用户又提「表格列数量过多」，" +
    "所以三列仍在，但没存过列偏好时默认收进「列设置」，勾上即回来。这是记录在案的取舍。" +
    "站点列也按这个决定保留了头像与未读角标；行高与单元格形状则按画板压回 34。" +
    "画板的「数据量」列落地叫「上传 / 下载（计费）」，「真实数据」改叫「实际传输」：" +
    "账号的下载量要写明是站点计费的口径、和实际传输分开（docs/design/free-expiry-safety.md 附录 B.2.17）。",

  /*
   * 移动画板 30 / 31 的筛选区与行卡走势图。两条都不是「忘了做」：
   */
  "/sites@375 filterRow":
    "画板 30 的筛选是一排 26 高的 chip（全部/正常/异常/已禁用 各带计数）。" +
    "落地现在是「已启用/全部」分段 + 三枚状态 chip（正常/异常/已禁用，带计数）+ 搜索 + " +
    "「批量」菜单 + 新增站点，两行共 81 —— 比画板高，但已经从 240 压下来，且入口一个没少。" +
    "还高的那部分是画板移动稿上根本没画的三个入口（探测启用站点 / 打开启用站点 / 新增站点）；" +
    "「全部」两边也不是一回事：画板那张图是 14 个已配置站点，这个产品的 /api/sites 回的是" +
    "全部 66 个内置定义，所以状态做成可叠加的 chip 而不是画板那种四档互斥分段。",
  "/tasks@375 filterRow":
    "画板 31 的筛选是一条 294×30 的分段器；落地是三个可叠加的筛选 chip + 搜索框 + 站点下拉。" +
    "那三个 chip 是可组合的（apiTasks 里逐个 AND），收成画板那条互斥分段器会删掉组合筛选能力 —— " +
    "这正是第九轮修掉的缺陷，不能为了对齐画板再改回去。",

  /*
   * 这张表现在是空的 —— 上一轮登记的偏离都实现掉了。
   * 再往里加一条就等于向 owner 承认「这里没按画板做」，所以原因必须写清是**为什么做不了**
   * （需要后端新接口、会造成重复写入口一类），不是「暂时不想做」。
   */
};

// ---------------------------------------------------------------- CDP 夹具

/*
 * 主区左边界与宽度。画板里所有带都是 x=328 w=1112（rail 64 + nav 264 两列并排）。
 * 落地按用户的决定改成**一块**侧栏：钉住时只有 264 宽的 nav（rail 淡出、不再占位；面跟主题走，明亮下是浅色），
 * 用户原话「展开时也应该是一体的而不是一个黑色侧栏 + 一个白色侧栏导航」。
 * 所以主区从 x=264 起、宽 1176；这不是页面各自漂了 64，而是外壳的一条偏离，
 * 在 shell.* 那组断言里单独量侧栏，这里只按新几何算主区。
 */
const SIDE_DOCKED_W = 264;
const MAIN_X = SIDE_DOCKED_W;
const MAIN_W = 1112;
/*
 * 桌面视口 = 侧栏 264 + 主区 1112 = 1376，而不是画板画布的 1440。
 * 页面里的卡是流式的：主区多出 64，两张并排的卡各分一截、三张各分一截，
 * 「516 / 548 / 1080」这些画板数就全对不上了。画板对页面内容的规格是按 1112 宽的主区画的，
 * 所以把主区量回 1112 —— 侧栏窄了是外壳的偏离，不该让每一页的卡跟着重新登记一遍。
 * 登录页没有侧栏，仍按画板 43 的 1440 量。
 */
const D_VIEWPORT = { width: SIDE_DOCKED_W + MAIN_W, height: 1024 };

const BASE = process.argv[2] ?? "http://127.0.0.1:8080";
const argOf = (name, fallback) => {
  const i = process.argv.indexOf(name);
  return i > 0 && process.argv[i + 1] ? process.argv[i + 1] : fallback;
};
const PORT = Number(argOf("--port", "19400"));
const ONLY = argOf("--route", "");
const CHROME =
  process.env.PT_CHROME ?? "/config/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome";
const USER = process.env.PT_USER ?? "admin";
const PASS = process.env.PT_PASS ?? "adminadmin";

if (!existsSync(CHROME)) {
  console.error(`找不到 Chrome：${CHROME}\n用 PT_CHROME 指定路径。`);
  process.exit(2);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/*
 * 起 Chrome 之前先看调试端口有没有人占着。
 *
 * 这一步值 40 分钟：上一次有个三小时前的孤儿 Chrome（父进程已经是 init）还占着 19400，
 * 新起的那个绑不上端口就自己退了，而脚本的 CDP 连上了**那个旧 Chrome** ——
 * 它是个能正常开页面的浏览器，所以「第一页打不开就退出」那道闸拦不住它，
 * 于是 37 条路由对着一个 cookie 与假数据全不对的浏览器空转了四十分钟，屏幕上一个字都没有。
 *
 * 端口被占时立刻退 2 并把占用者报出来：清掉它比猜为什么慢便宜得多。
 */
{
  const probe = await fetch(`http://127.0.0.1:${PORT}/json/version`, {
    signal: AbortSignal.timeout(1500),
  }).catch(() => null);
  if (probe?.ok) {
    console.error(
      `✗ 调试端口 ${PORT} 已被占用 —— 很可能是上一次留下的 Chrome（孤儿进程）。\n` +
        `  它是个能开页面的浏览器，所以不会在「第一页打不开」那道闸上被拦住，\n` +
        `  但它的 cookie 与假数据都不是这一轮的，跑下去只会空转。\n` +
        `  查占用者：ss -ltnp | grep ${PORT}；按 PID 清掉（别用 pkill -f，那会连自己的 shell 一起杀）。\n` +
        `  或者换个端口：--port <其他端口>。`,
    );
    process.exit(2);
  }
}

const profile = mkdtempSync(join(tmpdir(), "ptboard-"));
const chrome = spawn(
  CHROME,
  [
    "--headless=new",
    "--no-sandbox",
    "--disable-dev-shm-usage",
    "--disable-gpu",
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=${profile}`,
    "--window-size=1440,1024",
    "about:blank",
  ],
  { stdio: ["ignore", "ignore", "pipe"] },
);
chrome.stderr.on("data", () => {});

async function wsUrl() {
  for (let i = 0; i < 60; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${PORT}/json/version`);
      const j = await r.json();
      if (j.webSocketDebuggerUrl) return j.webSocketDebuggerUrl;
    } catch {
      /* Chrome 还没起来 */
    }
    await sleep(300);
  }
  throw new Error("Chrome 未就绪");
}

class CDP {
  constructor(ws) {
    this.ws = ws;
    this.id = 0;
    this.pending = new Map();
    this.onEvent = () => {};
    ws.addEventListener("message", (ev) => {
      const m = JSON.parse(ev.data);
      const slot = m.id && this.pending.get(m.id);
      if (!slot) {
        if (m.method) this.onEvent(m);
        return;
      }
      this.pending.delete(m.id);
      if (m.error) slot.reject(new Error(JSON.stringify(m.error)));
      else slot.resolve(m.result);
    });
  }
  send(method, params = {}, sessionId) {
    const id = ++this.id;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ id, method, params, sessionId }));
    });
  }
}

const ws = new WebSocket(await wsUrl());
await new Promise((r) => ws.addEventListener("open", r, { once: true }));
const cdp = new CDP(ws);
const { targetInfos } = await cdp.send("Target.getTargets");
const target = targetInfos.find((t) => t.type === "page");
const { sessionId } = await cdp.send("Target.attachToTarget", {
  targetId: target.targetId,
  flatten: true,
});
await cdp.send("Page.enable", {}, sessionId);
await cdp.send("Runtime.enable", {}, sessionId);
/** 假数据脚本装上了没有 —— 装上之后每条路由都要求当前文档真的铺到了（见 goto） */
let stubInstalled = false;

/* 控制台报错也算不一致：渲染崩了页面照样可能量不出东西，得说清是崩了还是没做 */
let consoleErrors = [];
cdp.onEvent = (m) => {
  if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") {
    const top = m.params.stackTrace?.callFrames?.[0];
    const at = top
      ? ` @ ${top.functionName || "(anonymous)"} ${top.url.split("/").pop()}:${top.lineNumber}`
      : "";
    consoleErrors.push(
      ((m.params.args ?? []).map((a) => a.value ?? a.description ?? "").join(" ") + at).slice(
        0,
        260,
      ),
    );
  }
  if (m.method === "Runtime.exceptionThrown") {
    const d = m.params.exceptionDetails;
    /*
     * 带上最上面那一帧。只报「TypeError: Cannot read properties of undefined」
     * 等于知道崩了但不知道在哪 —— 打包后的文件名+行列虽然不是源码位置，
     * 至少能定位到是哪个 chunk（各页面是分开打包的）。
     */
    const top = d?.stackTrace?.callFrames?.[0];
    const at = top
      ? ` @ ${top.functionName || "(anonymous)"} ${top.url.split("/").pop()}:${top.lineNumber}`
      : "";
    consoleErrors.push(((d?.exception?.description ?? "未捕获异常") + at).slice(0, 260));
  }
};

async function ev(expression) {
  const r = await cdp.send(
    "Runtime.evaluate",
    { expression, awaitPromise: true, returnByValue: true },
    sessionId,
  );
  if (r.exceptionDetails) throw new Error(JSON.stringify(r.exceptionDetails).slice(0, 300));
  return r.result.value;
}

async function waitReady() {
  for (let i = 0; i < 80; i++) {
    await sleep(250);
    const ready = await ev(
      `document.readyState === 'complete' && !!document.querySelector('#app > *, form')`,
    ).catch(() => false);
    if (ready === true) return;
  }
}

/**
 * 跳到一条路由。
 *
 * 这里有一个会把整轮验收变成假绿的坑：路由是 hash 模式，`Page.navigate` 到
 * `…/#/sites` 时如果当前文档已经是同一个路径，Chrome 只当成片段跳转，**不产生新文档**，
 * 于是 `Page.addScriptToEvaluateOnNewDocument` 注册的假数据脚本一次都不执行 ——
 * 页面拿到的是真后端的空库，量出来的「没有页脚带 / 卡片区是空的」看着像没实现。
 * 所以导航后要确认这一篇文档确实铺上了假数据（stubScript 会挂 window.__ptStub），
 * 没有就强制 reload 一次，reload 一定是新文档。
 */
async function goto(url) {
  await cdp.send("Page.navigate", { url }, sessionId);
  await waitReady();
  if (!stubInstalled) return;
  const stubbed = await ev(`window.__ptStub === true`).catch(() => false);
  if (stubbed === true) return;
  await cdp.send("Page.reload", {}, sessionId);
  await waitReady();
}

await goto(`${BASE}/`);
/*
 * 第一页都没打开就别往下跑了。
 *
 * 踩过一次：上一轮留下的 Chrome 还占着调试端口，新起的这个连不上，于是每条路由都在
 * goto 里空转 80×250ms —— 四十趟加起来十几分钟，屏幕上一个字都没有，看着像卡死。
 * 这里先确认首页真的加载出来了，否则立刻带着原因退出。
 */
{
  const ready = await ev(
    `document.readyState === 'complete' && !!document.querySelector('#app > *, form')`,
  ).catch(() => false);
  if (ready !== true) {
    console.log(
      `✗ 打不开 ${BASE}/ —— 服务没起来，或调试端口 ${PORT} 被上一次的 Chrome 占着。` +
        `先确认服务在跑，再确认没有残留的 chrome 进程。`,
    );
    chrome.kill("SIGKILL");
    process.exit(2);
  }
}
await ev(
  `fetch('/login', { method: 'POST', body: new URLSearchParams({ username: ${JSON.stringify(USER)}, password: ${JSON.stringify(PASS)} }) })`,
);
/*
 * 喂假数据 —— 画板的页脚带、多选条和一部分卡片只有有数据时才渲染，
 * 空库量出来的「没有页脚带」是没数据而不是没实现，不喂就分不开这两件事。
 */
await cdp.send("Page.addScriptToEvaluateOnNewDocument", { source: stubScript() }, sessionId);
stubInstalled = true;
await cdp.send(
  "Emulation.setDeviceMetricsOverride",
  { ...D_VIEWPORT, deviceScaleFactor: 1, mobile: false },
  sessionId,
);

// ---------------------------------------------------------------- 测量

const MEASURE = `(() => {
  const box = (el) => {
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const one = (sel) => { const el = document.querySelector(sel); return el ? box(el) : null; };
  const inner = document.querySelector('.pt-shell__inner');
  /*
   * 「带」的范围：工具栏带 / 页头带 / 批量选择带。画板的 bar-64 就是这几条，
   * 控件身份只在这里面采 —— 整页采会让卡片里或对话框里同名的按钮冒充工具栏控件。
   */
  ${CLIP_FN}
  const bars = [...document.querySelectorAll('.pt-band--toolbar, .pt-band--head, .pt-band--sel, .pt-head')];
  const inBars = (sel) => bars.flatMap((b) => [...b.querySelectorAll(sel)]);
  return {
    head: one('.pt-head'),
    kpi: one('.pt-kpi'),
    /* KPI 每格：画板要求每格都有变化 pill 与 48×22 柱图，所以逐格量，不只量整条高度 */
    kpiCells: [...document.querySelectorAll('.pt-kpi__cell')].map((el) => ({
      label: (el.querySelector('.pt-kpi__label')?.textContent ?? '').trim(),
      pill: Boolean(el.querySelector('.pt-pill, .pt-status-pill, [class*="pill"]')),
      bars: Boolean(el.querySelector('.pt-kpi__bars')),
    })),
    tabLabels: [...document.querySelectorAll('.pt-band__tab')].map((el) =>
      (el.textContent ?? '').replace(/\\s+/g, ' ').trim(),
    ),
    /*
     * 关键控件：按钮/分段这类「画板上画了、少了就不算落地」的东西。
     * 纯图标钮没有文字，身份在 aria-label / title 上，所以三者都收 ——
     * 只看 textContent 会把画板上那些 32×32 的图标钮全都漏掉。
     */
    /*
     * 下拉型控件（画板把它们画成 chip-0「认证: 全部」这类）不是 button，
     * 文案在 .el-select__placeholder / selected-item 上 —— 不收进来的话
     * 「工具栏少了两枚筛选 chip」这种缺失照样查不出来。
     */
    /*
     * 采集范围**限定在带里**（工具栏带 / 页头带 / 批量选择带），不是整页。
     * 原先扫的是整个 document，于是一个写着「导出」的按钮不管在哪 —— 卡片里、
     * 对话框里 —— 都能让「工具栏有导出」这条断言通过。那不是在查工具栏。
     */
    controlText: inBars(
      'button, .el-segmented__item, .pt-band__tab, .el-select__placeholder, .el-select__selected-item,' +
        /* 勾选框的文案也算控件身份：画板把「跟随: 开」这类画成 chip，落地是 el-checkbox */
        ' .el-checkbox__label, .el-radio__label',
    )
      .flatMap((el) => [
        (el.textContent ?? '').replace(/\\s+/g, ' ').trim(),
        el.getAttribute('aria-label') ?? '',
        el.getAttribute('title') ?? '',
      ])
      .concat(
        /*
         * 搜索框的身份是它的 placeholder（画板就是这么标的：「筛选命令、触发用户…」）。
         * 不收进来的话「这一页少了搜索框」查不出来 —— input 既没有 textContent 也不是 button。
         */
        inBars('input[placeholder], textarea[placeholder]').map(
          (el) => el.getAttribute('placeholder') ?? '',
        ),
      )
      .filter(Boolean),
    /*
     * 整页那一层单独留着：画板也在卡片里画按钮（试跑那张卡的「试跑这条」、
     * 通道详情三张卡的保存/测试钮、下载器控制台「状态」卡里的五档快捷筛选）。
     * 它们同样是画板画了就得有的控件，只是不属于 bar-64 —— 分成两层之后，
     * 「工具栏里有没有」和「这一页里有没有」各自说得清。
     */
    pageControlText: [...document.querySelectorAll(
      'button, .el-segmented__item, .pt-band__tab, .el-select__placeholder, .el-select__selected-item,' +
        ' .el-checkbox__label, .el-radio__label',
    )]
      .flatMap((el) => [
        /* 这一整段在模板字符串里：反斜杠 s 必须写两个反斜杠，写一个注入出去会塌成字母 s */
        (el.textContent ?? '').replace(/\\s+/g, ' ').trim(),
        el.getAttribute('aria-label') ?? '',
        el.getAttribute('title') ?? '',
      ])
      .concat(
        [...document.querySelectorAll('input[placeholder], textarea[placeholder]')].map(
          (el) => el.getAttribute('placeholder') ?? '',
        ),
      )
      .filter(Boolean),
    /*
     * **被裁掉的横向溢出**（桌面这一趟也查）。
     *
     * 移动那趟先有了这条，抓出过一张七列 nowrap 的表；后来在 1440 下又撞见同一类事
     * —— 审计页「失败与被拒」那张卡第四列的说明被卡片边缘切掉，而所有断言全绿。
     * 判据与移动那份一致：自己或任一祖先可横向滚动不算，带省略号或多行夹断的截断不算
     * （那是有意的），剩下的就是「没有任何提示、内容直接消失」。
     */
    clipped: clipIn(
      document.querySelectorAll('.pt-panel, .pt-cards, [data-card], .ch-card'),
    ).slice(0, 6),
    /* 分段器也只认带里的那个：卡片内部的分段不是画板 bar-64 上那条 */
    hasSeg: inBars('.pt-seg, .el-segmented').length > 0,
    /* 表格带的列名与行高 —— 画板对表格页规定了列集合与 34 的行节奏 */
    gridColumns: [...document.querySelectorAll('.pt-band--grid th .cell')]
      .map((el) => (el.textContent ?? '').replace(/\\s+/g, ' ').trim())
      .filter(Boolean),
    gridRowHeight: (() => {
      const row = document.querySelector('.pt-band--grid tbody tr td');
      return row ? Math.round(row.getBoundingClientRect().height) : null;
    })(),
    ownHead: one('.pt-band--head'),
    tabs: one('.pt-band--tabs'),
    toolbar: one('.pt-band--toolbar'),
    grid: one('.pt-band--grid'),
    foot: one('.pt-band--foot'),
    cardsBox: one('.pt-cards'),
    /* 卡片层里每一块的宽度：PtPanel、手搓卡和 hero 都算，空态块不算 */
    /*
     * 卡片层里每一块的宽度与标题。
     *
     * 三个坑，都踩过：
     * ① 页面可能有多个 .pt-cards（修改密码页是「居中窄卡」+「三栏说明卡」两段），
     *    只查第一个会漏掉后面整段；
     * ② 栏容器里可能还套着真正的卡（下载器控制台的左栏 aside 里有三张 PtPanel），
     *    只量栏容器就会把「三张卡」和「一整块面」判成一样；
     * ③ 宽度要按**多重集合**比，画板要两张 1080 就必须真有两张。
     * 所以遇到直接子节点里有 PtPanel 的栏容器就下钻一层，其余按自身算。
     */
    cards: (() => {
      const out = [];
      const push = (el) => {
        if (getComputedStyle(el).display === 'none') return;
        const r = el.getBoundingClientRect();
        if (r.width <= 4) return; /* 拖拽把手一类 */
        /*
         * 身份优先取 PtPanel 的标题；没有标题的块（hero、提示条一类）取 data-card。
         * 少了这一层，无标题的卡就没有可测身份 —— 删掉通道详情的 hero，
         * 剩下的卡照样能把宽度多重集合与张数下限配满，检查不会红。
         */
        out.push({
          w: Math.round(r.width),
          title:
            (el.querySelector('.pt-panel__title')?.textContent ?? '').trim() ||
            el.dataset.card ||
            '',
          /* 卡头高度：画板「卡片 p-*」规定发丝线在 29；没有卡头的块记 null */
          head: (() => {
            const h = el.querySelector('.pt-panel__head');
            return h ? Math.round(h.getBoundingClientRect().height) : null;
          })(),
        });
      };
      /* 只从最外层的卡片容器出发：里层容器的卡已经被外层的后代查询收进来了，
         两层都处理会把同一张卡数两遍（下载器设置页实测 9 张变 4 张真卡）。 */
      const boxes = [...document.querySelectorAll('.pt-cards')]
        .filter((el) => !el.parentElement?.closest('.pt-cards'));
      for (const box of boxes) {
        /* 卡可以嵌在任意深的栏容器里（控制台右栏是「表格卡 + 两张小卡」两层），
           所以直接把这一段里所有 PtPanel 都算上，不数层数。 */
        for (const panel of box.querySelectorAll('.pt-panel')) push(panel);
        /* 没有 PtPanel 的直接子节点也算一块卡：手搓卡、hero、提示条都在这一类 */
        for (const col of box.children) {
          if (col.classList.contains('pt-panel')) continue;
          if (col.querySelector('.pt-panel')) continue;
          push(col);
        }
        /*
         * 显式标了 data-card 的块，无论嵌在第几层都算一块。
         * 站点详情的 bn 横幅就嵌在含 PtPanel 的栏容器里，上面两条都收不到它 ——
         * 加了 data-card 却仍然不可测，等于没加。
         */
        for (const marked of box.querySelectorAll('[data-card]')) {
          if (marked.classList.contains('pt-panel')) continue;
          if (out.some((c) => c.title === marked.dataset.card)) continue;
          push(marked);
        }
      }
      return out;
    })(),
    /*
     * 纵向叠在一起的卡之间有没有缝。
     *
     * 画板的卡片层卡间一律 16。运行日志页的左栏容器 .lv-col 曾经一条样式都没有，
     * 于是三张卡首尾相接（120+156=276、276+289=565），而这件事**所有门禁都看不见**：
     * 带与栏宽都对，卡片张数与标题也都对，只有卡与卡之间那条缝没有人量。
     * 这里按「同一个父节点下、左边对齐、上下相邻的两张卡」量缝，小于 8 就算贴住。
     */
    stackTight: (() => {
      const out = [];
      const boxes = [...document.querySelectorAll('.pt-cards')]
        .filter((el) => !el.parentElement?.closest('.pt-cards'));
      for (const box of boxes) {
        const byParent = new Map();
        for (const panel of box.querySelectorAll('.pt-panel')) {
          /*
           * 隐藏的卡不算。站点详情按分区用 v-show 藏掉非当前分区的卡，
           * display:none 的元素 rect 全是 0 —— 两张藏起来的卡之间「只隔 0」，
           * 那是这条断言自己的假阳性，不是缺陷。
           */
          const r = panel.getBoundingClientRect();
          if (r.height === 0 || r.width === 0) continue;
          if (getComputedStyle(panel).display === 'none') continue;
          const list = byParent.get(panel.parentElement) ?? [];
          list.push(panel);
          byParent.set(panel.parentElement, list);
        }
        for (const list of byParent.values()) {
          if (list.length < 2) continue;
          const rows = list
            .map((el) => ({ el, r: el.getBoundingClientRect() }))
            .sort((a, b) => a.r.top - b.r.top);
          for (let i = 1; i < rows.length; i += 1) {
            const prev = rows[i - 1];
            const cur = rows[i];
            /* 只比同一列里上下相邻的两张：并排的两栏不该参与 */
            if (Math.abs(prev.r.left - cur.r.left) > 4) continue;
            const gap = Math.round(cur.r.top - prev.r.bottom);
            if (gap < 8) {
              const title = (cur.el.querySelector('.pt-panel__title')?.textContent ?? '').trim();
              /* 这段身处 MEASURE 的模板字符串内部，不能再用反引号 */
              out.push((title || '无标题卡') + ' 与上一张只隔 ' + gap);
            }
          }
        }
      }
      return out;
    })(),
    /*
     * 状态栏右端的下载器身份格（画板 statusbar 的「qb-main · 已连接 · v2.9.3」）。
     * 它是外壳构件，每条路由都该在 —— 之前这一格既没实现也没人量，
     * 而偏离表里却写着「已落」。
     */
    /*
     * rail 上的快捷入口。画板 01/D.rail 明确画了八个快捷入口加一个运行日志入口，
     * 而落地在导航列钉住时把它们全藏了（用户验收退回过「有些重复了吧」）。
     * 这里照画板量，红了就去偏离表里读原因 —— 让这条偏离在验收里露头，
     * 而不是只写在文档某一节里。
     */
    railItems: [...document.querySelectorAll('.pt-rail__item')].filter((el) => {
      const cs = getComputedStyle(el);
      /* 钉住时整条 rail 是 visibility:hidden 淡出的，不是 display:none；两种都算不可见 */
      return cs.display !== 'none' && cs.visibility !== 'hidden';
    }).length,
    navDocked: document.querySelector('.pt-shell')?.classList.contains('is-nav-docked') ?? false,
    /*
     * 侧栏这一块的几何。用户定的形态：钉住 = 一块 264 宽的 nav（面跟主题走），rail 淡出；
     * 品牌 logo 与账号头像在收/展两态里位置不变（这是过渡不割裂的前提）。
     */
    side: (() => {
      const side = document.querySelector('.pt-side');
      const nav = document.querySelector('.pt-shell__nav-col');
      const rail = document.querySelector('.pt-rail');
      if (!side || !nav || !rail) return null;
      const box = (el) => { const r = el?.getBoundingClientRect(); return r ? { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) } : null; };
      const cs = (el) => getComputedStyle(el);
      return {
        w: box(side).w,
        sideBg: cs(side).backgroundColor,
        navBg: cs(nav).backgroundColor,
        railVisible: cs(rail).visibility !== 'hidden' && parseFloat(cs(rail).opacity) > 0.5,
        navVisible: cs(nav).visibility !== 'hidden' && parseFloat(cs(nav).opacity) > 0.5,
        shellTransition: cs(document.querySelector('.pt-shell')).transitionProperty,
        navLogo: box(document.querySelector('.pt-nav__logo')),
        railLogo: box(document.querySelector('.pt-rail__brand')),
        navAvatar: box(document.querySelector('.pt-nav__account-av')),
        railAvatar: box(document.querySelector('.pt-rail__avatar')),
        /* 按角色类找，不按 aria-label 文案找：文案改一个字不该让这条断言失明 */
        themeBtn: Boolean([...document.querySelectorAll('.pt-nav__account-btn--theme, .pt-rail__tool--theme')]
          .find((b) => cs(b).visibility !== 'hidden' && cs(b).display !== 'none')),
      };
    })(),
    /*
     * 导航列顶部的版本行（画板 nav 的「v0.47.2 · 已是最新」11/400 t3）。
     * 这里连着量出来是因为它曾经写成「vv0.47.2」—— version.Version 是 ldflags 从
     * git describe 灌的，本仓库的 tag 自带 v，代码又无条件补了一个。
     * （这段身处 MEASURE 的模板字符串里，不能用反引号。）
     */
    navVer: (() => {
      const el = document.querySelector('.pt-nav__brand-ver');
      return el ? (el.textContent ?? '').trim() : null;
    })(),
    statusDl: (() => {
      const el = document.querySelector('.pt-status__dl');
      return el ? (el.textContent ?? '').replace(/\\s+/g, ' ').trim() : null;
    })(),
    overflow: inner ? inner.scrollWidth - Math.round(inner.getBoundingClientRect().width) : 0,
    unknown: [...new Set([...document.querySelectorAll('*')]
      .filter((el) => el instanceof HTMLUnknownElement)
      .map((el) => el.tagName.toLowerCase()))],
  };
})()`;

/** 容差：滚动条会让宽度少 10 左右，1px 的发丝线也会让高度差 1 */
const TOL = 12;

const near = (a, b, tol = TOL) => a !== null && a !== undefined && Math.abs(a - b) <= tol;

/**
 * 探针结果判定。
 *
 * 除了「要命中的片段」之外，还挡一层**明确的失败标记**：探针自己返回的
 * `no-xxx`（没找到触发器）、`probe 失败`（脚本抛了）、`MISMATCH`（探针自己算出不符）
 * 一律算失败，哪怕期望片段恰好还在串里。
 * 起因是一次真实的假绿：期望写「一致」，而失败串是「不一致」—— includes 照样通过。
 */
function probeMiss(text, want) {
  if (/^no-|probe 失败|MISMATCH/.test(text)) return [text.slice(0, 80)];
  return want.split("|").filter((w) => !text.includes(w));
}

// ---------------------------------------------------------------- 逐路由检查

/** --route 可以给逗号分隔的子集，调试时只跑几条 */
const wanted = ONLY ? ONLY.split(",").map((r) => r.trim()) : [];
const routes = Object.keys(EXPECT).filter((r) => wanted.length === 0 || wanted.includes(r));
const failures = [];
const gapsUsed = new Set();

for (const route of routes) {
  const want = EXPECT[route];
  consoleErrors = [];
  /*
   * 导航之前先铺状态。localStorage 里的历史数据会走到只有老用户才碰得到的分支
   * （例如「保存搜索」的 v1→v2 迁移），而每条路由都用全新的 Chrome profile，
   * 不铺就永远测不到那条路 —— 一次 review 正是从这里找出一个整页白屏的 TDZ。
   */
  if (want.seed) await ev(want.seed).catch(() => {});
  await goto(`${BASE}/#${route}`);
  await sleep(2400);
  if (want.prepare) {
    const outcome = await ev(want.prepare).catch((e) => `prepare 失败：${e.message}`);
    if (typeof outcome === "string" && outcome.startsWith("no-"))
      failures.push({
        route,
        board: want.board,
        key: "prepare",
        msg: `准备步骤没跑起来：${outcome}`,
      });
    await sleep(600);
  }
  const got = await ev(MEASURE);
  const fail = (key, msg) => {
    const gapKey = `${route} ${key}`;
    if (ALLOWED_GAPS[gapKey]) {
      gapsUsed.add(gapKey);
      return;
    }
    failures.push({ route, board: want.board, key, msg });
  };

  /*
   * 交互探针。**放在静态测量之后**：探针会点控件、改页面状态，跑在前面会把带与卡片的
   * 量测结果搅乱。带与栏宽量的是「构成对不对」，量不到「点下去有没有用」——
   * 一次评审正是指出分类分段器对没有分类的站点不起作用，而验收从不点它。
   */
  for (const probe of want.probes ?? []) {
    const outcome = await ev(probe.js).catch((e) => `probe 失败：${e.message}`);
    const text = typeof outcome === "string" ? outcome : JSON.stringify(outcome);
    const missing = probeMiss(text, probe.want);
    if (process.env.PT_PROBE_VERBOSE) console.log(`  probe> ${route} ${text}`);
    if (missing.length > 0) {
      fail(`probe(${probe.desc})`, `结果里缺「${missing.join("、")}」，实测 ${text}`);
    }
  }

  for (const err of [...new Set(consoleErrors)].slice(0, 3)) fail("console", err);
  if (got.overflow > 0) fail("overflow", `主区横向溢出 ${got.overflow}px`);
  if (got.unknown.length > 0) fail("unknown", `未解析组件 ${got.unknown.join(", ")}`);
  for (const tight of got.stackTight) fail("cards.gap", `卡片贴在一起（画板卡间 16）：${tight}`);
  /*
   * 状态栏那一格。画板给的是「名称 · 连接态 · 版本」，版本后端没有字段（见 ALLOWED_GAPS），
   * 所以这里只要求名称与连接态，并且连接态必须是真判出来的 —— 假数据里默认下载器可达，
   * 量到「未知」就说明这一格的判定链断了（比如 reachable 字段没读对）。
   */
  /*
   * rail 的快捷入口。只在导航列钉住时才判：导航列收起时它们本来就该出现（也确实出现），
   * 画板画的是「两者同时在」，这一条就是那个差异。
   */
  if (got.navDocked && got.railItems < 9)
    fail(
      "rail.items",
      `导航列钉住时 rail 上可见的快捷入口只有 ${got.railItems} 个，画板画了 8 个快捷入口 + 1 个运行日志入口`,
    );
  if (got.navVer !== null && got.navVer.startsWith("vv"))
    fail("nav.version", `导航列版本行是「${got.navVer}」，多了一个 v`);
  /*
   * 侧栏是一块（用户决定，见 MAIN_X 上面那段）。钉住时只量一次就够，不用每条路由都量 ——
   * 但这里每条都量的代价只是几个 getBoundingClientRect，换来的是任何一页把外壳撑坏都会被抓。
   */
  if (got.navDocked && got.side) {
    const sd = got.side;
    if (!near(sd.w, SIDE_DOCKED_W, 1))
      fail("shell.side", `钉住时侧栏 ${sd.w} 宽，应为一块 ${SIDE_DOCKED_W} 宽的 nav`);
    if (sd.railVisible)
      fail("shell.rail", "钉住时 rail 仍然可见：黑色空条贴着导航列，正是用户退回的那个样子");
    if (!sd.navVisible) fail("shell.nav", "钉住时导航列不可见");
    if (sd.sideBg !== sd.navBg)
      fail(
        "shell.tone",
        `侧栏格子底色 ${sd.sideBg} 与导航列底色 ${sd.navBg} 不一致，收/展会闪一下`,
      );
    if (!/grid-template-columns/.test(sd.shellTransition))
      fail(
        "shell.motion",
        `.pt-shell 没有对 grid-template-columns 做过渡（实测 ${sd.shellTransition}），收/展会一步跳到位`,
      );
    /* logo 与头像两态同位：rail 的盒子此刻虽然淡出了，几何还在，可以直接比 */
    if (
      sd.navLogo &&
      sd.railLogo &&
      (Math.abs(sd.navLogo.x - sd.railLogo.x) > 1 || Math.abs(sd.navLogo.y - sd.railLogo.y) > 1)
    )
      fail(
        "shell.logo",
        `nav 的 logo 在 (${sd.navLogo.x},${sd.navLogo.y})，rail 的在 (${sd.railLogo.x},${sd.railLogo.y})：收/展时 logo 会跳`,
      );
    if (sd.navAvatar && sd.railAvatar) {
      const cx = (b) => b.x + b.w / 2;
      const cy = (b) => b.y + b.h / 2;
      if (
        Math.abs(cx(sd.navAvatar) - cx(sd.railAvatar)) > 1.5 ||
        Math.abs(cy(sd.navAvatar) - cy(sd.railAvatar)) > 1.5
      )
        fail(
          "shell.avatar",
          `nav 头像中心 (${cx(sd.navAvatar)},${cy(sd.navAvatar)})，rail 头像中心 (${cx(sd.railAvatar)},${cy(sd.railAvatar)})：收/展时头像会跳`,
        );
    }
    if (!sd.themeBtn)
      fail("shell.theme", "钉住时找不到可见的明暗开关 —— 用户硬约束：主题入口必须一眼看得见");
  }
  /*
   * 画板 statusbar 右端是「名称 · 连接态 · 版本」三段。版本那段曾经空着（理由是 HTTP 层
   * 没有这个字段），后来补进了 transfer-stats，所以现在三段都要在。
   */
  if (got.statusDl === null) fail("statusbar.dl", "状态栏右端没有下载器身份格");
  else {
    const want = ["qb-main", "已连接", "v4.6.7"].filter((w) => !got.statusDl.includes(w));
    if (want.length > 0)
      fail("statusbar.dl", `状态栏那格是「${got.statusDl}」，缺「${want.join("、")}」`);
  }

  // 页头：画板里除 KPI 页与搜索页之外，每页都是 64（详情页 88）
  if (want.ownHead === "kpi") {
    if (!got.kpi) fail("kpi", "画板用 KPI 带替代 head，页面上找不到 .pt-kpi");
    else if (!near(got.kpi.h, 64)) fail("kpi", `KPI 带高 ${got.kpi.h}，画板 64`);
    /* 画板 kpibar：6 格，每格都有变化 pill 与 48×22 柱图 */
    const cells = got.kpiCells;
    if (cells.length !== (want.kpiCells ?? 6)) {
      fail("kpi.cells", `KPI 有 ${cells.length} 格，画板 ${want.kpiCells ?? 6} 格`);
    }
    const noBars = cells.filter((c) => !c.bars).map((c) => c.label);
    if (noBars.length > 0) fail("kpi.bars", `这些格没有柱图：${noBars.join("、")}`);
    const noPill = cells.filter((c) => !c.pill).map((c) => c.label);
    if (noPill.length > 0) fail("kpi.pill", `这些格没有变化 pill：${noPill.join("、")}`);
  } else if (want.ownHead === "head88") {
    if (!got.ownHead) fail("ownHead", "画板 search-head 88，页面上找不到 .pt-band--head");
    else if (!near(got.ownHead.h, 88, 2)) fail("ownHead", `自带页头高 ${got.ownHead.h}，画板 88`);
  } else {
    const wantH = want.detail ? 88 : 64;
    if (!got.head) fail("head", "找不到 .pt-head");
    else if (!near(got.head.h, wantH, 2)) fail("head", `页头高 ${got.head.h}，画板 ${wantH}`);
  }

  /* 表格页的行节奏与列集合（画板对表格页规定了这两样） */
  if (want.gridRowHeight && got.gridRowHeight !== null) {
    if (!near(got.gridRowHeight, want.gridRowHeight, 3)) {
      fail("grid.rowHeight", `行高 ${got.gridRowHeight}，画板 ${want.gridRowHeight}`);
    }
  }
  if (want.gridColumns) {
    /*
     * 列名比较**不能用 includes**：`站点活跃` 里含 `站点`，于是多出来的那一列会被当成
     * 画板的「站点」列而躲过检查（实测漏报过）。规则收紧成「整个相等，或画板名后面紧跟
     * `/` 或空格」—— 这样 `时魔/h` 仍能被画板的 `时魔` 认领，`站点活跃` 不会。
     */
    const matches = (header, col) =>
      header === col || header.startsWith(`${col}/`) || header.startsWith(`${col} `);
    /* 画板的列必须都在 */
    for (const col of want.gridColumns) {
      if (!got.gridColumns.some((c) => matches(c, col))) {
        fail("grid.columns", `表头里没有「${col}」；实测 ${got.gridColumns.join(" / ")}`);
      }
    }
    /*
     * 画板之外的列也要报。只查「缺了没」挡不住「多了几列」——
     * 而多列一样是与画板不一致，且会改变整表的列宽分配。
     */
    const extra = got.gridColumns.filter((c) => !want.gridColumns.some((w) => matches(c, w)));
    if (extra.length > 0) {
      fail("grid.columns", `多出画板之外的列：${extra.join(" / ")}`);
    }
  }

  /*
   * 卡头高度。画板「卡片 p-*」规定卡头发丝线在 29（docs/design/webui-board-spec.md）。
   * 这是 85 个 PtPanel 实例共用的一个数，错了会系统性改变所有卡的内部比例，
   * 而它在验收里一直没人看守 —— 实现曾经是 42，跑了很多轮都没红。
   */
  for (const card of got.cards) {
    if (card.head === null) continue;
    if (!near(card.head, 29, 3)) {
      fail(`cards.head(${card.title || card.w})`, `卡头高 ${card.head}，画板 29`);
    }
  }

  /*
   * 关键控件。画板上画了一枚按钮或一条分段器，少了就不算按稿落地 ——
   * 光量带高与栏宽看不出「工具栏里少了分类分段」这种事。
   */
  if (got.clipped.length > 0) {
    fail("clipped", `卡内有被裁掉的横向溢出：${got.clipped.join(" / ")}`);
  }

  if (want.needsSeg && !got.hasSeg) {
    fail("controls.seg", "画板这一页有分段器，页面上找不到");
  }
  for (const label of want.controls ?? []) {
    if (!got.controlText.some((t) => t.includes(label))) {
      fail(`controls(${label})`, `带里（工具栏 / 页头 / 批量条）找不到写着「${label}」的控件`);
    }
  }
  /* 画板画在卡片里的控件：不要求在带上，但这一页里必须有 */
  for (const label of want.pageControls ?? []) {
    if (!got.pageControlText.some((t) => t.includes(label))) {
      fail(`pageControls(${label})`, `整页都找不到写着「${label}」的控件`);
    }
  }

  /*
   * 张数下限。光比栏宽挡不住「用别的卡冒充」：画板 22 是四张通道卡（548/516 两栏）
   * 外加 p-policy 548 / p-stat 516 / p-recent 1080，通道卡本身就把 548 与 516 占了，
   * 宽度多重集合会给出假绿。张数是不含糊的第二道。
   * 用下限而不是精确值：通道卡、站点卡这类的数量随数据变。
   */
  if (want.minCards && got.cards.length < want.minCards) {
    fail(
      "cards.count",
      `卡片层只有 ${got.cards.length} 块，画板至少 ${want.minCards} 张：` +
        got.cards.map((c) => `${c.w}${c.title ? `(${c.title})` : ""}`).join(" / "),
    );
  }

  if (want.tabs) {
    if (!got.tabs) fail("tabs", "画板有 40 高的分区带，页面上找不到 .pt-band--tabs");
    else if (!near(got.tabs.h, 40, 2)) fail("tabs", `分区带高 ${got.tabs.h}，画板 40`);
    /* 分区带的项要逐个对上画板的标签，不能只对上带高 */
    for (const label of want.tabLabels ?? []) {
      if (!got.tabLabels.some((t) => t.includes(label))) {
        fail(`tabs.label(${label})`, `分区带里没有「${label}」；实测 ${got.tabLabels.join(" / ")}`);
      }
    }
  }

  // 带：存在、贴住主区两侧、高度对得上、彼此相邻
  for (const band of want.bands ?? []) {
    const b = got[band];
    if (!b) {
      fail(`bands.${band}`, `画板有 ${band} 带，页面上没有`);
      continue;
    }
    if (!near(b.x, MAIN_X) || !near(b.w, MAIN_W))
      fail(`bands.${band}`, `${band} 带在 x=${b.x} 宽 ${b.w}，画板 x=${MAIN_X} 宽 ${MAIN_W}`);
    const wantH = { toolbar: 40, foot: 34 }[band];
    if (wantH && !near(b.h, wantH, 3)) fail(`bands.${band}`, `${band} 带高 ${b.h}，画板 ${wantH}`);
  }
  if (got.toolbar && got.grid && !near(got.grid.y, got.toolbar.y + got.toolbar.h, 2))
    fail(
      "bands.adjacent",
      `表格带 y=${got.grid.y} 没有紧贴工具栏带底沿 ${got.toolbar.y + got.toolbar.h}`,
    );
  if (got.grid && got.foot && got.foot.y < got.grid.y)
    fail("bands.adjacent", "页脚带排在表格带之前");

  // 卡片层：内缩 16，栏宽按画板
  const wantCards = want.cards ?? [];
  if (wantCards.length > 0) {
    if (!got.cardsBox) {
      fail(`cards[${wantCards.join(",")}]`, "画板有卡片区，页面上找不到 .pt-cards");
    } else {
      if (!near(got.cardsBox.x, MAIN_X))
        fail("cards.x", `卡片区 x=${got.cardsBox.x}，画板 ${MAIN_X}`);
      const found = got.cards;
      if (found.length === 0) {
        fail(`cards[${wantCards.join(",")}]`, "卡片区是空的");
      } else {
        /*
         * 按多重集合配：每期望一张就从实测里消掉一张，不去重 ——
         * 画板要两张 1080，实际只有一张就得红。
         */
        const pool = [...found];
        const missing = [];
        for (const w of wantCards) {
          const hit = pool.findIndex((c) => near(c.w, w));
          if (hit < 0) missing.push(w);
          else pool.splice(hit, 1);
        }
        const shown = found.map((c) => `${c.w}${c.title ? `(${c.title})` : ""}`).join(" / ");
        /*
         * 身份断言。只比栏宽与张数挡不住「用另一张同宽的卡补位」——
         * 删掉通道详情里任意一张 1080 的卡，剩下的照样能把三张 1080 配满。
         * 所以每条路由列出画板那几张卡的标题，逐个要求它在页面上真的存在。
         */
        for (const title of want.titles ?? []) {
          if (!found.some((c) => c.title.includes(title))) {
            fail(`cards.title(${title})`, `找不到标题含「${title}」的卡；实测 ${shown}`);
          }
        }
        /* 逐张上报：一页里某一张卡做不到，不该把整页的卡片检查一起豁免 */
        const seen = new Map();
        for (const w of missing) {
          const n = (seen.get(w) ?? 0) + 1;
          seen.set(w, n);
          /* 同宽多张时键带序号：第 2 张缺了要单独登记，不能被第 1 张的偏离顺带豁免 */
          fail(`cards.${w}${n > 1 ? `#${n}` : ""}`, `缺画板 ${w} 宽的卡；实测 ${shown}`);
        }
      }
    }
  }
}

// ------------------------------------------------- 移动端（画板 30–35 与 §9）

/*
 * 桌面那 21 条路由之外，画板还有六块移动稿，此前完全没进验收 —— 脚本固定
 * 1440×1024，于是「零偏离」只在桌面口径内成立，而文档没把这个口径写出来。
 *
 * 六块移动画板读回来的骨架完全一样（docs/design/webui-board-spec.md §10）：
 *   topbar 0,0 375×88 fill chrome —— 但**前 36 是手机自己的状态栏**
 *          （14:32 / 信号 / Wi-Fi / 电量），浏览器里不存在也不该画。属于 app 的是
 *          40…88：左 h1 19/700 + sub 11/400，右两个 30×30 r=4 图标钮（search、bell）
 *   内容   x=16 宽 343（左右各内缩 16）
 *   tabbar 0,739 375×72 —— **状态行在这 72 之内**（sel 指示条在 765）
 *   五个 tab：概览 / 站点 / 任务 / 搜索 / 我的
 * §9 另外规定：触控目标 ≥ 44×44、桌面表格一律降级成行卡（不做横向滚动表格）、
 * KPI 条降级成 2×2。
 */
const M_VIEWPORT = { width: 375, height: 812 };
/** app 顶栏：画板属于 app 的那段是 48，落地 52（理由见 theme.scss 的注释） */
const M_TOPBAR_H = 52;
/** 底栏 = 状态行 24 + tab 行 48 = 72，与画板 tabbar 的 72 对上 */
const M_NAV_H = 72;
const M_TABS = ["概览", "站点", "任务", "搜索", "我的"];
/** 内容列：画板一律 x=16 宽 343 */
const M_INNER_X = 16;

const MOBILE_EXPECT = {
  "/sites": {
    board: 30,
    title: "站点",
    sub: true,
    rowCards: 1,
    activeTab: "站点",
    /* 画板 30 的筛选是一排 26 高的 chip（全部 14 / 正常 12 / 异常 2 / 已禁用 1） */
    filterRowH: 26,
    /* 画板 30 的行卡里有 120×18 的 8 根柱 */
    rowSpark: true,
  },
  "/tasks": {
    board: 31,
    title: "任务",
    sub: true,
    rowCards: 1,
    activeTab: "任务",
    /* 画板 31 是一条 294×30 的分段器 */
    filterRowH: 30,
  },
  "/sites/M-Team": {
    board: 32,
    title: "M-Team",
    sub: true,
    /* 画板 32：bn 横幅 + tabs + p-kv / p-rss / p-act 三张卡 */
    minBlocks: 4,
    anchors: ["站点凭据", "保号规则"],
  },
  "/chatops/notifications": {
    board: 33,
    title: "消息通知",
    sub: true,
    /* 画板 33：ch-0…3 四张通道卡 + 「最近投递」小节 */
    minBlocks: 4,
    anchors: ["投递策略", "最近的通知"],
  },
  "/logs": {
    board: 35,
    title: "运行日志",
    sub: true,
    /* 画板 35：seg + file + lg 正文 + kv */
    minBlocks: 3,
    anchors: ["日志文件", "级别筛选", "轮转归档"],
  },
  /*
   * 画板 34「我的」在产品里没有对应路由：它是个设置聚合页，落地折成了底栏「我的」tab
   * 打开的上拉面板。所以这一格挂在 /userinfo（概览 tab 指向的路由）上，
   * 既验 §9 的 KPI 2×2，又把面板点开验画板 34 的内容 —— 否则画板 34 等于没进验收。
   */
  "/userinfo": {
    board: "§9 + 34",
    title: "用户统计",
    kpiCols: 2,
    activeTab: "概览",
    probes: [
      {
        desc: "点底栏「我的」打开画板 34 的设置聚合面板",
        /* 画板 34 的三组：通知与 ChatOps / 站点与下载 / 系统；外加偏好与退出 */
        want: "系统|ChatOps|偏好",
        js: `(async () => {
          const tab = [...document.querySelectorAll('.pt-mnav__tab')]
            .find((b) => (b.textContent ?? '').includes('我的'));
          if (!tab) return 'no-tab';
          tab.click();
          await new Promise((r) => setTimeout(r, 700));
          const sheet = document.querySelector('.pt-msheet');
          if (!sheet) return 'no-sheet';
          const text = (sheet.textContent ?? '').replace(/\\s+/g, ' ');
          const groups = [...sheet.querySelectorAll('.pt-msheet__title')]
            .map((el) => (el.textContent ?? '').trim());
          const hasPrefs = text.includes('明亮') || text.includes('黑暗') || text.includes('主题');
          return groups.join('|') + (hasPrefs ? '|偏好' : '') + '|项 ' +
            sheet.querySelectorAll('.pt-msheet__item').length;
        })()`,
      },
    ],
  },
};

const MEASURE_M = `(() => {
  ${CLIP_FN}
  const box = (el) => {
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const one = (sel) => { const el = document.querySelector(sel); return el ? box(el) : null; };
  const txt = (sel) => (document.querySelector(sel)?.textContent ?? '').replace(/\\s+/g, ' ').trim();
  const inner = document.querySelector('.pt-shell__inner');
  return {
    topbar: one('.pt-mchrome'),
    title: txt('.pt-mchrome__title'),
    sub: txt('#pt-mhead-sub'),
    /*
     * 顶栏图标钮：可见方块按画板是 30×30，而 §9 要求命中区 ≥44×44。两者只能靠
     * 「方块 30 + ::before 四周外扩 7」同时满足，所以这里把外扩量算进来：
     * 命中宽 = 方块宽 + |左| + |右|，高同理。
     *
     * 为什么不用 elementFromPoint 打点：那个结果取决于那一刻谁在最上层 ——
     * 桌面那一轮留在 body 上的 popper/tooltip 会在切到 375 之后盖住顶栏一角，
     * 于是同一份代码在整轮跑里通过、只跑三条路由时失败。几何算法没有这个问题。
     */
    icons: [...document.querySelectorAll('.pt-mchrome__icon')].map((el) => {
      const r = el.getBoundingClientRect();
      const before = getComputedStyle(el, '::before');
      const grow = (side) => {
        if (before.content === 'none') return 0;
        const v = parseFloat(before[side]);
        /* 只有负的 inset 才是往外扩；正数是往里缩，不该算进命中区 */
        return Number.isFinite(v) && v < 0 ? -v : 0;
      };
      return {
        label: el.getAttribute('aria-label') ?? '',
        w: Math.round(r.width),
        h: Math.round(r.height),
        touchW: Math.round(r.width + grow('left') + grow('right')),
        touchH: Math.round(r.height + grow('top') + grow('bottom')),
      };
    }),
    /*
     * 内容列里的块：卡、行卡、以及显式标了 data-card 的块。
     * 画板 30–35 的内容列一律 343 宽，块数各页不同 —— 只量「有没有 343 宽的块、够不够数」，
     * 不量每块的高度：高度跟着真实数据变。
     */
    /*
     * **被裁掉的横向溢出**。页面级的 docOverflow 量不到它：外层 overflow: hidden 时
     * 溢出的内容既不产生文档滚动条，也不会被 clientWidth 察觉 —— 它只是消失。
     * 一次评审正是查出通知页在 375 下渲染了一张七列 nowrap 的表，右边两列直接没了，
     * 而所有断言全绿（块数与标题都对得上）。
     * 判据：任何块内出现 scrollWidth 明显超过 clientWidth 的元素就算裁切。
     * §9 的房规是「桌面表格一律降级成行卡，不做横向滚动表格」，所以顺带把窄屏里
     * 还活着的 table 也报出来。
     */
    clipped: clipIn(
      document.querySelectorAll('.pt-panel, .pt-rowcard, [data-card], .ch-card, .pt-cards'),
    ).slice(0, 6),
    /* 窄屏里还活着的表格（§9：桌面表格一律降级成行卡） */
    tables: [...document.querySelectorAll('.pt-panel table, [data-card] table, .ch-card table')]
      .map((t) => (t.className || 'table').toString().slice(0, 30)),
    blocks: (() => {
      const set = new Set(document.querySelectorAll('.pt-panel, .pt-rowcard, [data-card]'));
      /*
       * 手搓卡也算一块。通知页的四张通道卡是 .ch-card 而不是 PtPanel ——
       * 只认 PtPanel 的话这一页在移动端会少数四块，断言就成了假红。
       * 口径与桌面那份一致：卡片层的直接子节点里，自己不含 PtPanel 的也算一块。
       */
      for (const box of document.querySelectorAll('.pt-cards')) {
        for (const child of box.children) {
          if (child.querySelector('.pt-panel')) continue;
          set.add(child);
        }
      }
      return [...set]
        .filter((el) => getComputedStyle(el).display !== 'none')
        .map((el) => Math.round(el.getBoundingClientRect().width))
        .filter((w) => w > 4);
    })(),
    /* 工具栏带：画板 30 的筛选是一排 26 高的 chip，31 是一条 30 高的分段器 */
    toolbar: one('.pt-band--toolbar'),
    /* 行卡里有没有柱图（画板 30 的行卡带 120×18 的 8 根柱） */
    rowCardsWithSpark: [...document.querySelectorAll('.pt-rowcard')]
      .filter((el) => el.querySelector('.pt-bars, .pt-kpi__bars')).length,
    nav: one('.pt-mnav'),
    tabs: [...document.querySelectorAll('.pt-mnav__tab')].map((el) => {
      const r = el.getBoundingClientRect();
      return {
        label: (el.textContent ?? '').replace(/\\s+/g, ' ').trim(),
        w: Math.round(r.width),
        h: Math.round(r.height),
        active: el.classList.contains('is-active'),
      };
    }),
    /* 桌面外壳必须整套让位：375 下 rail 与桌面页头会把内容挤没 */
    deskVisible: ['.pt-rail', '.pt-status', '.pt-head'].filter((sel) => {
      const el = document.querySelector(sel);
      return el && getComputedStyle(el).display !== 'none';
    }),
    /*
     * 内容列的**内容边界**，不是元素边界。.pt-shell__inner 本身是整宽的，
     * 16 的内缩在它的 padding 里 —— 量 getBoundingClientRect().left 永远得到 0，
     * 那样的断言只会证明「元素贴着屏幕左边」，跟画板的内缩没关系。
     * （这段注释身处一个模板字符串内部，所以不能用反引号引类名。）
     */
    inner: inner
      ? (() => {
          const r = inner.getBoundingClientRect();
          const cs = getComputedStyle(inner);
          const padL = parseFloat(cs.paddingLeft) || 0;
          const padR = parseFloat(cs.paddingRight) || 0;
          return {
            x: Math.round(r.left + padL),
            w: Math.round(r.width - padL - padR),
          };
        })()
      : null,
    /* 行卡：§9 要求桌面表格在移动端降级成行卡 */
    rowCards: document.querySelectorAll('.pt-rowcard').length,
    /*
     * 行卡本体的形状。画板 30 / 31 量出来的一套：343 宽、r8、1px #DDE2E9、
     * 横向内边距 14（于是内容起点落在屏幕 x=30，两块板都是 30）、卡间 8，
     * 而且卡是摆在 #F2F4F7 画布上的 —— 白卡压在白色表格面上只剩一根边框，层次全平。
     * 状态胶囊左边还有一枚 5px 圆点（st > dot），这是行卡上唯一的状态标记。
     */
    rowCard: (() => {
      const cards = [...document.querySelectorAll('.pt-rowcard')];
      if (!cards.length) return null;
      const el = cards[0];
      const cs = getComputedStyle(el);
      const r = el.getBoundingClientRect();
      const band = el.parentElement?.parentElement ?? null;
      const gaps = [];
      for (let i = 1; i < cards.length; i++) {
        const prev = cards[i - 1].getBoundingClientRect();
        const cur = cards[i].getBoundingClientRect();
        if (cur.top >= prev.bottom - 1) gaps.push(Math.round(cur.top - prev.bottom));
      }
      const withStatus = cards.filter((c) => c.querySelector('.pt-rowcard__status .pt-pill'));
      return {
        w: Math.round(r.width),
        padX: Math.round(parseFloat(cs.paddingLeft) || 0),
        contentX: Math.round(r.left + (parseFloat(cs.paddingLeft) || 0)),
        radius: Math.round(parseFloat(cs.borderTopLeftRadius) || 0),
        gaps: [...new Set(gaps)],
        /* 承载行卡的那条带还是白的吗？画板要的是透出画布 */
        onSurface: band ? getComputedStyle(band).backgroundColor === 'rgb(255, 255, 255)' : false,
        statusPills: withStatus.length,
        statusDots: withStatus.filter((c) =>
          c.querySelector('.pt-rowcard__status .pt-pill__dot')).length,
        /*
         * 卡内图标钮的**命中区**：视觉可以只有 20/28，44 由 ::before 的负 inset 往外撑。
         * 用负 margin 撑是错的 —— 盒子还是 44，只是把兄弟挪走，父级会真的溢出（裁切那条会抓）。
         */
        touch: [...el.querySelectorAll('button, .el-button')].map((b) => {
          const br = b.getBoundingClientRect();
          const bf = getComputedStyle(b, '::before');
          const grow = (side) => {
            if (bf.content === 'none') return 0;
            const v = parseFloat(bf[side]);
            return Number.isFinite(v) && v < 0 ? -v : 0;
          };
          return {
            label: b.getAttribute('aria-label') || (b.textContent ?? '').trim().slice(0, 8) || '?',
            w: Math.round(br.width + grow('left') + grow('right')),
            h: Math.round(br.height + grow('top') + grow('bottom')),
          };
        }),
      };
    })(),
    /*
     * 还在横向滚的表格。§9 明确「不做横向滚动表格」，而一张 el-table 在 375 下默认
     * 就是横向滚的 —— 它自己滚，不会让 documentElement 溢出，所以只看页面溢出量不出来。
     */
    scrollers: [...document.querySelectorAll('table, .el-table__body-wrapper, .pt-band--grid')]
      .filter((el) => getComputedStyle(el).display !== 'none' && el.scrollWidth - el.clientWidth > 8)
      .map((el) => String(el.className).split(' ')[0] || el.tagName.toLowerCase()),
    /* 列数量在 .pt-kpi__grid 上，不在 .pt-kpi 上（外层是带，内层才是网格） */
    kpiCols: (() => {
      const grid = document.querySelector('.pt-kpi__grid');
      if (!grid) return null;
      return getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean).length;
    })(),
    kpiNoBars: [...document.querySelectorAll('.pt-kpi__cell')]
      .filter((el) => !el.querySelector('.pt-kpi__bars'))
      .map((el) => (el.querySelector('.pt-kpi__label')?.textContent ?? '').trim()),
    /* 内容区文本：用来核对画板上那几块的标题真的在（不必逐块定位） */
    text: (document.querySelector('.pt-shell__content')?.textContent ?? '').replace(/\\s+/g, ' '),
    /* 整页横向溢出：手机上出现横向滚动条等于这块画板完全不成立 */
    docOverflow: document.documentElement.scrollWidth - window.innerWidth,
    unknown: [...new Set([...document.querySelectorAll('*')]
      .filter((el) => el instanceof HTMLUnknownElement)
      .map((el) => el.tagName.toLowerCase()))],
  };
})()`;

const mobileRoutes = Object.keys(MOBILE_EXPECT).filter(
  (r) => wanted.length === 0 || wanted.includes(r),
);

if (mobileRoutes.length > 0) {
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { ...M_VIEWPORT, deviceScaleFactor: 1, mobile: true },
    sessionId,
  );

  for (const route of mobileRoutes) {
    const want = MOBILE_EXPECT[route];
    consoleErrors = [];
    await goto(`${BASE}/#${route}`);
    await sleep(2400);
    const got = await ev(MEASURE_M);
    /* 移动端的偏离键带 @375，桌面那份登记不会顺带豁免移动端 */
    const fail = (key, msg) => {
      const gapKey = `${route}@375 ${key}`;
      if (ALLOWED_GAPS[gapKey]) {
        gapsUsed.add(gapKey);
        return;
      }
      failures.push({ route: `${route}@375`, board: want.board, key, msg });
    };

    for (const err of [...new Set(consoleErrors)].slice(0, 3)) fail("console", err);
    if (got.docOverflow > 0) fail("overflow", `整页横向溢出 ${got.docOverflow}px`);
    /*
     * 交互探针放在静态测量之后：它会点控件、开浮层，跑在前面会把量测搅乱。
     * want 里可以写多个候选（用 | 分隔），命中任一个即通过。
     */
    for (const probe of want.probes ?? []) {
      const outcome = await ev(probe.js).catch((e) => `probe 失败：${e.message}`);
      const text = typeof outcome === "string" ? outcome : JSON.stringify(outcome);
      const missing = probeMiss(text, probe.want);
      if (missing.length > 0)
        fail(`probe(${probe.desc})`, `结果里缺「${missing.join("、")}」，实测 ${text}`);
    }
    if (got.unknown.length > 0) fail("unknown", `未解析组件 ${got.unknown.join(", ")}`);
    if (got.deskVisible.length > 0)
      fail("shell", `375 下桌面外壳没让位：${got.deskVisible.join(" / ")}`);

    if (!got.topbar) fail("topbar", "找不到 .pt-mchrome");
    else {
      if (!near(got.topbar.h, M_TOPBAR_H, 2))
        fail("topbar", `顶栏高 ${got.topbar.h}，应为 ${M_TOPBAR_H}`);
      if (!near(got.topbar.w, M_VIEWPORT.width, 2))
        fail("topbar", `顶栏宽 ${got.topbar.w}，应为 ${M_VIEWPORT.width}`);
    }
    if (!got.title.includes(want.title))
      fail("topbar.title", `标题是「${got.title}」，画板 h1 是「${want.title}」`);
    /*
     * 摘要行。画板 30–35 每一块的 topbar 都有 sub，而它是页面 Teleport 进来的 ——
     * 桌面页头在 ≤768 整条 display:none，这一行曾经在手机上完全消失。
     */
    if (want.sub && !got.sub) fail("topbar.sub", "画板 topbar 有 sub 摘要行，手机上是空的");

    for (const icon of got.icons) {
      if (!near(icon.w, 30, 2) || !near(icon.h, 30, 2))
        fail(`topbar.icon(${icon.label})`, `钮 ${icon.w}×${icon.h}，画板 30×30`);
      if (icon.touchW < 44 || icon.touchH < 44)
        fail(
          `topbar.touch(${icon.label})`,
          `命中区 ${icon.touchW}×${icon.touchH}，§9 的触控目标下限是 44×44`,
        );
    }
    for (const label of ["种子搜索", "消息通知"]) {
      if (!got.icons.some((i) => i.label.includes(label)))
        fail(`topbar.icon(${label})`, `画板 topbar 右侧有这个钮，页面上找不到`);
    }

    if (!got.nav) fail("nav", "找不到 .pt-mnav");
    else if (!near(got.nav.h, M_NAV_H, 2)) fail("nav", `底栏高 ${got.nav.h}，画板 ${M_NAV_H}`);
    if (got.tabs.length !== M_TABS.length)
      fail("nav.tabs", `底栏有 ${got.tabs.length} 个 tab，画板 ${M_TABS.length} 个`);
    for (const label of M_TABS) {
      if (!got.tabs.some((t) => t.label.includes(label)))
        fail(
          `nav.tab(${label})`,
          `底栏里没有「${label}」；实测 ${got.tabs.map((t) => t.label).join(" / ")}`,
        );
    }
    /* §9 的触控下限对 tab 同样成立：375/5 = 75 宽，高按 tab 行 48 */
    for (const tab of got.tabs) {
      if (tab.h < 44 || tab.w < 44)
        fail(`nav.touch(${tab.label})`, `tab ${tab.w}×${tab.h}，§9 要求 ≥44×44`);
    }
    if (want.activeTab && !got.tabs.some((t) => t.active && t.label.includes(want.activeTab)))
      fail(
        "nav.active",
        `这条路由应点亮「${want.activeTab}」tab；实测 ${
          got.tabs
            .filter((t) => t.active)
            .map((t) => t.label)
            .join(" / ") || "没有点亮的"
        }`,
      );

    if (got.inner) {
      if (!near(got.inner.x, M_INNER_X, 2))
        fail("inner.x", `内容列 x=${got.inner.x}，画板 ${M_INNER_X}`);
      /*
       * 画板的内容宽度是 375 - 16*2 = 343。容差走默认的 12：模拟视口里的竖向滚动条
       * 占掉 10 左右，实测 333 是滚动条占的，不是内缩写错了。
       */
      const wantW = M_VIEWPORT.width - M_INNER_X * 2;
      if (!near(got.inner.w, wantW)) fail("inner.w", `内容列宽 ${got.inner.w}，画板 ${wantW}`);
    }

    /*
     * 裁切与残留表格这两条**对每条移动路由都查**，不挂在 minBlocks 下面。
     * 第一版放在 `if (want.minBlocks)` 里，于是只有配了 minBlocks 的路由受查 ——
     * 评审指出我「全站生效」的说法过宽，说得对；这里把它挪出来，让说法成真。
     */
    if (got.clipped.length > 0) {
      fail("clipped", `375 下卡内有被裁掉的横向溢出：${got.clipped.join(" / ")}`);
    }
    if (got.tables.length > 0) {
      fail("tables", `375 下还有表格没降级成行卡（§9）：${got.tables.join(" / ")}`);
    }

    if (want.minBlocks) {
      const wide = got.blocks.filter((w) => near(w, M_VIEWPORT.width - M_INNER_X * 2));
      if (wide.length < want.minBlocks)
        fail(
          "blocks",
          `内容列只有 ${wide.length} 个 343 宽的块，画板至少 ${want.minBlocks} 个；实测宽度 ${got.blocks.join(" / ")}`,
        );
    }
    for (const anchor of want.anchors ?? []) {
      if (!got.text.includes(anchor))
        fail(`anchors(${anchor})`, `画板这一块上有「${anchor}」，页面上找不到`);
    }
    if (want.rowCards && got.rowCards < want.rowCards)
      fail("rowCards", `只有 ${got.rowCards} 张行卡，§9 要求表格降级成行卡`);
    /*
     * 筛选区的高度。画板在手机上只给一排 chip（30）或一条分段器（31），
     * 落地是把桌面工具栏整条搬了过来，控件换行堆高 —— 812 的屏上第一张卡被推得很低。
     */
    if (want.filterRowH && got.toolbar && !near(got.toolbar.h, want.filterRowH, 14))
      fail("filterRow", `筛选区高 ${got.toolbar.h}，画板 ${want.filterRowH}`);
    if (want.rowSpark && got.rowCardsWithSpark === 0)
      fail("rowCards.spark", "画板 30 的行卡里有 8 根柱的走势图，行卡里一个都没有");
    /*
     * 行卡的形状按画板 30 / 31 量。不挂在任何 want.* 开关下 —— 有行卡就查，
     * 因为这些数（343 / 14 / 8 / 圆点 / 灰画布）在两块板上完全一致，是全站行卡的规格。
     */
    if (got.rowCard) {
      const rc = got.rowCard;
      /* 宽度走默认容差：模拟视口的竖向滚动条占掉 10 左右，和 inner.w 那条同一个原因 */
      if (!near(rc.w, M_VIEWPORT.width - M_INNER_X * 2))
        fail("rowCard.w", `行卡 ${rc.w} 宽，画板 30 / 31 都是 343（外壳 16 槽之内的整宽）`);
      if (rc.padX !== 14) fail("rowCard.padX", `行卡横向内边距 ${rc.padX}，画板是 14`);
      if (!near(rc.contentX, M_INNER_X + 14, 2))
        fail("rowCard.contentX", `卡内容起点在屏幕 x=${rc.contentX}，画板 30 / 31 都是 30`);
      if (rc.radius !== 8) fail("rowCard.radius", `行卡圆角 ${rc.radius}，画板是 8`);
      if (rc.gaps.some((g) => g !== 8))
        fail("rowCard.gap", `卡间距 ${rc.gaps.join(" / ")}，画板是 8`);
      if (rc.onSurface)
        fail(
          "rowCard.canvas",
          "行卡摆在白色表格面上：画板 30 / 31 是白卡压 #F2F4F7 画布，压在白面上只剩一根边框",
        );
      if (rc.statusPills > 0 && rc.statusDots !== rc.statusPills)
        fail(
          "rowCard.statusDot",
          `${rc.statusPills} 张卡有状态胶囊，只有 ${rc.statusDots} 张带 5px 圆点（画板 30 / 31 的 st 都带 dot）`,
        );
      for (const t of rc.touch) {
        if (t.w < 44 || t.h < 44)
          fail(
            `rowCard.touch(${t.label})`,
            `卡内按钮命中区 ${t.w}×${t.h}，§9 的下限是 44×44。` +
              "卡内**不要**用负 margin 或绝对定位的 ::before 去撑命中区 —— 两种都会让祖先真的横向溢出，" +
              "裁切那条会抓（card-why 20<32 就是这么来的）。老老实实占满 44 宽，再用纵向负 margin 压高度。",
          );
      }
    }
    if (got.scrollers.length > 0)
      fail("scroller", `还有横向滚动的表格：${got.scrollers.join(" / ")}（§9 不允许）`);

    if (want.kpiCols) {
      if (got.kpiCols === null) fail("kpi", "找不到 .pt-kpi");
      else if (got.kpiCols !== want.kpiCols)
        fail("kpi.cols", `KPI 是 ${got.kpiCols} 列，§9 要求 ${want.kpiCols} 列（2×2）`);
      if (got.kpiNoBars.length > 0)
        fail("kpi.bars", `降级成 2×2 之后这些格丢了柱图：${got.kpiNoBars.join("、")}`);
    }
  }
}

// ------------------------------------------------- 空态（画板 45）

/*
 * 画板 45 列了全站 13 处空态文案。落地把 el-empty 全换成了 PtDataState（六态组件），
 * 但**空库下每个列表页画出了什么，一直没有人量**：正常数据那一轮永远走不到空态分支，
 * 而空态恰恰是「什么都没有的时候页面还说不说人话」这件事。
 *
 * 这一段装第二层假数据（列表接口回空集合），再走几条列表页，要求：
 *   · 真的出现了空态块（.pt-state），标题与说明都不是空字符串；
 *   · 没有控制台报错（空数组最容易把 `arr[0].x` 这类写法打崩）；
 *   · 没有横向溢出。
 */
const EMPTY_ROUTES = [
  "/tasks",
  "/paused",
  "/sites",
  "/filter-rules",
  "/chatops/notifications",
  "/chatops/bindings",
  "/chatops/audit",
  "/chatops/rss-notifications",
  "/logs",
];

const emptyRoutes = EMPTY_ROUTES.filter((r) => wanted.length === 0 || wanted.includes(r));

if (emptyRoutes.length > 0) {
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { ...D_VIEWPORT, deviceScaleFactor: 1, mobile: false },
    sessionId,
  );
  await cdp.send("Page.addScriptToEvaluateOnNewDocument", { source: emptyStubScript() }, sessionId);

  const MEASURE_EMPTY = `(() => {
    const states = [...document.querySelectorAll('.pt-state')]
      .filter((el) => getComputedStyle(el).display !== 'none')
      .map((el) => ({
        title: (el.querySelector('.pt-state__title')?.textContent ?? '').trim(),
        sub: (el.querySelector('.pt-state__sub')?.textContent ?? '').trim(),
      }));
    const inner = document.querySelector('.pt-shell__inner');
    return {
      states,
      overflow: inner ? inner.scrollWidth - Math.round(inner.getBoundingClientRect().width) : 0,
      unknown: [...new Set([...document.querySelectorAll('*')]
        .filter((el) => el instanceof HTMLUnknownElement)
        .map((el) => el.tagName.toLowerCase()))],
    };
  })()`;

  for (const route of emptyRoutes) {
    consoleErrors = [];
    /* reload 一次，保证空数据桩这一层真的装上了（同 goto 里的理由） */
    await goto(`${BASE}/#${route}`);
    if ((await ev(`window.__ptEmptyStub === true`).catch(() => false)) !== true) {
      await cdp.send("Page.reload", {}, sessionId);
      await sleep(2000);
    }
    await sleep(2200);
    const got = await ev(MEASURE_EMPTY);
    const fail = (key, msg) => {
      const gapKey = `${route}@empty ${key}`;
      if (ALLOWED_GAPS[gapKey]) {
        gapsUsed.add(gapKey);
        return;
      }
      failures.push({ route: `${route}@empty`, board: "45 空态", key, msg });
    };

    for (const err of [...new Set(consoleErrors)].slice(0, 3)) fail("console", err);
    if (got.overflow > 0) fail("overflow", `主区横向溢出 ${got.overflow}px`);
    if (got.unknown.length > 0) fail("unknown", `未解析组件 ${got.unknown.join(", ")}`);
    if (got.states.length === 0) {
      fail("empty", "空数据下没有出现任何空态块（.pt-state）—— 页面只是一片空白");
    } else {
      const mute = got.states.filter((st) => !st.title);
      if (mute.length > 0) fail("empty.title", `有 ${mute.length} 个空态块没有标题文案`);
      /*
       * 说明文案也要有。画板 45 列的 13 处空态原文里每一处都有下一步动作
       * （「添加第一个站点开始」这类）；只断言标题等于放过「有标题没说明」的半成品。
       */
      const noSub = got.states.filter((st) => st.title && !st.sub);
      if (noSub.length > 0)
        fail(
          "empty.sub",
          `这些空态块只有标题、没有说明：${noSub.map((st) => st.title).join("、")}`,
        );
    }
  }
}

/*
 * 裁切检测器的自检 —— 放在所有路由之后、登录页之前（这时还是登录态，页面随便用）。
 * 它不依赖任何一条路由，只需要一个能跑 JS 的文档。红了就说明判据被收窄过。
 */
{
  const r = await ev(CLIP_SELFTEST).catch((e) => ({
    ok: false,
    missed: [`自检没跑起来：${e.message}`],
  }));
  if (!r?.ok) {
    failures.push({
      route: "(自检)",
      board: "裁切检测器",
      key: "clip.selftest",
      msg:
        `合成样本没抓到：${(r?.missed ?? []).join("、") || "无"}；` +
        `误报（该放过却报了）：${(r?.wrong ?? []).join("、") || "无"}。` +
        `实测：${r?.all ?? "-"}`,
    });
  } else {
    console.log(
      "· 裁切检测器自检通过：单子元素 / 多兄弟合计 / 文本混排 / 纯文本 四类都抓到，省略号与可滚区域都放过",
    );
  }
}

// ------------------------------------------------- 按条目搜索（画板 06、09、10，媒体中心 M2a）

/*
 * 按条目搜索不是带式表格页：64 高的搜索带代替页头，左栏 256，结果区是 888 宽的分组卡片。
 * 画板 06 按 1440×1024 画（侧栏 264 + 主区 1176），所以这一趟按 1440 量，不按上面的 1376；
 * 视口 < 1280 时左栏收进抽屉，另量一次 1200；手机按画板 09、10 量 375×812。
 * 数字来自 docs/design/media-hub-board-spec.md 的 B06 / B09；假数据是 board-fixtures.mjs 的 ENTRY_SEARCH
 * （五组：两个站点的同一资源、整组已入库、锁定的版本、正在下载、不符合档案；第六组拉黑了，默认不显示，M2b）。
 * 条目从地址带进来（探索页的「搜资源」就是这么跳的），不经候选下拉。
 */
const ENTRY_ROUTE = "/search?media_type=tv&tmdb_id=231620";
const doEntry =
  wanted.length === 0 || wanted.includes("/search") || wanted.includes("/search?entry");

const MEASURE_ES = `(() => {
  const box = (el) => {
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const q = (sel, root = document) => root.querySelector(sel);
  const txt = (el) => (el?.textContent ?? '').replace(/\\s+/g, ' ').trim();
  const band = q('[data-testid="es-head"]');
  const rail = q('.es-body__rail');
  const btn = (root, label) => [...(root?.querySelectorAll('button') ?? [])].find((b) => txt(b) === label);
  return {
    band: box(band),
    query: box(q('.es-q .el-input__wrapper', band ?? document)),
    token: box(q('[data-testid="es-token"]')),
    tokenText: txt(q('[data-testid="es-token"]')),
    kbd: box(q('.pt-kbd', band ?? document)),
    search: box(btn(band, '搜索')),
    history: box(q('button[aria-label="最近搜索"]', band ?? document)),
    merge: box(q('.es-merge', band ?? document)),
    controls: [...(band?.querySelectorAll('button, label, input[placeholder]') ?? [])]
      .flatMap((el) => [txt(el), el.getAttribute('aria-label') ?? '', el.getAttribute('placeholder') ?? ''])
      .filter(Boolean),
    facetRow: Boolean(q('.pt-band__row--facet', band ?? document)),
    rail: box(rail),
    railPos: rail ? getComputedStyle(rail).position : '',
    railTop: rail ? getComputedStyle(rail).top : '',
    poster: box(q('.es-rail__item > *', rail ?? document)),
    railTitle: txt(q('.es-rail__title', rail ?? document)),
    railLines: [...(rail?.querySelectorAll('.es-rail__line') ?? [])].map(txt),
    seasons: [...(rail?.querySelectorAll('.es-rail__season') ?? [])].map((el) => ({ t: txt(el), on: el.classList.contains('is-on'), ...box(el) })),
    eps: [...(rail?.querySelectorAll('.es-eps__cell') ?? [])].map((el) => ({ t: txt(el), cls: el.className, ...box(el) })),
    note: txt(q('[data-testid="es-ep-note"]', rail ?? document)),
    profile: box(q('[data-testid="es-profile"]', rail ?? document)),
    /* 选中项的文字在 .el-select__placeholder 上；第一个 .el-select__selected-item 是空的输入框外壳 */
    profileText: txt(q('[data-testid="es-profile"] .el-select__placeholder', rail ?? document)),
    facetGroups: [...(rail?.querySelectorAll('.es-facets__label') ?? [])].map(txt),
    moreFacets: txt(q('[data-testid="es-more-facets"]', rail ?? document)),
    summary: box(q('[data-testid="es-summary"]')),
    summaryText: txt(q('[data-testid="es-summary"]')),
    siteErrors: txt(q('[data-testid="es-site-errors"]')),
    sorts: [...document.querySelectorAll('[data-testid^="es-sort-"]')].map((el) => ({ t: txt(el), on: el.classList.contains('is-on'), ...box(el) })),
    cards: [...document.querySelectorAll('[data-testid^="es-card-"]')].map((el) => ({
      key: el.getAttribute('data-testid'),
      ...box(el),
      members: el.querySelectorAll('[data-testid="es-member"]').length,
      score: box(q('[data-testid="es-score"]', el)),
      scoreText: txt(q('[data-testid="es-score"]', el)),
      pill: txt(q('.es-card__pill', el)),
      lock: txt(q('.es-card__lock', el)),
      more: txt(q('[data-testid="es-more"]', el)),
      /* 脚注右边的「等免费」「不要」（M2b）：位置、图标与字 */
      acts: [...el.querySelectorAll('.es-card__acts .es-act')].map((b) => ({
        t: txt(b),
        disabled: b.disabled,
        icon: box(b.querySelector('svg')),
        color: getComputedStyle(b).color,
        ...box(b),
      })),
      text: txt(el).slice(0, 400),
    })),
    unmatched: txt(q('[data-testid="es-unmatched-line"]')),
    narrow: Boolean(q('.es-narrow')),
    overflow: document.documentElement.scrollWidth - window.innerWidth,
  };
})()`;

/* 没加载完就量：先等概况那一行出来（第一次搜索要等假数据回来） */
async function waitEntry() {
  for (let i = 0; i < 40; i++) {
    const ok = await ev(`Boolean(document.querySelector('[data-testid="es-summary"]'))`).catch(
      () => false,
    );
    if (ok === true) return true;
    await sleep(250);
  }
  return false;
}

/* 打开一个视口下的按条目搜索：先换视口再整页重载，页面按新视口挂载（手机、窄屏的判断都在挂载时读） */
async function openEntry(metrics) {
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { deviceScaleFactor: 1, ...metrics },
    sessionId,
  );
  await goto(`${BASE}/#${ENTRY_ROUTE}`);
  await cdp.send("Page.reload", {}, sessionId);
  await waitReady();
  const ok = await waitEntry();
  /* 卡片的展开、筛选都是过渡后的几何：等动画走完再量 */
  await sleep(500);
  return ok;
}

if (doEntry) {
  const entryFail = (key, msg, mobile = false) => {
    const gapKey = `/search?entry${mobile ? "@375" : ""} ${key}`;
    if (ALLOWED_GAPS[gapKey]) {
      gapsUsed.add(gapKey);
      return;
    }
    failures.push({
      route: `/search（按条目${mobile ? "，375" : ""}）`,
      board: mobile ? "09/10 按条目搜索·手机" : "06 按条目搜索",
      key,
      msg,
    });
  };
  const expectBox = (key, got, want, tol = 4, mobile = false) => {
    if (!got) {
      entryFail(key, "找不到这一块", mobile);
      return;
    }
    const diffs = Object.entries(want)
      .filter(([k, v]) => !near(got[k], v, tol))
      .map(([k, v]) => `${k} ${got[k]}≠${v}`);
    if (diffs.length > 0)
      entryFail(key, `${diffs.join("，")}（实测 ${JSON.stringify(got)}）`, mobile);
  };

  /* ---- 桌面 1440×1024（画板 06） ---- */
  consoleErrors = [];
  if (!(await openEntry({ width: 1440, height: 1024, mobile: false }))) {
    entryFail(
      "load",
      "等了 10 秒还没有结果概况：页面没进按条目搜索，或 POST /api/media/search 没回来",
    );
  } else {
    const m = await ev(MEASURE_ES);
    expectBox("band", m.band, { x: 264, y: 0, w: 1176, h: 64 });
    expectBox("query", m.query, { x: 280, y: 13, w: 640, h: 38 });
    expectBox("token", m.token, { x: 316, h: 26 }, 6);
    if (m.tokenText !== "长夜将尽 · 2025 · 剧集")
      entryFail("token.text", `条目标记写的是「${m.tokenText}」`);
    /* ⌘K 在输入框里 9 下、20 高（画板 06 kbd 30×20）；行高没写死时它跟着输入框的行高变成 40 高、顶出框外 */
    expectBox("kbd", m.kbd, { y: 22, h: 20 }, 2);
    expectBox("search", m.search, { x: 928, y: 13, w: 74, h: 38 }, 6);
    expectBox("history", m.history, { x: 1010, y: 13, w: 38, h: 38 }, 6);
    if (!m.merge) entryFail("merge", "搜索带上没有「合并同一资源」开关");
    else if (!near(m.merge.x + m.merge.w, 1424, 6))
      entryFail("merge", `开关的右边在 ${m.merge.x + m.merge.w}，画板 1424`);
    for (const label of [
      "搜索",
      "最近搜索",
      "合并同一资源",
      "再输入关键字可以缩小范围",
      "去掉条目，回到关键字搜索",
    ]) {
      if (!m.controls.some((t) => t.includes(label)))
        entryFail(`controls(${label})`, `搜索带里找不到「${label}」`);
    }
    /* 按条目搜时搜索带只有一排：保存搜索与站点那一排属于关键字模式 */
    if (m.controls.some((t) => t.includes("保存搜索")))
      entryFail("controls(保存搜索)", "按条目搜时不该有「保存搜索」");
    if (m.facetRow) entryFail("facetRow", "按条目搜时不该有站点那一排");

    expectBox("rail", m.rail, { x: 264, y: 64, w: 256, h: 932 });
    if (m.railPos !== "sticky" || m.railTop !== "64px")
      entryFail("rail.sticky", `左栏 position ${m.railPos}、top ${m.railTop}，画板 sticky 64px`);
    expectBox("rail.poster", m.poster, { x: 280, y: 80, w: 56, h: 84 });
    if (m.railTitle !== "长夜将尽") entryFail("rail.title", `条目名「${m.railTitle}」`);
    const lines = m.railLines.join(" | ");
    if (lines !== "2025 · 剧集 · 追剧中 | 版本锁 HHWEB 2160p")
      entryFail("rail.lines", `两行说明是「${lines}」`);
    const seasonText = m.seasons.map((s) => s.t + (s.on ? "*" : "")).join(" ");
    if (seasonText !== "第 1 季 第 2 季*") entryFail("rail.seasons", `季是「${seasonText}」`);
    if (m.seasons.some((s) => !near(s.h, 26, 1)))
      entryFail("rail.seasons", `季的高度 ${m.seasons.map((s) => s.h).join("/")}，画板 26`);
    if (m.eps.length !== 8) entryFail("rail.eps", `每集格子 ${m.eps.length} 个，这一季 8 集`);
    else {
      const sizes = [...new Set(m.eps.map((e) => `${e.w}×${e.h}`))];
      if (sizes.join() !== "34×24")
        entryFail("rail.eps", `格子尺寸 ${sizes.join("、")}，画板 34×24`);
      /* 每行 6 个，列距 38、行距 30（格子 34 加 4、24 加 6） */
      const e0 = m.eps[0];
      const e1 = m.eps[1];
      const e6 = m.eps[6];
      if (
        !near(e0.x, 280, 2) ||
        !near(e1.x - e0.x, 38, 1) ||
        !near(e6.x, e0.x, 1) ||
        !near(e6.y - e0.y, 30, 1)
      ) {
        entryFail(
          "rail.eps",
          `格子位置 ${JSON.stringify([e0, e1, e6].map(({ x, y }) => [x, y]))}，画板首格 280、列距 38、每行 6 个、行距 30`,
        );
      }
      const states = m.eps.map(
        (e) => (e.cls.match(/is-(library|downloading|waiting_version|missing|upcoming)/) ?? [])[1],
      );
      const wantStates = [
        "library",
        "library",
        "library",
        "library",
        "library",
        "library",
        "downloading",
        "waiting_version",
      ];
      if (states.join() !== wantStates.join())
        entryFail("rail.eps", `格子的状态 ${states.join("/")}`);
    }
    if (m.note !== "缺 E08（等版本）· E07 下载中") entryFail("rail.note", `一句话是「${m.note}」`);
    expectBox("rail.profile", m.profile, { x: 280, w: 224, h: 30 }, 4);
    if (m.profileText !== "档案：默认（2160p 优先）")
      entryFail("rail.profile", `档案下拉写的是「${m.profileText}」`);
    /* 假数据里没有流媒体平台，平台那一组没有选项、不出现（筛选组只列有选项的） */
    const groups = m.facetGroups.join("、");
    if (groups !== "分辨率、来源、HDR、制作组、中字与优惠")
      entryFail("rail.facets", `筛选组是「${groups}」`);
    if (m.moreFacets !== "更多筛选：编码、季集、站点")
      entryFail("rail.more", `「更多筛选」写的是「${m.moreFacets}」`);

    expectBox("summary", m.summary, { x: 536, y: 80 }, 4);
    if (m.summaryText !== "6 条结果 · 合并成 5 组 · 符合档案 4")
      entryFail("summary", `概况是「${m.summaryText}」`);
    if (m.siteErrors !== "1 个站点没回应")
      entryFail("siteErrors", `站点错误写的是「${m.siteErrors}」`);
    const sortText = m.sorts.map((s) => s.t + (s.on ? "*" : "")).join(" ");
    if (sortText !== "档案分* 做种 体积 发布时间") entryFail("sorts", `排序是「${sortText}」`);
    else {
      const last = m.sorts[m.sorts.length - 1];
      if (!near(last.x + last.w, 1424, 4) || m.sorts.some((s) => !near(s.h, 24, 1))) {
        entryFail(
          "sorts",
          `排序的右边 ${last.x + last.w}、高 ${m.sorts.map((s) => s.h).join("/")}，画板右对齐到 1424、高 24`,
        );
      }
    }

    /* 卡片：888 宽从 536 起；默认只显示首选一行 = 150 高，整组已入库折叠 = 108 高；卡距 12，第一张 y 112 */
    const wantCards = [
      ["es-card-h:9f2c0d8e41b7a3c5", 150, 1],
      ["es-card-h:04aa71c3e9d25b80", 108, 0],
      ["es-card-h:c71e5a90b2d4f618", 150, 1],
      ["es-card-s:pthome:77310", 150, 1],
      ["es-card-s:pttime:351098", 150, 1],
    ];
    if (m.cards.map((c) => c.key).join() !== wantCards.map(([k]) => k).join()) {
      entryFail("cards", `卡片是 ${m.cards.map((c) => c.key).join("、")}`);
    } else {
      let y = 112;
      wantCards.forEach(([key, h, members], i) => {
        const c = m.cards[i];
        expectBox(`card[${i}]`, c, { x: 536, y, w: 888, h }, 2);
        if (c.members !== members)
          entryFail(`card[${i}].members`, `${key} 显示了 ${c.members} 行成员，应为 ${members}`);
        y += h + 12;
      });
      const [c0, c1, , c3, c4] = m.cards;
      expectBox("card[0].score", c0.score, { x: 552, y: 128, w: 48, h: 48 }, 2);
      if (c0.scoreText !== "92档案分")
        entryFail("card[0].score", `档案分写的是「${c0.scoreText}」`);
      if (c0.pill !== "能补上 E08") entryFail("card[0].pill", `覆盖结果是「${c0.pill}」`);
      if (c0.lock !== "不是锁定的版本（HHWEB）")
        entryFail("card[0].lock", `版本提醒是「${c0.lock}」`);
      if (!c0.more.startsWith("另有 1 个站点：CHDBits"))
        entryFail("card[0].more", `脚注是「${c0.more}」`);
      for (const want of ["OurBits", "首选", "做种 41", "免费 18 时", "无 H&R", "推送"]) {
        if (!c0.text.includes(want)) entryFail(`card[0](${want})`, "首选那一行没有这一项");
      }
      if (c1.pill !== "库里都有了" || !c1.text.includes("这一组的集都已入库，默认折叠")) {
        entryFail(
          "card[1]",
          `整组已入库的卡：覆盖结果「${c1.pill}」，正文「${c1.text.slice(0, 80)}」`,
        );
      }
      if (c3.pill !== "E07 正在下载（HDSky）")
        entryFail("card[3].pill", `覆盖结果是「${c3.pill}」`);
      if (
        c4.pill !== "不符合：分辨率 1080p 不在档案里" ||
        c4.scoreText !== "—不符合" ||
        !c4.text.includes("仍然推送")
      ) {
        entryFail("card[4]", `不符合档案的卡：覆盖结果「${c4.pill}」，档案分「${c4.scoreText}」`);
      }
      /*
       * 脚注右边（画板 06，M2b）：「等免费」「不要」，图标 13 t3 加 12 t3 字，右对齐到卡内 872，「不要」在最右，两项间 12；
       * 脚注在 y = 卡高 − 30、16 高。整组已入库折叠的卡没有脚注。
       */
      const acts = c0.acts;
      if (acts.map((a) => a.t).join("|") !== "等免费|不要") {
        entryFail("card[0].acts", `脚注右边是「${acts.map((a) => a.t).join("、")}」`);
      } else {
        const [watch, no] = acts;
        const footY = c0.y + c0.h - 30;
        if (
          !near(no.x + no.w, c0.x + 872, 4) ||
          !near(no.x - (watch.x + watch.w), 12, 2) ||
          acts.some((a) => !near(a.y, footY, 3) || !near(a.h, 16, 2)) ||
          acts.some((a) => !a.icon || !near(a.icon.w, 13, 1)) ||
          acts.some((a) => a.disabled)
        ) {
          entryFail(
            "card[0].acts",
            `位置 ${JSON.stringify(acts)}，画板「不要」右边在 ${c0.x + 872}、两项间 12、y ${footY}、高 16、图标 13，都点得了`,
          );
        }
      }
      if (c1.acts.length !== 0) entryFail("card[1].acts", "折叠的卡不该有脚注右边的两项");
    }
    /* 最后一行（画板 06 的 blocked 行）：拉黑的一条写标题与原因，再写没对上的条数 */
    if (
      m.unmatched !==
      "已拉黑 1 条：Chang.Ye.Jiang.Jin.S02E08.2160p.WEB-DL-XXWEB（不要这个种子：音画不同步）· 对不上这个条目的 3 条没显示"
    )
      entryFail("unmatched", `最后一行是「${m.unmatched}」`);
    if (m.overflow > 0) entryFail("overflow", `横向溢出 ${m.overflow}px`);

    /*
     * 探针：点了才看得出的效果。每条都返回 verdict=ok 或 MISMATCH 加实测，
     * 并在故意改坏的构建上跑过一遍确认会红（见 state/pt-tools-media-hub/m2a/qa-notes.md）。
     */
    const ENTRY_PROBES = [
      {
        key: "probe.expand",
        desc: "展开一组：成员两行、卡高 186，第二个站点的按钮是「推送这个」；再点收起回到 150",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const card = () => document.querySelector('[data-testid="es-card-h:9f2c0d8e41b7a3c5"]');
          const more = card()?.querySelector('[data-testid="es-more"]');
          if (!more) return 'no-more';
          more.click();
          await wait(400);
          const h1 = Math.round(card().getBoundingClientRect().height);
          const rows = card().querySelectorAll('[data-testid="es-member"]').length;
          const soft = (card().querySelector('[data-testid="es-push-CHDBits-286014"]')?.textContent ?? '').trim();
          const fold = (card().querySelector('[data-testid="es-more"]')?.textContent ?? '').trim();
          card().querySelector('[data-testid="es-more"]').click();
          await wait(400);
          const h2 = Math.round(card().getBoundingClientRect().height);
          const ok = Math.abs(h1 - 186) <= 2 && rows === 2 && soft === '推送这个' && fold.startsWith('收起') && Math.abs(h2 - 150) <= 2;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 展开高 ' + h1 + ' 行 ' + rows + ' 按钮 ' + soft + ' 脚注 ' + fold + ' 收起高 ' + h2;
        })()`,
      },
      {
        key: "probe.facet",
        desc: "勾「1080p」：只剩不符合档案那一组，同组 2160p 的数不变、制作组 CHDWEB 变 0，概况跟着变",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const n = (k, v) => (document.querySelector('[data-testid="es-facet-' + k + '-' + v + '"] .es-facets__n')?.textContent ?? '').trim();
          const opt = document.querySelector('[data-testid="es-facet-resolution-1080p"]');
          if (!opt) return 'no-facet';
          const before = [n('resolution', '2160p'), n('group', 'CHDWEB')].join('/');
          opt.click();
          await wait(400);
          const cards = [...document.querySelectorAll('[data-testid^="es-card-"]')].map((el) => el.getAttribute('data-testid'));
          const after = [n('resolution', '2160p'), n('group', 'CHDWEB')].join('/');
          const sum = (document.querySelector('[data-testid="es-summary"]')?.textContent ?? '').trim();
          opt.click();
          await wait(400);
          const back = document.querySelectorAll('[data-testid^="es-card-"]').length;
          const ok = before === '5/2' && after === '5/0' && cards.join() === 'es-card-s:pttime:351098' &&
            sum === '1 条结果 · 合并成 1 组 · 符合档案 0' && back === 5;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 计数 ' + before + '→' + after + ' 卡 ' + cards.join(',') + ' 概况 ' + sum + ' 取消后 ' + back;
        })()`,
      },
      {
        key: "probe.unfold",
        desc: "点折叠的卡：展开成一行成员、150 高",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const card = () => document.querySelector('[data-testid="es-card-h:04aa71c3e9d25b80"]');
          if (!card()) return 'no-card';
          card().click();
          await wait(400);
          const h = Math.round(card().getBoundingClientRect().height);
          const rows = card().querySelectorAll('[data-testid="es-member"]').length;
          return 'verdict=' + (Math.abs(h - 150) <= 2 && rows === 1 ? 'ok' : 'MISMATCH') + ' 高 ' + h + ' 行 ' + rows;
        })()`,
      },
      {
        key: "probe.episode",
        desc: "点 E07 的格子：只剩包含 E07 的那一组，概况旁出现「只看 E07」；再点取消",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const cell = document.querySelector('[data-testid="es-ep-7"]');
          if (!cell) return 'no-cell';
          cell.click();
          await wait(400);
          const cards = [...document.querySelectorAll('[data-testid^="es-card-"]')].map((el) => el.getAttribute('data-testid'));
          const chip = (document.querySelector('.es-sum__chip')?.textContent ?? '').trim();
          cell.click();
          await wait(400);
          const back = document.querySelectorAll('[data-testid^="es-card-"]').length;
          const ok = cards.join() === 'es-card-s:pthome:77310' && chip === '只看 E07' && back === 5;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 卡 ' + cards.join(',') + ' 提示 ' + chip + ' 取消后 ' + back;
        })()`,
      },
      {
        key: "probe.blocked",
        desc: "点「已拉黑 1 条」：拉黑的那张卡排在最后、灰显、成员行写已拉黑与「移出黑名单」，没有「不要」；再点收起",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const link = document.querySelector('[data-testid="es-show-blocked"]');
          if (!link) return 'no-link';
          link.click();
          await wait(400);
          const cards = [...document.querySelectorAll('[data-testid^="es-card-"]')];
          const last = cards[cards.length - 1];
          const key = last?.getAttribute('data-testid') ?? '';
          const text = (last?.textContent ?? '').replace(/\s+/g, ' ');
          const unblock = Boolean(last?.querySelector('[data-testid="es-unblock-HDFans-66120"]'));
          const noBlock = !last?.querySelector('[data-testid="es-act-block"]');
          const dim = last?.classList.contains('is-blocked') ?? false;
          const line = (document.querySelector('[data-testid="es-unmatched-line"]')?.textContent ?? '').replace(/\s+/g, ' ').trim();
          link.click();
          await wait(400);
          const back = document.querySelectorAll('[data-testid^="es-card-"]').length;
          const ok = cards.length === 6 && key === 'es-card-s:hdfans:66120' && text.includes('已拉黑') &&
            unblock && noBlock && dim && line.includes('灰显在上面') && back === 5;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 卡 ' + cards.length + ' 最后 ' + key + ' 已拉黑 ' + text.includes('已拉黑') +
            ' 移出 ' + unblock + ' 没有不要 ' + noBlock + ' 灰显 ' + dim + ' 最后一行 ' + line + ' 收起后 ' + back;
        })()`,
      },
      {
        key: "probe.merge",
        desc: "关掉「合并同一资源」：一个成员一张卡，概况不写组数；再打开回到 5 组",
        js: `(async () => {
          const wait = (ms) => new Promise((r) => setTimeout(r, ms));
          const sw = document.querySelector('[data-testid="es-merge"]');
          if (!sw) return 'no-switch';
          sw.click();
          await wait(400);
          const n = document.querySelectorAll('[data-testid^="es-card-"]').length;
          const sum = (document.querySelector('[data-testid="es-summary"]')?.textContent ?? '').trim();
          sw.click();
          await wait(400);
          const back = document.querySelectorAll('[data-testid^="es-card-"]').length;
          const ok = n === 6 && sum === '6 条结果 · 符合档案 5' && back === 5;
          return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 卡 ' + n + ' 概况 ' + sum + ' 打开后 ' + back;
        })()`,
      },
    ];
    for (const p of ENTRY_PROBES) {
      const r = String(await ev(p.js).catch((e) => `probe 失败：${e.message}`));
      if (probeMiss(r, "verdict=ok").length > 0) entryFail(p.key, `${p.desc}：${r}`);
    }
    for (const err of [...new Set(consoleErrors)].slice(0, 3)) entryFail("console", err);
  }

  /* ---- 桌面窄屏 1200×1024：左栏收成一排，筛选在右侧 296 宽的抽屉里 ---- */
  consoleErrors = [];
  if (await openEntry({ width: 1200, height: 1024, mobile: false })) {
    const r = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        if (document.querySelector('.es-body__rail')) return 'MISMATCH 窄屏还有常驻左栏';
        const row = document.querySelector('.es-narrow');
        const btn = document.querySelector('[data-testid="es-drawer-btn"]');
        if (!row || !btn) return 'no-narrow';
        btn.click();
        await wait(600);
        const drawer = [...document.querySelectorAll('.es-drawer')].find((el) => el.getBoundingClientRect().width > 0);
        const w = drawer ? Math.round(drawer.getBoundingClientRect().width) : 0;
        const rail = drawer?.querySelector('.es-rail');
        const ok = Boolean(rail) && Math.abs(w - 296) <= 2 && (row.textContent ?? '').includes('长夜将尽 第 2 季');
        return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 抽屉宽 ' + w + ' 左栏在抽屉里 ' + Boolean(rail) + ' 一排 ' + (row.textContent ?? '').replace(/\\s+/g, ' ').trim().slice(0, 40);
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(r, "verdict=ok").length > 0) entryFail("narrow", `视口 1200：${r}`);
    for (const err of [...new Set(consoleErrors)].slice(0, 3)) entryFail("console(1200)", err);
  } else {
    entryFail("narrow", "视口 1200 下没进按条目搜索");
  }

  /* ---- 手机 375×812（画板 09、10） ---- */
  consoleErrors = [];
  if (!(await openEntry({ ...M_VIEWPORT, mobile: true }))) {
    entryFail("load", "等了 10 秒还没有结果概况", true);
  } else {
    const mm = await ev(`(() => {
      const box = (el) => {
        if (!el) return null;
        const r = el.getBoundingClientRect();
        return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
      };
      const txt = (el) => (el?.textContent ?? '').replace(/\\s+/g, ' ').trim();
      const ctl = document.querySelector('.es-mctl');
      const scroller = document.querySelector('.pt-shell__content');
      return {
        /* 模拟视口里的竖向滚动条占 10 宽（真机是浮在上面的），内容列跟着窄这么多 */
        sb: scroller ? scroller.offsetWidth - scroller.clientWidth : 0,
        sub: txt(document.querySelector('#pt-mhead-sub')),
        query: box(document.querySelector('.es-q .el-input__wrapper')),
        token: txt(document.querySelector('[data-testid="es-token"]')),
        bandButtons: [...(document.querySelector('[data-testid="es-head"]')?.querySelectorAll('button') ?? [])].map(txt).filter(Boolean),
        season: box(ctl?.querySelector('.es-mctl__season')),
        profile: box(ctl?.querySelector('.es-mctl__profile')),
        filter: box(ctl?.querySelector('[data-testid="es-mfilter"]')),
        filterText: txt(ctl?.querySelector('[data-testid="es-mfilter"]')),
        item: box(document.querySelector('.es-mitem')),
        itemTitle: txt(document.querySelector('.es-mitem__title')),
        eps: [...document.querySelectorAll('.es-mitem .es-eps__cell')].map(box),
        summary: box(document.querySelector('[data-testid="es-summary"]')),
        summaryText: txt(document.querySelector('[data-testid="es-summary"]')),
        sum: box(document.querySelector('.es-sum')),
        sort: box(document.querySelector('.es-sum__msort')),
        sortText: txt(document.querySelector('.es-sum__msort .el-select__placeholder')),
        /* 选中的字有没有被省略号截掉：textContent 看不出来，要比滚动宽度 */
        sortClipped: (() => {
          const el = document.querySelector('.es-sum__msort .el-select__placeholder span') ?? document.querySelector('.es-sum__msort .el-select__placeholder');
          return el ? el.scrollWidth > el.clientWidth : true;
        })(),
        warn: box(document.querySelector('[data-testid="es-site-errors"]')),
        cards: [...document.querySelectorAll('[data-testid^="es-mcard-"]')].map(box),
        /* 第一张卡底部两半（画板 09，M2b）：每半的盒子、字、图标，中间的竖线 */
        acts: (() => {
          const el = document.querySelector('[data-testid^="es-mcard-"] .es-mcard__acts');
          if (!el) return null;
          return {
            ...box(el),
            halves: [...el.querySelectorAll('.es-mact')].map((b) => ({ t: txt(b), icon: box(b.querySelector('svg')), ...box(b) })),
            split: box(el.querySelector('.es-mcard__split')),
          };
        })(),
        rail: Boolean(document.querySelector('.es-body__rail')),
        overflow: document.documentElement.scrollWidth - window.innerWidth,
      };
    })()`);
    if (mm.sub !== "按条目 · 长夜将尽 第 2 季")
      entryFail("topbar.sub", `顶栏副标题是「${mm.sub}」`, true);
    const cw = 343 - mm.sb;
    expectBox("query", mm.query, { x: 16, y: 64, w: cw, h: 44 }, 2, true);
    /* 手机的条目标记 150 宽，画板 09 只写名字与类型 */
    if (mm.token !== "长夜将尽 · 剧集")
      entryFail("token.text", `条目标记写的是「${mm.token}」`, true);
    if (mm.bandButtons.includes("搜索"))
      entryFail("band", "手机上按条目搜时搜索带里不该有「搜索」按钮（画板 09 只有输入框）", true);
    expectBox("ctl.season", mm.season, { x: 16, y: 116, w: 92, h: 36 }, 2, true);
    /* 档案下拉吃掉内容列剩下的宽度，滚动条让出来的 10 也从它身上扣 */
    expectBox("ctl.profile", mm.profile, { x: 116, y: 116, w: 136 - mm.sb, h: 36 }, 2, true);
    expectBox("ctl.filter", mm.filter, { x: 260 - mm.sb, y: 116, w: 99, h: 36 }, 2, true);
    if (mm.filterText !== "筛选")
      entryFail("ctl.filter", `筛选按钮写的是「${mm.filterText}」`, true);
    expectBox("item", mm.item, { x: 16, y: 164, w: cw, h: 118 }, 2, true);
    if (mm.itemTitle !== "长夜将尽 第 2 季")
      entryFail("item.title", `条目卡标题是「${mm.itemTitle}」`, true);
    const msizes = [...new Set(mm.eps.map((e) => `${e.w}×${e.h}`))];
    if (mm.eps.length !== 8 || msizes.join() !== "38×22")
      entryFail(
        "item.eps",
        `格子 ${mm.eps.length} 个，尺寸 ${msizes.join("、")}，画板 38×22`,
        true,
      );
    expectBox("summary", mm.summary, { x: 16, y: 300 }, 8, true);
    if (mm.summaryText !== "6 条 · 合并成 5 组 · 符合档案 4")
      entryFail("summary", `概况是「${mm.summaryText}」`, true);
    /* 排序：h26 的胶囊，右对齐到内容列右边（画板 359），选中「档案分」时 82 宽（画板 71，照它给字会被省略，见规格的落地偏离） */
    expectBox("sort", mm.sort, { y: 297, w: 82, h: 26 }, 3, true);
    if (mm.sortClipped) entryFail("sort", "排序下拉里选中的字被省略号截掉了", true);
    if (mm.sort && !near(mm.sort.x + mm.sort.w, 16 + cw, 2))
      entryFail("sort", `排序的右边在 ${mm.sort.x + mm.sort.w}，应在 ${16 + cw}`, true);
    if (mm.sortText !== "档案分") entryFail("sort", `排序写的是「${mm.sortText}」`, true);
    /* 「1 个站点没回应」在概况那一行下面单独一行（画板 09 的概况一行没有它） */
    if (!mm.warn || !mm.sort || mm.warn.y < mm.sort.y + mm.sort.h || mm.warn.x !== 16) {
      entryFail("siteErrors", `站点错误没有单独一行：${JSON.stringify(mm.warn)}`, true);
    }
    if (mm.cards.length !== 5) entryFail("cards", `结果卡 ${mm.cards.length} 张，应为 5`, true);
    else {
      /* 第一张卡在概况（这里多一行站点错误）下面 8；画板没有错误那一行，卡从 332 起 */
      expectBox("cards[0]", mm.cards[0], { x: 16, y: mm.sum.y + mm.sum.h + 8, w: cw }, 2, true);
      const gaps = mm.cards.slice(1).map((c, i) => c.y - (mm.cards[i].y + mm.cards[i].h));
      /* 卡高带小数，取整后卡距会在 8、9 之间跳 */
      if (gaps.some((g) => Math.abs(g - 8) > 1) || mm.cards.some((c) => c.x !== 16 || c.w !== cw)) {
        entryFail(
          "cards",
          `卡距 ${gaps.join("/")}、宽 ${mm.cards.map((c) => c.w).join("/")}，画板 8 与 ${cw}`,
          true,
        );
      }
    }
    /* 底部两半：贯穿整张卡（上面 1px 分隔线），每半 171 宽（滚动条让出的宽度两半分）、44 高，图标 14；中间竖线 28 高 */
    const ma = mm.acts;
    if (!ma) entryFail("acts", "第一张结果卡底部没有「等免费」「不要」", true);
    else {
      const half = (cw - 1) / 2;
      const ok =
        ma.halves.map((h) => h.t).join("|") === "等免费|不要" &&
        near(ma.x, 17, 2) &&
        near(ma.w, cw - 2, 2) &&
        ma.halves.every((h) => near(h.w, half - 1, 2) && near(h.h, 44, 1)) &&
        ma.halves.every((h) => h.icon && near(h.icon.w, 14, 1)) &&
        ma.split &&
        near(ma.split.h, 28, 1) &&
        near(ma.split.x, ma.x + ma.halves[0].w, 1);
      if (!ok)
        entryFail(
          "acts",
          `底部两半 ${JSON.stringify(ma)}，画板每半 171 宽、44 高、图标 14、中间竖线 28 高`,
          true,
        );
    }
    if (mm.rail) entryFail("rail", "手机上不该有左栏", true);
    if (mm.overflow > 0) entryFail("overflow", `横向溢出 ${mm.overflow}px`, true);

    const panel = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        const btn = document.querySelector('[data-testid="es-mfilter"]');
        if (!btn) return 'no-filter';
        btn.click();
        await wait(700);
        const sheet = [...document.querySelectorAll('.es-mpanel')].find((el) => el.getBoundingClientRect().height > 0);
        if (!sheet) return 'no-sheet';
        const r = sheet.getBoundingClientRect();
        const pill = [...sheet.querySelectorAll('[data-testid="es-mfacet-resolution"] button')].find((b) => (b.textContent ?? '').includes('1080p'));
        if (!pill) return 'no-pill';
        pill.click();
        await wait(300);
        const apply = sheet.querySelector('[data-testid="es-mpanel-apply"]');
        const applyText = (apply?.textContent ?? '').trim();
        const before = document.querySelectorAll('[data-testid^="es-mcard-"]').length;
        apply.click();
        await wait(700);
        const after = document.querySelectorAll('[data-testid^="es-mcard-"]').length;
        const label = (btn.textContent ?? '').replace(/\\s+/g, ' ').trim();
        const ok = Math.round(r.top) === 120 && Math.round(r.height) === 692 && applyText === '看 1 条结果' && before === 5 && after === 1 && label === '筛选 · 1';
        return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 面板 y ' + Math.round(r.top) + ' 高 ' + Math.round(r.height) +
          ' 按钮 ' + applyText + ' 卡 ' + before + '→' + after + ' 筛选按钮 ' + label;
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(panel, "verdict=ok").length > 0)
      entryFail("probe.panel", `筛选面板：${panel}`, true);
    for (const err of [...new Set(consoleErrors)].slice(0, 3)) entryFail("console", err, true);
  }
  /* 后面的登录页一趟自己设视口；这里恢复桌面口径，免得以后在这一段后面加的检查拿到手机视口 */
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { ...D_VIEWPORT, deviceScaleFactor: 1, mobile: false },
    sessionId,
  );
}

// ------------------------------------------------- 条目详情（画板 04、08，媒体中心 M3）

/*
 * 条目详情不是带式表格页：88 高的页头带面包屑，下面是 1144 宽的大图与几个区块。画板 04 按 1440×1024 画
 * （侧栏 264 + 主区 1176），所以这一趟按 1440 量；手机按画板 08 量 375×812。剧集一部（长夜将尽第 2 季）、电影一部（沙丘2）。
 * 数字来自 docs/design/media-hub-board-spec.md 的 B04 / B08；块与块之间统一 24（偏离 ④）。
 * 媒体服务器（M6）：「你的 NAS 上」第 4 行、「在 Jellyfin 播放」（手机在白卡底部）、集卡的播放圆与 E03「Jellyfin 里没有了」。
 * 依赖后面里程碑的部分不量：删除（M7）；服务器里删了的动作先是「忽略」（画板写「处理」，那是 M7）。
 * 假数据是 board-fixtures.mjs 的 ITEM_TV / ITEM_TV_S2 / ITEM_MOVIE（图片路径都不给，走占位）。
 */
const ITEM_TV_ROUTE = "/media/item/tv/100088?from=subscriptions&season=2";
const ITEM_MOVIE_ROUTE = "/media/item/movie/693134?from=explore";
const doItem = wanted.length === 0 || wanted.includes("/media/item");

const MEASURE_MI = `(() => {
  const box = (el) => {
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const q = (sel, root = document) => root.querySelector(sel);
  const qa = (sel, root = document) => [...root.querySelectorAll(sel)];
  const txt = (el) => (el?.textContent ?? '').replace(/\\s+/g, ' ').trim();
  const hero = q('[data-testid="mi-hero"]');
  const nas = q('[data-testid="mi-nas"]');
  const scroller = document.querySelector('.pt-shell__content');
  return {
    sb: scroller ? scroller.offsetWidth - scroller.clientWidth : 0,
    head: box(q('.pt-head')),
    /* 面包屑的分隔是外壳的 chevron 图标（同其他详情页），按段取字再用斜杠连起来比画板的写法 */
    crumbs: qa('.pt-head__crumbs a, .pt-head__crumbs span').map(txt).join(' / '),
    headTitle: txt(q('.pt-head__title')),
    mheadSub: txt(q('#pt-mhead-sub')),
    hero: box(hero),
    poster: box(q('.mi-hero__poster', hero ?? document)),
    title: box(q('[data-testid="mi-title"]')),
    titleText: txt(q('[data-testid="mi-title"]')),
    titleSize: q('[data-testid="mi-title"]') ? getComputedStyle(q('[data-testid="mi-title"]')).fontSize : '',
    original: txt(q('.mi-hero__original')),
    originalBox: box(q('.mi-hero__original')),
    pills: qa('[data-testid="mi-pills"] .mi-glass').map(txt),
    /* 看得见的胶囊（没被两行的高度截掉的）与几行 */
    pillsShown: (() => {
      const c = q('[data-testid="mi-pills"]');
      if (!c) return { n: 0, rows: 0 };
      const cr = c.getBoundingClientRect();
      const shown = qa('.mi-glass', c).filter((el) => el.getBoundingClientRect().bottom <= cr.bottom + 0.5);
      return { n: shown.length, rows: new Set(shown.map((el) => Math.round(el.getBoundingClientRect().top))).size };
    })(),
    pillsBox: box(q('[data-testid="mi-pills"]')),
    ratingText: txt(q('[data-testid="mi-rating"]')),
    rating: box(q('[data-testid="mi-rating"]')),
    imdb: box(q('[data-testid="mi-imdb"]')),
    tvdb: box(q('[data-testid="mi-tvdb"]')),
    overview: box(q('[data-testid="mi-overview"]')),
    credits: txt(q('.mi-hero__credits')),
    creditsBox: box(q('.mi-hero__credits')),
    acts: qa('[data-testid="mi-acts"] .mi-btn').map((el) => ({ t: txt(el) || el.getAttribute('aria-label'), ...box(el) })),
    more: box(q('.mi-hero__more')),
    nas: box(nas),
    nasGlass: Boolean(nas?.classList.contains('mi-nas--glass')),
    nasRows: qa('[data-testid^="mi-nas-"]').map((el) => ({ key: el.getAttribute('data-testid'), t: txt(el), ...box(el) })),
    play: box(q('[data-testid="mi-play"]')),
    playText: txt(q('[data-testid="mi-play"]')),
    playInNas: Boolean(nas && q('[data-testid="mi-play"]') && nas.contains(q('[data-testid="mi-play"]'))),
    seasons: box(q('[data-testid="mi-seasons"]')),
    seasonTitle: box(q('[data-testid="mi-seasons"] .mi-block__title')),
    tabs: qa('[data-testid^="mi-tab-"]').map((el) => ({ t: qa('span', el).map(txt).join(' '), on: el.classList.contains('is-on'), ...box(el) })),
    legend: box(q('[data-testid="mi-legend"]')),
    legendText: txt(q('[data-testid="mi-legend"]')),
    collapse: box(q('[data-testid="mi-collapse"]')),
    collapseText: txt(q('[data-testid="mi-collapse"]')),
    eps: qa('.mi-ep').map((el) => ({
      id: el.getAttribute('data-testid'),
      ...box(el),
      still: box(q('.mi-ep__still', el)),
      title: txt(q('.mi-ep__title', el)),
      titleBox: box(q('.mi-ep__title', el)),
      air: txt(q('.mi-ep__air', el)),
      state: txt(q('.mi-ep__state', el)),
      act: txt(q('.mi-ep__act', el)),
      play: box(q('.mi-ep__play', el)),
    })),
    torTitle: box(q('[data-testid="mi-torrents"] .mi-block__title')),
    torTitleText: txt(q('[data-testid="mi-torrents"] .mi-block__title')),
    torSub: txt(q('[data-testid="mi-torrents"] .mi-block__sub')),
    torList: box(q('[data-testid="mi-torrents"] .mi-list')),
    torEmpty: txt(q('[data-testid="mi-torrents-empty"]')),
    rows: qa('[data-testid="mi-trow"]').map((el) => ({
      ...box(el),
      t: txt(el),
      cells: [...el.children].map((c) => ({ t: txt(c), ...box(c) })),
    })),
    castTitle: box(q('[data-testid="mi-cast"] .mi-block__title')),
    castAll: txt(q('[data-testid="mi-cast-all"]')),
    castAllBox: box(q('[data-testid="mi-cast-all"]')),
    people: qa('.mi-person').map((el) => ({
      ...box(el),
      face: box(q('.mi-person__face', el)),
      name: txt(q('.mi-person__name', el)),
      nameBox: box(q('.mi-person__name', el)),
      role: txt(q('.mi-person__role', el)),
    })),
    cast: box(q('.mi-cast')),
    recTitle: box(q('[data-testid="mi-recs"] .mi-block__title')),
    recSub: txt(q('[data-testid="mi-recs"] .mi-block__sub')),
    recs: qa('.mi-rec').map((el) => ({
      ...box(el),
      poster: box(q('.mi-rec__poster', el)),
      tag: txt(q('.mi-rec__tag', el)),
      name: txt(q('.mi-rec__name', el)),
    })),
    recBox: box(q('.mi-recs')),
    overflow: document.documentElement.scrollWidth - window.innerWidth,
  };
})()`;

/*
 * 删除弹窗（画板 03 的片段「删除这一季」，M7）：点开以后按弹窗内的坐标量，再点「仍然删除」看摘要跟着变，最后关掉。
 * open 是打开它的脚本（桌面点大图的「删除」，手机点「更多」里的删除）。
 */
const measureDelete = (open) => `(async () => {
  const wait = (ms) => new Promise((r) => setTimeout(r, ms));
  const opened = await (async () => { ${open} })();
  if (opened !== true) return { err: String(opened) };
  let dlg = null;
  for (let i = 0; i < 40; i++) {
    await wait(150);
    dlg = document.querySelector('.md-dialog');
    if (dlg && dlg.querySelectorAll('[data-testid="delete-torrent"]').length === 3) break;
  }
  if (!dlg || dlg.querySelectorAll('[data-testid="delete-torrent"]').length !== 3) return { err: 'no-dialog' };
  await wait(500);
  const r0 = dlg.getBoundingClientRect();
  const rel = (el) => {
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left - r0.left), y: Math.round(r.top - r0.top), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const txt = (el) => (el?.textContent ?? '').replace(/\\s+/g, ' ').trim();
  const q = (sel, root = dlg) => root.querySelector(sel);
  const qa = (sel, root = dlg) => [...root.querySelectorAll(sel)];
  const inner = (el) => el?.querySelector('.el-checkbox__inner, .el-radio__inner') ?? null;
  /* 字本身的框（块元素的框是整行宽，右对齐的字量不出来） */
  const textRel = (el) => {
    if (!el) return null;
    const range = document.createRange();
    range.selectNodeContents(el);
    const r = range.getBoundingClientRect();
    return { x: Math.round(r.left - r0.left), y: Math.round(r.top - r0.top), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const title = q('.el-dialog__title');
  const out = {
    dlg: { x: Math.round(r0.left), y: Math.round(r0.top), w: Math.round(r0.width), h: Math.round(r0.height) },
    fullscreen: dlg.classList.contains('is-fullscreen'),
    header: rel(q('.el-dialog__header')),
    title: rel(title),
    titleText: txt(title),
    titleFont: title ? getComputedStyle(title).fontSize + '/' + getComputedStyle(title).fontWeight : '',
    close: rel(q('.el-dialog__headerbtn .el-dialog__close')),
    filesCheck: rel(inner(q('[data-testid="delete-files-check"]'))),
    filesChecked: Boolean(q('[data-testid="delete-files-check"] input')?.checked),
    filesTitle: rel(q('[data-testid="delete-files"] .md-row__title')),
    filesSub: rel(q('[data-testid="delete-files"] .md-row__sub')),
    filesSubText: txt(q('[data-testid="delete-files"] .md-row__sub')),
    filesMore: rel(q('[data-testid="delete-files-more"]')),
    secs: qa('.md-sec').map(rel),
    torTitle: rel(q('[data-testid="delete-torrents"] .md-sec__title')),
    torTitleText: txt(q('[data-testid="delete-torrents"] .md-sec__title')),
    tors: qa('[data-testid="delete-torrent"]').map((el) => ({
      ...rel(el),
      check: rel(inner(q('[data-testid="delete-torrent-check"]', el))),
      checked: Boolean(q('[data-testid="delete-torrent-check"] input', el)?.checked),
      title: txt(q('.md-row__title', el)),
      titleBox: rel(q('.md-row__title', el)),
      sub: txt(q('.md-row__sub', el)),
      subBox: rel(q('.md-row__sub', el)),
      subText: rel(q('.md-guard span', el)),
      how: rel(q('[data-testid="delete-torrent-how"]', el)),
      howText: txt(q('[data-testid="delete-torrent-how"]', el)),
      force: rel(inner(q('[data-testid="delete-torrent-force"]', el))),
      forceLabel: rel(q('[data-testid="delete-torrent-force"] .el-checkbox__label', el)),
      main: rel(q('.md-tor__main', el)),
      ruled: q('.md-tor__main', el) ? getComputedStyle(q('.md-tor__main', el)).borderBottomWidth : '',
    })),
    subTitle: rel(q('[data-testid="delete-subscription"] .md-sec__title')),
    subs: qa('.md-sub').map((el) => ({
      radio: rel(inner(el)),
      title: txt(q('.md-row__title', el)),
      titleBox: rel(q('.md-row__title', el)),
      sub: txt(q('.md-row__sub', el)),
      on: Boolean(q('.el-radio.is-checked', el)),
    })),
    footer: rel(q('.el-dialog__footer')),
    summary: textRel(q('[data-testid="delete-summary"]')),
    summaryText: txt(q('[data-testid="delete-summary"]')),
    buttons: qa('.el-dialog__footer .el-button').map((b) => ({ t: txt(b), ...rel(b) })),
    runBg: q('[data-testid="delete-run"]') ? getComputedStyle(q('[data-testid="delete-run"]')).backgroundColor : '',
    overflow: Math.round(q('.el-dialog__body').scrollWidth - q('.el-dialog__body').clientWidth),
  };
  q('[data-testid="delete-torrent-force"] input')?.click();
  await wait(250);
  out.forcedSummary = txt(q('[data-testid="delete-summary"]'));
  out.forcedChecked = Boolean(qa('[data-testid="delete-torrent-check"] input')[1]?.checked);
  q('.el-dialog__headerbtn')?.click();
  await wait(600);
  return out;
})()`;

/* 等页面读到条目（剧集还要等这一季的集出来），再等过渡走完 */
async function openItem(metrics, route, tv) {
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { deviceScaleFactor: 1, ...metrics },
    sessionId,
  );
  await goto(`${BASE}/#${route}`);
  await cdp.send("Page.reload", {}, sessionId);
  await waitReady();
  for (let i = 0; i < 40; i++) {
    const ok = await ev(
      tv
        ? `document.querySelectorAll('.mi-ep').length > 0`
        : `Boolean(document.querySelector('[data-testid="mi-title"]'))`,
    ).catch(() => false);
    if (ok === true) {
      await sleep(500);
      return true;
    }
    await sleep(250);
  }
  return false;
}

if (doItem) {
  const itemFail = (key, msg, mobile = false) => {
    const gapKey = `/media/item${mobile ? "@375" : ""} ${key}`;
    if (ALLOWED_GAPS[gapKey]) {
      gapsUsed.add(gapKey);
      return;
    }
    failures.push({
      route: `/media/item${mobile ? "（375）" : ""}`,
      board: mobile ? "08 条目详情·手机" : "04 条目详情",
      key,
      msg,
    });
  };
  const expectBox = (key, got, want, tol = 4, mobile = false) => {
    if (!got) {
      itemFail(key, "找不到这一块", mobile);
      return;
    }
    const diffs = Object.entries(want)
      .filter(([k, v]) => !near(got[k], v, tol))
      .map(([k, v]) => `${k} ${got[k]}≠${v}`);
    if (diffs.length > 0)
      itemFail(key, `${diffs.join("，")}（实测 ${JSON.stringify(got)}）`, mobile);
  };
  const expectText = (key, got, want, mobile = false) => {
    if (got !== want) itemFail(key, `写的是「${got}」，应为「${want}」`, mobile);
  };
  /* 块与块：上一块的底到下一块标题的顶 24（偏离 ④） */
  const expectGap = (key, prevBottom, next, mobile = false) => {
    if (!next) itemFail(key, "找不到下一块的标题", mobile);
    else if (!near(next.y - prevBottom, 24, 2))
      itemFail(
        key,
        `上一块的底 ${prevBottom} 到标题 ${next.y} 是 ${next.y - prevBottom}，统一 24`,
        mobile,
      );
  };

  /* ---- 桌面 1440×1024，剧集（画板 04） ---- */
  consoleErrors = [];
  if (!(await openItem({ width: 1440, height: 1024, mobile: false }, ITEM_TV_ROUTE, true))) {
    itemFail("load", "等了 10 秒还没有集卡：页面没进条目详情，或条目、季的接口没回来");
  } else {
    const m = await ev(MEASURE_MI);
    if (process.env.MI_DUMP) console.log(JSON.stringify(m, null, 1));
    /* 页面比视口长，滚动区有竖向滚动条（无头 Chrome 里 10 宽，画板按没有滚动条画）：内容宽 W 与右边都让出 sb */
    const W = 1144 - m.sb;
    const R = 280 + W;
    expectBox("head", m.head, { x: 264, y: 0, w: 1176, h: 88 }, 2);
    expectText("head.crumbs", m.crumbs, "媒体 / 追剧与订阅 / 长夜将尽");
    expectText("head.title", m.headTitle, "长夜将尽");

    /* 大图 1144×340：海报 28,44 168×252；文字列从 224 起（名字 40、原名 80、胶囊 110、评分 142、简介 178、主创 228、动作 264） */
    expectBox("hero", m.hero, { x: 280, y: 104, w: W, h: 340 }, 2);
    expectBox("hero.poster", m.poster, { x: 308, y: 148, w: 168, h: 252 }, 2);
    expectBox("hero.title", m.title, { x: 504, y: 144 }, 4);
    if (m.titleSize !== "28px") itemFail("hero.title", `名字 ${m.titleSize}，画板 28/700`);
    expectBox("hero.original", m.originalBox, { x: 504, y: 184 }, 4);
    expectText("hero.original", m.original, "The Long Night · 2025 · 2 季 24 集");
    expectBox("hero.pills", m.pillsBox, { x: 504, y: 214, h: 22 }, 4);
    expectText("hero.pills", m.pills.join("|"), "剧集|悬疑|犯罪|剧情|中国大陆|每集 45 分钟|连载中");
    expectBox("hero.rating", m.rating, { x: 504, y: 246 }, 6);
    if (!m.ratingText.includes("8.4") || !m.ratingText.includes("TMDB 1,204 人评分"))
      itemFail("hero.rating", `评分那一行是「${m.ratingText}」`);
    expectBox("hero.imdb", m.imdb, { y: 248, h: 20 }, 4);
    if (!m.tvdb) itemFail("hero.tvdb", "有 TVDB 编号时要有 TVDB 胶囊");
    expectBox("hero.overview", m.overview, { x: 504, y: 282, w: 600 }, 4);
    if (m.overview && !near(m.overview.h, 42, 2))
      itemFail("hero.overview", `简介 ${m.overview.h} 高，画板两行省略 42`);
    expectBox("hero.credits", m.creditsBox, { x: 504, y: 332 }, 4);
    expectText("hero.credits", m.credits, "导演 陈默 · 主演 周屿、林晚晴、沈川、许曼");
    /* 动作：y 264，从 224 起、间距 8，h32：搜资源 96、订阅与「在 Jellyfin 播放」（玻璃按钮）、重新刮削、删除（M7）与更多三个方钮 32 */
    const acts = m.acts;
    if (
      acts.map((a) => a.t).join("|") !==
      "搜资源|追剧中 · 第 2 季|在 Jellyfin 播放|重新刮削|删除第 2 季|更多"
    ) {
      itemFail("hero.acts", `动作是「${acts.map((a) => a.t).join("、")}」`);
    } else {
      const [find, sub, play, rescrape, del, more] = acts;
      expectBox("hero.acts.search", find, { x: 504, y: 368, w: 96, h: 32 }, 2);
      expectBox("hero.acts.sub", sub, { x: 608, y: 368, h: 32 }, 2);
      expectBox("hero.acts.play", play, { x: sub.x + sub.w + 8, y: 368, h: 32 }, 2);
      expectBox(
        "hero.acts.rescrape",
        rescrape,
        { x: play.x + play.w + 8, y: 368, w: 32, h: 32 },
        2,
      );
      expectBox("hero.acts.delete", del, { x: rescrape.x + 40, y: 368, w: 32, h: 32 }, 2);
      expectBox("hero.acts.more", more, { x: del.x + 40, y: 368, w: 32, h: 32 }, 2);
    }
    /* 你的 NAS 上：868,44 248×252，玻璃底；四行，y 40 起、行距 51；第 4 行是媒体服务器（M6） */
    expectBox("nas", m.nas, { x: R - 28 - 248, y: 148, w: 248, h: 252 }, 2);
    if (!m.nasGlass) itemFail("nas", "1440 下「你的 NAS 上」应在大图里（玻璃底）");
    const nasText = m.nasRows.map((r) => r.t).join(" | ");
    expectText(
      "nas.rows",
      nasText,
      "库里 S01 12/12 · S02 6/12剧集库 · 18 个文件 · 82.6 GB | 做种 4 个种子1 个 H&R 还差 2 天 | 追剧中 · 第 2 季版本锁 HHWEB 2160p | Jellyfin 已入库 17 集S02E03 在服务器里被删了 · 处理 · 忽略",
    );
    if (m.nasRows.length === 4) {
      const ys = m.nasRows.map((r) => r.y - m.nas.y);
      if (!near(ys[0], 40, 2) || ys.slice(1).some((y, i) => !near(y - ys[i], 51, 2)))
        itemFail("nas.rows", `四行在卡内 y ${ys.join("、")}，画板 40 起、行距 51`);
      const last = m.nasRows[3];
      if (last.y + last.h > m.nas.y + m.nas.h - 8)
        itemFail(
          "nas.rows",
          `第 4 行的底 ${last.y + last.h} 出了卡（卡底 ${m.nas.y + m.nas.h}，留 14）`,
        );
    }

    /* 季与集：标题 280,468；季 tab 从 352 起、间距 8、h26；图例右对齐到 1424 */
    expectBox("season.title", m.seasonTitle, { x: 280, y: 468 }, 2);
    expectText(
      "season.tabs",
      m.tabs.map((t) => t.t + (t.on ? "*" : "")).join(" | "),
      "第 1 季 12/12 | 第 2 季 6/12* | 特别篇 0/2",
    );
    if (m.tabs.length === 3) {
      const [t1, t2, t3] = m.tabs;
      expectBox("season.tab1", t1, { x: 352, y: 466, h: 26 }, 2);
      if (!near(t2.x - (t1.x + t1.w), 8, 1) || !near(t3.x - (t2.x + t2.w), 8, 1))
        itemFail(
          "season.tabs",
          `tab 间距 ${t2.x - (t1.x + t1.w)}、${t3.x - (t2.x + t2.w)}，画板 8`,
        );
    }
    if (!m.legend || !near(m.legend.x + m.legend.w, R, 2))
      itemFail(
        "season.legend",
        `图例的右边在 ${m.legend ? m.legend.x + m.legend.w : "无"}，画板右对齐到 ${R}`,
      );
    expectText("season.legend", m.legendText, "已入库 6 下载中 1 等版本 1 还没播 4");
    if (m.collapse) itemFail("season.collapse", "一季 12 集时桌面不收起");
    /* 集卡三列（auto-fill、最小 340），列距 17：1144 宽时 370×92 在 x 280、667、1054，行距 104；剧照 10,10 128×72，标题 150,12 */
    const cardW = (W - 34) / 3;
    if (m.eps.length !== 12) itemFail("eps", `集卡 ${m.eps.length} 张，这一季 12 集`);
    else {
      m.eps.forEach((e, i) => {
        const want = {
          x: 280 + (i % 3) * (cardW + 17),
          y: 504 + Math.floor(i / 3) * 104,
          w: cardW,
          h: 92,
        };
        if (Object.entries(want).some(([k, v]) => !near(e[k], v, 2)))
          itemFail(
            `eps[${i}]`,
            `${e.id} 在 ${JSON.stringify({ x: e.x, y: e.y, w: e.w, h: e.h })}，画板 ${JSON.stringify(want)}`,
          );
      });
      const e0 = m.eps[0];
      expectBox("eps[0].still", e0.still, { x: e0.x + 10, y: e0.y + 10, w: 128, h: 72 }, 1);
      expectBox("eps[0].title", e0.titleBox, { x: e0.x + 150, y: e0.y + 12 }, 3);
      expectText("eps[0].title", e0.title, "E01 · 归来");
      /* 播放圆 58,30 32×32：只给已入库、服务器里有的集；E03 在服务器里被删了，写「Jellyfin 里没有了」、动作「处理」「忽略」 */
      expectBox("eps[0].play", e0.play, { x: e0.x + 58, y: e0.y + 30, w: 32, h: 32 }, 1);
      const e3 = m.eps[2];
      if (e3.play) itemFail("eps[2].play", "服务器里删了的集不给播放圆");
      /* 两个动作之间的「·」是单独的元素（字之间的空隙靠 gap），textContent 连在一起 */
      if (!e3.state.includes("Jellyfin 里没有了") || e3.act !== "处理·忽略")
        itemFail("eps[2]", `服务器里删了的集写的是「${e3.state}」、动作「${e3.act}」`);
      const noPlay = m.eps.filter((e, i) => i !== 2 && i < 6 && !e.play).map((e) => e.id);
      if (noPlay.length)
        itemFail("eps.play", `入库了、服务器里有的集没有播放圆：${noPlay.join("、")}`);
      if (m.eps.slice(6).some((e) => e.play)) itemFail("eps.play", "没入库的集不该有播放圆");
      const e7 = m.eps[6];
      if (!e7.state.includes("下载中") || !e7.state.includes("63% · 还要 12 分钟"))
        itemFail("eps[6]", `下载中的集写的是「${e7.state}」`);
      const e8 = m.eps[7];
      if (
        !e8.state.includes("等版本") ||
        !e8.state.includes("锁定 HHWEB 2160p") ||
        e8.act !== "看版本"
      )
        itemFail("eps[7]", `等版本的集写的是「${e8.state}」、动作「${e8.act}」`);
      const e9 = m.eps[8];
      if (!e9.state.includes("还没播") || !e9.state.includes("播出后 2 小时开始搜"))
        itemFail("eps[8]", `下一集写的是「${e9.state}」`);
      const e12 = m.eps[11];
      if (!e12.air.endsWith("· 季终")) itemFail("eps[11]", `最后一集的播出日是「${e12.air}」`);
    }

    /* 这一季的种子：标题 16/700，说明在标题右 24；列表 1144 宽、每行 50；列 x 16、100、460、500、576、800、1108 */
    const lastEp = m.eps[m.eps.length - 1];
    if (lastEp) expectGap("torrents.gap", lastEp.y + lastEp.h, m.torTitle);
    expectText("torrents.title", m.torTitleText, "这一季的种子");
    expectText(
      "torrents.sub",
      m.torSub,
      "删除时按这几条保护：下载中、最短保种 24 小时、H&R、保留与保护标签",
    );
    if (m.torTitle)
      expectBox("torrents.list", m.torList, { x: 280, y: m.torTitle.y + 32, w: W }, 2);
    if (m.rows.length !== 3) itemFail("torrents.rows", `种子 ${m.rows.length} 行，这一季 3 个`);
    else {
      m.rows.forEach((r, i) => {
        if (!near(r.h, 50, 1)) itemFail(`torrents.rows[${i}]`, `行高 ${r.h}，画板 50`);
      });
      const r0 = m.rows[0];
      const cellX = r0.cells.map((c) => c.x - 280);
      /* 站点、种子、下载器、状态、数据、能不能删、更多 */
      const want = [16, 100, 460, 500, 576, 800, 1108];
      const off = want
        .map((w, i) => `${cellX[i]}/${w}`)
        .filter((s) => {
          const [a, b] = s.split("/").map(Number);
          return Math.abs(a - b) > 16;
        });
      if (off.length > 0) itemFail("torrents.cols", `各列 x（实测/画板）${off.join("、")}`);
      expectText(
        "torrents.row0",
        r0.t,
        "OurBitsS02E01–E04 · 2160p HHWEB · 27.3 GBqb做种中分享率 1.31 · 做种 18 天H&R 已满足",
      );
      if (!m.rows[1].t.includes("H&R 还差 2 天") || !m.rows[2].t.includes("受保护：下载中"))
        itemFail("torrents.guard", `能不能删：${m.rows.map((r) => r.cells[5]?.t).join("、")}`);
    }

    /* 演职员：每格 108 宽、间距 8，头像 72 圆在格内 18；一行放得下几个放几个；右侧「全部 15 位」右对齐到 1424 */
    if (m.torList) expectGap("cast.gap", m.torList.y + m.torList.h, m.castTitle);
    expectText("cast.all", m.castAll, "全部 15 位");
    if (m.castAllBox && !near(m.castAllBox.x + m.castAllBox.w, R, 2))
      itemFail(
        "cast.all",
        `「全部 15 位」的右边在 ${m.castAllBox.x + m.castAllBox.w}，画板右对齐到 ${R}`,
      );
    /* 收起时一行放得下几个放几个、不露半个：W 宽放 floor((W + 8) / 116) 位，其余不渲染 */
    const castFit = Math.floor((W + 8) / 116);
    if (m.people.length !== castFit || m.people.some((p) => p.y !== m.people[0].y || p.x + p.w > R))
      itemFail(
        "cast.row",
        `收起时 ${m.people.length} 位（${[...new Set(m.people.map((p) => p.y))].length} 行），${W} 宽放 ${castFit} 位（108 + 8）、一行、不出右边`,
      );
    if (m.people[0]) {
      const p0 = m.people[0];
      expectBox("cast[0]", p0, { x: 280, y: m.castTitle.y + 36, w: 108 }, 2);
      expectBox("cast[0].face", p0.face, { x: 298, y: p0.y, w: 72, h: 72 }, 1);
      expectText("cast[0]", `${p0.name}/${p0.role}`, "陈默/导演");
      if (m.people[1] && !near(m.people[1].x - p0.x, 116, 1))
        itemFail("cast.pitch", `格距 ${m.people[1].x - p0.x}，画板 116`);
    }
    /* 推荐：海报 116×174（偏离 ⑤），间距 12；标牌写已入库、已订阅 */
    if (m.cast) expectGap("recs.gap", m.cast.y + m.cast.h, m.recTitle);
    expectText("recs.sub", m.recSub, "TMDB 按这部剧推荐，标出库里有的和已订阅的");
    const recFit = Math.floor((W + 12) / 128);
    if (m.recs.length !== recFit || m.recs.some((r) => r.y !== m.recs[0].y || r.x + r.w > R))
      itemFail(
        "recs.row",
        `推荐 ${m.recs.length} 个，${W} 宽一行放 ${recFit} 个（116 + 12）、不出右边`,
      );
    if (m.recs[0]) {
      const r0 = m.recs[0];
      expectBox("recs[0].poster", r0.poster, { x: 280, y: m.recTitle.y + 32, w: 116, h: 174 }, 2);
      if (m.recs[1] && !near(m.recs[1].x - r0.x, 128, 1))
        itemFail("recs.pitch", `间距 ${m.recs[1].x - r0.x - 116}，画板 12`);
      expectText(
        "recs.tags",
        m.recs
          .slice(0, 6)
          .map((r) => r.tag || "-")
          .join(" "),
        "已入库 已订阅 - - 已入库 已订阅",
      );
    }
    if (m.overflow > 0) itemFail("overflow", `横向溢出 ${m.overflow}px`);

    /* 探针：换到第 1 季 —— 路由跟着换，集与种子跟着这一季 */
    const tab = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        const t1 = document.querySelector('[data-testid="mi-tab-1"]');
        if (!t1) return 'no-tab';
        t1.click();
        for (let i = 0; i < 20; i++) {
          await wait(150);
          const first = document.querySelector('.mi-ep .mi-ep__title');
          if (first && (first.textContent ?? '').includes('第 1 集')) break;
        }
        const eps = document.querySelectorAll('.mi-ep').length;
        const first = (document.querySelector('.mi-ep .mi-ep__title')?.textContent ?? '').trim();
        const rows = [...document.querySelectorAll('[data-testid="mi-trow"]')].map((r) => (r.textContent ?? '').replace(/\\s+/g, ' '));
        const ok = location.hash.includes('season=1') && t1.classList.contains('is-on') && eps === 12 &&
          first === 'E01 · 第 1 集' && rows.length === 1 && rows[0].includes('S01 整季');
        return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' ' + location.hash + ' 集 ' + eps + ' 第一集 ' + first + ' 种子 ' + rows.join(' / ');
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(tab, "verdict=ok").length > 0) itemFail("probe.season", `换季：${tab}`);
    /* 探针：「全部 15 位」展开成多行，再点收起 */
    const castProbe = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        const btn = document.querySelector('[data-testid="mi-cast-all"]');
        if (!btn) return 'no-cast-all';
        const rowsOf = () => new Set([...document.querySelectorAll('.mi-person')].map((p) => Math.round(p.getBoundingClientRect().top))).size;
        const visible = () => document.querySelectorAll('.mi-person').length;
        const before = visible();
        btn.click();
        await wait(300);
        const open = visible();
        const rows = rowsOf();
        const label = (btn.textContent ?? '').trim();
        btn.click();
        await wait(300);
        const after = visible();
        const ok = before === ${castFit} && open === 15 && rows === 2 && label === '收起' && after === before;
        return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 看得到 ' + before + '→' + open + '（' + rows + ' 行，按钮 ' + label + '）→' + after;
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(castProbe, "verdict=ok").length > 0)
      itemFail("probe.cast", `全部演职员：${castProbe}`);

    /*
     * 删除弹窗（画板 03 的片段，弹窗内坐标）：600×568；标题 24,18 16/700，关闭 560,20；库文件勾选框 24,74、标题 46,70、说明 46,90，
     * 「看文件」右边到 576（画板 580，同画板 04 的右对齐到内容边）；分隔 y 120；种子块标题 24,132，三行 y 154 起、行距 54，
     * 勾选框 y + 4，下拉 456,y + 2 120×28，「仍然删除」框 492,y + 8；分隔 y 322；订阅块标题 24,334，单选 y 356 起、行距 46；
     * 页脚分隔 y 508，摘要 24 竖直居中，「取消」424、「删除」504（72×32，y 522，底色 dang）。
     */
    const d = await ev(
      measureDelete(`
        /* 前面的探针换到了第 1 季：换回第 2 季（假数据的删除预览是第 2 季的） */
        const tab = document.querySelector('[data-testid="mi-tab-2"]');
        if (!tab) return 'no-tab-2';
        if (!tab.classList.contains('is-on')) {
          tab.click();
          for (let i = 0; i < 20 && !(document.querySelector('.mi-ep .mi-ep__title')?.textContent ?? '').includes('归来'); i++) await wait(150);
        }
        const b = document.querySelector('[data-testid="mi-delete"]');
        if (!b) return 'no-delete-button';
        b.click();
        return true;
      `),
    ).catch((e) => ({ err: `probe 失败：${e.message}` }));
    if (process.env.MI_DUMP) console.log(JSON.stringify(d, null, 1));
    if (d.err) itemFail("delete", `删除弹窗：${d.err}`);
    else {
      const del = (key, got, want, tol = 2) => expectBox(`delete.${key}`, got, want, tol);
      del("dialog", d.dlg, { w: 600, h: 568 }, 4);
      del("header", d.header, { x: 0, y: 0, w: 600, h: 57 });
      del("title", d.title, { x: 24, y: 18 });
      expectText("delete.title", d.titleText, "删除《长夜将尽》第 2 季");
      if (d.titleFont !== "16px/700") itemFail("delete.title", `标题 ${d.titleFont}，画板 16/700`);
      del("close", d.close, { x: 560, y: 20, w: 16, h: 16 });
      del("files.check", d.filesCheck, { x: 24, y: 74, w: 14, h: 14 });
      del("files.title", d.filesTitle, { x: 46, y: 70 });
      del("files.sub", d.filesSub, { x: 46, y: 90 });
      expectText(
        "delete.files.sub",
        d.filesSubText,
        "6 个文件 · 41.2 GB · 都是硬链接，种子里的数据不受影响",
      );
      if (!d.filesChecked) itemFail("delete.files.check", "库里的文件默认要勾上");
      if (
        !d.filesMore ||
        !near(d.filesMore.x + d.filesMore.w, 576, 4) ||
        !near(d.filesMore.y, 72, 2)
      )
        itemFail(
          "delete.files.more",
          `「看文件」在 ${JSON.stringify(d.filesMore)}，画板右边到 580、y 72`,
        );
      if (d.secs.length !== 3)
        itemFail("delete.secs", `${d.secs.length} 块，画板三块（库文件、种子、订阅）`);
      else {
        /* 块的下沿是分隔线：120、322，订阅块一直到页脚分隔 508 */
        const bottoms = d.secs.map((b) => b.y + b.h);
        if (!near(bottoms[0], 121, 1) || !near(bottoms[1], 323, 1) || !near(bottoms[2], 508, 1))
          itemFail(
            "delete.rules",
            `三块的下沿 ${bottoms.join("、")}，画板分隔在 120、322、页脚 508`,
          );
      }
      del("torrents.title", d.torTitle, { x: 24, y: 132 });
      expectText("delete.torrents.title", d.torTitleText, "种子（每台下载器分开判断）");
      expectText(
        "delete.torrents",
        d.tors.map((t) => `${t.title}/${t.sub}/${t.checked ? "勾" : "不勾"}`).join(" | "),
        "OurBits · S02E01–E04 · qb/分享率 1.31 · 做种 18 天 · H&R 已满足/勾 | HDSky · S02E05–E06 · tr/受保护：H&R 要求做种 7 天，还差 2 天/不勾 | HDSky · S02E07 · qb/受保护：还在下载（63%）/不勾",
      );
      d.tors.forEach((t, i) => {
        const y = 154 + i * 54;
        del(`torrents[${i}].check`, t.check, { x: 24, y: y + 4, w: 14, h: 14 });
        del(`torrents[${i}].title`, t.titleBox, { x: 46, y });
        del(`torrents[${i}].sub`, t.subBox, { x: 46, y: y + 20 });
        if (i < 2 && t.ruled !== "1px") itemFail(`delete.torrents[${i}].rule`, "种子之间要有分隔");
        if (i < 2) del(`torrents[${i}].rule`, t.main, { x: 46, w: 530, h: 47 });
        if (i === 2 && t.ruled !== "0px") itemFail("delete.torrents[2].rule", "最后一行没有分隔");
      });
      const [t0, t1, t2] = d.tors;
      if (t0) del("torrents[0].how", t0.how, { x: 456, y: 156, w: 120, h: 28 });
      if (t0 && t0.howText !== "连数据一起删")
        itemFail("delete.torrents[0].how", `下拉写的是「${t0.howText}」`);
      for (const [i, t] of [
        [1, t1],
        [2, t2],
      ]) {
        if (!t) continue;
        if (t.how) itemFail(`delete.torrents[${i}].how`, "受保护的没勾「仍然删除」时没有下拉");
        del(`torrents[${i}].force`, t.force, { x: 492, y: 154 + i * 54 + 8, w: 14, h: 14 });
        del(`torrents[${i}].subText`, t.subText, { x: 62 });
      }
      del("subscription.title", d.subTitle, { x: 24, y: 334 });
      expectText(
        "delete.subscription",
        d.subs.map((o) => `${o.title}${o.on ? "*" : ""}/${o.sub}`).join(" | "),
        "暂停追剧*/推荐：不暂停的话，台账会把删掉的集当成缺的，再去下一遍 | 删除订阅/以后不再找这一季 | 保留，继续追/只删这次选的文件和种子",
      );
      d.subs.forEach((o, i) => {
        const y = 356 + i * 46;
        del(`subscription[${i}].radio`, o.radio, { x: 24, y: y + 4, w: 14, h: 14 });
        del(`subscription[${i}].title`, o.titleBox, { x: 46, y });
      });
      del("footer", d.footer, { x: 0, y: 508, w: 600, h: 60 });
      /* 摘要按字本身的框量：左边 24，竖直居中于页脚（画板 17 高的行在 y 530，中线 538.5） */
      del("summary", d.summary, { x: 24 });
      if (d.summary && !near(d.summary.y + d.summary.h / 2, 538.5, 2))
        itemFail(
          "delete.summary",
          `摘要的中线在 ${d.summary.y + d.summary.h / 2}，画板 538.5（页脚 508…568 居中）`,
        );
      expectText(
        "delete.summary",
        d.summaryText,
        "将删除 6 个库文件、1 个种子（连数据），暂停追剧",
      );
      expectText("delete.buttons", d.buttons.map((b) => b.t).join("|"), "取消|删除");
      if (d.buttons.length === 2) {
        del("buttons.cancel", d.buttons[0], { x: 424, y: 522, w: 72, h: 32 });
        del("buttons.delete", d.buttons[1], { x: 504, y: 522, w: 72, h: 32 });
      }
      if (d.runBg !== "rgb(179, 35, 24)")
        itemFail("delete.buttons.delete", `「删除」底色 ${d.runBg}，画板 --pt-dang #B32318`);
      if (d.overflow > 0) itemFail("delete.overflow", `正文横向溢出 ${d.overflow}px`);
      if (d.forcedSummary !== "将删除 6 个库文件、2 个种子（连数据），暂停追剧" || !d.forcedChecked)
        itemFail(
          "delete.force",
          `勾了「仍然删除」以后：${d.forcedSummary}，第二行${d.forcedChecked ? "" : "没"}勾上`,
        );
    }
    for (const err of [...new Set(consoleErrors)].slice(0, 3)) itemFail("console", err);
  }

  /* ---- 桌面 1200×1024：主区窄于 1000，「你的 NAS 上」移出大图，换成大图下面的白卡（B04 布局规则） ---- */
  consoleErrors = [];
  if (!(await openItem({ width: 1200, height: 1024, mobile: false }, ITEM_TV_ROUTE, true))) {
    itemFail("narrow.load", "视口 1200 下没进条目详情");
  } else {
    const m = await ev(MEASURE_MI);
    const W = 1200 - 264 - 32 - m.sb;
    expectBox("narrow.hero", m.hero, { x: 280, y: 104, w: W, h: 340 }, 2);
    if (m.nasGlass) itemFail("narrow.nas", "主区窄于 1000 时「你的 NAS 上」不该在大图里");
    expectBox("narrow.nas", m.nas, { x: 280, y: 104 + 340 + 24, w: W }, 2);
    /* 文字列伸到大图右边 28：简介最宽 600，W − 224 − 28 不到 600 时取它 */
    expectBox("narrow.overview", m.overview, { x: 504, w: Math.min(600, W - 224 - 28) }, 2);
    if (m.nas) expectGap("narrow.season.gap", m.nas.y + m.nas.h, m.seasonTitle);
    /* 两列集卡：(W − 17) / 2，最小 340 */
    const cardW = (W - 17) / 2;
    if (
      m.eps.length !== 12 ||
      m.eps.some((e, i) => !near(e.w, cardW, 2) || !near(e.x, 280 + (i % 2) * (cardW + 17), 2))
    )
      itemFail(
        "narrow.eps",
        `集卡 ${m.eps.length} 张，宽 ${[...new Set(m.eps.map((e) => e.w))].join("、")}，应为两列 ${cardW.toFixed(1)}`,
      );
    if (m.overflow > 0) itemFail("narrow.overflow", `横向溢出 ${m.overflow}px`);
    for (const err of [...new Set(consoleErrors)].slice(0, 3)) itemFail("narrow.console", err);
  }

  /* ---- 桌面 1440×1024，电影 ---- */
  consoleErrors = [];
  if (!(await openItem({ width: 1440, height: 1024, mobile: false }, ITEM_MOVIE_ROUTE, false))) {
    itemFail("movie.load", "等了 10 秒还没有条目");
  } else {
    const m = await ev(MEASURE_MI);
    if (process.env.MI_DUMP) console.log(JSON.stringify(m, null, 1));
    expectText("movie.crumbs", m.crumbs, "媒体 / 探索 / 沙丘2");
    expectBox("movie.hero", m.hero, { x: 280, y: 104, w: 1144 - m.sb, h: 340 }, 2);
    expectText("movie.original", m.original, "Dune: Part Two · 2024 · 2 小时 46 分");
    expectText("movie.pills", m.pills.join("|"), "电影|科幻|冒险|美国|已上映");
    if (m.seasons) itemFail("movie.seasons", "电影不该有「季与集」");
    expectText(
      "movie.nas.library",
      m.nasRows[0]?.t ?? "",
      "库里有 · 2160p FRDS电影库 · 1 个文件 · 58.2 GB",
    );
    expectText("movie.nas.subscription", m.nasRows[2]?.t ?? "", "已完成没开版本锁");
    expectText("movie.torrents.title", m.torTitleText, "这部片的种子");
    expectGap("movie.torrents.gap", m.hero ? m.hero.y + m.hero.h : 0, m.torTitle);
    if (m.rows.length !== 1 || !m.rows[0].t.startsWith("CHDBits2160p FRDS · 58.2 GB"))
      itemFail("movie.torrents.rows", `种子：${m.rows.map((r) => r.t).join(" / ")}`);
    expectText("movie.recs.sub", m.recSub, "TMDB 按这部片推荐，标出库里有的和已订阅的");
    if (m.overflow > 0) itemFail("movie.overflow", `横向溢出 ${m.overflow}px`);
    for (const err of [...new Set(consoleErrors)].slice(0, 3)) itemFail("movie.console", err);
  }

  /* ---- 手机 375×812，剧集（画板 08） ---- */
  consoleErrors = [];
  if (!(await openItem({ ...M_VIEWPORT, mobile: true }, ITEM_TV_ROUTE, true))) {
    itemFail("load", "等了 10 秒还没有集卡", true);
  } else {
    const m = await ev(MEASURE_MI);
    if (process.env.MI_DUMP) console.log(JSON.stringify(m, null, 1));
    const cw = 343 - m.sb;
    expectText("topbar.sub", m.mheadSub, "剧集 · 2025 · 追剧中", true);
    /* 大图 375×348 不留边、贴住顶栏（52）；海报 16,20 108×162；名字 136,20；简介 16,196；动作 y 292 h40 */
    expectBox("hero", m.hero, { x: 0, y: 52, w: 375 - m.sb, h: 348 }, 2, true);
    expectBox("hero.poster", m.poster, { x: 16, y: 72, w: 108, h: 162 }, 2, true);
    expectBox("hero.title", m.title, { x: 136, y: 72 }, 4, true);
    expectBox("hero.overview", m.overview, { x: 16, y: 248, w: cw }, 4, true);
    if (m.imdb) itemFail("hero.imdb", "手机不放 IMDb", true);
    /* 资料胶囊从 136,156 起两行（画板 08：类型、三个题材一行，国家、片长、状态一行），一个都不能被截掉 */
    expectBox("hero.pills", m.pillsBox, { x: 136, y: 156 }, 4, true);
    if (m.pillsShown.n !== m.pills.length || m.pillsShown.rows !== 2)
      itemFail(
        "hero.pills",
        `胶囊 ${m.pills.length} 个，看得见 ${m.pillsShown.n} 个、${m.pillsShown.rows} 行（画板两行全部看得见）`,
        true,
      );
    const acts = m.acts;
    if (acts.map((a) => a.t).join("|") !== "搜资源|追剧中 · 第 2 季")
      itemFail("hero.acts", `动作是「${acts.map((a) => a.t).join("、")}」`, true);
    else {
      expectBox("hero.acts.search", acts[0], { x: 16, y: 344, h: 40 }, 2, true);
      expectBox("hero.acts.sub", acts[1], { y: 344, h: 40 }, 2, true);
      if (
        !near(acts[1].x + acts[1].w, 16 + cw, 2) ||
        !near(acts[1].x - (acts[0].x + acts[0].w), 8, 2)
      )
        itemFail("hero.acts", `两个按钮 ${JSON.stringify(acts)}，画板平分 343、中间 8`, true);
    }
    /* 「更多」在大图右上角（顶栏留给外壳，见落地偏离 40） */
    expectBox("hero.more", m.more, { y: 72 }, 4, true);
    /* 你的 NAS 上：行卡 343 宽在 y 416；四行加播放 276（画板 08，少一行减 46、没有播放减 50）；行 y 34 起、行距 46 */
    expectBox("nas", m.nas, { x: 16, y: 416, w: cw, h: 276 }, 2, true);
    if (m.nasGlass) itemFail("nas", "手机上「你的 NAS 上」是大图下面的白卡", true);
    if (m.nasRows.length !== 4) itemFail("nas.rows", `${m.nasRows.length} 行，画板 4 行`, true);
    else {
      const ys = m.nasRows.map((r) => r.y - m.nas.y);
      if (!near(ys[0], 34, 2) || ys.slice(1).some((y, i) => !near(y - ys[i], 46, 2)))
        itemFail("nas.rows", `四行在卡内 y ${ys.join("、")}，画板 34 起、行距 46`, true);
      expectText(
        "nas.server",
        m.nasRows[3].t,
        "Jellyfin 已入库 17 集S02E03 在服务器里被删了 · 处理 · 忽略",
        true,
      );
    }
    /* 播放：白卡里 14,226 的 soft 按钮 315×36 */
    if (!m.playInNas) itemFail("nas.play", "手机的「在 Jellyfin 播放」要在白卡底部", true);
    expectBox("nas.play", m.play, { x: 30, y: 416 + 226, w: cw - 28, h: 36 }, 2, true);
    expectText("nas.play", m.playText, "在 Jellyfin 播放", true);
    /* 季与集：标题、横滑 tab（标题下 32）、图例（标题下 74）、收起行（标题下 100，343×44）、集卡（标题下 152，343×80，行距 88） */
    if (m.nas) expectGap("season.gap", m.nas.y + m.nas.h, m.seasonTitle, true);
    if (m.seasonTitle) {
      const t = m.seasonTitle.y;
      if (m.tabs[0]) expectBox("season.tab1", m.tabs[0], { x: 16, y: t + 32, h: 30 }, 2, true);
      expectBox("season.legend", m.legend, { x: 16, y: t + 74 }, 3, true);
      expectBox("season.collapse", m.collapse, { x: 16, y: t + 100, w: cw, h: 44 }, 2, true);
      expectText("season.collapse", m.collapseText, "E01–E06 · 6 集都已入库展开", true);
      if (m.eps.length !== 6)
        itemFail("eps", `收起以后 ${m.eps.length} 张集卡，应为 6（E07–E12）`, true);
      else {
        m.eps.forEach((e, i) => {
          const want = { x: 16, y: t + 152 + i * 88, w: cw, h: 80 };
          if (Object.entries(want).some(([k, v]) => !near(e[k], v, 2)))
            itemFail(
              `eps[${i}]`,
              `${e.id} 在 ${JSON.stringify({ x: e.x, y: e.y, w: e.w, h: e.h })}，画板 ${JSON.stringify(want)}`,
              true,
            );
        });
        expectBox(
          "eps[0].still",
          m.eps[0].still,
          { x: 24, y: m.eps[0].y + 8, w: 112, h: 63 },
          1,
          true,
        );
        /* E07 起没入库：没有播放圆。展开以后看 E05 的播放圆（50,25 28×28）在探针里量 */
        if (m.eps.some((e) => e.play)) itemFail("eps.play", "没入库的集不该有播放圆", true);
      }
    }
    /* 这一季的种子：行卡 343 宽，每个种子 64 */
    const lastEp = m.eps[m.eps.length - 1];
    if (lastEp) expectGap("torrents.gap", lastEp.y + lastEp.h, m.torTitle, true);
    /* 行在卡内 14（加 1px 边框是 15，横向差的 1px 不补，同 M2a 的落地偏离 16），每个 64 */
    if (m.torList) expectBox("torrents.list", m.torList, { x: 16, w: cw }, 2, true);
    if (
      m.rows.length !== 3 ||
      m.rows.some((r) => !near(r.h, 64, 1) || !near(r.x, 31, 1) || !near(r.w, cw - 30, 1))
    )
      itemFail(
        "torrents.rows",
        `种子行 ${JSON.stringify(m.rows.map(({ x, y, w, h }) => ({ x, y, w, h })))}，画板卡内 14 起、315 宽、每个 64`,
        true,
      );
    /* 演职员：横滑，每格 76、间距 8，头像 64；推荐：海报 104×156、间距 12 */
    if (m.people[0]) {
      expectBox("cast[0]", m.people[0], { x: 16, w: 76 }, 2, true);
      expectBox("cast[0].face", m.people[0].face, { x: 22, w: 64, h: 64 }, 1, true);
      if (m.people[1] && !near(m.people[1].x - m.people[0].x, 84, 1))
        itemFail("cast.pitch", `格距 ${m.people[1].x - m.people[0].x}，画板 84`, true);
    } else itemFail("cast", "没有演职员", true);
    if (m.recs[0]) {
      expectBox("recs[0].poster", m.recs[0].poster, { x: 16, w: 104, h: 156 }, 2, true);
      if (m.recs[1] && !near(m.recs[1].x - m.recs[0].x, 116, 1))
        itemFail("recs.pitch", `间距 ${m.recs[1].x - m.recs[0].x - 104}，画板 12`, true);
    } else itemFail("recs", "没有推荐", true);
    if (m.overflow > 0) itemFail("overflow", `横向溢出 ${m.overflow}px`, true);

    /* 探针：收起行「展开」以后 12 张、换成「收起」，再点回到 6 张 */
    const fold = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        const btn = document.querySelector('[data-testid="mi-expand"]');
        if (!btn) return 'no-expand';
        btn.click();
        await wait(300);
        const open = document.querySelectorAll('.mi-ep').length;
        const label = (btn.textContent ?? '').trim();
        const e5 = document.querySelector('[data-testid="mi-ep-5"]');
        const p5 = e5?.querySelector('.mi-ep__play');
        const er = e5?.getBoundingClientRect();
        const pr = p5?.getBoundingClientRect();
        const playOk = Boolean(er && pr) && Math.abs(pr.left - er.left - 50) <= 1 && Math.abs(pr.top - er.top - 25) <= 1 &&
          Math.round(pr.width) === 28 && Math.round(pr.height) === 28;
        btn.click();
        await wait(300);
        const closed = document.querySelectorAll('.mi-ep').length;
        const ok = open === 12 && label === '收起' && closed === 6 && playOk;
        return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 展开 ' + open + '（' + label + '）收起 ' + closed +
          ' E05 播放圆 ' + (pr ? Math.round(pr.left - er.left) + ',' + Math.round(pr.top - er.top) + ' ' + Math.round(pr.width) + '×' + Math.round(pr.height) : '无');
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(fold, "verdict=ok").length > 0) itemFail("probe.fold", `收起行：${fold}`, true);
    /* 探针：点简介展开全文（盖在大图上），大图高度不变，再点收起 */
    const ov = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        const p = document.querySelector('[data-testid="mi-overview"]');
        const hero = document.querySelector('[data-testid="mi-hero"]');
        if (!p || !hero) return 'no-overview';
        const h0 = Math.round(p.getBoundingClientRect().height);
        p.click();
        await wait(300);
        const r = p.getBoundingClientRect();
        const hr = hero.getBoundingClientRect();
        const full = p.scrollHeight <= p.clientHeight + 1 || getComputedStyle(p).overflowY === 'auto';
        const inside = r.top >= hr.top && r.bottom <= hr.bottom + 0.5;
        const heroH = Math.round(hr.height);
        p.click();
        await wait(300);
        const h1 = Math.round(p.getBoundingClientRect().height);
        const ok = r.height > h0 && full && inside && heroH === 348 && h1 === h0;
        return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 高 ' + h0 + '→' + Math.round(r.height) + '→' + h1 + ' 在大图里 ' + inside + ' 大图 ' + heroH;
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(ov, "verdict=ok").length > 0) itemFail("probe.overview", `简介展开：${ov}`, true);

    /* 删除弹窗在手机上是全屏页（画板 03 的片段，宽度换成 375 − 32）：「更多」里的删除打开；摘要一行、按钮一行 */
    const md = await ev(
      measureDelete(`
        document.querySelector('.mi-hero__more [data-testid="mi-more"]')?.click();
        for (let i = 0; i < 20 && !document.querySelector('[data-testid="mi-more-delete"]'); i++) await wait(100);
        const item = document.querySelector('[data-testid="mi-more-delete"]');
        if (!item) return 'no-more-delete';
        item.click();
        return true;
      `),
    ).catch((e) => ({ err: `probe 失败：${e.message}` }));
    if (process.env.MI_DUMP) console.log(JSON.stringify(md, null, 1));
    if (md.err) itemFail("delete", `删除弹窗：${md.err}`, true);
    else {
      const del = (key, got, want, tol = 2) => expectBox(`delete.${key}`, got, want, tol, true);
      if (!md.fullscreen) itemFail("delete", "手机上删除弹窗要全屏", true);
      del("dialog", md.dlg, { x: 0, y: 0, w: 375, h: 812 });
      del("title", md.title, { x: 16 });
      del("files.check", md.filesCheck, { x: 16, w: 14, h: 14 });
      del("files.title", md.filesTitle, { x: 38 });
      if (!md.filesMore || !near(md.filesMore.x + md.filesMore.w, 359, 2))
        itemFail(
          "delete.files.more",
          `「看文件」的右边 ${md.filesMore ? md.filesMore.x + md.filesMore.w : "无"}，应到 359`,
          true,
        );
      if (md.tors.length === 3) {
        del("torrents[0].title", md.tors[0].titleBox, { x: 38 });
        if (!md.tors[0].how || !near(md.tors[0].how.x + md.tors[0].how.w, 359, 2))
          itemFail(
            "delete.torrents[0].how",
            `下拉在 ${JSON.stringify(md.tors[0].how)}，右边应到 359`,
            true,
          );
      }
      /* 页脚：摘要占一行，取消、删除在下一行靠右 */
      const [c, r] = md.buttons;
      if (!md.summary || !c || !r || md.summary.y + md.summary.h > c.y || !near(r.x + r.w, 359, 2))
        itemFail(
          "delete.footer",
          `摘要 ${JSON.stringify(md.summary)}、按钮 ${JSON.stringify(md.buttons)}：摘要在上一行，按钮靠右到 359`,
          true,
        );
      if (md.overflow > 0) itemFail("delete.overflow", `正文横向溢出 ${md.overflow}px`, true);
    }
    for (const err of [...new Set(consoleErrors)].slice(0, 3)) itemFail("console", err, true);
  }
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { ...D_VIEWPORT, deviceScaleFactor: 1, mobile: false },
    sessionId,
  );
}

// ------------------------------------------------- 首页与系统体检（画板 02、07，媒体中心 M8）

/*
 * 首页不是带式表格页：页头 64 下面先是满宽的 NAS 概览（首页 NAS 概览 H2），再是今天播出、待处理条，
 * 最后两栏（左：继续观看、最近入库、这周还要播；右 368：下载器、存储、状态）。
 * NAS 概览与右栏的数字来自 docs/design/home-nas-spec.md；媒体区块的数字来自 docs/design/media-hub-board-spec.md 的 02 / 07
 * （画板 02 按 1440×1024 画：侧栏 264 + 主区 1176，内容 1144；手机按画板 07 量 375×812，块与块之间统一 24，偏离 ④），
 * 整体挪到 NAS 概览下面。加了 NAS 概览以后桌面总比视口高，滚动区的竖向滚动条让出 sb，各处按让出以后的宽量。
 * 1200 宽时内容 894 < 1100：右栏落到下面；≤ 1080：NAS 概览第二行两列。
 * 假数据是 board-fixtures.mjs 的 HOME 与 NAS_VIEW（照画板：今天五张卡、待处理六类、继续观看三张、最近入库七张、这周四条、
 * 三块盘两台下载器；NAS 概览是一台 TrueNAS：三个存储池、八块硬盘、七条告警）。localStorage 设了 pt-fixture-home-empty 时回
 * HOME_EMPTY 与 NAS_VIEW_EMPTY，量空态；设了 pt-fixture-home-nas=local 时 NAS 概览画本机来源。
 * 系统体检画板没有，只量不溢出、各组与项都在。
 */
const doHome = wanted.length === 0 || wanted.includes("/home");

const MEASURE_HM = `(() => {
  const box = (el) => {
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const q = (sel, root = document) => root.querySelector(sel);
  const qa = (sel, root = document) => [...root.querySelectorAll(sel)];
  const txt = (el) => (el?.textContent ?? '').replace(/\\s+/g, ' ').trim();
  const sec = (id) => {
    const el = q('[data-testid="' + id + '"]');
    if (!el) return null;
    return {
      ...box(el),
      title: box(q('.hm-sec__title', el)),
      titleText: txt(q('.hm-sec__title', el)),
      count: box(q('.hm-sec__count', el)),
      countText: txt(q('.hm-sec__count', el)),
      tag: txt(q('.hm-sec__tag', el)),
      link: box(q('.hm-sec__link', el)),
      linkText: txt(q('.hm-sec__link', el)),
      empty: txt(q('.hm-empty', el)),
    };
  };
  const panel = (id) => {
    const el = q('[data-testid="' + id + '"]');
    if (!el) return null;
    return { ...box(el), head: box(q('.pt-panel__head', el)), headText: txt(q('.pt-panel__title', el)), card: box(q('.hm-mcard', el)) };
  };
  const scroller = document.querySelector('.pt-shell__content');
  const inner = document.querySelector('.pt-shell__inner');
  return {
    sb: scroller ? scroller.offsetWidth - scroller.clientWidth : 0,
    overflow: document.documentElement.scrollWidth - window.innerWidth,
    innerOverflow: inner ? inner.scrollWidth - Math.round(inner.getBoundingClientRect().width) : 0,
    head: box(q('.pt-head')),
    headSub: txt(q('#pt-head-sub')),
    mheadSub: txt(q('#pt-mhead-sub')),
    refresh: box(q('#pt-head-acts [data-testid="hm-refresh"]')),
    mrefresh: box(q('.hm-mrefresh [data-testid="hm-refresh"]')),
    today: sec('hm-today'),
    tcs: qa('.hm-tc').map((el) => ({
      id: el.getAttribute('data-testid'),
      ...box(el),
      img: box(q('.hm-tc__img', el)),
      state: box(q('.hm-tc__state', el)),
      stateText: txt(q('.hm-tc__state', el)),
      time: box(q('.hm-tc__time', el)),
      title: box(q('.hm-tc__title', el)),
      sub: box(q('.hm-tc__sub', el)),
      subText: txt(q('.hm-tc__sub', el)),
      act: box(q('.hm-tc__act, .hm-tc__note', el)),
      actText: txt(q('.hm-tc__act, .hm-tc__note', el)),
    })),
    row: box(q('[data-testid="hm-today-row"]')),
    rowScroll: (() => { const r = q('[data-testid="hm-today-row"]'); return r ? r.scrollWidth - r.clientWidth : 0; })(),
    pages: qa('.hm-today__page').map(box),
    dots: qa('[data-testid="hm-today-dots"] > *').map(box),
    todo: box(q('[data-testid="hm-todo"]')),
    todoIcon: box(q('[data-testid="hm-todo"] .hm-todo__icon')),
    todoTitle: box(q('[data-testid="hm-todo"] .hm-todo__title')),
    todoCount: box(q('[data-testid="hm-todo-count"]')),
    todoNote: txt(q('[data-testid="hm-todo-empty"]')),
    chips: qa('[data-testid^="hm-chip-"]').map((el) => ({ t: txt(el), ...box(el) })),
    todoAll: box(q('[data-testid="hm-todo-all"]')),
    resume: sec('hm-resume'),
    rcs: qa('.hm-rc').map((el) => ({
      ...box(el),
      img: box(q('.hm-rc__img', el)),
      play: box(q('.hm-rc__play', el)),
      track: box(q('.hm-rc__track', el)),
      title: box(q('.hm-rc__title', el)),
      sub: box(q('.hm-rc__sub', el)),
      subText: txt(q('.hm-rc__sub', el)),
    })),
    recent: sec('hm-recent'),
    pcs: qa('.hm-pc').map((el) => ({
      ...box(el),
      poster: box(q('.hm-pc__poster', el)),
      q: box(q('.hm-pc__q', el)),
      title: box(q('.hm-pc__title', el)),
      sub: box(q('.hm-pc__sub', el)),
    })),
    week: panel('hm-week'),
    weekSec: sec('hm-week'),
    weekEmpty: txt(q('[data-testid="hm-week-empty"]')),
    wks: qa('.hm-wk').map((el) => ({
      ...box(el),
      date: box(q('.hm-wk__date', el)),
      title: box(q('.hm-wk__title', el)),
      note: box(q('.hm-wk__note', el)),
      line: (() => { const st = getComputedStyle(el, '::before'); return st.content === 'none' || st.display === 'none' ? null : { left: st.left, top: st.top, w: st.width, h: st.height }; })(),
    })),
    /* NAS 概览（H2）：标题行、卡、主机行、四张数值卡、第二行三块（本机来源时两块）、手机的两行摘要 */
    hn: (() => {
      const el = q('[data-testid="hn"]');
      if (!el) return null;
      const shown = (e) => Boolean(e) && getComputedStyle(e).display !== 'none' && e.getBoundingClientRect().width > 0;
      const vbox = (e) => (shown(e) ? box(e) : null);
      const clip = (e) => (e ? e.scrollWidth - e.clientWidth : 0);
      return {
        ...box(el),
        title: box(q('.hm-sec__title', el)),
        titleText: txt(q('.hm-sec__title', el)),
        countText: txt(q('.hm-sec__count', el)),
        tag: txt(q('.hm-sec__tag', el)),
        link: box(q('.hm-sec__link', el)),
        linkText: txt(q('.hm-sec__link', el)),
        others: box(q('[data-testid="hn-others"]', el)),
        othersText: txt(q('[data-testid="hn-others"]', el)),
        live: box(q('[data-testid="hn-live"]', el)),
        liveText: txt(q('[data-testid="hn-live"]', el)),
        liveDot: box(q('.hn-live__dot', el)),
        card: box(q('[data-testid="hn-card"]', el)),
        empty: box(q('[data-testid="hn-empty"]', el)),
        emptyIcon: box(q('.hn-empty__icon', el)),
        emptyTitle: txt(q('.hn-empty__title', el)),
        emptySub: txt(q('.hn-empty__sub', el)),
        emptyBtn: box(q('[data-testid="hn-connect"]', el)),
        emptyBtnText: txt(q('[data-testid="hn-connect"]', el)),
        host: box(q('[data-testid="hn-host"]', el)),
        logo: box(q('.hn-host__logo', el)),
        hostTitle: box(q('.hn-host__title', el)),
        hostTitleText: txt(q('.hn-host__title', el)),
        ver: box(q('[data-testid="hn-platform"]', el)),
        verText: txt(q('[data-testid="hn-platform"]', el)),
        meta: qa('.hn-host__m', el).map((m) => ({ t: txt(m), ...box(m) })),
        facts: qa('[data-testid="hn-facts"] > *', el).filter(shown).map((f) => ({ t: txt(f), ...box(f) })),
        body: box(q('.hn-body', el)),
        kpis: qa('.hn-kpi', el).map((k) => {
          const sub = q('.hn-kpi__sub', k);
          const aside = q('.hn-kpi__aside', k);
          return {
            key: (k.getAttribute('data-testid') ?? '').replace('hn-kpi-', ''),
            ...box(k),
            ic: box(q('.hn-kpi__ic', k)),
            label: box(q('.hn-kpi__label', k)),
            labelText: txt(q('.hn-kpi__label', k)),
            aside: vbox(aside),
            asideText: shown(aside) ? txt(aside) : '',
            asideClip: clip(aside),
            val: box(q('.hn-kpi__val', k)),
            valText: txt(q('.hn-kpi__val', k)),
            valSize: getComputedStyle(q('.hn-kpi__val', k)).fontSize,
            sub: box(sub),
            subText: sub ? [...sub.children].filter(shown).map(txt).join('') : '',
            subClip: clip(sub),
            bars: box(q('.pt-bars', k)),
            seg: box(q('.hn-seg', k)),
            segs: qa('.hn-seg__s', k).map(box),
            legend: vbox(q('.hn-seg__legend', k)),
          };
        }),
        row: box(q('.hn-row', el)),
        blks: qa('.hn-blk', el).map((b) => ({
          key: (b.getAttribute('data-testid') ?? '').replace('hn-', ''),
          ...box(b),
          head: box(q('.hn-blk__head', b)),
          headText: txt(q('.hn-blk__head', b)),
        })),
        pools: qa('[data-testid^="hn-pool-"]', el).map((r) => ({
          ...box(r),
          name: txt(q('.hn-pool__name', r)),
          pill: txt(q('.pt-pill', r)),
          pct: box(q('.hn-pool__pct', r)),
          pctText: txt(q('.hn-pool__pct', r)),
          bar: box(q('.pt-progress', r)),
          sub: txt(q('.hn-pool__sub', r)),
        })),
        tiles: qa('[data-testid^="hn-disk-tile-"]', el).map((d) => ({
          ...box(d),
          name: txt(q('.hn-disk__name', d)),
          temp: txt(q('.hn-disk__temp', d)),
          warn: d.classList.contains('is-warn'),
          gauge: box(q('.hn-disk__gauge', d)),
          clip: Math.max(...['.hn-disk__name', '.hn-disk__sub'].map((c) => clip(q(c, d)))),
        })),
        localRows: qa('[data-testid="hn-local-storage"] .hn-pool', el).map((r) => ({
          ...box(r),
          name: txt(q('.hn-pool__name', r)),
          pctText: txt(q('.hn-pool__pct', r)),
          bar: box(q('.pt-progress', r)),
          sub: txt(q('.hn-pool__sub', r)),
        })),
        localConnect: txt(q('[data-testid="hn-local-connect"]', el)),
        acounts: qa('[data-testid^="hn-acount-"]', el).map((c) => ({ t: txt(c), ...box(c) })),
        arows: qa('.hn-arow', el).map((r) => ({ t: txt(q('.hn-arow__t', r)), ...box(r) })),
        apps: box(q('[data-testid="hn-apps"]', el)),
        appsText: txt(q('[data-testid="hn-apps"]', el)),
        msums: qa('.hn-msum', el).map((m) => ({ t: txt(m), ...box(m) })),
      };
    })(),
    /* 右栏（H2）：下载器、存储（NAS 概览没画本机的存储时）、状态，各一个区块标题加一张卡 */
    side: (() => {
      const el = q('[data-testid="hm-side"]');
      if (!el) return null;
      return {
        ...box(el),
        secs: qa('.hm-sec', el).map((s) => ({
          ...box(s),
          title: box(q('.hm-sec__title', s)),
          titleText: txt(q('.hm-sec__title', s)),
          countText: txt(q('.hm-sec__count', s)),
          link: box(q('.hm-sec__link', s)),
          linkText: txt(q('.hm-sec__link', s)),
          card: box(q('.hm-card', s)),
        })),
      };
    })(),
    speed: box(q('[data-testid="hm-speed"]')),
    speedCols: qa('[data-testid="hm-speed"] .hm-speed__col').map((el) => ({
      ...box(el),
      icon: box(q('.hm-speed__icon', el)),
      label: box(q('.hm-speed__label span', el)),
      labelText: txt(q('.hm-speed__label span', el)),
      value: box(q('.hm-speed__value', el)),
      bars: box(q('.pt-bars', el)),
    })),
    storage: box(q('[data-testid="hm-storage"]')),
    disks: qa('[data-testid^="hm-disk-"]').map((el) => ({
      ...box(el),
      name: box(q('.hm-disk__name', el)),
      nameText: txt(q('.hm-disk__name', el)),
      free: box(q('.hm-disk__free', el)),
      freeText: txt(q('.hm-disk__free', el)),
      bar: box(q('.hm-disk__bar', el)),
    })),
    dlPart: box(q('[data-testid="hm-downloaders"]')),
    dls: qa('[data-testid="hm-downloaders"] .hm-dl').map((el) => ({
      ...box(el),
      dot: box(q('.hm-dl__dot', el)),
      name: box(q('.hm-dl__name', el)),
      speed: box(q('.hm-dl__speed', el)),
    })),
    linesPart: box(q('[data-testid="hm-lines"]')),
    lines: qa('[data-testid^="hm-line-"]').map((el) => ({
      key: el.getAttribute('data-testid'),
      ...box(el),
      icon: box(q('.hm-line__ic', el)),
      title: box(q('.hm-line__title', el)),
      titleText: txt(q('.hm-line__title', el)),
      desc: box(q('.hm-line__desc', el)),
      right: box(q('.hm-line__right, .hm-line__act', el)),
      rightText: txt(q('.hm-line__right, .hm-line__act', el)),
    })),
  };
})()`;

/*
 * 等首页读到数据（今天播出出了卡，或空态那一句；NAS 概览出了四张数值卡，或空状态卡），再等过渡走完。
 * nas 是 NAS 概览画哪一种：truenas（默认，接入的 TrueNAS）或 local（只有本机来源），见 board-fixtures.mjs 的 NAS_VIEW。
 */
async function openHome(metrics, { empty = false, nas = "truenas" } = {}) {
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { deviceScaleFactor: 1, ...metrics },
    sessionId,
  );
  await goto(`${BASE}/#/home`);
  await ev(
    [
      empty
        ? `localStorage.setItem('pt-fixture-home-empty', '1')`
        : `localStorage.removeItem('pt-fixture-home-empty')`,
      nas === "local"
        ? `localStorage.setItem('pt-fixture-home-nas', 'local')`
        : `localStorage.removeItem('pt-fixture-home-nas')`,
    ].join(";"),
  );
  await cdp.send("Page.reload", {}, sessionId);
  await waitReady();
  for (let i = 0; i < 40; i++) {
    const ok = await ev(
      empty
        ? `Boolean(document.querySelector('[data-testid="hm-today-empty"]')) && Boolean(document.querySelector('[data-testid="hn-empty"]'))`
        : `document.querySelectorAll('.hm-tc').length > 0 && document.querySelectorAll('[data-testid^="hm-line-"]').length === 3 && document.querySelectorAll('.hn-kpi').length === 4`,
    ).catch(() => false);
    if (ok === true) {
      /* 数值卡的大字从旧值过渡到新值：连着两次读到的一样再量 */
      let last = null;
      for (let j = 0; j < 20; j++) {
        await sleep(200);
        const now = String(
          await ev(
            `[...document.querySelectorAll('[data-testid="hn-kpi-val"]')].map((e) => e.textContent).join('|')`,
          ).catch(() => ""),
        );
        if (now === last) break;
        last = now;
      }
      await sleep(600);
      return true;
    }
    await sleep(250);
  }
  return false;
}

if (doHome) {
  /* HM_SHOTS=<目录> 时每一趟量完存一张截图（整页，验收时逐张看） */
  const shot = async (name) => {
    if (!process.env.HM_SHOTS) return;
    const { data } = await cdp.send("Page.captureScreenshot", { format: "png" }, sessionId);
    writeFileSync(join(process.env.HM_SHOTS, `${name}.png`), Buffer.from(data, "base64"));
  };
  /* 整页：外壳的内容区自己滚，把视口临时拉高到放得下全部内容再截，截完换回来 */
  const shotFull = async (name, metrics) => {
    if (!process.env.HM_SHOTS) return;
    const extra = await ev(`(() => {
      const sc = document.querySelector('.pt-shell__content');
      return sc ? sc.scrollHeight - sc.clientHeight : 0;
    })()`).catch(() => 0);
    if (extra > 0) {
      await cdp.send(
        "Emulation.setDeviceMetricsOverride",
        { deviceScaleFactor: 1, ...metrics, height: metrics.height + extra },
        sessionId,
      );
      await sleep(400);
    }
    await shot(name);
    if (extra > 0) {
      await cdp.send(
        "Emulation.setDeviceMetricsOverride",
        { deviceScaleFactor: 1, ...metrics },
        sessionId,
      );
      await sleep(300);
    }
  };
  const homeFail = (key, msg, where = "") => {
    const gapKey = `/home${where} ${key}`;
    if (ALLOWED_GAPS[gapKey]) {
      gapsUsed.add(gapKey);
      return;
    }
    failures.push({
      route: `/home${where ? `（${where.replace("@", "")}）` : ""}`,
      board:
        where === "@375" ? "07 首页·手机" : where.startsWith("@check") ? "系统体检" : "02 首页",
      key,
      msg,
    });
  };
  const hmBox = (key, got, want, tol = 2, where = "") => {
    if (!got) {
      homeFail(key, "找不到这一块", where);
      return;
    }
    const diffs = Object.entries(want)
      .filter(([k, v]) => !near(got[k], v, tol))
      .map(([k, v]) => `${k} ${got[k]}≠${v}`);
    if (diffs.length > 0)
      homeFail(key, `${diffs.join("，")}（实测 ${JSON.stringify(got)}）`, where);
  };
  const rel = (b, o) => (b && o ? { x: b.x - o.x, y: b.y - o.y, w: b.w, h: b.h } : null);

  /*
   * ---- 桌面 1440×1024（home-nas-spec.md §2；媒体区块是画板 02，整体在 NAS 概览下面） ----
   * 首页加了 NAS 概览以后桌面总比视口高，滚动区有竖向滚动条：内容宽与右边让出 sb（画板按没有滚动条画）。
   */
  consoleErrors = [];
  if (!(await openHome({ width: 1440, height: 1024, mobile: false }))) {
    homeFail(
      "load",
      "等了 10 秒还没有今天播出的卡、三条状态行或四张数值卡：页面没进首页，或 /api/home、/api/nas 没回来",
    );
  } else {
    const m = await ev(MEASURE_HM);
    if (process.env.HM_DUMP) console.log(JSON.stringify(m, null, 1));
    await shot("home-1440");
    const W = 1144 - m.sb;
    const R = 280 + W;
    hmBox("head", m.head, { x: 264, y: 0, w: 1176, h: 64 });
    if (!m.refresh) homeFail("head.refresh", "页头右侧没有「刷新」");
    if (!m.headSub.startsWith("今天 5 集播出 · 其中 2 集已入库 · "))
      homeFail("head.sub", `副标题是「${m.headSub}」`);
    if (m.overflow > 0 || m.innerOverflow > 0)
      homeFail("overflow", `横向溢出 ${Math.max(m.overflow, m.innerOverflow)}px`);

    /* NAS 概览的标题行：标题 280,80；右边依次是「另有 1 个来源」、状态点与更新时间、「NAS 监控 ›」，彼此隔 8 */
    const H = m.hn;
    hmBox("nas.title", H?.title, { x: 280, y: 80, h: 21 });
    if (H?.titleText !== "NAS 概览" || H?.countText !== "TrueNAS")
      homeFail("nas.title", `标题是「${H?.titleText}」、类型是「${H?.countText}」`);
    if (!H?.link || !near(H.link.x + H.link.w, R, 2) || H.linkText !== "NAS 监控")
      homeFail("nas.link", `「NAS 监控」${JSON.stringify(H?.link)}，右边应到 ${R}`);
    if (!H?.live || !H.liveText.endsWith("更新") || !near(H.live.x + H.live.w, H.link.x - 8, 2))
      homeFail(
        "nas.live",
        `更新时间「${H?.liveText}」${JSON.stringify(H?.live)}：在「NAS 监控」左边 8`,
      );
    if (H?.liveDot?.w !== 7 || H?.liveDot?.h !== 7) homeFail("nas.live.dot", "状态点 7×7");
    if (
      H?.othersText !== "另有 1 个来源" ||
      !H?.others ||
      !near(H.others.x + H.others.w, H.live.x - 8, 2)
    )
      homeFail("nas.others", `「${H?.othersText}」${JSON.stringify(H?.others)}：在更新时间左边 8`);

    /* 卡：280,112 满宽；主机行 69 高（标志 40 在卡里 17,15；主机名 69,15；版本牌在主机名右边 8；型号、内存、运行时长在 39，隔 14） */
    const C = H?.card;
    hmBox("nas.card", C, { x: 280, y: 112, w: W });
    hmBox("nas.host", rel(H?.host, C), { x: 1, y: 1, w: W - 2, h: 69 });
    hmBox("nas.host.logo", rel(H?.logo, C), { x: 17, y: 15, w: 40, h: 40 });
    hmBox("nas.host.title", rel(H?.hostTitle, C), { x: 69, y: 15, h: 21 });
    if (H?.hostTitleText !== "truenas")
      homeFail("nas.host.title", `主机名是「${H?.hostTitleText}」`);
    if (
      H?.verText !== "TrueNAS SCALE 25.04.1" ||
      !H?.ver ||
      !near(H.ver.x, H.hostTitle.x + H.hostTitle.w + 8, 2) ||
      H.ver.h !== 20
    )
      homeFail(
        "nas.host.ver",
        `版本牌「${H?.verText}」${JSON.stringify(H?.ver)}：在主机名右边 8、20 高`,
      );
    const metaText = (H?.meta ?? []).map((x) => x.t).join("|");
    if (metaText !== "Intel Core i5-12400 · 12 核|64.0 GB|已运行 18 天 18 小时")
      homeFail("nas.host.meta", `主机行第二行是「${metaText}」`);
    (H?.meta ?? []).forEach((x, i) => {
      if (!near(x.y - C.y, 39, 2)) homeFail(`nas.host.meta[${i}]`, `y ${x.y - C.y}，应为 39`);
      if (i > 0 && !near(x.x - (H.meta[i - 1].x + H.meta[i - 1].w), 14, 1))
        homeFail(`nas.host.meta[${i}]`, "与前一项隔 14");
    });
    if (!H?.meta?.[0] || !near(H.meta[0].x - C.x, 69, 2))
      homeFail("nas.host.meta[0]", "第一项从 69 起");
    const facts = (H?.facts ?? []).map((x) => x.t).join("|");
    if (facts !== "2 条严重告警|tank 已用 85%") homeFail("nas.facts", `主机行右边是「${facts}」`);
    const lastFact = H?.facts?.[H.facts.length - 1];
    if (!lastFact || !near(lastFact.x + lastFact.w, R - 17, 2) || !near(lastFact.y - C.y, 25, 2))
      homeFail("nas.facts", `最后一个标签 ${JSON.stringify(lastFact)}：右边到 ${R - 17}、y 25`);

    /* 数值卡：卡里 17,84 起四张一行、间距 12、158 高；图标 24 在 15,13，标签 47，大字 45（26px），副行 78，柱 104（34 高） */
    const kw = (W - 70) / 4;
    const kpiText = (k) => `${k.labelText}=${k.valText}（${k.asideText}）${k.subText}`;
    const wantKpi = [
      "CPU=26%（12 核）负载 3.7 · IO 等待 1.2% · 52 °C",
      "内存=61%（共 64.0 GB）已用 39.2 GB · 可用 24.8 GB",
      "网络=↓3.4 MB/s（enp3s0）上行 1.8 MB/s · 所有网卡合计",
      "存储=75%（3 个存储池）已用 28.6 TB · 剩余 9.3 TB",
    ];
    if ((H?.kpis ?? []).length !== 4)
      homeFail("nas.kpis", `数值卡 ${H?.kpis?.length} 张，应为 4 张`);
    (H?.kpis ?? []).forEach((k, i) => {
      const at = `nas.kpi[${i}]`;
      hmBox(at, rel(k, C), { x: 17 + (kw + 12) * i, y: 84, w: kw, h: 158 }, 2);
      hmBox(`${at}.ic`, rel(k.ic, k), { x: 15, y: 13, w: 24, h: 24 });
      hmBox(`${at}.label`, rel(k.label, k), { x: 47 });
      hmBox(`${at}.val`, rel(k.val, k), { x: 15, y: 45, w: kw - 30 });
      hmBox(`${at}.sub`, rel(k.sub, k), { x: 15, y: 78, w: kw - 30, h: 16 });
      if (k.valSize !== "26px") homeFail(`${at}.val`, `大字 ${k.valSize}，应为 26px`);
      if (kpiText(k) !== wantKpi[i])
        homeFail(`${at}.text`, `是「${kpiText(k)}」，应为「${wantKpi[i]}」`);
      if (k.subClip > 0 || k.asideClip > 0)
        homeFail(`${at}.clip`, `文字被截断：副行多 ${k.subClip}px、右上角多 ${k.asideClip}px`);
      if (i < 3) hmBox(`${at}.bars`, rel(k.bars, k), { x: 15, y: 104, w: kw - 30, h: 34 });
      else {
        /* 存储卡：分段条 108（12 高），三段按容量，图例 132 */
        hmBox(`${at}.seg`, rel(k.seg, k), { x: 15, y: 108, w: kw - 30, h: 12 });
        hmBox(`${at}.legend`, rel(k.legend, k), { x: 15, y: 132 });
        if (k.segs.length !== 3)
          homeFail(`${at}.seg`, `分段 ${k.segs.length} 段，应为三个池各一段`);
      }
    });

    /* 第二行：卡里 17,254 起，存储池 : 硬盘 : 告警 = 1.1 : 1 : 1、间距 12、一样高；块头在块里 15,13 */
    const row = H?.row;
    hmBox("nas.row", rel(row, C), { x: 17, y: 254, w: W - 34 });
    const blkKeys = (H?.blks ?? []).map((b) => b.key).join("|");
    if (blkKeys !== "pools|disks|alerts") homeFail("nas.blocks", `第二行是「${blkKeys}」`);
    const unit = (W - 34 - 24) / 3.1;
    let bx = row?.x ?? 0;
    (H?.blks ?? []).forEach((b, i) => {
      const bw = (i === 0 ? 1.1 : 1) * unit;
      hmBox(`nas.blk[${i}]`, b, { x: bx, y: row?.y, w: bw, h: row?.h }, 2);
      hmBox(`nas.blk[${i}].head`, rel(b.head, b), { x: 15, y: 13, h: 18 });
      bx += bw + 12;
    });
    const [pb, db, ab] = H?.blks ?? [];
    /* 存储池：每个池一行（从块里 39 起、71 高），百分比右边到块里 w − 15，条 7 高；≥ 80% 写「快满了」 */
    if ((H?.pools ?? []).length !== 3)
      homeFail("nas.pools", `存储池 ${H?.pools?.length} 个，假数据 3 个`);
    (H?.pools ?? []).forEach((p, i) => {
      hmBox(`nas.pool[${i}]`, rel(p, pb), { x: 15, y: 39 + 71 * i, w: pb.w - 30 }, 2);
      hmBox(`nas.pool[${i}].bar`, rel(p.bar, p), { x: 0, w: pb.w - 30, h: 7 });
      if (!p.pct || !near(p.pct.x + p.pct.w - pb.x, pb.w - 15, 2))
        homeFail(
          `nas.pool[${i}].pct`,
          `百分比 ${JSON.stringify(p.pct)}，右边应到块里 ${pb.w - 15}`,
        );
    });
    const poolText = (H?.pools ?? [])
      .map((p) => `${p.name} ${p.pill} ${p.pctText} ${p.sub}`)
      .join("|");
    if (
      poolText !==
      "tank ONLINE 85% 已用 24.7 TB · 共 28.9 TB · 4 块硬盘 · 快满了|fast ONLINE 33% 已用 614 GB · 共 1.8 TB · 2 块硬盘|backup ONLINE 46% 已用 3.3 TB · 共 7.2 TB · 2 块硬盘"
    )
      homeFail("nas.pools.text", `存储池是「${poolText}」`);
    /* 硬盘：方块三列、间距 8、81 高，从块里 15,39 起；sdc 52 °C 到提醒线，nvme1n1 没有温度 */
    const tw = ((db?.w ?? 0) - 30 - 16) / 3;
    if ((H?.tiles ?? []).length !== 8)
      homeFail("nas.disks", `硬盘方块 ${H?.tiles?.length} 个，假数据 8 块`);
    (H?.tiles ?? []).forEach((t, i) =>
      hmBox(
        `nas.disk[${i}]`,
        rel(t, db),
        { x: 15 + (tw + 8) * (i % 3), y: 39 + 89 * Math.floor(i / 3), w: tw, h: 81 },
        2,
      ),
    );
    const tiles = (H?.tiles ?? []).map((t) => `${t.name}=${t.temp}${t.warn ? "!" : ""}`).join("|");
    if (
      tiles !==
      "sda=38°C|sdb=41°C|sdc=52°C!|sdd=40°C|nvme0n1=47°C|nvme1n1=没有温度|sde=36°C|sdf=37°C"
    )
      homeFail("nas.disks.text", `硬盘是「${tiles}」`);
    if (!db?.headText?.includes("最高 52 °C · sdc"))
      homeFail("nas.disks.head", `块头是「${db?.headText}」`);
    (H?.tiles ?? []).forEach((t, i) => {
      if (t.clip > 0)
        homeFail(`nas.disk[${i}].clip`, `「${t.name}」的名字或类型容量被截断 ${t.clip}px`);
    });
    /* 告警：三个计数块（34 高、间距 8）、最新三条（30 高）、底下是应用那一行（贴着块底 15） */
    const cw3 = ((ab?.w ?? 0) - 30 - 16) / 3;
    (H?.acounts ?? []).forEach((c, i) =>
      hmBox(
        `nas.alert.count[${i}]`,
        rel(c, ab),
        { x: 15 + (cw3 + 8) * i, y: 39, w: cw3, h: 34 },
        2,
      ),
    );
    const counts = (H?.acounts ?? []).map((c) => c.t).join("|");
    if (counts !== "2严重|1警告|4信息") homeFail("nas.alert.counts", `计数是「${counts}」`);
    (H?.arows ?? []).forEach((r, i) =>
      hmBox(`nas.alert.row[${i}]`, rel(r, ab), { x: 15, y: 81 + 30 * i, w: ab.w - 30, h: 30 }, 2),
    );
    const arows = (H?.arows ?? []).map((r) => r.t).join("|");
    if (arows !== "Rsync 任务「每日备份」失败|应用目录同步失败|Rsync 任务「相册」成功")
      homeFail("nas.alert.rows", `最新三条是「${arows}」`);
    if (!H?.apps || !near(ab.y + ab.h - (H.apps.y + H.apps.h), 15, 2))
      homeFail("nas.apps", `应用那一行 ${JSON.stringify(H?.apps)}：底边离块底 15`);
    if (H?.appsText !== "应用 运行中 5 个 · 共 6 个 · 1 个可更新")
      homeFail("nas.apps.text", `应用那一行是「${H?.appsText}」`);
    if (C && row && !near(C.y + C.h - (row.y + row.h), 17, 2))
      homeFail("nas.card.bottom", `第二行底下到卡底 ${C.y + C.h - (row.y + row.h)}，应为 17`);

    /* 媒体区块整体在 NAS 概览下面 16：画板 02 的 y 全部加 dy；内容宽少了 sb，五张、三张、七张按 1134 平分 */
    const dy = H ? H.y + H.h + 16 - 80 : 0;
    hmBox("today.title", m.today?.title, { x: 280, y: 80 + dy, h: 21 });
    if (m.today?.countText !== "5 集") homeFail("today.count", `计数是「${m.today?.countText}」`);
    if (
      !m.today?.link ||
      !near(m.today.link.x + m.today.link.w, R, 2) ||
      m.today.linkText !== "看日历"
    )
      homeFail("today.link", `「看日历」${JSON.stringify(m.today?.link)}，右边应到 ${R}`);
    if (m.tcs.length !== 5) homeFail("today.cards", `今天播出有 ${m.tcs.length} 张卡，假数据 5 张`);
    const tcw = (W - 64) / 5;
    m.tcs.forEach((c, i) => {
      hmBox(`today.card[${i}]`, c, { x: 280 + (tcw + 16) * i, y: 112 + dy, w: tcw, h: 200 });
      hmBox(`today.card[${i}].img`, rel(c.img, c), { x: 0, y: 0, w: tcw, h: 122 }, 2);
      hmBox(`today.card[${i}].state`, rel(c.state, c), { x: 8, y: 8, h: 20 }, 2);
      if (c.time) {
        const t = rel(c.time, c);
        if (!near(t.x + t.w, tcw - 8, 2) || !near(t.y + t.h, 114, 2) || t.h !== 20)
          homeFail(
            `today.card[${i}].time`,
            `时刻牌 ${JSON.stringify(t)}：右下各留 8（右边 ${tcw - 8}、底 114）`,
          );
      } else homeFail(`today.card[${i}].time`, "按播出时间追剧的卡要有时刻牌");
      hmBox(`today.card[${i}].title`, rel(c.title, c), { x: 12, y: 132 }, 2);
      hmBox(`today.card[${i}].sub`, rel(c.sub, c), { x: 12, y: 152 }, 2);
      hmBox(`today.card[${i}].act`, rel(c.act, c), { x: 12, y: 172 }, 2);
    });
    const acts = m.tcs.map((c) => c.actText).join("|");
    if (acts !== "播出后 2 小时开始搜|播放|下载中，预计 12 分钟|搜资源|播放")
      homeFail("today.acts", `动作行是「${acts}」`);
    if (m.rowScroll > 0)
      homeFail("today.fit", `五张卡一行放不下（多 ${m.rowScroll}px）：有滚动条时卡要缩到 ${tcw}`);

    /* 待处理条：满宽 48；图标 16,16、标题 40、计数 92、chip 从 132 起 h26 间距 8、「全部」右边到 W − 16 */
    hmBox("todo", m.todo, { x: 280, y: 328 + dy, w: W, h: 48 });
    hmBox("todo.icon", rel(m.todoIcon, m.todo), { x: 16, y: 16, w: 16, h: 16 }, 2);
    hmBox("todo.title", rel(m.todoTitle, m.todo), { x: 40 }, 2);
    hmBox("todo.count", rel(m.todoCount, m.todo), { x: 92, y: 15, h: 18 }, 2);
    if (m.chips.length !== 6)
      homeFail("todo.chips", `待处理有 ${m.chips.length} 个 chip，假数据 6 类`);
    m.chips.forEach((c, i) => {
      const r = rel(c, m.todo);
      if (i === 0) hmBox("todo.chip[0]", r, { x: 132, y: 11, h: 26 }, 2);
      else {
        const prev = m.chips[i - 1];
        if (!near(c.x - (prev.x + prev.w), 8, 1) || !near(r.y, 11, 1) || c.h !== 26)
          homeFail(`todo.chip[${i}]`, `「${c.t}」${JSON.stringify(r)}：同一行、间距 8、h26`);
      }
    });
    const all = rel(m.todoAll, m.todo);
    if (!all || !near(all.x + all.w, W - 16, 2))
      homeFail("todo.all", `「全部」${JSON.stringify(all)}，右边应到 ${W - 16}`);

    /* 两栏：左栏 W − 368 − 16、右栏 368，都从 392 + dy 起 */
    const LW = W - 368 - 16;
    hmBox("resume.title", m.resume?.title, { x: 280, y: 392 + dy, h: 21 });
    if (m.resume?.tag !== "Jellyfin · admin")
      homeFail("resume.tag", `身份 chip 是「${m.resume?.tag}」`);
    if (
      !m.resume?.link ||
      !near(m.resume.link.x + m.resume.link.w, 280 + LW, 2) ||
      m.resume.linkText !== "在 Jellyfin 打开"
    )
      homeFail(
        "resume.link",
        `「在 Jellyfin 打开」${JSON.stringify(m.resume?.link)}，右边应到 ${280 + LW}`,
      );
    if (m.rcs.length !== 3)
      homeFail("resume.cards", `继续观看有 ${m.rcs.length} 张卡，假数据 3 张`);
    const rcw = (LW - 34) / 3;
    m.rcs.forEach((c, i) => {
      hmBox(`resume.card[${i}]`, c, { x: 280 + (rcw + 17) * i, y: 424 + dy, w: rcw, h: 182 }, 2);
      hmBox(`resume.card[${i}].img`, rel(c.img, c), { x: 0, y: 0, w: rcw - 2, h: 136 }, 2);
      hmBox(
        `resume.card[${i}].play`,
        rel(c.play, c),
        { x: (rcw - 40) / 2, y: 48, w: 40, h: 40 },
        2,
      );
      hmBox(`resume.card[${i}].track`, rel(c.track, c), { x: 0, y: 132, w: rcw - 2, h: 4 }, 2);
      hmBox(`resume.card[${i}].title`, rel(c.title, c), { x: 12, y: 145 }, 2);
      hmBox(`resume.card[${i}].sub`, rel(c.sub, c), { x: 12, y: 163 }, 2);
    });
    hmBox("recent.title", m.recent?.title, { x: 280, y: 626 + dy, h: 21 });
    if (m.recent?.countText !== "今天 7")
      homeFail("recent.count", `计数是「${m.recent?.countText}」`);
    const pw = (LW - 72) / 7;
    const shownPosters = m.pcs.filter((c) => c.y === m.pcs[0]?.y && c.x + c.w <= 280 + LW + 1);
    if (shownPosters.length !== 7)
      homeFail("recent.posters", `一行放得下的海报 ${shownPosters.length} 张，应为 7 张`);
    m.pcs.slice(0, 7).forEach((c, i) => {
      hmBox(
        `recent.poster[${i}]`,
        rel(c.poster, { x: 0, y: 0 }),
        { x: 280 + (pw + 12) * i, y: 658 + dy, w: pw, h: pw * 1.5 },
        2,
      );
      hmBox(`recent.poster[${i}].q`, rel(c.q, c), { x: 6, y: 6, h: 17 }, 2);
      hmBox(`recent.poster[${i}].title`, rel(c.title, c), { x: 0, y: pw * 1.5 + 6, w: pw }, 2);
      hmBox(`recent.poster[${i}].sub`, rel(c.sub, c), { x: 0, y: pw * 1.5 + 24 }, 2);
    });
    hmBox("week", m.week, { x: 280, y: 864 + dy, w: LW, h: 116 });
    if (m.week?.headText !== "这周还要播") homeFail("week.head", `卡头是「${m.week?.headText}」`);
    if (m.wks.length !== 4) homeFail("week.cols", `这周还要播有 ${m.wks.length} 列，假数据 4 条`);
    const colW = (LW - 16) / 4;
    m.wks.forEach((c, i) => {
      hmBox(`week.col[${i}].date`, rel(c.date, m.week), { x: 16 + colW * i, y: 42 }, 2);
      hmBox(`week.col[${i}].title`, rel(c.title, m.week), { x: 16 + colW * i, y: 60 }, 2);
      hmBox(`week.col[${i}].note`, rel(c.note, m.week), { x: 16 + colW * i, y: 80 }, 2);
      if (i === 0 && c.line) homeFail("week.col[0].rule", "第一列前不要竖线");
      if (i > 0 && (!c.line || c.line.left !== "-12px" || c.line.h !== "52px"))
        homeFail(`week.col[${i}].rule`, `竖线 ${JSON.stringify(c.line)}：在文字左边 12、52 高`);
    });

    /* 右栏（home-nas-spec.md §4）：下载器、存储、状态三个区块，标题 21 高、到卡顶 32，区块间 20 */
    const S = m.side;
    hmBox("side", S, { x: R - 368, y: 392 + dy, w: 368 }, 2);
    const secs = (S?.secs ?? [])
      .map((x) => `${x.titleText}（${x.countText}）${x.linkText}`)
      .join("|");
    if (secs !== "下载器（2 台）任务列表|存储（下载盘与媒体库）|状态（）系统体检")
      homeFail("side.secs", `右栏是「${secs}」`);
    (S?.secs ?? []).forEach((x, i) => {
      hmBox(`side.sec[${i}].title`, x.title, { x: R - 368, h: 21 }, 2);
      hmBox(`side.sec[${i}].card`, x.card, { x: R - 368, y: x.title.y + 32, w: 368 }, 2);
      if (x.link && !near(x.link.x + x.link.w, R, 2))
        homeFail(
          `side.sec[${i}].link`,
          `「${x.linkText}」${JSON.stringify(x.link)}，右边应到 ${R}`,
        );
      const prev = S.secs[i - 1];
      if (prev && !near(x.y - (prev.y + prev.h), 20, 1))
        homeFail(`side.sec[${i}]`, `与上一个区块隔 ${x.y - (prev.y + prev.h)}，应为 20`);
    });
    const [dlSec, stSec, lnSec] = S?.secs ?? [];
    const DC = dlSec?.card;
    /* 下载器卡：两列速度（17、192，标签 14、大字 32、柱 59 高 26）；下面各台下载器一行 26（圆点 17,120，名字 31,114，速度右边到 351） */
    m.speedCols.forEach((c, i) => {
      const x = i === 0 ? 17 : 192;
      hmBox(`dl.speed[${i}].icon`, rel(c.icon, DC), { x, y: 14, w: 14, h: 14 }, 2);
      hmBox(`dl.speed[${i}].label`, rel(c.label, DC), { x: x + 20, y: 14 }, 2);
      hmBox(`dl.speed[${i}].value`, rel(c.value, DC), { x, y: 32 }, 2);
      hmBox(`dl.speed[${i}].bars`, rel(c.bars, DC), { x, y: 59, w: 159, h: 26 }, 2);
    });
    const labels = m.speedCols.map((c) => c.labelText).join("|");
    if (labels !== "下载 · 下载中 4|上传 · 做种 1,284")
      homeFail("dl.speed.labels", `标签是「${labels}」`);
    if (m.dls.length !== 2) homeFail("dl.rows", `下载器 ${m.dls.length} 台，假数据 2 台`);
    m.dls.forEach((d, i) => {
      hmBox(`dl.row[${i}].dot`, rel(d.dot, DC), { x: 17, y: 120 + 26 * i, w: 7, h: 7 }, 2);
      hmBox(`dl.row[${i}].name`, rel(d.name, DC), { x: 31, y: 114 + 26 * i }, 2);
      const sp = rel(d.speed, DC);
      if (!sp || !near(sp.x + sp.w, 351, 2))
        homeFail(`dl.row[${i}].speed`, `速度 ${JSON.stringify(sp)}，右边应到 351`);
    });
    /* 存储卡：每块盘一行 42（名字 17,13 + 42i，条 35 + 42i、334×5，剩余右边到 351） */
    const SC = stSec?.card;
    if (m.disks.length !== 3) homeFail("disks", `存储有 ${m.disks.length} 块盘，假数据 3 块`);
    const diskNames = m.disks.map((d) => d.nameText).join("|");
    if (diskNames !== "下载盘 · qb、tr /downloads|媒体库 · 电影 /media|媒体库 · 剧集 /tv")
      homeFail("disks.names", `盘的名字是「${diskNames}」（名字与路径之间要有空格）`);
    m.disks.forEach((d, i) => {
      hmBox(`disk[${i}].name`, rel(d.name, SC), { x: 17, y: 13 + 42 * i }, 2);
      hmBox(`disk[${i}].bar`, rel(d.bar, SC), { x: 17, y: 35 + 42 * i, w: 334, h: 5 }, 2);
      const f = rel(d.free, SC);
      if (!f || !near(f.x + f.w, 351, 2))
        homeFail(`disk[${i}].free`, `剩余 ${JSON.stringify(f)}，右边应到 351`);
    });
    /* 状态卡：三行各 52（图标 28 在 17,13，标题 55,9，说明 55,28，右侧右边到 351） */
    const LC = lnSec?.card;
    if (m.lines.length !== 3)
      homeFail("lines", `状态行有 ${m.lines.length} 条，应是体检、备份、媒体服务器`);
    m.lines.forEach((l, i) => {
      const y = 52 * i;
      hmBox(`line[${i}]`, rel(l, LC), { x: 1, y: y + 1, h: 52 }, 2);
      hmBox(`line[${i}].icon`, rel(l.icon, LC), { x: 17, y: y + 13, w: 28, h: 28 }, 2);
      hmBox(`line[${i}].title`, rel(l.title, LC), { x: 55, y: y + 9 }, 2);
      hmBox(`line[${i}].desc`, rel(l.desc, LC), { x: 55, y: y + 28 }, 2);
      const rt = rel(l.right, LC);
      if (!rt || !near(rt.x + rt.w, 351, 2))
        homeFail(`line[${i}].right`, `右侧 ${JSON.stringify(rt)}，右边应到 351`);
    });
    const titles = m.lines.map((l) => l.titleText).join("|");
    if (titles !== "体检：13 项通过，1 项提醒|备份：3 天没成功|Jellyfin 12.2 · 14:00 同步")
      homeFail("lines.titles", `状态行标题是「${titles}」`);

    /* 探针：「全部」打开右侧 440 宽的抽屉，一件事一行；点一个去别的页面的 chip，地址带着筛选 */
    const drawer = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        document.querySelector('[data-testid="hm-todo-all"]')?.click();
        let d = null;
        for (let i = 0; i < 20; i++) {
          await wait(100);
          d = [...document.querySelectorAll('.hm-todo-drawer')].find((e) => e.getBoundingClientRect().width > 0);
          if (d) break;
        }
        if (!d) return 'no-drawer';
        await wait(400);
        const r = d.getBoundingClientRect();
        const rows = d.querySelectorAll('.hm-row').length;
        const acts = [...d.querySelectorAll('.hm-row .el-button')].map((b) => (b.textContent ?? '').trim()).join(',');
        const over = d.scrollWidth - d.clientWidth;
        d.querySelector('.el-drawer__close-btn')?.click();
        await wait(500);
        const ok = Math.round(r.width) === 440 && Math.round(r.right) === innerWidth && rows === 7 && acts.includes('处理,忽略') && over <= 0;
        return 'verdict=' + (ok ? 'ok' : 'MISMATCH') + ' 宽 ' + Math.round(r.width) + ' 右 ' + Math.round(r.right) + ' 行 ' + rows + ' 按钮 ' + acts + ' 溢出 ' + over;
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(drawer, "verdict=ok").length > 0)
      homeFail("probe.drawer", `待处理抽屉：${drawer}`);
    const chip = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        document.querySelector('[data-testid="hm-chip-organize_failed"]')?.click();
        for (let i = 0; i < 20 && !location.hash.startsWith('#/media/history'); i++) await wait(100);
        const at = location.hash;
        history.back();
        for (let i = 0; i < 20 && !location.hash.startsWith('#/home'); i++) await wait(100);
        return 'verdict=' + (at === '#/media/history?status=failed' ? 'ok' : 'MISMATCH') + ' ' + at;
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(chip, "verdict=ok").length > 0)
      homeFail("probe.chip", `「整理失败」chip：${chip}`);
  }
  for (const err of [...new Set(consoleErrors)].slice(0, 3)) homeFail("console", err);

  /*
   * ---- 1200×900：内容 894（< 1100 且 ≤ 1080）：右栏落到左栏下面、占满宽；
   * NAS 概览的数值卡仍是四张一行（卡里窄到 210 以下，副行换成短的），第二行两列、告警一块跨两列 ----
   */
  consoleErrors = [];
  if (await openHome({ width: 1200, height: 900, mobile: false })) {
    const m = await ev(MEASURE_HM);
    await shot("home-1200");
    const W = 1200 - 264 - 32 - m.sb;
    const weekBottom = m.week ? m.week.y + m.week.h : 0;
    hmBox("@1200 side", m.side, { x: 280, w: W }, 2, "@1200");
    if (m.side && !near(m.side.y - weekBottom, 16, 2))
      homeFail(
        "@1200 side",
        `右栏的顶 ${m.side.y} 到这周还要播的底 ${weekBottom} 应隔 16`,
        "@1200",
      );
    hmBox("@1200 week", m.week, { x: 280, w: W }, 2, "@1200");
    hmBox("@1200 nas.card", m.hn?.card, { x: 280, w: W }, 2, "@1200");
    const kw = (W - 70) / 4;
    const shorts = (m.hn?.kpis ?? []).map((k) => k.subText).join("|");
    if (shorts !== "负载 3.7 · 52 °C|已用 39.2 GB|上行 1.8 MB/s|剩余 9.3 TB")
      homeFail(
        "@1200 nas.kpi.sub",
        `卡宽 ${kw}（里面窄到 210 以下）时副行是「${shorts}」`,
        "@1200",
      );
    (m.hn?.kpis ?? []).forEach((k, i) => {
      hmBox(`@1200 nas.kpi[${i}]`, k, { x: 297 + (kw + 12) * i, w: kw }, 2, "@1200");
      if (k.subClip > 0 || k.asideClip > 0)
        homeFail(
          `@1200 nas.kpi[${i}].clip`,
          `文字被截断：副行多 ${k.subClip}px、右上角多 ${k.asideClip}px`,
          "@1200",
        );
    });
    const row = m.hn?.row;
    const [pb, db, ab] = m.hn?.blks ?? [];
    const half = row ? (row.w - 12) / 2 : 0;
    hmBox("@1200 nas.blk[0]", pb, { x: row?.x, y: row?.y, w: half }, 2, "@1200");
    hmBox("@1200 nas.blk[1]", db, { x: (row?.x ?? 0) + half + 12, y: row?.y, w: half }, 2, "@1200");
    if (pb && db && pb.h !== db.h) homeFail("@1200 nas.blk[0]", "存储池与硬盘两块一样高", "@1200");
    hmBox(
      "@1200 nas.blk[2]",
      ab,
      { x: row?.x, y: pb ? pb.y + pb.h + 12 : 0, w: row?.w },
      2,
      "@1200",
    );
    if (m.overflow > 0 || m.innerOverflow > 0)
      homeFail("@1200 overflow", `横向溢出 ${Math.max(m.overflow, m.innerOverflow)}px`, "@1200");
    /* 今天播出一行排不下：横向滚，两端有翻页按钮，点「下一页」往后滚 */
    const page = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        const row = document.querySelector('[data-testid="hm-today-row"]');
        const next = document.querySelector('[data-testid="hm-today-next"]');
        if (!row || !next) return 'no-pager ' + Boolean(row) + ' ' + Boolean(next);
        const s0 = row.scrollLeft;
        next.click();
        await wait(900);
        const s1 = row.scrollLeft;
        const prev = Boolean(document.querySelector('[data-testid="hm-today-prev"]'));
        const rows = new Set([...row.querySelectorAll('.hm-tc')].map((c) => Math.round(c.getBoundingClientRect().top))).size;
        return 'verdict=' + (s1 > s0 && prev && rows === 1 ? 'ok' : 'MISMATCH') + ' 滚 ' + s0 + '→' + s1 + ' 上一页 ' + prev + ' 行 ' + rows;
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(page, "verdict=ok").length > 0)
      homeFail("@1200 probe.pager", `今天播出翻页：${page}`, "@1200");
  } else homeFail("@1200 load", "1200 下首页没读到数据", "@1200");
  for (const err of [...new Set(consoleErrors)].slice(0, 3))
    homeFail("@1200 console", err, "@1200");

  /* ---- 手机 375×812（画板 07）：区块标题 16,y；标题顶到内容顶 32；块与块之间统一 24（偏离 ④） ---- */
  consoleErrors = [];
  if (!(await openHome({ width: 375, height: 812, mobile: true }))) {
    homeFail("load", "手机上首页没读到数据", "@375");
  } else {
    const m = await ev(MEASURE_HM);
    if (process.env.HM_DUMP) console.log(JSON.stringify(m, null, 1));
    await shotFull("home-375", { width: 375, height: 812, mobile: true });
    const fail = (k, msg) => homeFail(k, msg, "@375");
    const box = (k, got, want, tol = 2) => hmBox(k, got, want, tol, "@375");
    /* 无头 Chrome 的竖向滚动条占 sb（画板按没有滚动条画）：卡宽 343 − sb，右边 359 − sb，卡里右边 329 − sb */
    const sb = m.sb;
    const cw = 343 - sb;
    if (m.overflow > 0 || m.innerOverflow > 0)
      fail("overflow", `横向溢出 ${Math.max(m.overflow, m.innerOverflow)}px`);
    if (m.mheadSub !== "今天 5 集播出 · 7 件事要处理")
      fail("head.sub", `顶栏副标题是「${m.mheadSub}」（画板 07 不写入库的那段）`);
    if (m.refresh) fail("head.refresh", "手机上页头不放「刷新」（在页面底部）");
    if (!m.mrefresh) fail("refresh", "手机上页面底部要有「刷新」");
    const sec = (k, s, y) => {
      box(`${k}.title`, s?.title, { x: 16, y, h: 21 });
      if (s?.link && !near(s.link.x + s.link.w, 359 - sb, 2))
        fail(`${k}.link`, `文字链 ${JSON.stringify(s.link)}，右边应到 ${359 - sb}`);
    };
    /*
     * NAS 概览在最上面（home-nas-spec.md §5）：标题 16,68，卡 16,100 宽 cw；主机行在卡里 15 起、标志 34；
     * 数值卡 2×2（间距 8、126 高，大字 22px、柱 22 高，副行写短的）；第二行只有存储池；告警与硬盘各一行摘要（38 高、间距 10）
     */
    const H = m.hn;
    box("nas.title", H?.title, { x: 16, y: 68, h: 21 });
    if (H?.linkText !== "NAS 监控" || !H?.link || !near(H.link.x + H.link.w, 359 - sb, 2))
      fail("nas.link", `「NAS 监控」${JSON.stringify(H?.link)}，右边应到 ${359 - sb}`);
    if (H?.others) fail("nas.others", "手机上标题行不写其余来源（在 NAS 监控里看）");
    if (!H?.live || !near(H.live.x + H.live.w, H.link.x - 8, 2))
      fail("nas.live", `更新时间 ${JSON.stringify(H?.live)}：在「NAS 监控」左边 8`);
    const C = H?.card;
    box("nas.card", C, { x: 16, y: 100, w: cw });
    box("nas.host", rel(H?.host, C), { x: 1, y: 1, w: cw - 2 });
    box("nas.host.logo", rel(H?.logo, C), { x: 15, w: 34, h: 34 });
    if (H?.facts?.length) fail("nas.facts", "手机上主机行右边不放标签");
    const kw = (cw - 38) / 2;
    const hostBottom = H?.host ? H.host.y + H.host.h : 0;
    (H?.kpis ?? []).forEach((k, i) => {
      const y0 = hostBottom + 12 + (126 + 8) * Math.floor(i / 2);
      box(`nas.kpi[${i}]`, k, { x: 16 + 15 + (kw + 8) * (i % 2), y: y0, w: kw, h: 126 });
      box(`nas.kpi[${i}].ic`, rel(k.ic, k), { x: 13, y: 11, w: 22, h: 22 });
      if (k.valSize !== "22px") fail(`nas.kpi[${i}].val`, `大字 ${k.valSize}，应为 22px`);
      if (k.subClip > 0 || k.asideClip > 0)
        fail(`nas.kpi[${i}].clip`, `文字被截断：副行多 ${k.subClip}px、右上角多 ${k.asideClip}px`);
      if (i < 3) box(`nas.kpi[${i}].bars`, rel(k.bars, k), { x: 13, y: 93, w: kw - 26, h: 22 });
      else if (k.legend) fail("nas.kpi[3].legend", "手机上存储卡不放图例");
    });
    const shortsM = (H?.kpis ?? []).map((k) => k.subText).join("|");
    if (shortsM !== "负载 3.7 · 52 °C|已用 39.2 GB|上行 1.8 MB/s|剩余 9.3 TB")
      fail("nas.kpi.sub", `手机上副行是「${shortsM}」`);
    const blkM = (H?.blks ?? []).map((b) => b.key).join("|");
    if (blkM !== "pools") fail("nas.blocks", `手机上第二行是「${blkM}」，只列存储池`);
    const kLast = H?.kpis?.[3];
    const pbM = H?.blks?.[0];
    box("nas.pools", pbM, { x: 31, y: kLast ? kLast.y + kLast.h + 10 : 0, w: cw - 30 });
    const sums = (H?.msums ?? []).map((x) => x.t).join("|");
    if (sums !== "告警 2 严重 · 1 警告 · 4 信息|硬盘 8 块 · 最高 52 °C（sdc）")
      fail("nas.msums", `摘要是「${sums}」`);
    let my = pbM ? pbM.y + pbM.h + 10 : 0;
    (H?.msums ?? []).forEach((x, i) => {
      box(`nas.msum[${i}]`, x, { x: 31, y: my, w: cw - 30, h: 38 });
      my = x.y + x.h + 10;
    });
    const lastSum = H?.msums?.[H.msums.length - 1];
    if (C && lastSum && !near(C.y + C.h - (lastSum.y + lastSum.h), 15, 2))
      fail("nas.card.bottom", `最后一行摘要到卡底 ${C.y + C.h - (lastSum.y + lastSum.h)}，应为 15`);
    /* 下面是画板 07 的区块，整体在 NAS 概览下面 24 */
    const dyM = H ? H.y + H.h + 24 - 68 : 0;
    sec("today", m.today, 68 + dyM);
    m.tcs.slice(0, 2).forEach((c, i) => {
      box(`today.card[${i}]`, c, { x: 16 + 252 * i, y: 100 + dyM, w: 240, h: 196 });
      box(`today.card[${i}].img`, rel(c.img, c), { x: 0, y: 0, w: 240, h: 135 });
      box(`today.card[${i}].state`, rel(c.state, c), { x: 8, y: 8, h: 20 });
      box(`today.card[${i}].title`, rel(c.title, c), { x: 12, y: 144 });
      box(`today.card[${i}].sub`, rel(c.sub, c), { x: 12, y: 164 });
    });
    if (!m.tcs[0]?.subText?.includes("20:00 播出"))
      fail("today.sub", `时刻要并进副行：「${m.tcs[0]?.subText}」`);
    if (m.tcs.some((c) => c.act)) fail("today.acts", "手机上没有动作行（整张卡可点）");
    if (m.dots.length !== 5) fail("today.dots", `翻页圆点 ${m.dots.length} 个，五张卡五个`);
    else {
      const x0 = Math.round((375 - sb) / 2 - 27);
      m.dots.forEach((d, i) =>
        box(`today.dot[${i}]`, d, { x: x0 + 12 * i, y: 304 + dyM, w: 6, h: 6 }, 2),
      );
    }
    const dotsBottom = m.dots.length ? m.dots[0].y + m.dots[0].h : 0;
    /* 待处理卡：343 宽，标题行在卡里；chip h30 从 14,42 起 */
    box("todo", m.todo, { x: 16, w: cw });
    if (m.todo && !near(m.todo.y - dotsBottom, 24, 2))
      fail("todo.gap", `圆点的底 ${dotsBottom} 到待处理卡 ${m.todo.y} 应隔 24`);
    box("todo.icon", rel(m.todoIcon, m.todo), { x: 14, y: 13, w: 16, h: 16 });
    box("todo.title", rel(m.todoTitle, m.todo), { x: 38, y: 11 });
    box("todo.count", rel(m.todoCount, m.todo), { x: 90, y: 11 });
    const allM = rel(m.todoAll, m.todo);
    if (!allM || !near(allM.x + allM.w, 329 - sb, 2) || !near(allM.y, 12, 2))
      fail("todo.all", `「全部」${JSON.stringify(allM)}，右边到 ${329 - sb}、y 12`);
    if (m.chips.length) {
      box("todo.chip[0]", rel(m.chips[0], m.todo), { x: 14, y: 42, h: 30 });
      const tops = [...new Set(m.chips.map((c) => c.y))].sort((a, b) => a - b);
      for (let i = 1; i < tops.length; i++)
        if (!near(tops[i] - tops[i - 1], 36, 1))
          fail("todo.chips", `chip 的行距 ${tops[i] - tops[i - 1]}，应为 36`);
      const last = Math.max(...m.chips.map((c) => c.y + c.h));
      if (m.todo && !near(m.todo.y + m.todo.h - last, 8, 2))
        fail("todo.bottom", `最后一行 chip 底下留 ${m.todo.y + m.todo.h - last}，应为 8`);
      if (m.chips.some((c) => c.x + c.w > m.todo.x + 329 - sb + 1))
        fail("todo.wrap", "chip 要在卡里右边 14 以内换行");
    }
    const todoBottom = m.todo ? m.todo.y + m.todo.h : 0;
    sec("resume", m.resume, todoBottom + 24);
    if (m.resume?.linkText !== "Jellyfin")
      fail("resume.link", `手机上文字链只写服务器名：「${m.resume?.linkText}」`);
    const rTop = m.resume?.title ? m.resume.title.y + 32 : 0;
    m.rcs.slice(0, 1).forEach((c) => {
      box("resume.card[0]", c, { x: 16, y: rTop, w: 240, h: 188 });
      box("resume.card[0].img", rel(c.img, c), { x: 0, y: 0, w: 240, h: 135 });
      box("resume.card[0].play", rel(c.play, c), { x: 100, y: 47, w: 40, h: 40 });
      box("resume.card[0].track", rel(c.track, c), { x: 0, y: 131, w: 240, h: 4 });
      box("resume.card[0].title", rel(c.title, c), { x: 12, y: 144 });
      box("resume.card[0].sub", rel(c.sub, c), { x: 12, y: 163 });
    });
    if (m.rcs[1]) box("resume.card[1]", m.rcs[1], { x: 268, y: rTop, w: 240, h: 188 });
    const resumeBottom = rTop + 188;
    sec("recent", m.recent, resumeBottom + 24);
    const pTop = m.recent?.title ? m.recent.title.y + 32 : 0;
    m.pcs.slice(0, 3).forEach((c, i) => {
      box(`recent.poster[${i}]`, rel(c.poster, { x: 0, y: 0 }), {
        x: 16 + 112 * i,
        y: pTop,
        w: 100,
        h: 150,
      });
      box(`recent.poster[${i}].title`, rel(c.title, c), { x: 0, y: 156, w: 100 });
      box(`recent.poster[${i}].sub`, rel(c.sub, c), { x: 0, y: 174 });
    });
    const recentBottom = m.pcs[0] ? m.pcs[0].y + m.pcs[0].h : 0;
    /* 这周还要播：标题在卡外（画板 07 的 week / title），卡 343 宽 */
    sec("week", m.weekSec, recentBottom + 24);
    if (m.weekSec?.linkText !== "看日历") fail("week.link", `文字链是「${m.weekSec?.linkText}」`);
    const wc = m.week?.card;
    box("week.card", wc, { x: 16, y: (m.weekSec?.title?.y ?? 0) + 32, w: cw, h: 208 });
    m.wks.forEach((c, i) => {
      box(`week.row[${i}]`, rel(c, wc), { x: 14, y: 52 * i, h: 52 }, 2);
      box(`week.row[${i}].date`, rel(c.date, wc), { x: 14, y: 52 * i + 17 }, 2);
      box(`week.row[${i}].title`, rel(c.title, wc), { x: 96, y: 52 * i + 9 }, 2);
      box(`week.row[${i}].note`, rel(c.note, wc), { x: 96, y: 52 * i + 28 }, 2);
    });
    const weekBottom = wc ? wc.y + wc.h : 0;
    /*
     * 右栏的三个区块（home-nas-spec.md §5）：下载器、存储、状态，标题在卡外，在这周还要播下面 24、区块间 24；
     * 下载器卡只有两列速度（不列各台下载器），存储列下载盘与媒体库各一块，状态三行各 48、没有说明行
     */
    const S = m.side;
    const secsM = (S?.secs ?? [])
      .map((x) => `${x.titleText}（${x.countText}）${x.linkText}`)
      .join("|");
    if (secsM !== "下载器（2 台）任务列表|存储（下载盘与媒体库）|状态（）系统体检")
      fail("side.secs", `右栏是「${secsM}」`);
    let sy = weekBottom + 24;
    (S?.secs ?? []).forEach((x, i) => {
      box(`side.sec[${i}].title`, x.title, { x: 16, y: sy, h: 21 });
      box(`side.sec[${i}].card`, x.card, { x: 16, y: sy + 32, w: cw });
      if (x.link && !near(x.link.x + x.link.w, 359 - sb, 2))
        fail(
          `side.sec[${i}].link`,
          `「${x.linkText}」${JSON.stringify(x.link)}，右边应到 ${359 - sb}`,
        );
      sy = x.y + x.h + 24;
    });
    const [dlSecM, stSecM, lnSecM] = S?.secs ?? [];
    const DC = dlSecM?.card;
    const bw = (cw - 48) / 2;
    m.speedCols.forEach((c, i) => {
      const x = i === 0 ? 15 : 15 + bw + 18;
      box(`dl.speed[${i}].icon`, rel(c.icon, DC), { x, y: 14, w: 14, h: 14 });
      box(`dl.speed[${i}].label`, rel(c.label, DC), { x: x + 20, y: 14 });
      box(`dl.speed[${i}].value`, rel(c.value, DC), { x, y: 32 });
      box(`dl.speed[${i}].bars`, rel(c.bars, DC), { x, y: 59, w: bw, h: 24 });
    });
    if (m.speedCols.map((c) => c.labelText).join("|") !== "下载|上传")
      fail(
        "dl.speed.labels",
        `手机上标签只写「下载」「上传」：「${m.speedCols.map((c) => c.labelText).join("|")}」`,
      );
    if (m.dlPart) fail("dl.rows", "手机上不列各台下载器");
    const SC = stSecM?.card;
    if (m.disks.length !== 2)
      fail("disks", `手机上存储列 ${m.disks.length} 块，应是下载盘与媒体库盘各一块`);
    m.disks.forEach((d, i) => {
      box(`disk[${i}].name`, rel(d.name, SC), { x: 15, y: 12 + 42 * i });
      box(`disk[${i}].bar`, rel(d.bar, SC), { x: 15, y: 34 + 42 * i, w: cw - 30, h: 5 });
      const f = rel(d.free, SC);
      if (!f || !near(f.x + f.w, cw - 15, 2))
        fail(`disk[${i}].free`, `剩余 ${JSON.stringify(f)}，右边应到卡里 ${cw - 15}`);
    });
    const LCm = lnSecM?.card;
    m.lines.forEach((l, i) => {
      const y = 1 + 48 * i;
      box(`line[${i}]`, rel(l, LCm), { y, h: 48 }, 2);
      box(`line[${i}].icon`, rel(l.icon, LCm), { x: 15, y: y + 10, w: 28, h: 28 });
      box(`line[${i}].title`, rel(l.title, LCm), { x: 53, y: y + 15 });
      if (l.desc) fail(`line[${i}].desc`, "手机上状态行没有说明行");
      const rt = rel(l.right, LCm);
      if (!rt || !near(rt.x + rt.w, cw - 15, 2))
        fail(`line[${i}].right`, `右侧 ${JSON.stringify(rt)}，右边应到卡里 ${cw - 15}`);
    });
    /* 探针：「全部」打开全屏的列表 */
    const full = String(
      await ev(`(async () => {
        const wait = (ms) => new Promise((r) => setTimeout(r, ms));
        document.querySelector('[data-testid="hm-todo-all"]')?.click();
        let d = null;
        for (let i = 0; i < 20; i++) {
          await wait(100);
          d = [...document.querySelectorAll('.hm-todo-drawer')].find((e) => e.getBoundingClientRect().width > 0);
          if (d) break;
        }
        if (!d) return 'no-drawer';
        await wait(400);
        const r = d.getBoundingClientRect();
        const over = d.scrollWidth - d.clientWidth;
        d.querySelector('.el-drawer__close-btn')?.click();
        await wait(500);
        return 'verdict=' + (Math.round(r.width) === innerWidth && over <= 0 ? 'ok' : 'MISMATCH') + ' 宽 ' + Math.round(r.width) + ' 溢出 ' + over;
      })()`).catch((e) => `probe 失败：${e.message}`),
    );
    if (probeMiss(full, "verdict=ok").length > 0) fail("probe.drawer", `手机上「全部」：${full}`);
  }
  for (const err of [...new Set(consoleErrors)].slice(0, 3)) homeFail("console", err, "@375");

  /* ---- 空态（1440 与 375）：今天播出、待处理、最近入库、这周还要播各写一句，继续观看整块不显示 ---- */
  for (const [where, metrics] of [
    ["@empty", { width: 1440, height: 1024, mobile: false }],
    ["@empty375", { width: 375, height: 812, mobile: true }],
  ]) {
    consoleErrors = [];
    if (!(await openHome(metrics, { empty: true }))) {
      homeFail("load", "空态下首页没读到数据", where);
      continue;
    }
    const m = await ev(MEASURE_HM);
    await shotFull(`home-${where.slice(1)}`, metrics);
    const fail = (k, msg) => homeFail(k, msg, where);
    if (m.today?.empty !== "还没有追剧") fail("today.empty", `今天播出写的是「${m.today?.empty}」`);
    if (m.today?.linkText !== "去探索")
      fail("today.link", `没有追剧时文字链是「${m.today?.linkText}」`);
    if (m.todoNote !== "没有要处理的事") fail("todo.empty", `待处理写的是「${m.todoNote}」`);
    if (m.resume) fail("resume", "没连媒体服务器时继续观看整块不显示");
    if (m.recent?.empty !== "最近没有入库")
      fail("recent.empty", `最近入库写的是「${m.recent?.empty}」`);
    if (m.today?.countText || m.recent?.countText)
      fail(
        "counts",
        `没有数据时标题旁不写计数：「${m.today?.countText}」「${m.recent?.countText}」`,
      );
    if (m.weekEmpty !== "这周没有要播的集")
      fail("week.empty", `这周还要播写的是「${m.weekEmpty}」`);
    const titles = m.lines.map((l) => l.titleText).join("|");
    if (titles !== "体检：没体检过|备份：每天定时备份关着|媒体服务器")
      fail("lines", `状态行是「${titles}」`);
    /* NAS 概览：一个能显示的来源都没有时是一张空状态卡（图标 48、标题、说明与「接入 NAS」），不画数据卡与更新时间 */
    const H = m.hn;
    if (H?.emptyTitle !== "还没有 NAS 监控来源")
      fail("nas.empty", `空状态卡写的是「${H?.emptyTitle}」`);
    if (!H?.emptySub?.startsWith("接入 TrueNAS 以后") || !H.emptySub.includes("本机来源已关闭"))
      fail("nas.empty.sub", `说明是「${H?.emptySub}」`);
    if (H?.emptyBtnText !== "接入 NAS") fail("nas.empty.btn", `按钮是「${H?.emptyBtnText}」`);
    if (H?.card || H?.live || H?.countText)
      fail("nas.empty", "没有来源时不画数据卡、更新时间与类型");
    const mob = where === "@empty375";
    hmBox(
      "nas.empty.card",
      H?.empty,
      { x: mob ? 16 : 280, y: mob ? 100 : 112, w: (mob ? 343 : 1144) - m.sb },
      2,
      where,
    );
    hmBox("nas.empty.icon", rel(H?.emptyIcon, H?.empty), { w: 48, h: 48 }, 2, where);
    const secsE = (m.side?.secs ?? []).map((x) => x.titleText).join("|");
    if (secsE !== "下载器|存储|状态") fail("side.secs", `右栏是「${secsE}」`);
    if (m.overflow > 0 || m.innerOverflow > 0)
      fail("overflow", `横向溢出 ${Math.max(m.overflow, m.innerOverflow)}px`);
    for (const err of [...new Set(consoleErrors)].slice(0, 3)) fail("console", err);
  }
  /*
   * ---- 只有本机来源（1440，home-nas-spec.md §3）：网络一格换成下载器速度；第二行是存储（首页的下载盘与媒体库）
   * 与一块虚线框「接入 NAS」；右栏不再列存储 ----
   */
  consoleErrors = [];
  if (!(await openHome({ width: 1440, height: 1024, mobile: false }, { nas: "local" }))) {
    homeFail("load", "只有本机来源时首页没读到数据", "@local");
  } else {
    const m = await ev(MEASURE_HM);
    await shot("home-local");
    const fail = (k, msg) => homeFail(k, msg, "@local");
    const H = m.hn;
    if (H?.countText !== "本机") fail("nas.title", `类型是「${H?.countText}」`);
    if (H?.others) fail("nas.others", "只有本机来源时标题行不写其余来源");
    if (H?.hostTitleText !== "pt-tools 所在主机" || H?.verText !== "Linux 6.12.15")
      fail("nas.host", `主机行是「${H?.hostTitleText}」「${H?.verText}」`);
    if (H?.facts?.length) fail("nas.facts", "本机来源不放提醒标签");
    const heads = (H?.kpis ?? []).map((k) => `${k.labelText}（${k.asideText}）`).join("|");
    if (heads !== "CPU（8 核）|内存（共 32.0 GB）|下载器速度（本机读不到网卡）|存储（所在的盘）")
      fail("nas.kpis", `数值卡是「${heads}」`);
    const netSub = H?.kpis?.[2]?.subText ?? "";
    if (!/^上传 .+ · 所有下载器合计$/.test(netSub))
      fail("nas.kpi[2].sub", `下载器速度的副行是「${netSub}」`);
    (H?.kpis ?? []).forEach((k, i) => {
      if (k.subClip > 0 || k.asideClip > 0)
        fail(`nas.kpi[${i}].clip`, `文字被截断：副行多 ${k.subClip}px、右上角多 ${k.asideClip}px`);
    });
    const row = H?.row;
    const keys = (H?.blks ?? []).map((b) => b.key).join("|");
    if (keys !== "local-storage|local-more") fail("nas.blocks", `第二行是「${keys}」`);
    const half = row ? (row.w - 12) / 2 : 0;
    (H?.blks ?? []).forEach((b, i) =>
      hmBox(
        `nas.blk[${i}]`,
        b,
        { x: (row?.x ?? 0) + (half + 12) * i, y: row?.y, w: half, h: row?.h },
        2,
        "@local",
      ),
    );
    const lrows = (H?.localRows ?? []).map((r) => r.name).join("|");
    if (lrows !== "下载盘 · qb、tr /downloads|媒体库 · 电影 /media|媒体库 · 剧集 /tv")
      fail("nas.local.rows", `存储列的是「${lrows}」`);
    if (H?.localConnect !== "接入 NAS")
      fail("nas.local.connect", `虚线框里的链接是「${H?.localConnect}」`);
    const secsL = (m.side?.secs ?? []).map((x) => x.titleText).join("|");
    if (secsL !== "下载器|状态")
      fail("side.secs", `右栏是「${secsL}」（本机的存储已经画在 NAS 概览里）`);
    if (m.overflow > 0 || m.innerOverflow > 0)
      fail("overflow", `横向溢出 ${Math.max(m.overflow, m.innerOverflow)}px`);
  }
  for (const err of [...new Set(consoleErrors)].slice(0, 3)) homeFail("console", err, "@local");
  await ev(
    `localStorage.removeItem('pt-fixture-home-empty'); localStorage.removeItem('pt-fixture-home-nas')`,
  ).catch(() => {});

  /*
   * ---- 系统体检（画板没有，照预览 system-check.html，首页 NAS 概览 H1）：1440 与 375 ----
   * 总览卡一张；「需要处理」按失败、提醒排，「去处理」去对应页面；「全部检查项」十组按顺序、每项都在；
   * 几何（问题卡网格、分组卡瀑布流、手机环形图 92 与 2×2 计数块）；文字换行不裁切；两个筛选；
   * 动效：正常时卡片有动画与过渡，系统要求减少动态效果时 animationDuration 与 transitionDuration 都是 0s。
   */
  const SC_PROBS = "downloader:2,site:pttime,library:1,backup";
  const SC_GO = "#/downloaders,#/sites/pttime,#/media/library,#/global";
  const scMotion = (reduced) => `(() => {
    const durs = (v) => v.split(',').map((s) => parseFloat(s) * (s.trim().endsWith('ms') ? 1 : 1000));
    const cards = [...document.querySelectorAll('.sc-hero, .sc-prob, .sc-grp')];
    const bad = [];
    for (const el of cards) {
      const cs = getComputedStyle(el);
      const a = durs(cs.animationDuration);
      const t = el.classList.contains('sc-hero') ? [] : durs(cs.transitionDuration);
      const want = ${reduced} ? (d) => d === 0 : (d) => d > 0;
      if (!a.every(want) || !t.every(want))
        bad.push((el.dataset.testid || el.className) + ' ' + cs.animationDuration + ' / ' + cs.transitionDuration);
    }
    return { cards: cards.length, bad };
  })()`;
  for (const [where, metrics] of [
    ["@check", { width: 1440, height: 1024, mobile: false }],
    ["@check375", { width: 375, height: 812, mobile: true }],
  ]) {
    consoleErrors = [];
    await cdp.send(
      "Emulation.setDeviceMetricsOverride",
      { deviceScaleFactor: 1, ...metrics },
      sessionId,
    );
    const loadCheck = async () => {
      await goto(`${BASE}/#/system/check`);
      await cdp.send("Page.reload", {}, sessionId);
      await waitReady();
      for (let i = 0; i < 40; i++) {
        const ok = await ev(
          `document.querySelectorAll('[data-testid^="sc-item-"]').length > 0`,
        ).catch(() => false);
        if (ok === true) break;
        await sleep(250);
      }
      /* 卡片依次出现：等动画都放完（最多 3 秒）再量与截图 */
      await ev(
        `Promise.race([Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {}))), new Promise((r) => setTimeout(r, 3000))]).then(() => true)`,
      ).catch(() => {});
      await sleep(100);
      return (
        (await ev(`document.querySelectorAll('[data-testid^="sc-item-"]').length`).catch(() => 0)) >
        0
      );
    };
    const fail = (k, msg) => homeFail(k, msg, where);
    if (!(await loadCheck())) {
      fail("load", "系统体检没列出任何一项");
      continue;
    }
    await shotFull(`system-${where.slice(1)}`, metrics);
    const runSel =
      where === "@check"
        ? '#pt-head-acts [data-testid="sc-run"]'
        : '.sc-mbar [data-testid="sc-run"]';
    const got = await ev(`(async () => {
      const wait = (ms) => new Promise((r) => setTimeout(r, ms));
      const box = (el) => {
        const r = el.getBoundingClientRect();
        return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
      };
      const inner = document.querySelector('.pt-shell__inner');
      /* 内容区按内容盒量：桌面 .pt-cards 有内边距，手机没有 */
      const pageEl = document.querySelector('[data-testid="system-check"]');
      const pcs = getComputedStyle(pageEl);
      const pb = box(pageEl);
      const page = {
        x: pb.x + Math.round(parseFloat(pcs.paddingLeft)),
        w: pb.w - Math.round(parseFloat(pcs.paddingLeft) + parseFloat(pcs.paddingRight)),
      };
      const heroes = [...document.querySelectorAll('[data-testid="sc-hero"]')];
      const probs = [...document.querySelectorAll('.sc-prob')];
      const groups = [...document.querySelectorAll('.sc-grp')];
      const statuses = () => [...document.querySelectorAll('[data-testid^="sc-item-"]')]
        .map((el) => [...el.classList].find((c) => c.startsWith('st-'))).join(',');
      const clipped = [...document.querySelectorAll('.sc-prob__title, .sc-prob__detail, .sc-prob__hint, .sc-row__title, .sc-row__detail, .sc-prob, .sc-grp')]
        .filter((el) => el.scrollWidth > el.clientWidth + 1 || el.scrollHeight > el.clientHeight + 1)
        .map((el) => el.className.split(' ')[0]);
      const over = Math.max(document.documentElement.scrollWidth - innerWidth,
        inner ? inner.scrollWidth - Math.round(inner.getBoundingClientRect().width) : 0);
      const ring = document.querySelector('.sc-ring');
      const strip = document.querySelector('[data-testid="sc-strip"]');
      const out = {
        page,
        heroes: heroes.length,
        hero: heroes[0] ? box(heroes[0]) : null,
        ringW: ring ? Math.round(ring.getBoundingClientRect().width) : 0,
        tileYs: [...new Set([...document.querySelectorAll('[data-testid^="sc-tile-"]')].map((el) => box(el).y))].length,
        stripOver: strip ? strip.scrollWidth - strip.clientWidth : 0,
        probs: probs.map((el) => el.dataset.testid.slice(8)).join(','),
        probBoxes: probs.map(box),
        go: probs.map((el) => el.querySelector('[data-testid="sc-prob-go"]')?.getAttribute('href') ?? '').join(','),
        hints: document.querySelectorAll('.sc-prob__hint').length,
        groups: groups.map((el) => el.dataset.testid.slice(9)).join(','),
        groupBoxes: groups.map(box),
        items: document.querySelectorAll('[data-testid^="sc-item-"]').length,
        run: Boolean(document.querySelector(${JSON.stringify(runSel)})),
        clipped,
        over,
      };
      const sw = document.querySelector('[data-testid="sc-filter"]');
      sw?.click();
      await wait(400);
      out.onlyProblems = statuses();
      sw?.click();
      await wait(400);
      document.querySelector('[data-testid="sc-tile-fail"]')?.click();
      await wait(400);
      out.onlyFail = statuses();
      out.chip = (document.querySelector('[data-testid="sc-status-chip"]')?.textContent ?? '').trim();
      document.querySelector('[data-testid="sc-tile-fail"]')?.click();
      await wait(400);
      out.restored = document.querySelectorAll('[data-testid^="sc-item-"]').length;
      return out;
    })()`);
    if (got.heroes !== 1) fail("hero", `总览卡有 ${got.heroes} 张`);
    else if (Math.abs(got.hero.w - got.page.w) > 1)
      fail("hero.w", `总览卡宽 ${got.hero.w}，内容区宽 ${got.page.w}（应满宽）`);
    if (got.ringW !== (metrics.mobile ? 92 : 148))
      fail("ring", `环形图宽 ${got.ringW}，应为 ${metrics.mobile ? 92 : 148}`);
    if (got.tileYs !== (metrics.mobile ? 2 : 1))
      fail("tiles", `四个计数块排成 ${got.tileYs} 行（桌面一行，手机 2×2）`);
    if (metrics.mobile && got.stripOver <= 0)
      fail("strip", "手机上各组状态条没有横向滚动（十组放不下一行）");
    if (got.probs !== SC_PROBS) fail("probs", `问题卡依次是「${got.probs}」，应为「${SC_PROBS}」`);
    if (got.go !== SC_GO) fail("go", `「去处理」去的是「${got.go}」，应为「${SC_GO}」`);
    if (got.hints !== 4)
      fail("hints", `问题卡上「怎么修」有 ${got.hints} 条，假数据有问题的 4 项各一条`);
    if (metrics.mobile) {
      if (got.probBoxes.some((b) => Math.abs(b.w - got.page.w) > 1 || b.x !== got.probBoxes[0].x))
        fail("probs.grid", "手机上问题卡不是满宽一列");
      if (got.groupBoxes.some((b) => Math.abs(b.w - got.page.w) > 1 || b.x !== got.groupBoxes[0].x))
        fail("groups.grid", "手机上分组卡不是满宽一列");
    } else {
      const cols = Math.max(1, Math.floor((got.page.w + 12) / 352));
      const first = got.probBoxes.slice(0, cols);
      if (first.some((b) => b.y !== first[0].y))
        fail("probs.grid", `问题卡第一行没有排满 ${cols} 列`);
      if (got.probBoxes.some((b) => b.w < 340 || Math.abs(b.w - got.probBoxes[0].w) > 1))
        fail(
          "probs.w",
          `问题卡宽 ${got.probBoxes.map((b) => b.w).join("/")}（应一样宽、不小于 340）`,
        );
      const lefts = new Set(got.groupBoxes.map((b) => b.x));
      if (lefts.size < 2)
        fail("groups.cols", `分组卡只排了 ${lefts.size} 列（瀑布流，桌面至少两列）`);
      if (got.groupBoxes.some((b) => b.w < 300))
        fail("groups.w", `分组卡最窄 ${Math.min(...got.groupBoxes.map((b) => b.w))}px`);
    }
    if (got.groups !== "tmdb,sites,downloaders,servers,paths,libraries,disks,time,backup,secret")
      fail("groups", `分组是「${got.groups}」`);
    if (got.items !== 15) fail("items", `列了 ${got.items} 项，假数据 15 项`);
    if (!got.run) fail("run", "「立即体检」不在该在的地方（桌面在页头，手机在总览卡里）");
    if (got.clipped.length)
      fail(
        "clipped",
        `有 ${got.clipped.length} 处文字被裁切（应换行）：${got.clipped.slice(0, 3).join("、")}`,
      );
    if (got.over > 0) fail("overflow", `横向溢出 ${got.over}px`);
    if (got.onlyProblems !== "st-warn,st-fail,st-warn,st-warn")
      fail("filter", `「只看有问题的」留下的是「${got.onlyProblems}」`);
    if (got.onlyFail !== "st-fail" || got.chip !== "只看失败")
      fail("tile.filter", `点「失败」计数块留下「${got.onlyFail}」、标签「${got.chip}」`);
    if (got.restored !== 15) fail("tile.reset", `再点一次计数块后列了 ${got.restored} 项`);
    const normal = await ev(scMotion(false));
    if (!normal.cards || normal.bad.length)
      fail(
        "motion",
        `正常时有卡片没有动画或过渡：${normal.bad.slice(0, 2).join("；") || "没有卡片"}`,
      );
    for (const err of [...new Set(consoleErrors)].slice(0, 3)) fail("console", err);

    /* 系统要求减少动态效果：重新打开页面，卡片上没有动画与过渡 */
    await cdp.send(
      "Emulation.setEmulatedMedia",
      { features: [{ name: "prefers-reduced-motion", value: "reduce" }] },
      sessionId,
    );
    if (await loadCheck()) {
      const reduced = await ev(scMotion(true));
      if (!reduced.cards || reduced.bad.length)
        fail(
          "motion.reduced",
          `减少动态效果时还有动画或过渡：${reduced.bad.slice(0, 2).join("；") || "没有卡片"}`,
        );
      if (where === "@check") await shotFull("system-check-reduced", metrics);
    } else fail("motion.reduced", "减少动态效果时系统体检没列出任何一项");
    await cdp.send("Emulation.setEmulatedMedia", { features: [] }, sessionId);
  }
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { ...D_VIEWPORT, deviceScaleFactor: 1, mobile: false },
    sessionId,
  );
}

// ------------------------------------------------- 登录页（画板 43）

/*
 * 画板 43：桌面 1440×1024，左半 600 品牌区，右侧 400 宽的卡居中；失败态**卡内联**
 * （画板原话「建议改为卡内联，替换现有 alert()」）；移动端同一张卡、左右各 20 边距。
 *
 * 登录页在 SPA 之外（Go 模板），所以放在最后单独一趟：清掉 cookie 再打开它。
 * 这一趟跑完 cookie 就没了，后面不能再验别的路由 —— 所以它必须是最后一段。
 * Go 侧已有 login_page_test.go 断言模板内容与「没有 alert(」；这里补的是**真实渲染**：
 * 卡宽、失败态出现在卡里、以及 375 下不横向溢出。
 */
const doLogin = wanted.length === 0 || wanted.includes("/login");

if (doLogin) {
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { width: 1440, height: 1024, deviceScaleFactor: 1, mobile: false },
    sessionId,
  );
  await cdp.send("Network.enable", {}, sessionId).catch(() => {});
  await cdp.send("Network.clearBrowserCookies", {}, sessionId).catch(() => {});
  consoleErrors = [];
  await goto(`${BASE}/login`);
  await sleep(600);

  const loginFail = (key, msg) => {
    const gapKey = `/login ${key}`;
    if (ALLOWED_GAPS[gapKey]) {
      gapsUsed.add(gapKey);
      return;
    }
    failures.push({ route: "/login", board: "43 登录", key, msg });
  };

  const card = await ev(`(() => {
    const el = document.querySelector('.login-card');
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { w: Math.round(r.width), h: Math.round(r.height) };
  })()`);
  if (!card) loginFail("card", "找不到 .login-card");
  else if (!near(card.w, 400, 8)) loginFail("card", `登录卡宽 ${card.w}，画板 400`);

  /* 失败态：填错密码提交，错误必须出现在卡里（不是 alert） */
  const failState = await ev(`(async () => {
    const form = document.querySelector('.login-form');
    if (!form) return 'no-form';
    const setVal = (el, v) => {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setter.call(el, v);
      el.dispatchEvent(new Event('input', { bubbles: true }));
    };
    const inputs = [...form.querySelectorAll('input')];
    if (inputs.length < 2) return 'no-inputs';
    setVal(inputs[0], 'admin');
    setVal(inputs[1], 'definitely-wrong-password');
    let alerted = false;
    const realAlert = window.alert;
    window.alert = () => { alerted = true; };
    /*
     * 用 requestSubmit 而不是点第一个 button：表单里第一个 button 是「显示密码」，
     * 点它只会切 input.type，根本不会提交 —— 那样这条探针永远量到「没有失败态」。
     */
    form.requestSubmit();
    await new Promise((r) => setTimeout(r, 2000));
    window.alert = realAlert;
    const box = document.querySelector('.login-alert');
    /* 这个盒子一直在 DOM 里，靠 is-open 显形（见 server.go 的 showError） */
    const shown = box ? box.classList.contains('is-open') : false;
    const text = box ? (box.textContent ?? '').replace(/\\s+/g, ' ').trim() : '';
    return 'verdict=' + (shown && !alerted ? 'ok' : 'MISMATCH') +
      ' 卡内联=' + shown + ' alert=' + alerted + ' 文案=' + text.slice(0, 40);
  })()`).catch((e) => `probe 失败：${e.message}`);
  const miss = probeMiss(String(failState), "verdict=ok");
  if (miss.length > 0) loginFail("failState", `失败态没有卡内联：${failState}`);

  /* 移动端同一张卡，左右各 20 —— 375 下不该横向溢出 */
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { width: 375, height: 812, deviceScaleFactor: 1, mobile: true },
    sessionId,
  );
  await goto(`${BASE}/login`);
  await sleep(600);
  const mOverflow = await ev(`document.documentElement.scrollWidth - window.innerWidth`);
  if (mOverflow > 0) loginFail("overflow", `375 下登录页横向溢出 ${mOverflow}px`);
  for (const err of [...new Set(consoleErrors)].slice(0, 3)) loginFail("console", err);
}

// ---------------------------------------------------------------- 报告

const stale = Object.keys(ALLOWED_GAPS).filter(
  (k) =>
    !gapsUsed.has(k) &&
    /* --route 只跑子集时，别把没跑到的路由的偏离判成多余。移动端的键是 `<路由>@375 <项>` */
    (wanted.length === 0 ||
      wanted.some((r) => k.startsWith(`${r} `) || k.startsWith(`${r}@375 `)) ||
      (doEntry && k.startsWith("/search?entry")) ||
      (doItem && k.startsWith("/media/item")) ||
      (doHome && k.startsWith("/home"))),
);

for (const f of failures) {
  console.log(`✗ ${f.route.padEnd(28)} [画板 ${f.board}] ${f.key}: ${f.msg}`);
}
/*
 * 同一条原因会被很多路由触发（rail 那条外壳级偏离每页都量得到），逐条打出来是 21 行
 * 一样的话，真正的信息反而被埋了。按原因归并，列出受影响的检查项。
 */
const byReason = new Map();
for (const k of gapsUsed) {
  const reason = ALLOWED_GAPS[k];
  const list = byReason.get(reason) ?? [];
  list.push(k);
  byReason.set(reason, list);
}
for (const [reason, keys] of byReason) {
  const where =
    keys.length > 3 ? `${keys.length} 条路由的 ${keys[0].split(" ")[1]}` : keys.join("、");
  console.log(`· 已记偏离 ${where} —— ${reason}`);
}

/*
 * 「画板画了、落地没有」单独再报一遍。
 *
 * 混在上面那串「已记偏离」里等于藏起来：那串里大多数是外壳级的一条原因乘以 21 条路由，
 * 真正的设计缺项被稀释掉了，而收尾那句还是「与画板一致」。
 */
const unimplemented = [...gapsUsed].filter((k) => ALLOWED_GAPS[k].startsWith(UNIMPL));
const ownerAccepted = [...gapsUsed].filter((k) => ALLOWED_GAPS[k].startsWith(OWNER_OK));
if (unimplemented.length > 0) {
  console.log(`\n⚠ 画板画了、落地没有，等 owner 拍板（${unimplemented.length} 项）：`);
  for (const k of unimplemented) {
    console.log(`    ${k} —— ${ALLOWED_GAPS[k].slice(UNIMPL.length)}`);
  }
}
if (ownerAccepted.length > 0) {
  console.log(`\n· 画板画了、落地没有，owner 已逐项接受（${ownerAccepted.length} 项）：`);
  for (const k of ownerAccepted) {
    console.log(`    ${k} —— ${ALLOWED_GAPS[k].slice(OWNER_OK.length)}`);
  }
}
for (const k of stale) {
  console.log(`? 偏离登记多余（这一项其实已经对上了，删掉它）：${k}`);
}

/* 桌面与移动分开报数：把两者合成一个数字会让「覆盖了多少画板」重新变得含糊 */
const scope =
  `桌面 ${routes.length} 条 + 移动 ${mobileRoutes.length} 条` +
  (emptyRoutes.length > 0 ? ` + 空态 ${emptyRoutes.length} 条` : "") +
  (doEntry ? " + 按条目搜索（1440、1200、375）" : "") +
  (doItem ? " + 条目详情（1440 剧集与电影、1200、375）" : "") +
  (doHome ? " + 首页（1440、1200、375、空态、本机来源）与系统体检" : "") +
  (doLogin ? " + 登录页" : "");
if (failures.length === 0 && stale.length === 0) {
  /*
   * 绝不单独输出「与画板一致」。退 0 只说明「没有未登记的差异」，
   * 而登记在案的设计缺项一样是差异 —— 那句话得把它们带上才不算替我说谎。
   */
  const tail = [
    unimplemented.length > 0 ? `${unimplemented.length} 项等 owner 拍板` : "",
    ownerAccepted.length > 0 ? `${ownerAccepted.length} 项未落地但 owner 已逐项接受` : "",
  ].filter(Boolean);
  console.log(
    `\n${scope}：没有未登记的差异；另有 ${gapsUsed.size} 条已记偏离` +
      (tail.length > 0 ? `，其中 ${tail.join("、")}（见上）` : ""),
  );
} else {
  console.log(`\n${scope}：${failures.length} 处与画板不一致，${stale.length} 条偏离登记多余`);
}

/*
 * 退出码三档，而不是两档。
 *
 * 一次评审判「完成门禁继续把未实现设计当成功」—— 判得对：只要还有画板画了而落地没有的项，
 * 退 0 就等于替我说了一句「按设计稿实现完了」。但也不该一律退 1：那会把「结构对不上」
 * 和「结构对上了、只剩几项待 owner 拍板」混成一件事，前者是缺陷，后者是待决范围。
 *
 *   0 —— 没有未登记差异，且没有「等 owner 拍板」的未落地项。
 *   2 —— 结构与画板一致，但还有 N 项未落地的设计项等 owner 逐项接受。
 *   1 —— 有未登记的差异，或偏离登记多余。
 *
 * owner 拍过板的项记在 ALLOWED_GAPS 里（OWNER_OK 前缀），不再计入退 2 ——
 * 逐项记，而不是给一个全盘接受的开关：新冒出来的未落地项不在那张表里，照旧退 2。
 */
let code = 0;
if (failures.length > 0 || stale.length > 0) {
  code = 1;
} else if (unimplemented.length > 0) {
  code = 2;
  console.log(
    `退出码 2：结构与画板一致，但还有 ${unimplemented.length} 项画板画了而落地没有 —— ` +
      "需要 owner 逐项接受（接受后把那几条的前缀从 UNIMPL 换成 OWNER_OK）。",
  );
}

chrome.kill("SIGKILL");
process.exit(code);
