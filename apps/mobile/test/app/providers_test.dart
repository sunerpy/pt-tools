// 定时刷新：App 在后台（连接停了）时不触发。
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/app/providers.dart';
import 'package:pt_tools_app/src/remote/connection.dart';
import 'package:pt_tools_app/src/remote/host_record.dart';
import 'package:pt_tools_app/src/remote/noise.dart';

void main() {
  test('定时刷新：连接停了（App 在后台）不触发，回到前台接着触发', () async {
    final hostKey = await DhKey.generate();
    final device = await DhKey.generate();
    final conn = HostConnection(
      HostRecord(
        hostId: 'ph5tr32bj27nk7zphjn2qf4z5v',
        hostKey: hostKey.publicKey,
        devicePrivate: device.privateKey,
        relays: const ['wss://relay.example.com'],
        deviceId: 1,
        deviceName: '测试手机',
        scopes: const ['app:read'],
        pairedAt: DateTime(2026, 10, 9),
      ),
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
