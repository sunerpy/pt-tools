// 前后台：App 在后台时不连接（包括后台才读完主机记录时新建的连接），定时刷新也不触发。
import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/app/providers.dart';
import 'package:pt_tools_app/src/app/storage.dart';
import 'package:pt_tools_app/src/remote/connection.dart';
import 'package:pt_tools_app/src/remote/host_record.dart';
import 'package:pt_tools_app/src/remote/noise.dart';

/// 读主机记录要等 [loaded]（模拟安全存储读得慢）。
class _SlowStore implements HostStore {
  _SlowStore(this.host);
  final HostRecord host;
  final loaded = Completer<void>();

  @override
  Future<HostRecord?> load() async {
    await loaded.future;
    return host;
  }

  @override
  Future<void> save(HostRecord h) async {}

  @override
  Future<void> clear() async {}
}

Future<HostRecord> _record() async {
  final hostKey = await DhKey.generate();
  final device = await DhKey.generate();
  return HostRecord(
    hostId: 'ph5tr32bj27nk7zphjn2qf4z5v',
    hostKey: hostKey.publicKey,
    devicePrivate: device.privateKey,
    relays: const ['wss://relay.example.com'],
    deviceId: 1,
    deviceName: '测试手机',
    scopes: const ['app:read'],
    pairedAt: DateTime(2026, 10, 9),
  );
}

void main() {
  test('后台才读完主机记录：新建的连接不连，回到前台才连', () async {
    final store = _SlowStore(await _record());
    var dials = 0;
    final container = ProviderContainer(
      overrides: [
        hostStoreProvider.overrideWithValue(store),
        channelFactoryProvider.overrideWithValue((_, _) async {
          dials++;
          throw StateError('测试里不连接');
        }),
      ],
    );
    addTearDown(container.dispose);
    expect(container.read(connectionProvider), isNull, reason: '主机记录还没读完');
    container.read(foregroundProvider.notifier).set(false);
    store.loaded.complete();
    await container.read(hostProvider.future);
    final c = container.read(connectionProvider)!;
    expect(c.paused, isTrue);
    await Future<void>.delayed(const Duration(milliseconds: 50));
    expect(dials, 0, reason: '后台不连接');

    container.read(foregroundProvider.notifier).set(true);
    await Future<void>.delayed(const Duration(milliseconds: 50));
    expect(c.paused, isFalse);
    expect(dials, greaterThan(0));
  });

  test('定时刷新：连接停了（App 在后台）不触发，回到前台接着触发', () async {
    final conn = HostConnection(
      await _record(),
      channel: (_, _) async => throw StateError('测试里不连接'),
      backoff: (_) => const Duration(hours: 1),
    );
    addTearDown(conn.dispose);
    final container = ProviderContainer(
      overrides: [
        connectionProvider.overrideWithValue(conn),
        refreshIntervalProvider.overrideWithValue(
          const Duration(milliseconds: 10),
        ),
      ],
    );
    addTearDown(container.dispose);
    final ticks = <int>[];
    container.listen(tickerProvider, (_, next) {
      if (next.hasValue) ticks.add(next.requireValue);
    });

    await Future<void>.delayed(const Duration(milliseconds: 80));
    expect(ticks, isNotEmpty);

    conn.stop();
    final n = ticks.length;
    await Future<void>.delayed(const Duration(milliseconds: 80));
    expect(ticks.length, n, reason: '后台不刷新');

    conn.start();
    await Future<void>.delayed(const Duration(milliseconds: 80));
    expect(ticks.length, greaterThan(n));
  });
}
