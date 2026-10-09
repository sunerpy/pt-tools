// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Chinese (`zh`).
class SZh extends S {
  SZh([String locale = 'zh']) : super(locale);

  @override
  String get appTitle => 'pt-tools';

  @override
  String get retry => '重试';

  @override
  String get cancel => '取消';

  @override
  String get paste => '粘贴';

  @override
  String get actions => '操作';

  @override
  String get empty => '没有内容';

  @override
  String errorPrefix(String message) {
    return '出错了：$message';
  }

  @override
  String get navOverview => '概览';

  @override
  String get navTorrents => '种子';

  @override
  String get navSearch => '搜索';

  @override
  String get navMedia => '媒体';

  @override
  String get navMore => '更多';

  @override
  String get connConnecting => '正在连接…';

  @override
  String get connOnline => '已连上';

  @override
  String connFailed(String message) {
    return '连不上：$message';
  }

  @override
  String get connRevoked => '这台设备被撤销了，请重新配对';

  @override
  String get connKeyRotated => '主机的密钥换了，请重新配对';

  @override
  String get connDisabled => '主机关掉了远程访问';

  @override
  String get connOffline => '未连接';

  @override
  String get repair => '重新配对';

  @override
  String get viaDirect => '直连';

  @override
  String get viaRelay => 'relay';

  @override
  String get pairTitle => '配对 pt-tools';

  @override
  String get pairIntro => '在 pt-tools 网页上打开「系统 → 远程访问 → 添加设备」，扫二维码，或者把链接粘贴到下面。';

  @override
  String get pairScan => '扫码';

  @override
  String get pairLinkLabel => '配对链接';

  @override
  String get pairNameLabel => '设备名';

  @override
  String get pairNameDefault => '我的手机';

  @override
  String get pairButton => '配对';

  @override
  String get pairWorking => '正在配对…';

  @override
  String pairInvalidLink(String message) {
    return '链接不对：$message';
  }

  @override
  String get pairUpgrade => '这个配对链接要更新版本的 App';

  @override
  String pairSaveFailed(String error, String name) {
    return '主机已经记下了这台设备，但存进手机的安全存储时出错（$error）：请在网页上撤销「$name」，再重新配对。';
  }

  @override
  String get pairPrivacy =>
      '连接是端到端加密的：经 relay 时 relay 只转发，看不到内容。这台设备的私钥只存在手机上。';

  @override
  String get scanTitle => '扫描配对二维码';

  @override
  String get scanUnavailable => '这台设备不能扫码，请返回粘贴链接';

  @override
  String hostVersion(String version) {
    return 'pt-tools $version';
  }

  @override
  String apiLevelMismatch(int level, int supported) {
    return '主机的 App API 级别是 $level，这个 App 认得 $supported，请升级';
  }

  @override
  String get kpiUploaded => '上传';

  @override
  String get kpiDownloaded => '下载';

  @override
  String get kpiRatio => '分享率';

  @override
  String get kpiBonus => '魔力';

  @override
  String perHour(String value) {
    return '每小时 $value';
  }

  @override
  String get kpiSeeding => '做种';

  @override
  String get kpiUnread => '站内信未读';

  @override
  String siteCount(int count) {
    return '$count 个站点';
  }

  @override
  String get todayTitle => '今天';

  @override
  String todayError(String message) {
    return '今天的增量算不出来：$message';
  }

  @override
  String get todayBySite => '各站点';

  @override
  String get downloadersTitle => '下载器';

  @override
  String get downloaderUnreachable => '连不上';

  @override
  String freeSpace(String size) {
    return '剩余 $size';
  }

  @override
  String updatedAt(String time) {
    return '站点数据更新于 $time';
  }

  @override
  String get torrentsAll => '全部';

  @override
  String get stateDownloading => '下载中';

  @override
  String get stateSeeding => '做种中';

  @override
  String get statePaused => '已暂停';

  @override
  String get stateStopped => '已停止';

  @override
  String get stateQueued => '排队中';

  @override
  String get stateChecking => '校验中';

  @override
  String get stateError => '出错';

  @override
  String get searchTorrentsHint => '按标题筛选';

  @override
  String get noTorrents => '下载器里没有种子';

  @override
  String get actionPause => '暂停';

  @override
  String get actionResume => '继续';

  @override
  String get actionDelete => '删除';

  @override
  String get deleteTitle => '删除这个种子？';

  @override
  String get deleteWithFiles => '连同已下载的数据一起删除';

  @override
  String actionDone(int ok, int failed) {
    return '成功 $ok 个，失败 $failed 个';
  }

  @override
  String torrentFailures(int count) {
    return '有 $count 个下载器没有读到，列表不完整';
  }

  @override
  String etaLabel(String eta) {
    return '剩余 $eta';
  }

  @override
  String get searchHint => '搜索站点上的种子';

  @override
  String get searchIntro => '输入关键字，在所有启用的站点上一起搜';

  @override
  String get searchFreeOnly => '只看免费';

  @override
  String get searchNoResults => '没有找到';

  @override
  String searchSites(int count, int ms) {
    return '$count 个站点 · $ms ms';
  }

  @override
  String searchErrors(int count) {
    return '$count 个站点出错';
  }

  @override
  String get pushTitle => '推送到下载器';

  @override
  String get pushDefault => '默认下载器';

  @override
  String pushOk(String downloader) {
    return '已推送到 $downloader';
  }

  @override
  String get pushSkipped => '下载器里已经有这个种子';

  @override
  String pushBlocked(String message) {
    return '没有推送：$message';
  }

  @override
  String get defaultBadge => '默认';

  @override
  String get free => '免费';

  @override
  String get hr => 'H&R';

  @override
  String get tabSubscriptions => '订阅';

  @override
  String get tabHistory => '最近入库';

  @override
  String get tabExplore => '探索';

  @override
  String get noSubscriptions => '没有订阅；可以在「探索」里订阅';

  @override
  String get noHistory => '没有整理记录';

  @override
  String get subActive => '订阅中';

  @override
  String get subPaused => '已暂停';

  @override
  String get subPending => '等待中';

  @override
  String get subDone => '已完成';

  @override
  String get subUpgrade => '洗版';

  @override
  String subProgress(int inLibrary, int aired) {
    return '已入库 $inLibrary/$aired 集';
  }

  @override
  String subMissing(String list) {
    return '缺 $list';
  }

  @override
  String get subPause => '暂停订阅';

  @override
  String get subResume => '恢复订阅';

  @override
  String get subSearchNow => '立即搜索';

  @override
  String get subDelete => '删除订阅';

  @override
  String get subDeleteConfirm => '删除这个订阅？下载器里的种子与媒体库里的文件不动。';

  @override
  String subNextSearch(String time) {
    return '下次搜索 $time';
  }

  @override
  String get episodesTitle => '剧集';

  @override
  String get epLibrary => '已入库';

  @override
  String get epDownloading => '下载中';

  @override
  String get epMissing => '缺';

  @override
  String get epUpcoming => '未播出';

  @override
  String get subTorrentsTitle => '下载过的种子';

  @override
  String get exploreMovie => '电影';

  @override
  String get exploreTv => '剧集';

  @override
  String get exploreTrending => '热门';

  @override
  String get explorePopular => '流行';

  @override
  String get exploreSearchHint => '搜索电影或剧集';

  @override
  String get subscribe => '订阅';

  @override
  String get subscribed => '已订阅';

  @override
  String get inLibrary => '已入库';

  @override
  String subscribeOk(String title) {
    return '已订阅 $title';
  }

  @override
  String get moreSites => '站点';

  @override
  String get moreTasks => '任务';

  @override
  String get moreBrush => '刷流';

  @override
  String get moreDownloaders => '下载器';

  @override
  String get moreSettings => '连接与设备';

  @override
  String get siteDisabled => '未启用';

  @override
  String disabledSites(int count) {
    return '未启用的站点（$count）';
  }

  @override
  String get noEnabledSites => '没有启用的站点';

  @override
  String get movieNotInLibrary => '未入库';

  @override
  String loginDays(int days) {
    return '离封号还有 $days 天';
  }

  @override
  String get loginUnknown => '没有访问记录';

  @override
  String get attend => '签到';

  @override
  String get attendOk => '签到完成';

  @override
  String get attendSigned => '今天已签到';

  @override
  String get attendFailed => '签到失败';

  @override
  String get attendPending => '今天未签到';

  @override
  String get attendUnsupported => '不支持签到';

  @override
  String unreadMessages(int count) {
    return '$count 条未读';
  }

  @override
  String siteUserError(String message) {
    return '站点上的用户数据没读到：$message';
  }

  @override
  String get noTasks => '没有推送记录';

  @override
  String get taskPushed => '已推送';

  @override
  String get taskNotPushed => '未推送';

  @override
  String get taskCompleted => '已完成';

  @override
  String get noBrush => '没有刷流任务';

  @override
  String get brushDisabled => '已停用';

  @override
  String brushActive(int count) {
    return '在刷 $count 个';
  }

  @override
  String brushToday(String up, String down) {
    return '今天 ↑$up ↓$down';
  }

  @override
  String brushTotal(String up, String down) {
    return '累计 ↑$up ↓$down';
  }

  @override
  String get noDownloaders => '没有启用的下载器';

  @override
  String get settingsConnection => '连接';

  @override
  String get settingsVia => '方式';

  @override
  String get settingsEndpoint => '地址';

  @override
  String get settingsHost => '主机';

  @override
  String get settingsReconnect => '重新连接';

  @override
  String get settingsDevice => '这台设备';

  @override
  String settingsPairedAt(String time) {
    return '配对于 $time';
  }

  @override
  String get settingsScopes => '权限';

  @override
  String get scopeFull => '完全控制';

  @override
  String get scopeRead => '只读';

  @override
  String get settingsForget => '忘掉这台主机';

  @override
  String get forgetConfirm => '忘掉以后要重新扫码配对。主机上的设备记录不会删除，需要的话在网页上撤销。';
}
