import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_zh.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of S
/// returned by `S.of(context)`.
///
/// Applications need to include `S.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'l10n/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: S.localizationsDelegates,
///   supportedLocales: S.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the S.supportedLocales
/// property.
abstract class S {
  S(String locale)
    : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static S of(BuildContext context) {
    return Localizations.of<S>(context, S)!;
  }

  static const LocalizationsDelegate<S> delegate = _SDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates =
      <LocalizationsDelegate<dynamic>>[
        delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
      ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('en'),
    Locale('zh'),
  ];

  /// No description provided for @appTitle.
  ///
  /// In zh, this message translates to:
  /// **'pt-tools'**
  String get appTitle;

  /// No description provided for @retry.
  ///
  /// In zh, this message translates to:
  /// **'重试'**
  String get retry;

  /// No description provided for @cancel.
  ///
  /// In zh, this message translates to:
  /// **'取消'**
  String get cancel;

  /// No description provided for @paste.
  ///
  /// In zh, this message translates to:
  /// **'粘贴'**
  String get paste;

  /// No description provided for @actions.
  ///
  /// In zh, this message translates to:
  /// **'操作'**
  String get actions;

  /// No description provided for @empty.
  ///
  /// In zh, this message translates to:
  /// **'没有内容'**
  String get empty;

  /// No description provided for @errorPrefix.
  ///
  /// In zh, this message translates to:
  /// **'出错了：{message}'**
  String errorPrefix(String message);

  /// No description provided for @navOverview.
  ///
  /// In zh, this message translates to:
  /// **'概览'**
  String get navOverview;

  /// No description provided for @navTorrents.
  ///
  /// In zh, this message translates to:
  /// **'种子'**
  String get navTorrents;

  /// No description provided for @navSearch.
  ///
  /// In zh, this message translates to:
  /// **'搜索'**
  String get navSearch;

  /// No description provided for @navMedia.
  ///
  /// In zh, this message translates to:
  /// **'媒体'**
  String get navMedia;

  /// No description provided for @navMore.
  ///
  /// In zh, this message translates to:
  /// **'更多'**
  String get navMore;

  /// No description provided for @connConnecting.
  ///
  /// In zh, this message translates to:
  /// **'正在连接…'**
  String get connConnecting;

  /// No description provided for @connOnline.
  ///
  /// In zh, this message translates to:
  /// **'已连上'**
  String get connOnline;

  /// No description provided for @connFailed.
  ///
  /// In zh, this message translates to:
  /// **'连不上：{message}'**
  String connFailed(String message);

  /// No description provided for @connRevoked.
  ///
  /// In zh, this message translates to:
  /// **'这台设备被撤销了，请重新配对'**
  String get connRevoked;

  /// No description provided for @connKeyRotated.
  ///
  /// In zh, this message translates to:
  /// **'主机的密钥换了，请重新配对'**
  String get connKeyRotated;

  /// No description provided for @connDisabled.
  ///
  /// In zh, this message translates to:
  /// **'主机关掉了远程访问'**
  String get connDisabled;

  /// No description provided for @connOffline.
  ///
  /// In zh, this message translates to:
  /// **'未连接'**
  String get connOffline;

  /// No description provided for @repair.
  ///
  /// In zh, this message translates to:
  /// **'重新配对'**
  String get repair;

  /// No description provided for @viaDirect.
  ///
  /// In zh, this message translates to:
  /// **'直连'**
  String get viaDirect;

  /// No description provided for @viaRelay.
  ///
  /// In zh, this message translates to:
  /// **'relay'**
  String get viaRelay;

  /// No description provided for @pairTitle.
  ///
  /// In zh, this message translates to:
  /// **'配对 pt-tools'**
  String get pairTitle;

  /// No description provided for @pairIntro.
  ///
  /// In zh, this message translates to:
  /// **'在 pt-tools 网页上打开「系统 → 远程访问 → 添加设备」，扫二维码，或者把链接粘贴到下面。'**
  String get pairIntro;

  /// No description provided for @pairScan.
  ///
  /// In zh, this message translates to:
  /// **'扫码'**
  String get pairScan;

  /// No description provided for @pairLinkLabel.
  ///
  /// In zh, this message translates to:
  /// **'配对链接'**
  String get pairLinkLabel;

  /// No description provided for @pairNameLabel.
  ///
  /// In zh, this message translates to:
  /// **'设备名'**
  String get pairNameLabel;

  /// No description provided for @pairNameDefault.
  ///
  /// In zh, this message translates to:
  /// **'我的手机'**
  String get pairNameDefault;

  /// No description provided for @pairButton.
  ///
  /// In zh, this message translates to:
  /// **'配对'**
  String get pairButton;

  /// No description provided for @pairWorking.
  ///
  /// In zh, this message translates to:
  /// **'正在配对…'**
  String get pairWorking;

  /// No description provided for @pairInvalidLink.
  ///
  /// In zh, this message translates to:
  /// **'链接不对：{message}'**
  String pairInvalidLink(String message);

  /// No description provided for @pairUpgrade.
  ///
  /// In zh, this message translates to:
  /// **'这个配对链接要更新版本的 App'**
  String get pairUpgrade;

  /// No description provided for @pairPrivacy.
  ///
  /// In zh, this message translates to:
  /// **'连接是端到端加密的：经 relay 时 relay 只转发，看不到内容。这台设备的私钥只存在手机上。'**
  String get pairPrivacy;

  /// No description provided for @scanTitle.
  ///
  /// In zh, this message translates to:
  /// **'扫描配对二维码'**
  String get scanTitle;

  /// No description provided for @scanUnavailable.
  ///
  /// In zh, this message translates to:
  /// **'这台设备不能扫码，请返回粘贴链接'**
  String get scanUnavailable;

  /// No description provided for @hostVersion.
  ///
  /// In zh, this message translates to:
  /// **'pt-tools {version}'**
  String hostVersion(String version);

  /// No description provided for @apiLevelMismatch.
  ///
  /// In zh, this message translates to:
  /// **'主机的 App API 级别是 {level}，这个 App 认得 {supported}，请升级'**
  String apiLevelMismatch(int level, int supported);

  /// No description provided for @kpiUploaded.
  ///
  /// In zh, this message translates to:
  /// **'上传'**
  String get kpiUploaded;

  /// No description provided for @kpiDownloaded.
  ///
  /// In zh, this message translates to:
  /// **'下载'**
  String get kpiDownloaded;

  /// No description provided for @kpiRatio.
  ///
  /// In zh, this message translates to:
  /// **'分享率'**
  String get kpiRatio;

  /// No description provided for @kpiBonus.
  ///
  /// In zh, this message translates to:
  /// **'魔力'**
  String get kpiBonus;

  /// No description provided for @perHour.
  ///
  /// In zh, this message translates to:
  /// **'每小时 {value}'**
  String perHour(String value);

  /// No description provided for @kpiSeeding.
  ///
  /// In zh, this message translates to:
  /// **'做种'**
  String get kpiSeeding;

  /// No description provided for @kpiUnread.
  ///
  /// In zh, this message translates to:
  /// **'站内信未读'**
  String get kpiUnread;

  /// No description provided for @siteCount.
  ///
  /// In zh, this message translates to:
  /// **'{count} 个站点'**
  String siteCount(int count);

  /// No description provided for @todayTitle.
  ///
  /// In zh, this message translates to:
  /// **'今天'**
  String get todayTitle;

  /// No description provided for @todayError.
  ///
  /// In zh, this message translates to:
  /// **'今天的增量算不出来：{message}'**
  String todayError(String message);

  /// No description provided for @todayBySite.
  ///
  /// In zh, this message translates to:
  /// **'各站点'**
  String get todayBySite;

  /// No description provided for @downloadersTitle.
  ///
  /// In zh, this message translates to:
  /// **'下载器'**
  String get downloadersTitle;

  /// No description provided for @downloaderUnreachable.
  ///
  /// In zh, this message translates to:
  /// **'连不上'**
  String get downloaderUnreachable;

  /// No description provided for @freeSpace.
  ///
  /// In zh, this message translates to:
  /// **'剩余 {size}'**
  String freeSpace(String size);

  /// No description provided for @updatedAt.
  ///
  /// In zh, this message translates to:
  /// **'站点数据更新于 {time}'**
  String updatedAt(String time);

  /// No description provided for @torrentsAll.
  ///
  /// In zh, this message translates to:
  /// **'全部'**
  String get torrentsAll;

  /// No description provided for @stateDownloading.
  ///
  /// In zh, this message translates to:
  /// **'下载中'**
  String get stateDownloading;

  /// No description provided for @stateSeeding.
  ///
  /// In zh, this message translates to:
  /// **'做种中'**
  String get stateSeeding;

  /// No description provided for @statePaused.
  ///
  /// In zh, this message translates to:
  /// **'已暂停'**
  String get statePaused;

  /// No description provided for @stateStopped.
  ///
  /// In zh, this message translates to:
  /// **'已停止'**
  String get stateStopped;

  /// No description provided for @stateQueued.
  ///
  /// In zh, this message translates to:
  /// **'排队中'**
  String get stateQueued;

  /// No description provided for @stateChecking.
  ///
  /// In zh, this message translates to:
  /// **'校验中'**
  String get stateChecking;

  /// No description provided for @stateError.
  ///
  /// In zh, this message translates to:
  /// **'出错'**
  String get stateError;

  /// No description provided for @searchTorrentsHint.
  ///
  /// In zh, this message translates to:
  /// **'按标题筛选'**
  String get searchTorrentsHint;

  /// No description provided for @noTorrents.
  ///
  /// In zh, this message translates to:
  /// **'下载器里没有种子'**
  String get noTorrents;

  /// No description provided for @actionPause.
  ///
  /// In zh, this message translates to:
  /// **'暂停'**
  String get actionPause;

  /// No description provided for @actionResume.
  ///
  /// In zh, this message translates to:
  /// **'继续'**
  String get actionResume;

  /// No description provided for @actionDelete.
  ///
  /// In zh, this message translates to:
  /// **'删除'**
  String get actionDelete;

  /// No description provided for @deleteTitle.
  ///
  /// In zh, this message translates to:
  /// **'删除这个种子？'**
  String get deleteTitle;

  /// No description provided for @deleteWithFiles.
  ///
  /// In zh, this message translates to:
  /// **'连同已下载的数据一起删除'**
  String get deleteWithFiles;

  /// No description provided for @actionDone.
  ///
  /// In zh, this message translates to:
  /// **'成功 {ok} 个，失败 {failed} 个'**
  String actionDone(int ok, int failed);

  /// No description provided for @torrentFailures.
  ///
  /// In zh, this message translates to:
  /// **'有 {count} 个下载器没有读到，列表不完整'**
  String torrentFailures(int count);

  /// No description provided for @etaLabel.
  ///
  /// In zh, this message translates to:
  /// **'剩余 {eta}'**
  String etaLabel(String eta);

  /// No description provided for @searchHint.
  ///
  /// In zh, this message translates to:
  /// **'搜索站点上的种子'**
  String get searchHint;

  /// No description provided for @searchIntro.
  ///
  /// In zh, this message translates to:
  /// **'输入关键字，在所有启用的站点上一起搜'**
  String get searchIntro;

  /// No description provided for @searchFreeOnly.
  ///
  /// In zh, this message translates to:
  /// **'只看免费'**
  String get searchFreeOnly;

  /// No description provided for @searchNoResults.
  ///
  /// In zh, this message translates to:
  /// **'没有找到'**
  String get searchNoResults;

  /// No description provided for @searchSites.
  ///
  /// In zh, this message translates to:
  /// **'{count} 个站点 · {ms} ms'**
  String searchSites(int count, int ms);

  /// No description provided for @searchErrors.
  ///
  /// In zh, this message translates to:
  /// **'{count} 个站点出错'**
  String searchErrors(int count);

  /// No description provided for @pushTitle.
  ///
  /// In zh, this message translates to:
  /// **'推送到下载器'**
  String get pushTitle;

  /// No description provided for @pushDefault.
  ///
  /// In zh, this message translates to:
  /// **'默认下载器'**
  String get pushDefault;

  /// No description provided for @pushOk.
  ///
  /// In zh, this message translates to:
  /// **'已推送到 {downloader}'**
  String pushOk(String downloader);

  /// No description provided for @pushSkipped.
  ///
  /// In zh, this message translates to:
  /// **'下载器里已经有这个种子'**
  String get pushSkipped;

  /// No description provided for @pushBlocked.
  ///
  /// In zh, this message translates to:
  /// **'没有推送：{message}'**
  String pushBlocked(String message);

  /// No description provided for @defaultBadge.
  ///
  /// In zh, this message translates to:
  /// **'默认'**
  String get defaultBadge;

  /// No description provided for @free.
  ///
  /// In zh, this message translates to:
  /// **'免费'**
  String get free;

  /// No description provided for @hr.
  ///
  /// In zh, this message translates to:
  /// **'H&R'**
  String get hr;

  /// No description provided for @tabSubscriptions.
  ///
  /// In zh, this message translates to:
  /// **'订阅'**
  String get tabSubscriptions;

  /// No description provided for @tabHistory.
  ///
  /// In zh, this message translates to:
  /// **'最近入库'**
  String get tabHistory;

  /// No description provided for @tabExplore.
  ///
  /// In zh, this message translates to:
  /// **'探索'**
  String get tabExplore;

  /// No description provided for @noSubscriptions.
  ///
  /// In zh, this message translates to:
  /// **'没有订阅；可以在「探索」里订阅'**
  String get noSubscriptions;

  /// No description provided for @noHistory.
  ///
  /// In zh, this message translates to:
  /// **'没有整理记录'**
  String get noHistory;

  /// No description provided for @subActive.
  ///
  /// In zh, this message translates to:
  /// **'订阅中'**
  String get subActive;

  /// No description provided for @subPaused.
  ///
  /// In zh, this message translates to:
  /// **'已暂停'**
  String get subPaused;

  /// No description provided for @subPending.
  ///
  /// In zh, this message translates to:
  /// **'等待中'**
  String get subPending;

  /// No description provided for @subDone.
  ///
  /// In zh, this message translates to:
  /// **'已完成'**
  String get subDone;

  /// No description provided for @subUpgrade.
  ///
  /// In zh, this message translates to:
  /// **'洗版'**
  String get subUpgrade;

  /// No description provided for @subProgress.
  ///
  /// In zh, this message translates to:
  /// **'已入库 {inLibrary}/{aired} 集'**
  String subProgress(int inLibrary, int aired);

  /// No description provided for @subMissing.
  ///
  /// In zh, this message translates to:
  /// **'缺 {list}'**
  String subMissing(String list);

  /// No description provided for @subPause.
  ///
  /// In zh, this message translates to:
  /// **'暂停订阅'**
  String get subPause;

  /// No description provided for @subResume.
  ///
  /// In zh, this message translates to:
  /// **'恢复订阅'**
  String get subResume;

  /// No description provided for @subSearchNow.
  ///
  /// In zh, this message translates to:
  /// **'立即搜索'**
  String get subSearchNow;

  /// No description provided for @subDelete.
  ///
  /// In zh, this message translates to:
  /// **'删除订阅'**
  String get subDelete;

  /// No description provided for @subDeleteConfirm.
  ///
  /// In zh, this message translates to:
  /// **'删除这个订阅？下载器里的种子与媒体库里的文件不动。'**
  String get subDeleteConfirm;

  /// No description provided for @subNextSearch.
  ///
  /// In zh, this message translates to:
  /// **'下次搜索 {time}'**
  String subNextSearch(String time);

  /// No description provided for @episodesTitle.
  ///
  /// In zh, this message translates to:
  /// **'剧集'**
  String get episodesTitle;

  /// No description provided for @epLibrary.
  ///
  /// In zh, this message translates to:
  /// **'已入库'**
  String get epLibrary;

  /// No description provided for @epDownloading.
  ///
  /// In zh, this message translates to:
  /// **'下载中'**
  String get epDownloading;

  /// No description provided for @epMissing.
  ///
  /// In zh, this message translates to:
  /// **'缺'**
  String get epMissing;

  /// No description provided for @epUpcoming.
  ///
  /// In zh, this message translates to:
  /// **'未播出'**
  String get epUpcoming;

  /// No description provided for @subTorrentsTitle.
  ///
  /// In zh, this message translates to:
  /// **'下载过的种子'**
  String get subTorrentsTitle;

  /// No description provided for @exploreMovie.
  ///
  /// In zh, this message translates to:
  /// **'电影'**
  String get exploreMovie;

  /// No description provided for @exploreTv.
  ///
  /// In zh, this message translates to:
  /// **'剧集'**
  String get exploreTv;

  /// No description provided for @exploreTrending.
  ///
  /// In zh, this message translates to:
  /// **'热门'**
  String get exploreTrending;

  /// No description provided for @explorePopular.
  ///
  /// In zh, this message translates to:
  /// **'流行'**
  String get explorePopular;

  /// No description provided for @exploreSearchHint.
  ///
  /// In zh, this message translates to:
  /// **'搜索电影或剧集'**
  String get exploreSearchHint;

  /// No description provided for @subscribe.
  ///
  /// In zh, this message translates to:
  /// **'订阅'**
  String get subscribe;

  /// No description provided for @subscribed.
  ///
  /// In zh, this message translates to:
  /// **'已订阅'**
  String get subscribed;

  /// No description provided for @inLibrary.
  ///
  /// In zh, this message translates to:
  /// **'已入库'**
  String get inLibrary;

  /// No description provided for @subscribeOk.
  ///
  /// In zh, this message translates to:
  /// **'已订阅 {title}'**
  String subscribeOk(String title);

  /// No description provided for @moreSites.
  ///
  /// In zh, this message translates to:
  /// **'站点'**
  String get moreSites;

  /// No description provided for @moreTasks.
  ///
  /// In zh, this message translates to:
  /// **'任务'**
  String get moreTasks;

  /// No description provided for @moreBrush.
  ///
  /// In zh, this message translates to:
  /// **'刷流'**
  String get moreBrush;

  /// No description provided for @moreDownloaders.
  ///
  /// In zh, this message translates to:
  /// **'下载器'**
  String get moreDownloaders;

  /// No description provided for @moreSettings.
  ///
  /// In zh, this message translates to:
  /// **'连接与设备'**
  String get moreSettings;

  /// No description provided for @siteDisabled.
  ///
  /// In zh, this message translates to:
  /// **'未启用'**
  String get siteDisabled;

  /// No description provided for @disabledSites.
  ///
  /// In zh, this message translates to:
  /// **'未启用的站点（{count}）'**
  String disabledSites(int count);

  /// No description provided for @noEnabledSites.
  ///
  /// In zh, this message translates to:
  /// **'没有启用的站点'**
  String get noEnabledSites;

  /// No description provided for @movieNotInLibrary.
  ///
  /// In zh, this message translates to:
  /// **'未入库'**
  String get movieNotInLibrary;

  /// No description provided for @loginDays.
  ///
  /// In zh, this message translates to:
  /// **'离封号还有 {days} 天'**
  String loginDays(int days);

  /// No description provided for @loginUnknown.
  ///
  /// In zh, this message translates to:
  /// **'没有访问记录'**
  String get loginUnknown;

  /// No description provided for @attend.
  ///
  /// In zh, this message translates to:
  /// **'签到'**
  String get attend;

  /// No description provided for @attendOk.
  ///
  /// In zh, this message translates to:
  /// **'签到完成'**
  String get attendOk;

  /// No description provided for @attendSigned.
  ///
  /// In zh, this message translates to:
  /// **'今天已签到'**
  String get attendSigned;

  /// No description provided for @attendFailed.
  ///
  /// In zh, this message translates to:
  /// **'签到失败'**
  String get attendFailed;

  /// No description provided for @attendPending.
  ///
  /// In zh, this message translates to:
  /// **'今天未签到'**
  String get attendPending;

  /// No description provided for @attendUnsupported.
  ///
  /// In zh, this message translates to:
  /// **'不支持签到'**
  String get attendUnsupported;

  /// No description provided for @unreadMessages.
  ///
  /// In zh, this message translates to:
  /// **'{count} 条未读'**
  String unreadMessages(int count);

  /// No description provided for @siteUserError.
  ///
  /// In zh, this message translates to:
  /// **'站点上的用户数据没读到：{message}'**
  String siteUserError(String message);

  /// No description provided for @noTasks.
  ///
  /// In zh, this message translates to:
  /// **'没有推送记录'**
  String get noTasks;

  /// No description provided for @taskPushed.
  ///
  /// In zh, this message translates to:
  /// **'已推送'**
  String get taskPushed;

  /// No description provided for @taskNotPushed.
  ///
  /// In zh, this message translates to:
  /// **'未推送'**
  String get taskNotPushed;

  /// No description provided for @taskCompleted.
  ///
  /// In zh, this message translates to:
  /// **'已完成'**
  String get taskCompleted;

  /// No description provided for @noBrush.
  ///
  /// In zh, this message translates to:
  /// **'没有刷流任务'**
  String get noBrush;

  /// No description provided for @brushDisabled.
  ///
  /// In zh, this message translates to:
  /// **'已停用'**
  String get brushDisabled;

  /// No description provided for @brushActive.
  ///
  /// In zh, this message translates to:
  /// **'在刷 {count} 个'**
  String brushActive(int count);

  /// No description provided for @brushToday.
  ///
  /// In zh, this message translates to:
  /// **'今天 ↑{up} ↓{down}'**
  String brushToday(String up, String down);

  /// No description provided for @brushTotal.
  ///
  /// In zh, this message translates to:
  /// **'累计 ↑{up} ↓{down}'**
  String brushTotal(String up, String down);

  /// No description provided for @noDownloaders.
  ///
  /// In zh, this message translates to:
  /// **'没有启用的下载器'**
  String get noDownloaders;

  /// No description provided for @settingsConnection.
  ///
  /// In zh, this message translates to:
  /// **'连接'**
  String get settingsConnection;

  /// No description provided for @settingsVia.
  ///
  /// In zh, this message translates to:
  /// **'方式'**
  String get settingsVia;

  /// No description provided for @settingsEndpoint.
  ///
  /// In zh, this message translates to:
  /// **'地址'**
  String get settingsEndpoint;

  /// No description provided for @settingsHost.
  ///
  /// In zh, this message translates to:
  /// **'主机'**
  String get settingsHost;

  /// No description provided for @settingsReconnect.
  ///
  /// In zh, this message translates to:
  /// **'重新连接'**
  String get settingsReconnect;

  /// No description provided for @settingsDevice.
  ///
  /// In zh, this message translates to:
  /// **'这台设备'**
  String get settingsDevice;

  /// No description provided for @settingsPairedAt.
  ///
  /// In zh, this message translates to:
  /// **'配对于 {time}'**
  String settingsPairedAt(String time);

  /// No description provided for @settingsScopes.
  ///
  /// In zh, this message translates to:
  /// **'权限'**
  String get settingsScopes;

  /// No description provided for @scopeFull.
  ///
  /// In zh, this message translates to:
  /// **'完全控制'**
  String get scopeFull;

  /// No description provided for @scopeRead.
  ///
  /// In zh, this message translates to:
  /// **'只读'**
  String get scopeRead;

  /// No description provided for @settingsForget.
  ///
  /// In zh, this message translates to:
  /// **'忘掉这台主机'**
  String get settingsForget;

  /// No description provided for @forgetConfirm.
  ///
  /// In zh, this message translates to:
  /// **'忘掉以后要重新扫码配对。主机上的设备记录不会删除，需要的话在网页上撤销。'**
  String get forgetConfirm;
}

class _SDelegate extends LocalizationsDelegate<S> {
  const _SDelegate();

  @override
  Future<S> load(Locale locale) {
    return SynchronousFuture<S>(lookupS(locale));
  }

  @override
  bool isSupported(Locale locale) =>
      <String>['en', 'zh'].contains(locale.languageCode);

  @override
  bool shouldReload(_SDelegate old) => false;
}

S lookupS(Locale locale) {
  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'en':
      return SEn();
    case 'zh':
      return SZh();
  }

  throw FlutterError(
    'S.delegate failed to load unsupported locale "$locale". This is likely '
    'an issue with the localizations generation tool. Please file an issue '
    'on GitHub with a reproducible sample app and the gen-l10n configuration '
    'that was used.',
  );
}
