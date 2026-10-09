// 界面测试：整个 App 对着内存里的假主机（罐头 App API）跑，中文界面。
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/app/app.dart';
import 'package:pt_tools_app/src/app/providers.dart';
import 'package:pt_tools_app/src/app/storage.dart';
import 'package:pt_tools_app/src/remote/host_record.dart';
import 'package:pt_tools_app/src/remote/noise.dart';
import 'package:pt_tools_app/src/remote/session.dart';

import '../remote/fake_host.dart';

const _hostId = 'ph5tr32bj27nk7zphjn2qf4z5v';

Map<String, Object?> _meta(List<String> scopes) => {
  'name': 'pt-tools',
  'version': 'v1.0.0-test',
  'remote_api_level': 1,
  'features': ['overview'],
  'principal': {'kind': 'remote_device', 'name': '测试手机', 'scopes': scopes},
};

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

class _World {
  _World(this.scopes);
  List<String> scopes;
  final hosts = <FakeHost>[];
  final requests = <String>[];
  late DhKey hostKey;

  Object? api(String method, String path) {
    requests.add('$method $path');
    final p = Uri.parse(path).path;
    return switch (p) {
      '/api/app/v1/meta' => _meta(scopes),
      '/api/app/v1/overview' => _overview,
      '/api/app/v1/downloaders' => {'items': []},
      '/api/app/v1/torrents' => _torrents,
      '/api/app/v1/torrents/actions' => {
        'succeeded': 1,
        'failed': 0,
        'results': [],
      },
      _ => null,
    };
  }

  Future<MsgChannel> dial(Uri url, Duration timeout) async {
    final (dev, host) = Pipe.pair();
    final fake = FakeHost(host, hostKey, _hostId, api: api);
    hosts.add(fake);
    unawaited(fake.run());
    return dev;
  }
}

Future<_World> _pumpApp(
  WidgetTester tester, {
  List<String> scopes = const ['app:read', 'app:write'],
  bool paired = true,
}) async {
  tester.platformDispatcher.localesTestValue = const [Locale('zh')];
  addTearDown(tester.platformDispatcher.clearLocalesTestValue);
  tester.view.physicalSize = const Size(1170, 2532);
  tester.view.devicePixelRatio = 3;
  addTearDown(tester.view.reset);
  final w = _World([...scopes]);
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
          MemoryHostStore(paired ? rec : null),
        ),
        channelFactoryProvider.overrideWithValue(w.dial),
        refreshIntervalProvider.overrideWithValue(const Duration(days: 1)),
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
  testWidgets('概览：显示主机的合计数字与站内信未读', (tester) async {
    final w = await _pumpApp(tester);
    await _settle(tester);
    expect(find.text('上传'), findsOneWidget);
    expect(find.text('10.5 TiB'), findsOneWidget);
    expect(find.text('5.25'), findsOneWidget);
    expect(find.text('4,567.8'), findsOneWidget);
    expect(find.text('2'), findsWidgets);
    expect(find.text('pt-tools v1.0.0-test'), findsOneWidget);
    expect(w.requests, contains('GET /api/app/v1/overview'));
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
