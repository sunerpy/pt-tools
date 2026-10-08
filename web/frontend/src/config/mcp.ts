/*
 * MCP 接入页的数据（路线图 M14）：各种客户端的填法与工具清单。工具清单要和 internal/mcp 的 ToolNames 一致
 * （web/mcp_docs_test.go 守着）。放在 .ts 里而不是页面里：配置片段里的 args 之类的键名会被 style-scan 当成类名。
 */

export interface McpSnippet {
  key: string;
  title: string;
  text: string;
}

export interface McpTool {
  name: string;
  write: boolean;
  desc: string;
}

/** 三种客户端的填法；origin 是这台 pt-tools 的地址（浏览器里的 window.location.origin） */
export function mcpSnippets(origin: string): McpSnippet[] {
  const endpoint = `${origin}/mcp`;
  return [
    {
      key: "http",
      title: "支持 HTTP 的客户端（Cursor、Cherry Studio、VS Code 等）",
      text: JSON.stringify(
        {
          mcpServers: {
            "pt-tools": { url: endpoint, headers: { Authorization: "Bearer <令牌>" } },
          },
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
  ];
}

/** 工具清单（和 internal/mcp 的 ToolNames 一致） */
export const MCP_TOOLS: readonly McpTool[] = [
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
