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

/** MCP 默认的路径（「系统 → MCP 接入」里能换） */
export const MCP_DEFAULT_PATH = "/mcp";

/**
 * 三种客户端的填法；origin 是这台 pt-tools 的地址（浏览器里的 window.location.origin），path 是 MCP 的路径。
 * stdio 转发的 --url 只给主机与端口时补 /mcp，换了路径时给完整地址。
 */
export function mcpSnippets(origin: string, path: string = MCP_DEFAULT_PATH): McpSnippet[] {
  const endpoint = `${origin}${path}`;
  const bridgeURL = path === MCP_DEFAULT_PATH ? origin : endpoint;
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
              args: ["mcp", "--url", bridgeURL],
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
  { name: "list_brush_tasks", write: false, desc: "刷流任务、状态与收益" },
  { name: "get_brush_task", write: false, desc: "刷流任务的配置" },
  { name: "list_brush_runs", write: false, desc: "刷流每轮运行记录与步骤" },
  { name: "change_brush_task", write: true, desc: "预览并确认新增、修改、启停或删除刷流任务" },
  { name: "list_tasks", write: false, desc: "推送记录：RSS、订阅、App 与 MCP 推过的种子" },
  { name: "list_downloader_torrents", write: false, desc: "下载器里的种子，分页，可以按状态筛选" },
  { name: "get_downloader_stats", write: false, desc: "各台下载器的速度、累计量与剩余空间" },
  { name: "search_torrents", write: false, desc: "在已启用的站点上搜索种子" },
  {
    name: "get_site_userinfo",
    write: false,
    desc: "各站点的上传、下载量（计费）、分享率、魔力、未读消息与登录状态",
  },
  { name: "check_updates", write: false, desc: "有没有新版本（只看，不升级）" },
  { name: "explore_media", write: false, desc: "按片名找电影与剧集，或者看本周趋势、热门" },
  { name: "list_subscriptions", write: false, desc: "订阅与进度（已播、已入库、下载中、缺的集）" },
  {
    name: "get_subscription",
    write: false,
    desc: "一个订阅的详情：设置、每一集的情况、下载过的种子与每个文件整理刮削得怎样",
  },
  {
    name: "list_subscription_runs",
    write: false,
    desc: "订阅的运行记录：每次搜到了什么、为什么没要、推送了哪个",
  },
  { name: "get_episode_rules", write: false, desc: "追剧的集号规则（综艺按期号、播出日期对集）" },
  {
    name: "preview_schedule",
    write: false,
    desc: "看一个定时（间隔、每天、每周、cron）接下来几次什么时候搜",
  },
  {
    name: "recognize_media",
    write: false,
    desc: "识别种子标题：解析名字、季集与规格，到 TMDB 找对应的条目",
  },
  { name: "list_media_libraries", write: false, desc: "媒体库：类型、目录、整理方式与刮削" },
  { name: "list_media_history", write: false, desc: "整理历史：每个视频放进了哪里、失败的原因" },
  {
    name: "preview_media_organize",
    write: false,
    desc: "预览整理一个种子：认出的条目、媒体库与每个视频的目标位置",
  },
  {
    name: "get_config",
    write: false,
    desc: "读 pt-tools 的配置（令牌另要「配置」权限）；不带资源名时是配置目录，密钥只显示有没有设置",
  },
  {
    name: "get_skill",
    write: false,
    desc: "内置技能：pt-tools-config 讲怎么读改配置（附配置目录），pt-tools-follow 讲追剧",
  },
  { name: "pause_torrent", write: true, desc: "暂停一个种子" },
  { name: "resume_torrent", write: true, desc: "继续一个暂停的种子" },
  { name: "delete_torrent", write: true, desc: "删除一个种子，可以连数据一起删" },
  {
    name: "push_torrent",
    write: true,
    desc: "把搜到的种子推到下载器，照常经过磁盘空间与站点做种容量检查",
  },
  {
    name: "add_subscription",
    write: true,
    desc: "订阅一部电影或追一季剧集：可以带定时、版本锁、起始集、从哪个种子开始",
  },
  {
    name: "update_subscription",
    write: true,
    desc: "改订阅的设置：定时、版本锁、起始集、站点、下载器、媒体库等",
  },
  { name: "set_episode_skip", write: true, desc: "跳过一集，或者重新要它" },
  { name: "unlock_episode", write: true, desc: "让一集不受版本锁限制、换别的版本" },
  { name: "save_episode_rule", write: true, desc: "给一个制作组设集号规则" },
  { name: "delete_episode_rule", write: true, desc: "删掉一个制作组的集号规则" },
  {
    name: "reorganize_subscription_torrent",
    write: true,
    desc: "重新整理订阅下载的一个种子（整季包与单集一样）",
  },
  {
    name: "rescrape_subscription_torrent",
    write: true,
    desc: "重新刮削订阅下载的一个种子整理进的剧集或电影",
  },
  { name: "set_subscription_status", write: true, desc: "暂停或恢复一个订阅" },
  { name: "search_subscription", write: true, desc: "立即搜索一个订阅，搜到就下载" },
  {
    name: "delete_subscription",
    write: true,
    desc: "删除一个订阅（下载过的种子与库里的文件不动）",
  },
  {
    name: "organize_media",
    write: true,
    desc: "把下载完的种子整理进媒体库：按命名模板放好、刮削、通知媒体服务器扫描",
  },
  { name: "retry_media_organize", write: true, desc: "重新整理一条整理记录（多半是失败的）" },
  { name: "rescrape_media", write: true, desc: "重新刮削一条整理记录所在的电影或剧集" },
  {
    name: "change_config",
    write: true,
    desc: "改 pt-tools 的配置（令牌另要「配置」权限）：先预览改前改后与影响，确认以后才保存",
  },
];
