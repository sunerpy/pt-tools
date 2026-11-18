package models

// AllModels 是启动时 AutoMigrate 的全部表，顺序就是迁移顺序（版本表必须最先迁移）。
// 加密列清单的测试（TestEncryptedColumnsCoverEveryEncryptedField）也按它找以 Encrypted 结尾的字段。
func AllModels() []any {
	return []any{
		&SchemaVersion{}, // 版本表必须最先迁移
		&TorrentInfo{},
		&TorrentInfoArchive{},
		&AdminUser{},
		&SettingsGlobal{},
		&QbitSettings{},
		&SiteSetting{},
		&RSSSubscription{},
		// New tables for downloader and site extensibility
		&DownloaderSetting{},
		&DownloaderDirectory{},
		&SiteTemplate{},
		// Filter rules for RSS filtering
		&FilterRule{},
		// RSS-Filter association table for many-to-many relationship
		&RSSFilterAssociation{},
		// Favicon cache for site icons
		&FaviconCache{},
		&SiteRateLimit{},
		// ChatOps subsystem (additive; not bumping CurrentSchemaVersion)
		&NotificationConf{},
		&ChannelBinding{},
		&ActionAudit{},
		&BotToken{},
		&NotificationOutbox{},
		&RSSNotificationLog{},
		&SiteLoginState{},
		&MigrationState{},
		&CloakSettings{},
		&MonitorNotificationLog{},
		&SiteAttendanceLog{},
		&SiteAttendanceEvent{},
		// 刷流（M3）
		&BrushTask{},
		&BrushTorrent{},
		&BrushTorrentSample{},
		&BrushDailyStat{},
		&BrushRun{},
		// 转移做种（M5）
		&TorrentTransferJob{},
		&DownloaderPathMap{},
		&TransferRule{},
		// IYUU 辅种（M6）
		&ReseedSetting{},
		&ReseedRecord{},
		&ReseedRun{},
		&ReseedAttempt{},
		&ReseedIYUUDaily{},
		&ReseedDailySkip{},
		// CookieCloud 导入（M7）
		&CookieCloudSetting{},
		// 媒体识别（M9）
		&MediaSetting{},
		&MediaWordRule{},
		&MediaOverride{},
		&MediaCache{},
		// 整理入库（M10）
		&MediaLibrary{},
		&MediaPathMap{},
		&MediaServer{},
		&MediaOrganizeSetting{},
		&MediaTransferHistory{},
		&MediaReplaceJournal{},
		// 目录监控（M23）
		&MediaWatchDir{},
		// 订阅（M11）
		&MediaQualityProfile{},
		&MediaSubscription{},
		&MediaSubscriptionTorrent{},
		&MediaSubscribeSetting{},
		&MediaDoubanSource{},
		&MediaDoubanItem{},
		// 一键追剧：台账、集号规则、运行记录
		&MediaSubscriptionEpisode{},
		&MediaEpisodeRule{},
		&MediaSubscriptionRun{},
		// M12：API 令牌
		&APIToken{},
		&QbitCompatSetting{},
		&QbitCompatTorrent{},
		// MCP 接入的开关与路径
		&MCPSetting{},
		// 远程访问（M15）
		&RemoteSetting{},
		&RemoteDevice{},
		// AI 助手（M19）
		&AssistantSetting{},
		&AssistantConversation{},
		&AssistantMessage{},
		&AssistantConfirmation{},
		&AssistantAttachment{},
		// 消息模板（M21）
		&NotificationTemplate{},
		// 站点代理（全局设置）
		&SiteProxySetting{},
		// 备份与恢复（媒体中心 M0）
		&AppMeta{},
		&BackupSetting{},
		// 种子黑名单与等免费（媒体中心 M2b）
		&MediaTorrentBlock{},
		&FreeWatch{},
		// 媒体服务器索引与待处理问题（媒体中心 M6）
		&MediaServerItem{},
		&MediaIssue{},
		// Servarr 兼容入口的标签（媒体中心 M10）
		&ServarrTag{},
		&ServarrTagLink{},
		// NAS 监控的来源与设置（首页 NAS 概览 H2）
		&NasSource{},
		&NasSetting{},
	}
}
