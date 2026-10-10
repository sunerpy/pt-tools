// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class SEn extends S {
  SEn([String locale = 'en']) : super(locale);

  @override
  String get appTitle => 'pt-tools';

  @override
  String get retry => 'Retry';

  @override
  String get cancel => 'Cancel';

  @override
  String get paste => 'Paste';

  @override
  String get actions => 'Actions';

  @override
  String get empty => 'Nothing here';

  @override
  String errorPrefix(String message) {
    return 'Something went wrong: $message';
  }

  @override
  String get navOverview => 'Overview';

  @override
  String get navTorrents => 'Torrents';

  @override
  String get navSearch => 'Search';

  @override
  String get navMedia => 'Media';

  @override
  String get navMore => 'More';

  @override
  String get connConnecting => 'Connecting…';

  @override
  String get connOnline => 'Connected';

  @override
  String connFailed(String message) {
    return 'Can\'t connect: $message';
  }

  @override
  String get connRevoked => 'This device was revoked. Pair it again.';

  @override
  String get connKeyRotated =>
      'The host\'s keys changed. Pair this device again.';

  @override
  String get connDisabled => 'Remote access is turned off on the host';

  @override
  String get connOffline => 'Not connected';

  @override
  String get repair => 'Pair again';

  @override
  String get viaDirect => 'Direct';

  @override
  String get viaRelay => 'Relay';

  @override
  String get pairTitle => 'Pair with pt-tools';

  @override
  String get pairIntro =>
      'In pt-tools, open System → Remote access → Add device, then scan the QR code or paste the link below.';

  @override
  String get pairScan => 'Scan QR code';

  @override
  String get pairLinkLabel => 'Pairing link';

  @override
  String get pairNameLabel => 'Device name';

  @override
  String get pairNameDefault => 'My phone';

  @override
  String get pairButton => 'Pair';

  @override
  String get pairWorking => 'Pairing…';

  @override
  String pairInvalidLink(String message) {
    return 'That link isn\'t valid: $message';
  }

  @override
  String get pairUpgrade =>
      'This pairing link needs a newer version of the app';

  @override
  String pairSaveFailed(String error, String name) {
    return 'The host has added this device, but saving it to the phone\'s secure storage failed ($error). Revoke \"$name\" in the web UI, then pair again.';
  }

  @override
  String get pairPrivacy =>
      'Connections are end-to-end encrypted: a relay only forwards traffic and cannot read it. This device\'s private key never leaves the phone.';

  @override
  String get scanTitle => 'Scan the pairing QR code';

  @override
  String get scanUnavailable =>
      'Scanning isn\'t available on this device. Go back and paste the link.';

  @override
  String hostVersion(String version) {
    return 'pt-tools $version';
  }

  @override
  String apiLevelMismatch(int level, int supported) {
    return 'The host uses App API level $level; this app supports $supported. Please update.';
  }

  @override
  String get kpiUploaded => 'Uploaded';

  @override
  String get kpiDownloaded => 'Downloaded';

  @override
  String get kpiRatio => 'Ratio';

  @override
  String get kpiBonus => 'Bonus';

  @override
  String perHour(String value) {
    return '$value per hour';
  }

  @override
  String get kpiSeeding => 'Seeding';

  @override
  String get kpiUnread => 'Unread messages';

  @override
  String siteCount(int count) {
    return '$count sites';
  }

  @override
  String get todayTitle => 'Today';

  @override
  String todayError(String message) {
    return 'Today\'s numbers aren\'t available: $message';
  }

  @override
  String get todayBySite => 'By site';

  @override
  String get downloadersTitle => 'Downloaders';

  @override
  String get downloaderUnreachable => 'Unreachable';

  @override
  String freeSpace(String size) {
    return '$size free';
  }

  @override
  String updatedAt(String time) {
    return 'Site data updated $time';
  }

  @override
  String get torrentsAll => 'All';

  @override
  String get stateDownloading => 'Downloading';

  @override
  String get stateSeeding => 'Seeding';

  @override
  String get statePaused => 'Paused';

  @override
  String get stateStopped => 'Stopped';

  @override
  String get stateQueued => 'Queued';

  @override
  String get stateChecking => 'Checking';

  @override
  String get stateError => 'Error';

  @override
  String get searchTorrentsHint => 'Filter by title';

  @override
  String get noTorrents => 'No torrents in your downloaders';

  @override
  String get actionPause => 'Pause';

  @override
  String get actionResume => 'Resume';

  @override
  String get actionDelete => 'Delete';

  @override
  String get deleteTitle => 'Delete this torrent?';

  @override
  String get deleteWithFiles => 'Also delete downloaded data';

  @override
  String actionDone(int ok, int failed) {
    return '$ok succeeded, $failed failed';
  }

  @override
  String torrentFailures(int count) {
    return '$count downloader(s) couldn\'t be read; the list is incomplete';
  }

  @override
  String etaLabel(String eta) {
    return 'ETA $eta';
  }

  @override
  String get searchHint => 'Search torrents on your sites';

  @override
  String get searchIntro =>
      'Type a keyword to search all enabled sites at once';

  @override
  String get searchFreeOnly => 'Free only';

  @override
  String get searchNoResults => 'No results';

  @override
  String searchSites(int count, int ms) {
    return '$count sites · $ms ms';
  }

  @override
  String searchErrors(int count) {
    return '$count site(s) failed';
  }

  @override
  String get pushTitle => 'Send to downloader';

  @override
  String get pushDefault => 'Default downloader';

  @override
  String pushOk(String downloader) {
    return 'Sent to $downloader';
  }

  @override
  String get pushSkipped => 'The downloader already has this torrent';

  @override
  String pushBlocked(String message) {
    return 'Not sent: $message';
  }

  @override
  String get defaultBadge => 'Default';

  @override
  String get free => 'Free';

  @override
  String get hr => 'H&R';

  @override
  String get tabSubscriptions => 'Subscriptions';

  @override
  String get tabHistory => 'Recently added';

  @override
  String get tabExplore => 'Explore';

  @override
  String get noSubscriptions => 'No subscriptions yet; subscribe from Explore';

  @override
  String get noHistory => 'Nothing organized yet';

  @override
  String get subActive => 'Active';

  @override
  String get subPaused => 'Paused';

  @override
  String get subPending => 'Pending';

  @override
  String get subDone => 'Done';

  @override
  String get subUpgrade => 'Upgrade';

  @override
  String subProgress(int inLibrary, int aired) {
    return '$inLibrary/$aired episodes in library';
  }

  @override
  String subMissing(String list) {
    return 'Missing $list';
  }

  @override
  String get subPause => 'Pause';

  @override
  String get subResume => 'Resume';

  @override
  String get subSearchNow => 'Search now';

  @override
  String get subDelete => 'Delete subscription';

  @override
  String get subDeleteConfirm =>
      'Delete this subscription? Torrents in your downloaders and files in your library are kept.';

  @override
  String subNextSearch(String time) {
    return 'Next search $time';
  }

  @override
  String get episodesTitle => 'Episodes';

  @override
  String get epLibrary => 'In library';

  @override
  String get epDownloading => 'Downloading';

  @override
  String get epMissing => 'Missing';

  @override
  String get epUpcoming => 'Not aired';

  @override
  String get subTorrentsTitle => 'Downloaded torrents';

  @override
  String get exploreMovie => 'Movies';

  @override
  String get exploreTv => 'TV';

  @override
  String get exploreTrending => 'Trending';

  @override
  String get explorePopular => 'Popular';

  @override
  String get exploreSearchHint => 'Search movies or TV';

  @override
  String get subscribe => 'Subscribe';

  @override
  String get subscribed => 'Subscribed';

  @override
  String get inLibrary => 'In library';

  @override
  String subscribeOk(String title) {
    return 'Subscribed to $title';
  }

  @override
  String get moreSites => 'Sites';

  @override
  String get moreTasks => 'Tasks';

  @override
  String get moreBrush => 'Brush';

  @override
  String get moreDownloaders => 'Downloaders';

  @override
  String get moreSettings => 'Connection & device';

  @override
  String get siteDisabled => 'Disabled';

  @override
  String disabledSites(int count) {
    return 'Disabled sites ($count)';
  }

  @override
  String get noEnabledSites => 'No sites enabled';

  @override
  String get movieNotInLibrary => 'Not in library';

  @override
  String loginDays(int days) {
    return '$days days before the inactivity limit';
  }

  @override
  String get loginUnknown => 'No visit recorded';

  @override
  String get attend => 'Check in';

  @override
  String get attendOk => 'Checked in';

  @override
  String get attendSigned => 'Checked in today';

  @override
  String get attendFailed => 'Check-in failed';

  @override
  String get attendPending => 'Not checked in today';

  @override
  String get attendUnsupported => 'Check-in not supported';

  @override
  String unreadMessages(int count) {
    return '$count unread';
  }

  @override
  String siteUserError(String message) {
    return 'Couldn\'t read user data from the sites: $message';
  }

  @override
  String get noTasks => 'No tasks yet';

  @override
  String get taskPushed => 'Sent';

  @override
  String get taskNotPushed => 'Not sent';

  @override
  String get taskCompleted => 'Completed';

  @override
  String get noBrush => 'No brush tasks';

  @override
  String get brushDisabled => 'Disabled';

  @override
  String brushActive(int count) {
    return '$count active';
  }

  @override
  String brushToday(String up, String down) {
    return 'Today ↑$up ↓$down';
  }

  @override
  String brushTotal(String up, String down) {
    return 'Total ↑$up ↓$down';
  }

  @override
  String get noDownloaders => 'No downloaders enabled';

  @override
  String get settingsConnection => 'Connection';

  @override
  String get settingsVia => 'Via';

  @override
  String get settingsEndpoint => 'Address';

  @override
  String get settingsHost => 'Host';

  @override
  String get settingsReconnect => 'Reconnect';

  @override
  String get settingsDevice => 'This device';

  @override
  String settingsPairedAt(String time) {
    return 'Paired $time';
  }

  @override
  String get settingsScopes => 'Permissions';

  @override
  String get scopeFull => 'Full control';

  @override
  String get scopeRead => 'Read-only';

  @override
  String get settingsForget => 'Forget this host';

  @override
  String get forgetConfirm =>
      'You\'ll need to scan again to pair. The device record stays on the host; revoke it in the web UI if needed.';
}
