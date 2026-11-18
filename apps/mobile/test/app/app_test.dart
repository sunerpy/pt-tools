// 界面测试：整个 App 对着内存里的假主机（罐头 App API）跑，中文界面。
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:pt_tools_app/src/app/app.dart';
import 'package:pt_tools_app/src/app/appearance.dart';
import 'package:pt_tools_app/src/app/pages/assistant_page.dart';
import 'package:pt_tools_app/src/app/providers.dart';
import 'package:pt_tools_app/src/app/storage.dart';
import 'package:pt_tools_app/src/app/theme.dart';
import 'package:pt_tools_app/src/app/widgets/common.dart';
import 'package:pt_tools_app/src/remote/bytes.dart';
import 'package:pt_tools_app/src/remote/host_record.dart';
import 'package:pt_tools_app/src/remote/noise.dart';
import 'package:pt_tools_app/src/remote/session.dart';

import '../remote/fake_host.dart';

const _hostId = 'ph5tr32bj27nk7zphjn2qf4z5v';

Map<String, Object?> _meta(List<String> scopes) => {
  'name': 'pt-tools',
  'version': 'v1.0.0-test',
  'remote_api_level': 1,
  'features': ['overview', 'assistant'],
  'principal': {'kind': 'remote_device', 'name': '测试手机', 'scopes': scopes},
};

const _at = '2026-10-11T08:00:00Z';

/// AI 助手：一个会话，最后一条是要确认的暂停（确认以后变 approved）。
Map<String, Object?> _assistantDetail(String status) => {
  'id': 7,
  'title': '暂停那个种子',
  'entry': 'app',
  'running': status == 'pending',
  'created_at': _at,
  'updated_at': _at,
  'run_id': status == 'pending' ? 'r1' : null,
  'last_seq': 100,
  'messages': [
    {
      'id': 1,
      'role': 'user',
      'content': '暂停 abc',
      'run_id': 'r1',
      'created_at': _at,
    },
    {
      'id': 2,
      'role': 'assistant',
      'content': '**好的**，暂停这个种子。',
      'run_id': 'r1',
      'created_at': _at,
      'tool_calls': [
        {
          'id': 'c1',
          'name': 'pause_torrent',
          'arguments': '{"task_id":"abc"}',
          'write': true,
        },
      ],
    },
  ],
  'confirmations': [
    {
      'id': 9,
      'run_id': 'r1',
      'tool_call_id': 'c1',
      'tool': 'pause_torrent',
      'summary': '暂停种子：下载器 1，任务 abc',
      'arguments': {'task_id': 'abc'},
      'status': status,
      'expires_at': _at,
      'created_at': _at,
    },
  ],
}..removeWhere((k, v) => v == null);

const _gib = 1024 * 1024 * 1024;

String _ymd(DateTime d) =>
    '${d.year}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';

/// GET /brush/stats 的罐头：按日 30 段（今天 5 GiB），按周 12 段（8 月 17 日那一周起，上周 20 GiB、本周 30 GiB）；
/// 任务 1 本期按日 5 GiB / 1 GiB、按周 7 GiB / 2 GiB。
Map<String, Object?> _brushStats(String gran) {
  final week = gran == 'week';
  final n = week ? 12 : 30;
  Map<String, Object?> point(int i, int up, int down) {
    final start = week
        ? DateTime(2026, 8, 17 + i * 7)
        : DateTime(2026, 9, 30 + i);
    final end = week ? start.add(const Duration(days: 6)) : start;
    return {
      'start': _ymd(start),
      'end': _ymd(end),
      'uploaded': up,
      'downloaded': down,
      'added': i,
      'removed': 0,
    };
  }

  final last = n - 1;
  return {
    'granularity': week ? 'week' : 'day',
    'periods': n,
    'from': point(0, 0, 0)['start'],
    'to': '2026-10-29',
    'totals': [
      for (var i = 0; i < n; i++)
        point(
          i,
          i == last
              ? (week ? 30 : 5) * _gib
              : (i == last - 1 ? (week ? 20 : 4) * _gib : 0),
          i == last ? 4 * _gib : 0,
        ),
    ],
    'tasks': [
      {
        'id': 1,
        'name': '测试刷流',
        'periods': [
          for (var i = 0; i < n; i++)
            point(
              i,
              i == last ? (week ? 7 : 5) * _gib : 0,
              i == last ? (week ? 2 : 1) * _gib : 0,
            ),
        ],
      },
    ],
  };
}

final _overview = {
  'totals': {
    'uploaded': 11544872091648,
    'downloaded': 2199023255552,
    'ratio': 5.25,
    'seeding': 8,
    'leeching': 1,
    'bonus': 4567.8,
    'bonus_per_hour': 12.5,
    'seeding_size': 1099511627776,
    'site_count': 3,
    'unread_messages': 2,
  },
  'today': {
    'from': '2026-10-09',
    'to': '2026-10-09',
    'uploaded': 1073741824,
    'downloaded': 0,
    'bonus': 10,
    'sites': [],
  },
  'updated_at': 1791500000,
};

/// 下载器做种：一台在做种（另有下载完了没在做种的），一台连不上（原因很长）
final _seeding = {
  'total_seeding_count': 3,
  'total_seeding_size': 101468602368,
  'items': [
    {
      'id': 1,
      'name': 'qb-nas',
      'type': 'qbittorrent',
      'reachable': true,
      'seeding_count': 3,
      'seeding_size': 101468602368,
      'idle_count': 1,
      'idle_size': 12884901888,
    },
    {
      'id': 2,
      'name': 'qb-offline',
      'type': 'qbittorrent',
      'reachable': false,
      'error': '下载器 qb-offline 暂不可用，26s 前创建失败，33s 后重试: failed to create downloader qb-offline after 6 attempts: 连接被拒绝，请检查: 1) qBittorrent 是否正在运行 2) WebUI 是否已启用 3) 端口是否正确 (原始错误: Post "<地址>": dial tcp <地址>: connect: connection refused)',
      'seeding_count': 0,
      'seeding_size': 0,
      'idle_count': 0,
      'idle_size': 0,
    },
  ],
};

/// 站点列表：一个站点，带站点上的用户数据
final _sites = {
  'items': [
    {
      'name': 'hdsky',
      'display_name': 'HDSky',
      'enabled': true,
      'login': {'tier': 'none', 'days_remaining': 30},
      'attendance': {'supported': false, 'enabled': false, 'day': '2026-10-09'},
      'user': {
        'username': 'fakeuser',
        'uploaded': 11544872091648,
        'downloaded': 2199023255552,
        'ratio': 5.25,
        'bonus': 4567.8,
        'seeding': 8,
        'leeching': 1,
        'unread_messages': 0,
        'updated_at': 1791500000,
      },
    },
  ],
};

final _torrents = {
  'items': [
    {
      'downloader_id': 1,
      'downloader': 'qb',
      'task_id': 'h1',
      'info_hash': 'h1',
      'title': 'Some.Movie.2026.1080p',
      'progress': 42.5,
      'size': 2147483648,
      'state': 'downloading',
      'ratio': 0.1,
      'seeds': 5,
      'peers': 2,
      'upload_speed': 1024,
      'download_speed': 2048000,
      'eta': 600,
      'added_at': 1791400000,
      'completed_at': 0,
    },
  ],
  'total': 1,
  'page': 1,
  'page_size': 200,
  'failures': [],
};

/// 安全存储写不进去（例如 Keystore 出错）。
class _BrokenStore extends MemoryHostStore {
  @override
  Future<void> save(HostRecord h) async => throw StateError('keystore 坏了');
}

class _World {
  _World(
    this.scopes, {
    this.mode = 'device',
    this.mobileFeatures = false,
    this.brushStats = false,
    this.downloaderSeeding = false,
  });
  final bool mobileFeatures;

  /// 主机提供各台下载器的做种情况（features 里有 downloader_seeding）
  final bool downloaderSeeding;

  /// 接下来几次 GET /downloaders/seeding 回 502
  int failSeeding = 0;

  /// 主机提供按周期分段的刷流统计（features 里有 brush_stats）
  final bool brushStats;

  /// 这些周期的 GET /brush/stats 回 502
  final Set<String> failStats = {};
  List<String> scopes;
  final String mode;
  final hosts = <FakeHost>[];
  final requests = <String>[];
  late DhKey hostKey;

  /// 种子列表回 502（下载器连不上）
  bool failTorrents = false;

  /// 探索里第一部是不是已订阅（模拟在网页上删掉或建了这部剧的订阅）
  bool exploreSubscribed = true;

  /// 订阅 5 的版本锁：开没开、锁住的组与分辨率
  bool lockOn = true;
  String lockGroup = 'HHWEB';
  String lockRes = '1080p';

  /// AI 助手启用没有
  bool assistantEnabled = true;

  /// 确认过了没有
  bool approved = false;

  /// 接下来几次长轮询回 reset（每回一次，下一次读会话先失败一次）
  int eventsReset = 0;

  /// 接下来几次读会话回 502
  int detailFailures = 0;

  /// 设了以后长轮询一直等着它（模拟主机上没有新事件）
  Completer<void>? holdEvents;

  /// 设了以后发消息等着它才回
  Completer<void>? holdSend;

  Object? api(String method, String path) {
    requests.add('$method $path');
    final p = Uri.parse(path).path;
    return switch (p) {
      '/api/app/v1/meta' =>
        _meta(scopes)
          ..['features'] = [
            'overview',
            'assistant',
            if (mobileFeatures) ...[
              'mobile_editors',
              'brush_manage',
              'stats_export',
            ],
            if (brushStats) 'brush_stats',
            if (downloaderSeeding) 'downloader_seeding',
          ],
      '/api/app/v1/overview' => _overview,
      '/api/app/v1/downloaders' => {'items': []},
      '/api/app/v1/downloaders/seeding' when failSeeding > 0 => () {
        failSeeding--;
        return const ApiReply(502, {'error': 'internal', 'message': '主机在重启'});
      }(),
      '/api/app/v1/downloaders/seeding' => _seeding,
      '/api/app/v1/stats/summary' => {
        'totalUploaded': 1073741824,
        'totalDownloaded': 1024,
        'totalBonus': 12,
        'sites': [
          {'site': 'hdsky', 'hasBaseline': true},
        ],
      },
      '/api/app/v1/brush/tasks' => [
        {
          'id': 1,
          'name': '测试刷流',
          'enabled': true,
          'site': 'hdsky',
          'site_enabled': true,
          'downloader': 'qb',
          'active': 1,
          'downloading': 0,
          'active_size': 0,
          'today': {'uploaded': 0, 'downloaded': 0, 'added': 0, 'removed': 0},
          'total': {'uploaded': 0, 'downloaded': 0, 'added': 0, 'removed': 0},
        },
      ],
      '/api/app/v1/brush/stats'
          when failStats.contains(
            Uri.parse(path).queryParameters['granularity'],
          ) =>
        const ApiReply(502, {'error': 'unavailable', 'message': '主机在重启'}),
      '/api/app/v1/brush/stats' => _brushStats(
        Uri.parse(path).queryParameters['granularity'] ?? 'day',
      ),
      '/api/app/v1/brush/tasks/1' => {'id': 1, 'name': '测试刷流', 'enabled': true},
      '/api/app/v1/brush/tasks/1/runs' => {
        'total': 1,
        'items': [
          {
            'id': 1,
            'status': 'partial',
            'summary': '一轮结束',
            'error': '下载器暂不可用',
            'started_at': _at,
            'steps': [
              {'kind': 'error', 'text': '磁盘查询失败'},
            ],
          },
        ],
      },
      '/api/app/v1/sites' => _sites,
      '/api/app/v1/explore' => {
        'items': [
          for (final (i, t) in const [
            '一部名字很长很长的剧集，标题要占满两行',
            '短名',
            '第三部',
          ].indexed)
            {
              'id': 100 + i,
              'media_type': 'tv',
              'title': t,
              'year': 2026,
              'vote_average': 7.9,
              'in_library': false,
              'subscribed': i == 0 && exploreSubscribed,
            },
        ],
        'page': 1,
        'total_pages': 1,
      },
      '/api/app/v1/subscriptions' => <Object?>[],
      '/api/app/v1/subscriptions/5' => {
        'id': 5,
        'media_type': 'tv',
        'tmdb_id': 100,
        'season': 4,
        'title': '现在就出发',
        'status': 'active',
        'upgrade': false,
        'source': 'manual',
        'progress': {
          'total': 6,
          'aired': 3,
          'in_library': 2,
          'downloading': 0,
          'missing': 1,
        },
        'created_at': 1791400000,
        'sites': <Object?>[],
        'lock_version': lockOn,
        'lock_group': lockGroup,
        'lock_resolution': lockRes,
        'episodes': <Object?>[],
        'torrents': <Object?>[],
      },
      '/api/app/v1/favicon/hdsky' => const ApiReply(404, {
        'error': 'not_found',
        'message': '没有图标',
      }),
      '/api/app/v1/torrents' =>
        failTorrents
            ? const ApiReply(502, {
                'error': 'downloader_unavailable',
                'message': '下载器 qb 连不上',
              })
            : _torrents,
      '/api/app/v1/torrents/actions' => {
        'succeeded': 1,
        'failed': 0,
        'results': [],
      },
      '/api/app/v1/assistant/status' => {
        'enabled': assistantEnabled,
        'model': assistantEnabled ? 'deepseek-flash' : '',
        'can_confirm': assistantEnabled && scopes.contains('app:write'),
        'chatops': false,
      },
      '/api/app/v1/assistant/conversations' => [
        {
          'id': 7,
          'title': '暂停那个种子',
          'entry': 'app',
          'running': !approved,
          'created_at': _at,
          'updated_at': _at,
        },
      ],
      '/api/app/v1/assistant/conversations/7' when detailFailures > 0 => () {
        detailFailures--;
        return const ApiReply(502, {
          'error': 'unavailable',
          'message': '主机在重启',
        });
      }(),
      '/api/app/v1/assistant/conversations/7' => _assistantDetail(
        approved ? 'approved' : 'pending',
      ),
      '/api/app/v1/assistant/conversations/7/events' when eventsReset > 0 =>
        () {
          eventsReset--;
          detailFailures++;
          return {'events': [], 'reset': true, 'last': 0, 'running': true};
        }(),
      '/api/app/v1/assistant/conversations/7/events' when holdEvents != null =>
        holdEvents!.future.then<Object?>(
          (_) => {'events': [], 'reset': false, 'last': 100, 'running': false},
        ),
      '/api/app/v1/assistant/conversations/7/events' => {
        'events': [],
        'reset': false,
        'last': 100,
        'running': !approved,
      },
      '/api/app/v1/assistant/conversations/7/messages' =>
        (holdSend?.future ?? Future<void>.value()).then<Object?>(
          (_) => {
            'run_id': 'r2',
            'message': {
              'id': 3,
              'role': 'user',
              'content': '再问一句',
              'run_id': 'r2',
              'created_at': _at,
            },
          },
        ),
      '/api/app/v1/assistant/confirmations/9' => () {
        approved = true;
        return (_assistantDetail('approved')['confirmations'] as List).first;
      }(),
      _ => null,
    };
  }

  Future<MsgChannel> dial(Uri url, Duration timeout) async {
    final (dev, host) = Pipe.pair();
    final fake = FakeHost(host, hostKey, _hostId, mode: mode, api: api);
    hosts.add(fake);
    unawaited(fake.run());
    return dev;
  }
}

/// 读主机记录要等 [gate]（模拟冷启动时安全存储读得慢）。
class _SlowHostStore extends MemoryHostStore {
  _SlowHostStore(super.host, this.gate);
  final Future<void> gate;

  @override
  Future<HostRecord?> load() async {
    await gate;
    return super.load();
  }
}

Future<_World> _pumpApp(
  WidgetTester tester, {
  List<String> scopes = const ['app:read', 'app:write'],
  bool paired = true,
  String mode = 'device',
  bool mobileFeatures = false,
  bool brushStats = false,
  bool downloaderSeeding = false,
  HostStore? store,
  Future<void>? hostLoaded,
  AppearanceStore? appearance,
  UrlOpener? openUrl,
}) async {
  tester.platformDispatcher.localesTestValue = const [Locale('zh')];
  addTearDown(tester.platformDispatcher.clearLocalesTestValue);
  tester.view.physicalSize = const Size(1170, 2532);
  tester.view.devicePixelRatio = 3;
  addTearDown(tester.view.reset);
  final w = _World(
    [...scopes],
    mode: mode,
    mobileFeatures: mobileFeatures,
    brushStats: brushStats,
    downloaderSeeding: downloaderSeeding,
  );
  late HostRecord rec;
  await tester.runAsync(() async {
    w.hostKey = await DhKey.generate();
    final device = await DhKey.generate();
    rec = HostRecord(
      hostId: _hostId,
      hostKey: w.hostKey.publicKey,
      devicePrivate: device.privateKey,
      relays: const ['wss://relay.example.com'],
      deviceId: 1,
      deviceName: '测试手机',
      scopes: scopes,
      pairedAt: DateTime(2026, 10, 9),
    );
  });
  await tester.pumpWidget(
    ProviderScope(
      retry: (_, _) => null,
      overrides: [
        hostStoreProvider.overrideWithValue(
          store ??
              (hostLoaded != null
                  ? _SlowHostStore(paired ? rec : null, hostLoaded)
                  : MemoryHostStore(paired ? rec : null)),
        ),
        channelFactoryProvider.overrideWithValue(w.dial),
        refreshIntervalProvider.overrideWithValue(const Duration(days: 1)),
        appearanceStoreProvider.overrideWithValue(
          appearance ?? MemoryAppearanceStore(),
        ),
        if (openUrl != null) urlOpenerProvider.overrideWithValue(openUrl),
      ],
      child: const PtToolsApp(),
    ),
  );
  return w;
}

/// 等真的异步（握手里的加密是 Future）走完，再画几帧。
Future<void> _settle(WidgetTester tester, [int rounds = 8]) async {
  for (var i = 0; i < rounds; i++) {
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 30)),
    );
    await tester.pump(const Duration(milliseconds: 50));
  }
}

void main() {
  testWidgets('站点周期选择通过加密通道请求正确周期，海报有入口', (tester) async {
    final w = await _pumpApp(tester, mobileFeatures: true);
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    await tester.tap(find.text('站点'));
    await _settle(tester);
    expect(w.requests, contains('GET /api/app/v1/stats/summary?range=today'));
    await tester.tap(find.text('本周'));
    await _settle(tester);
    expect(w.requests, contains('GET /api/app/v1/stats/summary?range=7d'));
    expect(find.byTooltip('分享统计海报'), findsOneWidget);
    expect(find.textContaining('1.0 GiB'), findsOneWidget);
  });
  testWidgets('更多页底部的 Star 提示：点开仓库，打不开浏览器时复制地址', (tester) async {
    final opened = <Uri>[];
    var canOpen = true;
    await _pumpApp(
      tester,
      openUrl: (uri) async {
        opened.add(uri);
        return canOpen;
      },
    );
    String? copied;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          copied = (call.arguments as Map)['text'] as String?;
        }
        return null;
      },
    );
    addTearDown(
      () => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      ),
    );
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    final star = find.text('如果 pt-tools 对你有帮助，\n还请动动小手到 GitHub 点个 Star');
    await tester.ensureVisible(star);
    await tester.tap(star);
    await _settle(tester, 2);
    expect(opened, [Uri.parse('https://github.com/sunerpy/pt-tools')]);
    expect(copied, isNull);

    canOpen = false;
    await tester.tap(star);
    await _settle(tester, 2);
    expect(opened, hasLength(2));
    expect(copied, 'https://github.com/sunerpy/pt-tools');
    expect(find.text('打不开浏览器，仓库地址已复制'), findsOneWidget);
  });
  testWidgets('刷流显示运行错误与步骤，取消删除不会写入', (tester) async {
    final w = await _pumpApp(tester, mobileFeatures: true);
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    await tester.tap(find.text('刷流'));
    await _settle(tester);
    await tester.tap(find.text('测试刷流'));
    await _settle(tester);
    expect(find.textContaining('下载器暂不可用'), findsOneWidget);
    await tester.tap(find.textContaining('一轮结束'));
    await tester.pumpAndSettle();
    expect(find.text('磁盘查询失败'), findsOneWidget);
    await tester.tap(find.byType(PopupMenuButton<String>));
    await tester.pumpAndSettle();
    await tester.tap(find.text('删除'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('取消'));
    await tester.pumpAndSettle();
    expect(w.requests.where((r) => r.startsWith('DELETE')), isEmpty);
  });

  /// 打开「更多」里的刷流页
  Future<void> openBrush(WidgetTester tester) async {
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    await tester.tap(find.text('刷流'));
    await _settle(tester);
  }

  String subtitleOf(WidgetTester tester, String title) {
    final tile = tester.widget<ListTile>(
      find.ancestor(of: find.text(title), matching: find.byType(ListTile)),
    );
    return (tile.subtitle! as Text).data!;
  }

  String readout(WidgetTester tester) => tester
      .widget<Text>(find.byKey(const ValueKey('brush-trend-readout')))
      .textSpan!
      .toPlainText();

  // 2026-10-28 用户：刷流任务要能按天、周、月、季度、年看统计，手机端也要同步
  testWidgets('刷流：按周期看收益，换成按周时重新请求，本期数字与任务的本期收益跟着换', (tester) async {
    final w = await _pumpApp(tester, mobileFeatures: true, brushStats: true);
    await openBrush(tester);
    expect(
      w.requests,
      contains('GET /api/app/v1/brush/stats?granularity=day&periods=30'),
    );
    expect(find.text('今日上传'), findsOneWidget);
    expect(find.text('昨日 4.0 GiB'), findsOneWidget);
    expect(subtitleOf(tester, '测试刷流'), contains('今日 ↑5.0 GiB ↓1.0 GiB'));
    expect(readout(tester), contains('（今日）'));

    await tester.tap(find.text('周'));
    await _settle(tester);
    expect(
      w.requests,
      contains('GET /api/app/v1/brush/stats?granularity=week&periods=12'),
    );
    expect(find.text('本周上传'), findsOneWidget);
    expect(find.text('30.0 GiB'), findsOneWidget);
    expect(find.text('上周 20.0 GiB'), findsOneWidget);
    expect(find.text('最近 12 周收益'), findsOneWidget);
    expect(subtitleOf(tester, '测试刷流'), contains('本周 ↑7.0 GiB ↓2.0 GiB'));
    expect(readout(tester), contains('11月2日–11月8日（本周）'));

    // 点第一根柱子：读数换成那一周
    final plot = tester.getTopLeft(
      find.byKey(const ValueKey('brush-trend-plot')),
    );
    await tester.tapAt(plot + const Offset(3, 40));
    await tester.pump();
    expect(readout(tester), contains('8月17日–8月23日'));
  });

  testWidgets('刷流：统计读取失败时说明并能重试，不拿今天的冒充本周', (tester) async {
    final w = await _pumpApp(tester, mobileFeatures: true, brushStats: true);
    w.failStats.add('week');
    await openBrush(tester);
    await tester.tap(find.text('周'));
    await _settle(tester);
    expect(find.text('收益统计读取失败'), findsOneWidget);
    expect(subtitleOf(tester, '测试刷流'), contains('本周 ↑— ↓—'));

    w.failStats.clear();
    await tester.tap(find.text('重试'));
    await _settle(tester);
    expect(find.text('收益统计读取失败'), findsNothing);
    expect(subtitleOf(tester, '测试刷流'), contains('本周 ↑7.0 GiB ↓2.0 GiB'));
  });

  testWidgets('刷流：主机没有分段统计时不显示周期，任务照旧写今天', (tester) async {
    final w = await _pumpApp(tester, mobileFeatures: true);
    await openBrush(tester);
    expect(find.byKey(const ValueKey('brush-period')), findsNothing);
    expect(subtitleOf(tester, '测试刷流'), contains('今天 ↑0 B ↓0 B'));
    expect(w.requests.where((r) => r.contains('/brush/stats')), isEmpty);
  });

  testWidgets('外观：点一套主题马上换上并存下来，打开跟随系统又回到明亮与暗黑', (tester) async {
    final store = MemoryAppearanceStore();
    await _pumpApp(tester, appearance: store);
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    expect(find.text('跟随系统'), findsOneWidget, reason: '外观那一行写着现在的选择');
    await tester.tap(find.byKey(const Key('more-appearance')));
    await _settle(tester);
    expect(find.text('暖纸'), findsOneWidget);
    expect(find.text('石墨'), findsOneWidget);
    await tester.tap(find.byKey(const Key('theme-warm')));
    await _settle(tester);
    MaterialApp app() => tester.widget<MaterialApp>(find.byType(MaterialApp));
    expect(app().themeMode, ThemeMode.light);
    expect(app().theme!.scaffoldBackgroundColor, PtPalette.warm.canvas);
    expect(store.choice, ThemeChoice.warm);
    final follow = find.byKey(const Key('appearance-follow-system'));
    expect(tester.widget<SwitchListTile>(follow).value, isFalse);
    await tester.tap(follow);
    await _settle(tester);
    expect(store.choice, ThemeChoice.system);
    expect(app().themeMode, ThemeMode.system);
    expect(app().theme!.scaffoldBackgroundColor, PtPalette.light.canvas);
    expect(app().darkTheme!.scaffoldBackgroundColor, PtPalette.dark.canvas);
  });

  testWidgets('概览：显示主机的合计数字与站内信未读', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    expect(find.text('上传'), findsOneWidget);
    expect(find.text('下载（计费）'), findsOneWidget, reason: '账号的下载量写明是计费口径');
    expect(find.textContaining('每日快照只存这个口径'), findsOneWidget);
    expect(find.text('10.5 TiB'), findsOneWidget);
    expect(find.text('5.25'), findsOneWidget);
    expect(find.text('4,567.8'), findsOneWidget);
    expect(find.text('2'), findsWidgets);
    expect(find.text('pt-tools v1.0.0-test'), findsOneWidget);
    expect(w.requests, contains('GET /api/app/v1/overview'));
  });

  testWidgets('概览：每台下载器正在做种的个数与体积，没在做种的写在下面，连不上的写原因', (tester) async {
    final w = await _pumpApp(tester, downloaderSeeding: true);
    await _settle(tester);
    await tester.scrollUntilVisible(find.text('下载器做种'), 200);
    await tester.scrollUntilVisible(find.textContaining('各算一次'), 200);
    expect(w.requests, contains('GET /api/app/v1/downloaders/seeding'));
    // 卡头的合计与 qb-nas 那一行一样
    expect(find.text('3 个 · 94.5 GiB'), findsNWidgets(2));
    expect(find.text('另有 1 个下载完了没在做种（12.0 GiB）'), findsOneWidget);
    expect(find.text('连不上'), findsOneWidget);
    final reason = tester.widget<Text>(find.textContaining('暂不可用'));
    expect(reason.maxLines, 2, reason: '原因很长，只留两行');
  });

  testWidgets('概览：主机的版本没有下载器做种时不显示，也不请求', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    await tester.scrollUntilVisible(find.textContaining('更新于'), 200);
    expect(find.text('下载器做种'), findsNothing);
    expect(
      w.requests.where((r) => r.contains('/downloaders/seeding')),
      isEmpty,
    );
  });

  testWidgets('概览：下载器做种读不到时写一句，点重试再读', (tester) async {
    final w = await _pumpApp(tester, downloaderSeeding: true);
    w.failSeeding = 1;
    await _settle(tester);
    await tester.scrollUntilVisible(find.text('没取到下载器的做种情况'), 200);
    await tester.tap(find.widgetWithText(TextButton, '重试'));
    await _settle(tester);
    await tester.scrollUntilVisible(find.text('下载器做种'), 200);
    expect(find.text('3 个 · 94.5 GiB'), findsWidgets);
    expect(
      w.requests.where((r) => r == 'GET /api/app/v1/downloaders/seeding'),
      hasLength(2),
    );
  });

  for (final (width, scale) in const [(320.0, 2.0), (390.0, 1.3)]) {
    testWidgets('概览：$width 宽、字号 $scale 倍时下载器做种不溢出', (tester) async {
      await _pumpApp(tester, downloaderSeeding: true);
      tester.view.physicalSize = Size(width * 3, 2532);
      tester.platformDispatcher.textScaleFactorTestValue = scale;
      addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
      await _settle(tester);
      // 溢出时 Flutter 报 RenderFlex overflowed，测试就失败
      await tester.scrollUntilVisible(find.textContaining('各算一次'), 200);
      expect(find.text('qb-nas'), findsOneWidget);
      expect(find.text('连不上'), findsOneWidget);
    });
  }

  testWidgets('站点：站点上的下载量写明是计费口径', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    await tester.tap(find.text('站点'));
    await _settle(tester);
    expect(w.requests, contains('GET /api/app/v1/sites'));
    expect(find.text('↓2.0 TiB（计费）'), findsOneWidget);
  });

  testWidgets('种子：完全控制的设备有操作菜单，删除要确认，取消以后不发请求', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    await tester.tap(find.text('种子'));
    await _settle(tester);
    expect(find.text('Some.Movie.2026.1080p'), findsOneWidget);
    expect(find.text('下载中'), findsWidgets);
    await tester.tap(find.byTooltip('操作'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('删除').last);
    await tester.pumpAndSettle();
    expect(find.text('删除这个种子？'), findsOneWidget);
    expect(find.text('连同已下载的数据一起删除'), findsOneWidget);
    await tester.tap(find.text('取消'));
    await _settle(tester, 3);
    expect(w.requests.where((r) => r.contains('/torrents/actions')), isEmpty);
  });

  testWidgets('种子：只读设备没有操作菜单', (tester) async {
    await _pumpApp(tester, scopes: const ['app:read']);
    await _settle(tester);
    await tester.tap(find.text('种子'));
    await _settle(tester);
    expect(find.text('Some.Movie.2026.1080p'), findsOneWidget);
    expect(find.byTooltip('操作'), findsNothing);
  });

  testWidgets('撤销：连接条提示重新配对，点了回到配对页', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    await tester.runAsync(() => w.hosts.last.goAway('revoked'));
    await _settle(tester);
    expect(find.text('这台设备被撤销了，请重新配对'), findsOneWidget);
    await tester.tap(find.text('重新配对'));
    await _settle(tester);
    expect(find.text('配对 pt-tools'), findsOneWidget);
  });

  testWidgets('撤销以后种子页刷新：说明要重新配对，不显示原始异常、不给重试', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    await tester.tap(find.text('种子'));
    await _settle(tester);
    expect(find.text('Some.Movie.2026.1080p'), findsOneWidget);
    await tester.runAsync(() => w.hosts.last.goAway('revoked'));
    await _settle(tester);
    // 测试里定时刷新是一天一次，这里直接让种子列表重读
    ProviderScope.containerOf(tester.element(find.byType(PtToolsApp)))
        .invalidate(torrentsProvider);
    await _settle(tester);
    expect(find.textContaining('Exception occurred'), findsNothing);
    expect(find.textContaining('出错了'), findsNothing);
    expect(find.text('这台设备被撤销了，请重新配对'), findsNWidgets(2), reason: '连接条与页面里各一处');
    expect(find.text('重试'), findsNothing);
  });

  testWidgets('其他错误照旧：带「出错了」与重试', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    await tester.tap(find.text('种子'));
    await _settle(tester);
    w.failTorrents = true;
    ProviderScope.containerOf(tester.element(find.byType(PtToolsApp)))
        .invalidate(torrentsProvider);
    await _settle(tester);
    expect(find.text('出错了：下载器 qb 连不上'), findsOneWidget);
    expect(find.text('重试'), findsOneWidget);
  });

  testWidgets('配对成功但存不进安全存储：提示到网页上撤销这台设备', (tester) async {
    final w = await _pumpApp(
      tester,
      paired: false,
      mode: 'pairing',
      store: _BrokenStore(),
    );
    await _settle(tester);
    final link =
        'pttools://pair?v=1&h=$_hostId&k=${encodeKey(w.hostKey.publicKey)}'
        '&s=${encodeKey(Uint8List(32))}&r=wss://relay.example.com';
    await tester.enterText(find.byKey(const Key('pair-link')), link);
    await tester.enterText(find.byKey(const Key('pair-name')), '测试手机');
    await tester.tap(find.byKey(const Key('pair-submit')));
    await _settle(tester, 12);
    expect(find.byKey(const Key('pair-error')), findsOneWidget);
    expect(find.textContaining('撤销「测试手机」'), findsOneWidget);
    expect(find.textContaining('keystore 坏了'), findsOneWidget);
    expect(find.text('配对 pt-tools'), findsOneWidget, reason: '还在配对页');
  });

  testWidgets('AI 助手：更多里有入口，打开最近的会话，确认卡片点「确认执行」发确认', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    expect(find.byKey(const Key('more-assistant')), findsOneWidget);
    await tester.tap(find.byKey(const Key('more-assistant')));
    await _settle(tester);
    expect(find.text('暂停 abc'), findsOneWidget);
    expect(find.text('好的，暂停这个种子。'), findsOneWidget, reason: '回答按纯文本显示');
    expect(find.text('暂停种子：下载器 1，任务 abc'), findsOneWidget);
    expect(find.text('等你确认'), findsOneWidget);
    expect(find.text('等你确认上面的操作…'), findsOneWidget);
    await tester.tap(find.text('确认执行'));
    await _settle(tester);
    expect(w.requests, contains('POST /api/app/v1/assistant/confirmations/9'));
    expect(find.text('已确认'), findsOneWidget);
    expect(find.text('确认执行'), findsNothing);
    expect(
      w.requests.where((r) => r.contains('/assistant/conversations/7/events')),
      isNotEmpty,
      reason: '长轮询事件',
    );
  });

  testWidgets('AI 助手：只读设备看得到确认卡片，但没有确认按钮', (tester) async {
    await _pumpApp(tester, scopes: const ['app:read']);
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    await tester.tap(find.byKey(const Key('more-assistant')));
    await _settle(tester);
    expect(find.text('暂停种子：下载器 1，任务 abc'), findsOneWidget);
    expect(find.text('确认执行'), findsNothing);
    expect(find.text('这台设备只能查询：改动类操作要在网页上确认。'), findsOneWidget);
  });

  testWidgets('AI 助手：要求重读时会话一时读不到，过一会儿再读并接着长轮询', (tester) async {
    final w = await _pumpApp(tester);
    w.eventsReset = 1;
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    await tester.tap(find.byKey(const Key('more-assistant')));
    await _settle(tester);
    expect(find.text('主机在重启'), findsOneWidget, reason: '重读失败时显示原因');
    await tester.pump(const Duration(seconds: 3));
    await _settle(tester);
    expect(find.text('主机在重启'), findsNothing, reason: '再读成功以后错误消失');
    expect(find.text('暂停 abc'), findsOneWidget);
    const detail = 'GET /api/app/v1/assistant/conversations/7';
    expect(w.requests.where((r) => r == detail), hasLength(3));
    final polls = w.requests
        .where((r) => r.contains('/assistant/conversations/7/events'))
        .toList();
    expect(polls.length, greaterThanOrEqualTo(2), reason: '长轮询接着读');
    expect(polls.last, contains('after=100'), reason: '从重读到的 last_seq 接着读');
  });

  testWidgets('AI 助手：离开页面时取消进行中的长轮询（主机收到 CANCEL）', (tester) async {
    final w = await _pumpApp(tester);
    w.holdEvents = Completer<void>();
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    await tester.tap(find.byKey(const Key('more-assistant')));
    await _settle(tester);
    final host = w.hosts.last;
    final poll = host.requests.lastWhere(
      (r) =>
          (r['path'] as String).contains('/assistant/conversations/7/events'),
    );
    expect(host.cancelled, isNot(contains(poll['id'])), reason: '还在等');
    GoRouter.of(tester.element(find.byType(AssistantPage))).go('/more');
    await _settle(tester);
    expect(find.byType(AssistantPage), findsNothing);
    expect(host.cancelled, contains(poll['id']), reason: '关页面时取消长轮询');
    w.holdEvents!.complete();
  });

  testWidgets('AI 助手：发送中不能开新对话，回应回来以后落在原来的会话上', (tester) async {
    final w = await _pumpApp(tester);
    w.approved = true;
    w.holdSend = Completer<void>();
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    await tester.tap(find.byKey(const Key('more-assistant')));
    await _settle(tester);
    IconButton newChat() =>
        tester.widget<IconButton>(find.byKey(const Key('assistant-new')));
    expect(newChat().onPressed, isNotNull);
    await tester.enterText(find.byKey(const Key('assistant-input')), '再问一句');
    await tester.tap(find.byKey(const Key('assistant-send')));
    await _settle(tester);
    expect(
      w.requests,
      contains('POST /api/app/v1/assistant/conversations/7/messages'),
    );
    expect(newChat().onPressed, isNull, reason: '发送请求还没回来');
    w.holdSend!.complete();
    await _settle(tester);
    expect(find.text('再问一句'), findsOneWidget);
    expect(find.text('暂停 abc'), findsOneWidget, reason: '还是原来的会话');
  });

  testWidgets('AI 助手：冷启动直接打开这一页时等主机记录读出来再读会话', (tester) async {
    tester.platformDispatcher.defaultRouteNameTestValue = '/more/assistant';
    addTearDown(tester.platformDispatcher.clearDefaultRouteNameTestValue);
    final loaded = Completer<void>();
    await _pumpApp(tester, hostLoaded: loaded.future);
    await _settle(tester);
    expect(find.byType(AssistantPage), findsOneWidget);
    expect(find.textContaining('Null check'), findsNothing);
    loaded.complete();
    await _settle(tester);
    expect(find.text('暂停 abc'), findsOneWidget, reason: '主机记录读出来以后打开最近的会话');
  });

  testWidgets('AI 助手：主机没启用时更多里没有入口', (tester) async {
    final w = await _pumpApp(tester);
    w.assistantEnabled = false;
    await _settle(tester);
    await tester.tap(find.text('更多'));
    await _settle(tester);
    expect(find.byKey(const Key('more-assistant')), findsNothing);
  });

  /// 媒体 → 探索（默认看电影的热门：罐头不分电影剧集）
  Future<void> openExplore(WidgetTester tester) async {
    await tester.tap(find.text('媒体'));
    await _settle(tester);
    await tester.tap(find.text('探索'));
    await _settle(tester);
  }

  for (final (width, scale) in const [
    (320.0, 1.0),
    (390.0, 1.0),
    (390.0, 1.3),
    (390.0, 2.0),
    (600.0, 1.0),
    (900.0, 1.5),
  ]) {
    testWidgets('探索：$width 宽、字号 $scale 倍时卡片不溢出', (tester) async {
      await _pumpApp(tester);
      tester.view.physicalSize = Size(width * 3, 2532);
      tester.platformDispatcher.textScaleFactorTestValue = scale;
      addTearDown(tester.platformDispatcher.clearTextScaleFactorTestValue);
      await _settle(tester);
      await openExplore(tester);
      // 溢出时 Flutter 报 RenderFlex overflowed，测试就失败
      expect(find.text('短名'), findsOneWidget);
      expect(find.text('已订阅'), findsOneWidget);
      expect(find.widgetWithText(FilledButton, '订阅'), findsNWidgets(2));
      // 同一行的卡片一样高：「已订阅」与同一行那张的「订阅」都在卡片底部，底边对齐
      final pill = find.ancestor(
        of: find.text('已订阅'),
        matching: find.byType(StatusPill),
      );
      expect(
        tester.getBottomLeft(pill).dy,
        moreOrLessEquals(
          tester
              .getBottomLeft(find.widgetWithText(FilledButton, '订阅').first)
              .dy,
          epsilon: 0.5,
        ),
      );
    });
  }

  testWidgets('探索：网页上删掉订阅以后，下拉刷新或离开再回来就更新', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    await openExplore(tester);
    expect(find.text('已订阅'), findsOneWidget);

    w.exploreSubscribed = false;
    final before = w.requests.where((r) => r.contains('/explore')).length;
    await tester.fling(find.text('短名'), const Offset(0, 400), 1200);
    await tester.pump();
    await _settle(tester, 12);
    expect(
      w.requests.where((r) => r.contains('/explore')).length,
      greaterThan(before),
      reason: '下拉刷新重新取了探索',
    );
    expect(find.text('已订阅'), findsNothing);
    expect(find.widgetWithText(FilledButton, '订阅'), findsNWidgets(3));

    // 去了别的页再回来：重新取一次
    w.exploreSubscribed = true;
    await tester.tap(find.text('概览'));
    await _settle(tester);
    await tester.tap(find.text('媒体'));
    await _settle(tester);
    expect(find.text('已订阅'), findsOneWidget);
  });

  for (final (on, group, res, line) in const [
    (true, 'HHWEB', '1080p', '版本锁 HHWEB 1080p'),
    (true, '', '2160p', '版本锁 2160p'),
    (true, '', '', '版本锁（按第一个下载的版本定）'),
    (false, 'HHWEB', '1080p', null),
  ]) {
    testWidgets('订阅详情：版本锁 $on「$group $res」', (tester) async {
      final w = await _pumpApp(tester);
      w
        ..lockOn = on
        ..lockGroup = group
        ..lockRes = res;
      await _settle(tester);
      ProviderScope.containerOf(tester.element(find.byType(PtToolsApp)))
          .read(routerProvider)
          .go('/media/subscriptions/5');
      await _settle(tester);
      expect(find.text('现在就出发'), findsWidgets);
      if (line == null) {
        expect(find.textContaining('版本锁'), findsNothing);
      } else {
        expect(find.text(line), findsOneWidget);
      }
    });
  }

  testWidgets('配对页：链接不对时说明原因', (tester) async {
    await _pumpApp(tester, paired: false);
    await _settle(tester);
    expect(find.text('配对 pt-tools'), findsOneWidget);
    await tester.enterText(
      find.byKey(const Key('pair-link')),
      'https://example.com/not-a-link',
    );
    await tester.tap(find.byKey(const Key('pair-submit')));
    await tester.pump();
    expect(find.byKey(const Key('pair-error')), findsOneWidget);
    expect(find.textContaining('链接不对'), findsOneWidget);
  });
}
