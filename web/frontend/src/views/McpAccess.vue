<script setup lang="ts">
/*
 * MCP 接入（路线图 M14）：AI 助手（Claude、Cursor、Cherry Studio 这类 MCP 客户端）经 /mcp 调 pt-tools 的工具。
 * 这页没有设置：地址就是这台 pt-tools 的 /mcp，用有 MCP 权限的 API 令牌，这里写明客户端怎么填、有哪些工具。
 * 画板没有这一页，沿用 qB 兼容入口页的形状。客户端的填法与工具清单在 config/mcp.ts。
 */
import PtIcon from "@/components/PtIcon";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { MCP_TOOLS, mcpSnippets } from "@/config/mcp";
import { ElMessage } from "element-plus";

const origin = window.location.origin;
const endpoint = `${origin}/mcp`;

const SNIPPETS = mcpSnippets(origin);
const TOOLS = MCP_TOOLS;

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
        <li>
          回应里没有 Cookie、Passkey、密码与种子下载链接，下载器的错误里也去掉了它的地址；推送由
          pt-tools 用自己的站点配置下载。
        </li>
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
