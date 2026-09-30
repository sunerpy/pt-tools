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
import { existsSync, mkdtempSync } from "node:fs";
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
     * + chip-1「排序: 分享率」+ 右端三枚图标钮。周期那枚没落地（聚合接口没有历史序列），
     * 偏离见 ALLOWED_GAPS。
     */
    needsSeg: true,
    /* 「周期」按画板列进来，落地没有 —— 让它红，原因在 ALLOWED_GAPS 里 */
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
          const pickMode = (label) => {
            const el = [...document.querySelectorAll('.el-segmented__item')]
              .find((b) => (b.textContent ?? '').trim() === label);
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
          if (!pickMode('侧边')) { window.fetch = realFetch; return 'no-mode-seg'; }
          await wait(400);
          if (!(await openRow())) { window.fetch = realFetch; return 'no-detail-btn'; }
          const drawer = document.querySelector('.hub__drawer');
          const dw = drawer ? Math.round(drawer.getBoundingClientRect().width) : 0;
          const dtext = drawer ? (drawer.textContent ?? '').replace(/\\s+/g, ' ') : '';
          esc();
          await wait(500);
          if (!pickMode('下方')) { window.fetch = realFetch; return 'no-mode-seg-2'; }
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
        desc: "状态栏调度器浮层 = 画板 41 的 sched-pop（300 宽，含状态与同步口径）",
        /* 胶囊里是状态本身（运行中 / 已停止 / 状态未知），不是又一遍「调度器」 */
        want: "宽 300|调度器|状态未知|最后同步|停止所有任务|启动所有任务",
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
    cards: [612, 612, 612], // p-pending / p-active / p-note
    minCards: 3, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["待绑定", "已绑定", "绑定是怎么走的"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
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
    cards: [1080], // p-cfg（画板另有一条 warn 提示，那是状态不是常驻卡）
    minCards: 1, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["全局配置"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
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
  "/userinfo controls(周期)":
    OWNER_OK +
    "画板 10 的 chip-0 是「周期: 本周」，落地没有：聚合接口只回当前快照，没有历史序列，" +
    "按周期筛在数据上不成立 —— 产品一条历史快照都不存，要做得新增快照表 + 采集任务 + 迁移。" +
    OWNER_DECISION,

  "/tasks controls.seg":
    OWNER_OK +
    "画板 16 的分段是五档下载器状态（全部/下载中/做种中/等待中/已暂停）；这一页是 RSS 流水，" +
    "只有已下载 / 已推送 / 已过期三个标记，而且它们是可叠加的（apiTasks 里逐个 AND）——" +
    "「已下载 + 已推送」= 推成功了的，收成互斥分段会把这个组合删掉（注意：这里的组合在 " +
    "apiTasks 里逐个 AND、是真的在工作的，与审计页那个假组合不同）。下载器状态在画板 18 那一页；" +
    "要按画板做就得把下载器实时状态按 hash 关联进列表页，等于每次翻页多一次下载器往返。" +
    OWNER_DECISION,

  "/filter-rules grid.columns":
    "画板 20 九列；落地多一列「启用」——规则的开关本来就在表里改（画板把开关画进了操作列的省略菜单），" +
    "把它收起来会让「这条规则现在生效吗」看不出来。按 owner 的原则保留功能列。",

  "/sites grid.columns":
    "画板 12 七列；落地十一列 —— 多出「判定活跃 / 剩余天数 / 站点活跃 / 启用」。" +
    "按 owner 的原则保留（用画板的样式设计，但保留现有功能），并且已经做成可在列设置里关掉。",

  "/userinfo grid.columns":
    "画板十列；落地十四列 —— 多出「真实数据 / 入站 / 剩余天数 / 操作」。" +
    "**owner 已决定保留这四列**（原话：用画板的样式设计，但要保留现有的头像、未读角标、" +
    "真实数据、入站、剩余天数、操作），所以这不是待办，是记录在案的取舍。" +
    "站点列也按这个决定保留了头像与未读角标；行高与单元格形状则按画板压回 34。",

  /*
   * 移动画板 30 / 31 的筛选区与行卡走势图。两条都不是「忘了做」：
   */
  "/sites@375 filterRow":
    "画板 30 的筛选是一排 26 高的 chip（全部/正常/异常/已禁用 各带计数）。" +
    "落地现在是「已启用/全部」分段 + 三枚状态 chip（正常/异常/已禁用，带计数）+ 搜索 + " +
    "「批量」菜单 + 新增站点，两行共 81 —— 比画板高，但已经从 240 压下来，且入口一个没少。" +
    "还高的那部分是画板移动稿上根本没画的三个入口（探测已启用 / 打开已启用 / 新增站点）；" +
    "「全部」两边也不是一回事：画板那张图是 14 个已配置站点，这个产品的 /api/sites 回的是" +
    "全部 66 个内置定义，所以状态做成可叠加的 chip 而不是画板那种四档互斥分段。",
  "/tasks@375 filterRow":
    "画板 31 的筛选是一条 294×30 的分段器；落地是三个可叠加的筛选 chip + 搜索框 + 站点下拉。" +
    "那三个 chip 是可组合的（apiTasks 里逐个 AND），收成画板那条互斥分段器会删掉组合筛选能力 —— " +
    "这正是第九轮修掉的缺陷，不能为了对齐画板再改回去。",
  "/sites@375 rowCards.spark":
    OWNER_OK +
    "画板 30 的行卡里有一条 120×18 的 8 根柱走势图，落地没有：后端没有按站点的历史序列接口。" +
    "perSiteStats 只给当前快照（uploaded / ratio / seeding …），造一条假的走势比不画更糟。" +
    "依赖与「周期」那条同一套历史快照。" +
    OWNER_DECISION,

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
 * v2 升级横幅不在任何画板的构成里，关掉它才能量到画板的坐标。
 *
 * **代价要说清**：关掉它之后，这份验收就完全看不见它对首屏的影响了 ——
 * 一次评审正是指出「开场那 252px」一路绿灯，原因就在这一行。所以下面单独量一次
 * 它在首屏的高度（`bannerCost`），让这笔账出现在报告里，而不是被这一行悄悄抹掉。
 */
await ev(`localStorage.setItem('pt_tools_v2_banner_dismissed_v1', '1')`);
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

/*
 * 单独量一次 v2 横幅的首屏代价。
 *
 * 上面为了对齐画板坐标把它关掉了，代价是这份验收完全看不见它 —— 而真实的首次访问
 * 是带着它的。这里把 localStorage 那把开关放回去、重进一次 /userinfo，量两个数：
 * 横幅自己多高、表格带因此被推到哪里。只报数不判失败：它是一条已登记的偏离，
 * 但数字必须出现在报告里，不能被那一行 setItem 抹掉。
 */
let bannerCost = null;
if (!wanted.length || wanted.includes("/userinfo")) {
  await ev(`localStorage.removeItem('pt_tools_v2_banner_dismissed_v1')`);
  await goto(`${BASE}/#/userinfo`);
  /*
   * 必须真的重载：横幅的 visible 是在 onMounted 里读 localStorage 定的，
   * 而 hash 导航不会重新挂载 App.vue —— 只清掉那个键、不重载，量到的永远是 0。
   */
  await cdp.send("Page.reload", {}, sessionId);
  await waitReady();
  await sleep(2600);
  bannerCost = await ev(`(() => {
    const b = document.querySelector('.v2-deprecation-banner');
    const grid = document.querySelector('.pt-band--grid');
    return {
      banner: b ? Math.round(b.getBoundingClientRect().height) : 0,
      firstContentTop: grid ? Math.round(grid.getBoundingClientRect().top) : null,
    };
  })()`);
  await ev(`localStorage.setItem('pt_tools_v2_banner_dismissed_v1', '1')`);
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
    (wanted.length === 0 || wanted.some((r) => k.startsWith(`${r} `) || k.startsWith(`${r}@375 `))),
);

for (const f of failures) {
  console.log(`✗ ${f.route.padEnd(28)} [画板 ${f.board}] ${f.key}: ${f.msg}`);
}
if (bannerCost) {
  console.log(
    `· v2 横幅的首屏代价：横幅 ${bannerCost.banner}px，表格带被推到 ` +
      `${bannerCost.firstContentTop}（画板 104）。上面的几何是在关掉它之后量的。`,
  );
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
