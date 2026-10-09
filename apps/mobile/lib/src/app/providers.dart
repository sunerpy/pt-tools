// 全局状态：配对过的主机、连接、App API 客户端与各页面的数据。
import 'dart:async';
import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../remote/connection.dart';
import '../remote/host_record.dart';
import 'storage.dart';

/// App 的名字与版本（握手时告诉主机）。
const appClientName = 'pt-tools-app/1.0.0';

/// App 认得的 App API 兼容级别（主机的 remote_api_level 不是它时提示升级）。
const supportedApiLevel = 1;

final hostStoreProvider = Provider<HostStore>((ref) => SecureHostStore());

/// 连接用的通道（测试里换成内存的；null 时用 WebSocket）。
final channelFactoryProvider = Provider<ChannelFactory?>((ref) => null);

/// 配对过的主机，没有时为 null。
final hostProvider = AsyncNotifierProvider<HostNotifier, HostRecord?>(
  HostNotifier.new,
);

class HostNotifier extends AsyncNotifier<HostRecord?> {
  @override
  Future<HostRecord?> build() => ref.read(hostStoreProvider).load();

  /// 配对成功：存下来。
  Future<void> paired(HostRecord host) async {
    await ref.read(hostStoreProvider).save(host);
    state = AsyncData(host);
  }

  /// 忘掉这台主机（之后要重新配对）。
  Future<void> forget() async {
    await ref.read(hostStoreProvider).clear();
    state = const AsyncData(null);
  }
}

/// 到这台主机的连接（没有配对时为 null）。App 在前台时保持连接，见 AppLifecycle。
final connectionProvider = Provider<HostConnection?>((ref) {
  final host = ref.watch(hostProvider.select((v) => v.value));
  if (host == null) return null;
  final c = HostConnection(
    host,
    clientName: appClientName,
    channel: ref.read(channelFactoryProvider),
  );
  ref.onDispose(c.dispose);
  c.start();
  return c;
});

/// 连接状态。
final connectionStatusProvider = StreamProvider<ConnectionStatus>((ref) {
  final c = ref.watch(connectionProvider);
  if (c == null) return Stream.value(const ConnectionStatus(LinkState.offline));
  return Stream.multi((ctl) {
    ctl.add(c.status);
    final sub = c.statuses.listen(ctl.add);
    ctl.onCancel = sub.cancel;
  });
});

/// 每连上一次加一：依赖它的数据在重连（例如权限改了）以后重新读。
final onlineEpochProvider = NotifierProvider<OnlineEpoch, int>(OnlineEpoch.new);

class OnlineEpoch extends Notifier<int> {
  @override
  int build() {
    ref.listen(connectionStatusProvider, (prev, next) {
      final was = prev?.value?.state == LinkState.online;
      if (!was && next.value?.state == LinkState.online) state++;
    });
    return 0;
  }
}

/// App API 客户端（经隧道；没有配对时为 null）。
final apiProvider = Provider<AppApi?>((ref) {
  final c = ref.watch(connectionProvider);
  if (c == null) return null;
  final client = ApiClient(basePath: 'https://pt-tools/api/app/v1')
    ..client = c.client;
  return AppApi(client);
});

AppApi requireApi(Ref ref) =>
    ref.watch(apiProvider) ?? (throw StateError('还没有配对'));

T _need<T>(T? v) => v ?? (throw const FormatException('回应是空的'));

/// 版本、兼容级别与这台设备的权限。
final metaProvider = FutureProvider<AppMeta>((ref) async {
  ref.watch(onlineEpochProvider);
  return _need(await requireApi(ref).getMeta());
});

/// 这台设备能不能做写操作（权限以主机现在说的为准）。
final canWriteProvider = Provider<bool>((ref) {
  final meta = ref.watch(metaProvider).value;
  if (meta != null) return meta.principal.scopes.contains('app:write');
  return ref.watch(hostProvider).value?.canWrite ?? false;
});

final overviewProvider = FutureProvider.autoDispose<AppOverview>((ref) async {
  ref.watch(onlineEpochProvider);
  return _need(await requireApi(ref).getOverview());
});

final downloadersProvider = FutureProvider.autoDispose<AppDownloaderList>((
  ref,
) async {
  ref.watch(onlineEpochProvider);
  return _need(await requireApi(ref).listDownloaders());
});

final sitesProvider = FutureProvider.autoDispose<AppSiteList>((ref) async {
  ref.watch(onlineEpochProvider);
  return _need(await requireApi(ref).listSites());
});

/// 种子列表的筛选。
class TorrentQuery {
  const TorrentQuery({this.state, this.q});
  final String? state;
  final String? q;
  @override
  bool operator ==(Object other) =>
      other is TorrentQuery && other.state == state && other.q == q;
  @override
  int get hashCode => Object.hash(state, q);
}

final torrentsProvider = FutureProvider.autoDispose
    .family<AppTorrentPage, TorrentQuery>((ref, query) async {
      ref.watch(onlineEpochProvider);
      return _need(
        await requireApi(ref)
            .listTorrents(state: query.state, q: query.q, pageSize: 200),
      );
    });

final tasksProvider = FutureProvider.autoDispose<AppTaskPage>((ref) async {
  ref.watch(onlineEpochProvider);
  return _need(await requireApi(ref).listTasks(pageSize: 100));
});

final brushTasksProvider = FutureProvider.autoDispose<List<AppBrushTask>>((
  ref,
) async {
  ref.watch(onlineEpochProvider);
  return _need(await requireApi(ref).listBrushTasks());
});

final subscriptionsProvider = FutureProvider.autoDispose<List<AppSubscription>>(
  (ref) async {
    ref.watch(onlineEpochProvider);
    return _need(await requireApi(ref).listSubscriptions());
  },
);

final subscriptionProvider = FutureProvider.autoDispose
    .family<AppSubscriptionDetail, int>((ref, id) async {
      ref.watch(onlineEpochProvider);
      return _need(await requireApi(ref).getSubscription(id));
    });

final mediaHistoryProvider = FutureProvider.autoDispose<AppMediaHistoryPage>((
  ref,
) async {
  ref.watch(onlineEpochProvider);
  return _need(await requireApi(ref).listMediaHistory(pageSize: 50));
});

/// 探索的条件。
class ExploreQuery {
  const ExploreQuery(this.kind, {this.list = 'trending', this.q});
  final String kind;
  final String list;
  final String? q;
  @override
  bool operator ==(Object other) =>
      other is ExploreQuery &&
      other.kind == kind &&
      other.list == list &&
      other.q == q;
  @override
  int get hashCode => Object.hash(kind, list, q);
}

final exploreProvider = FutureProvider.autoDispose
    .family<AppExplorePage, ExploreQuery>((ref, query) async {
      ref.watch(onlineEpochProvider);
      return _need(
        await requireApi(ref).explore(query.kind, list: query.list, q: query.q),
      );
    });

/// 把接口的错误写成给人看的一句话（App API 的错误是 {"error","message"}）。
String describeError(Object e) {
  if (e is ApiException) {
    final body = e.message;
    if (body != null && body.isNotEmpty) {
      try {
        final v = jsonDecode(body);
        if (v is Map && v['message'] is String) return v['message'] as String;
      } on FormatException {
        // 不是 JSON，原样给
      }
      return body;
    }
    return 'HTTP ${e.code}';
  }
  return e.toString().replaceFirst(RegExp(r'^[A-Za-z]+Exception: '), '');
}

/// 便于测试替换的定时刷新间隔。
final refreshIntervalProvider = Provider<Duration>(
  (ref) => const Duration(seconds: 5),
);

/// 每隔一段时间触发一次（种子列表、下载器速度用）；App 在后台（连接停了）时不触发。
final tickerProvider = StreamProvider.autoDispose<int>((ref) {
  final every = ref.watch(refreshIntervalProvider);
  final conn = ref.watch(connectionProvider);
  var n = 0;
  return Stream<void>.periodic(every)
      .where((_) => !(conn?.paused ?? false))
      .map((_) => ++n);
});
