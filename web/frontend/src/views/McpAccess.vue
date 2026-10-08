<script setup lang="ts">
/*
 * MCP 接入（路线图 M14）：AI 助手（Claude、Cursor、Cherry Studio 这类 MCP 客户端）经 /mcp 调 pt-tools 的工具。
 * 这页没有设置：地址就是这台 pt-tools 的 /mcp，用有 MCP 权限的 API 令牌，这里写明客户端怎么填、有哪些工具。
 * 画板没有这一页，沿用 qB 兼容入口页的形状。工具清单要和 internal/mcp 的 ToolNames 一致（web/mcp_docs_test.go 守着）。
 */
import PtIcon from "@/components/PtIcon";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { ElMessage } from "element-plus";
import { computed } from "vue";

const origin = window.location.origin;
const endpoint = `${origin}/mcp`;

const SNIPPETS = computed(() => [
  {
    key: "http",
    title: "支持 HTTP 的客户端（Cursor、Cherry Studio、VS Code 等）",
    text: JSON.stringify(
      {
        mcpServers: { "pt-tools": { url: endpoint, headers: { Authorization: "Bearer <令牌>" } } },
      },
      null,
      2,
    ),
  },
  {
    key: "claude-code",
    title: "Claude Code",
    text: `claude mcp add --transport http pt-tools ${endpoint} --header "Authorization: Bearer <令牌>"`,
  },
  {
    key: "stdio",
    title: "只支持 stdio 的客户端（Claude Desktop 等）：在客户端那台机器上放一份 pt-tools 转发",
    text: JSON.stringify(
      {
        mcpServers: {
          "pt-tools": {
            command: "pt-tools",
            args: ["mcp", "--url", origin],
            env: { PT_TOOLS_MCP_TOKEN: "<令牌>" },
          },
        },
      },
      null,
      2,
    ),
  },
]);

/** 工具清单（和 internal/mcp 的 ToolNames 一致） */
const TOOLS: readonly { name: string; write: boolean; desc: string }[] = [
  { name: "list_tasks", write: false, desc: "推送记录：RSS、订阅、App 与 MCP 推过的种子" },
  { name: "list_downloader_torrents", write: false, desc: "下载器里的种子，分页，可以按状态筛选" },
  { name: "get_downloader_stats", write: false, desc: "各台下载器的速度、累计量与剩余空间" },
  { name: "search_torrents", write: false, desc: "在已启用的站点上搜索种子" },
  {
    name: "get_site_userinfo",
    write: false,
    desc: "各站点的上传、下载、分享率、魔力、未读消息与登录状态",
  },
  { name: "check_updates", write: false, desc: "有没有新版本（只看，不升级）" },
  { name: "explore_media", write: false, desc: "按片名找电影与剧集，或者看本周趋势、热门" },
  { name: "list_subscriptions", write: false, desc: "订阅与进度（已播、已入库、下载中、缺的集）" },
  { name: "pause_torrent", write: true, desc: "暂停一个种子" },
  { name: "resume_torrent", write: true, desc: "继续一个暂停的种子" },
  { name: "delete_torrent", write: true, desc: "删除一个种子，可以连数据一起删" },
  {
    name: "push_torrent",
    write: true,
    desc: "把搜到的种子推到下载器，照常经过磁盘空间与站点做种容量检查",
  },
  { name: "add_subscription", write: true, desc: "订阅一部电影或一季剧集" },
];

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    ElMessage.success("已复制");
  } catch {
    ElMessage.warning("浏览器不让复制：请手动选中复制");
  }
}
</script>

<template>
  <div class="pt-cards pt-cards--main">
    <PtHeadSub
      >让 AI 助手（Claude、Cursor、Cherry Studio 这类 MCP 客户端）查看种子、下载器与站点数据，
      并在你确认以后暂停、删除、推送种子</PtHeadSub
    >

    <PtPanel title="地址与令牌" icon="link" data-testid="mcp-endpoint">
      <ul class="mcp-kv">
        <li>
          <span class="mcp-k">MCP 地址</span>
          <code data-testid="mcp-url">{{ endpoint }}</code>
          <el-button link type="primary" data-testid="mcp-copy-url" @click="copy(endpoint)"
            >复制</el-button
          >
        </li>
        <li><span class="mcp-k">传输方式</span><span>Streamable HTTP（无状态）</span></li>
        <li>
          <span class="mcp-k">令牌</span>
          <span
            >在
            <router-link to="/api-tokens" data-testid="mcp-tokens">系统 → API 令牌</router-link>
            新建，选「MCP 读取」；要让它暂停、删除、推送种子或添加订阅，再选「MCP 操作」</span
          >
        </li>
      </ul>
    </PtPanel>

    <PtPanel title="安全" icon="shield-check" data-testid="mcp-safety">
      <ul class="mcp-list">
        <li>只认 API 令牌，网页登录的会话不能用；令牌撤销以后马上失效。</li>
        <li>
          会改动下载器或订阅的工具，每次都要 AI 先向你确认，并记在「操作审计」里，通道是「MCP」。
        </li>
        <li>回应里没有 Cookie、Passkey、密码与下载链接，推送由 pt-tools 用自己的站点配置下载。</li>
        <li>从公网访问时请放在 HTTPS 反向代理后面，否则令牌会以明文经过网络。</li>
      </ul>
    </PtPanel>

    <PtPanel class="pt-cards__full" title="客户端怎么填" icon="terminal" data-testid="mcp-howto">
      <div
        v-for="s in SNIPPETS"
        :key="s.key"
        class="mcp-snippet"
        :data-testid="`mcp-snippet-${s.key}`">
        <div class="mcp-snippet__head">
          <span>{{ s.title }}</span>
          <el-button link type="primary" @click="copy(s.text)">
            <PtIcon name="copy" :size="13" /><span>复制</span>
          </el-button>
        </div>
        <pre class="mcp-code">{{ s.text }}</pre>
      </div>
      <p class="mcp-tip">
        把 &lt;令牌&gt; 换成新建的 API 令牌。用 Docker
        部署时，地址里的主机名与端口是映射到宿主机的那一组。
      </p>
    </PtPanel>

    <PtPanel
      class="pt-cards__full"
      title="工具"
      icon="wrench"
      :count="TOOLS.length"
      data-testid="mcp-tools">
      <ul class="mcp-tools">
        <li v-for="t in TOOLS" :key="t.name" :data-testid="`mcp-tool-${t.name}`">
          <code>{{ t.name }}</code>
          <PtStatusPill :tone="t.write ? 'warn' : 'info'" size="sm">{{
            t.write ? "MCP 操作" : "MCP 读取"
          }}</PtStatusPill>
          <span class="mcp-tools__desc">{{ t.desc }}</span>
        </li>
      </ul>
    </PtPanel>
  </div>
</template>

<style scoped>
.mcp-kv,
.mcp-list,
.mcp-tools {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.mcp-kv li,
.mcp-tools li {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  min-width: 0;
  font-size: 13px;
  color: var(--pt-t1);
}

.mcp-k {
  flex: 0 0 72px;
  color: var(--pt-t3);
}

/* 长的说明和标签排在同一行，放不下时整段换到下一行 */
.mcp-kv li > span:last-child:not(.mcp-k) {
  flex: 1 1 240px;
  min-width: 0;
}

.mcp-kv code,
.mcp-tools code {
  word-break: break-all;
}

.mcp-list li {
  position: relative;
  padding-left: 14px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--pt-t1);
}

.mcp-list li::before {
  position: absolute;
  top: 0.62em;
  left: 2px;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--pt-t3);
  content: "";
}

.mcp-snippet + .mcp-snippet {
  margin-top: 14px;
}

.mcp-snippet__head {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
  font-size: 13px;
  color: var(--pt-t2);
}

.mcp-code {
  margin: 0;
  padding: 10px 12px;
  overflow-x: auto;
  border-radius: 8px;
  background: var(--pt-bg-surface-muted);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-all;
}

.mcp-tip {
  margin: 10px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.mcp-tools li {
  padding-bottom: 10px;
  border-bottom: 1px solid var(--pt-border);
}

.mcp-tools li:last-child {
  padding-bottom: 0;
  border-bottom: 0;
}

.mcp-tools__desc {
  flex: 1 1 240px;
  min-width: 0;
  color: var(--pt-t2);
}
</style>
