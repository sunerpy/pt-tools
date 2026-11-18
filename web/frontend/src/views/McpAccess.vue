<script setup lang="ts">
/*
 * MCP 接入（路线图 M14）：AI 客户端（Claude、Cursor、Cherry Studio 这类 MCP 客户端）经 MCP 地址调 pt-tools 的工具。
 * 设置是开关与路径：关着时外部的客户端连不上（地址回 404），内置 AI 助手在进程内调工具，不受影响；路径默认 /mcp，
 * 保存后马上生效。下面写明客户端怎么填、有哪些工具。画板没有这一页，沿用 qB 兼容入口页的形状。
 * 客户端的填法与工具清单在 config/mcp.ts。
 */
import { type McpSettings, mcpApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { MCP_DEFAULT_PATH, MCP_TOOLS, mcpSnippets } from "@/config/mcp";
import { useDataState } from "@/composables/useDataState";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

const origin = window.location.origin;
const TOOLS = MCP_TOOLS;

const settings = ref<McpSettings | null>(null);
const ds = useDataState();
const form = ref({ enabled: false, path: MCP_DEFAULT_PATH });
const saving = ref(false);

/** 现在生效的路径（读不到设置时按默认的写） */
const savedPath = computed(() => settings.value?.path || MCP_DEFAULT_PATH);
const endpoint = computed(() => `${origin}${savedPath.value}`);
const SNIPPETS = computed(() => mcpSnippets(origin, savedPath.value));
const dirty = computed(
  () =>
    Boolean(settings.value) &&
    (form.value.enabled !== settings.value!.enabled ||
      (form.value.path.trim() || MCP_DEFAULT_PATH) !== settings.value!.path),
);

function fill(v: McpSettings) {
  settings.value = v;
  form.value = { enabled: v.enabled, path: v.path };
}

async function load() {
  const pending = ds.run(() => mcpApi.settings());
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  fill(got);
}

async function save() {
  saving.value = true;
  try {
    const got = await mcpApi.save({ enabled: form.value.enabled, path: form.value.path.trim() });
    fill(got);
    ElMessage.success(
      got.enabled ? `已打开 MCP 接入，地址 ${origin}${got.path}` : "已关闭 MCP 接入",
    );
  } catch (e) {
    ElMessage.error((e as Error)?.message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    ElMessage.success("已复制");
  } catch {
    ElMessage.warning("浏览器不让复制：请手动选中复制");
  }
}

onMounted(load);
</script>

<template>
  <div class="pt-cards pt-cards--main">
    <PtHeadSub
      >让 AI 助手（Claude、Cursor、Cherry Studio 这类 MCP 客户端）查看种子、下载器、站点与追剧，
      并在你确认以后推送种子、管理订阅与追剧、整理媒体库、改配置</PtHeadSub
    >

    <PtPanel title="设置" icon="settings" data-testid="mcp-settings">
      <PtDataState
        v-if="!settings"
        :state="ds.state.value"
        :sub="ds.errorText.value"
        data-testid="mcp-settings-state" />
      <el-form v-else class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="MCP 接入">
          <el-switch v-model="form.enabled" data-testid="mcp-enabled" />
          <div class="mcp-tip">
            打开以后，AI 客户端用有 MCP 权限的 API 令牌经下面的地址连
            pt-tools，能查看与（经你确认后）操作种子、订阅、追剧、媒体库与配置；关着时这个地址回
            404。内置 AI 助手不经这个地址，不受影响。
          </div>
        </el-form-item>
        <el-form-item label="路径">
          <el-input
            v-model="form.path"
            :placeholder="MCP_DEFAULT_PATH"
            aria-label="MCP 的路径"
            data-testid="mcp-path">
            <template #prepend>{{ origin }}</template>
          </el-input>
          <div class="mcp-tip">
            默认 /mcp。可以换成别的路径（比如反向代理只转发某个前缀时），不能放在 /api、/assets
            下面。保存后马上生效；换了以后旧地址回 404，客户端里的地址要跟着改。
          </div>
        </el-form-item>
      </el-form>
      <template v-if="settings" #footer>
        <el-button
          type="primary"
          :loading="saving"
          :disabled="!dirty"
          data-testid="mcp-save"
          @click="save">
          <PtIcon name="save" :size="14" /><span>保存</span>
        </el-button>
      </template>
    </PtPanel>

    <PtPanel title="地址与令牌" icon="link" data-testid="mcp-endpoint">
      <ul class="mcp-kv">
        <li v-if="settings" data-testid="mcp-state">
          <span class="mcp-k">状态</span>
          <!-- 包一层：值那一列的 span 会撑满，药丸按自己的宽度 -->
          <span>
            <!-- 用户自己关掉的功能是正常状态：灰色「已关闭」，与远程访问、自动删种同一套说法；橙色留给出了问题的时候 -->
            <PtStatusPill v-if="settings.enabled" tone="ok" dot size="sm">已开启</PtStatusPill>
            <PtStatusPill v-else tone="neutral" dot size="sm">已关闭</PtStatusPill>
          </span>
        </li>
        <li>
          <span class="mcp-k">MCP 地址</span>
          <code data-testid="mcp-url">{{ endpoint }}</code>
          <el-button link type="primary" data-testid="mcp-copy-url" @click="copy(endpoint)"
            >复制</el-button
          >
        </li>
        <li v-if="settings && settings.tokens >= 0" data-testid="mcp-token-count">
          <span class="mcp-k">能用的令牌</span>
          <span v-if="settings.tokens > 0">{{ settings.tokens }} 个</span>
          <span v-else
            >还没有：<router-link to="/api-tokens?new=mcp" data-testid="mcp-new-token"
              >新建一个</router-link
            ></span
          >
        </li>
        <li><span class="mcp-k">传输方式</span><span>Streamable HTTP（无状态）</span></li>
        <li>
          <span class="mcp-k">令牌</span>
          <span
            >在
            <router-link to="/api-tokens" data-testid="mcp-tokens">系统 → API 令牌</router-link>
            新建，选「MCP 读取」；要让它暂停、删除、推送种子，管理订阅与追剧，整理媒体库，再选「MCP
            操作」；要让它读改 pt-tools 的配置（get_config、change_config），再选「配置」</span
          >
        </li>
      </ul>
    </PtPanel>

    <PtPanel title="安全" icon="shield-check" data-testid="mcp-safety">
      <ul class="mcp-list">
        <li>
          只认 API 令牌，网页登录的会话不能用；令牌撤销以后马上失效。不用的时候在上面关掉 MCP 接入。
        </li>
        <li>
          会改动下载器、订阅、媒体库或配置的工具，不带确认调用时只回预览（改以前、改以后与可能的影响），AI
          要先向你确认才执行；执行的记在「操作审计」里，通道是「MCP」。
        </li>
        <li>配置工具读到的密钥（密码、Cookie、令牌）只显示有没有设置，改的时候可以写新值。</li>
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

/* 工具清单在宽卡里排成多栏：一行一个时右边空出一大半（2048 宽的屏上空 65%） */
.mcp-tools {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 460px), 1fr));
  gap: 10px 24px;
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

/* 表单项里的说明紧跟在开关、输入框下面 */
.pt-form .mcp-tip {
  flex-basis: 100%;
  margin-top: 6px;
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
