// 连接管理：先直连再 relay、GOAWAY 的处理、撤销以后不再重连。
import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/remote/connection.dart';
import 'package:pt_tools_app/src/remote/host_record.dart';
import 'package:pt_tools_app/src/remote/noise.dart';
import 'package:pt_tools_app/src/remote/session.dart';
import 'package:pt_tools_app/src/remote/transport.dart';

import 'fake_host.dart';

const _hostId = 'ph5tr32bj27nk7zphjn2qf4z5v';

class _World {
  _World(this.hostKey);
  final DhKey hostKey;
  final hosts = <FakeHost>[];
  bool directUp = false;
  String? handshakeError;
  final dialed = <Uri>[];

  Future<MsgChannel> dial(Uri url, Duration timeout) async {
    dialed.add(url);
    if (url.path.endsWith('/remote/v1/stream') && !directUp) {
      throw StateError('connection refused');
    }
    final (dev, host) = Pipe.pair();
    final fake = FakeHost(host, hostKey, _hostId, error: handshakeError);
    hosts.add(fake);
    unawaited(fake.run());
    return dev;
  }
}

Future<(HostConnection, _World)> _setup() async {
  final hostKey = await DhKey.generate();
  final device = await DhKey.generate();
  final w = _World(hostKey);
  final rec = HostRecord(
    hostId: _hostId,
    hostKey: hostKey.publicKey,
    devicePrivate: device.privateKey,
    relays: const ['wss://relay-a.example.com', 'wss://relay-b.example.com'],
    direct: 'http://192.168.1.10:8080',
    deviceId: 1,
    deviceName: '测试手机',
    scopes: const ['app:read', 'app:write'],
    pairedAt: DateTime(2026, 10, 9),
  );
  final c = HostConnection(
    rec,
    channel: w.dial,
    backoff: (_) => const Duration(milliseconds: 10),
  );
  return (c, w);
}

Future<ConnectionStatus> _until(HostConnection c, LinkState s) async {
  if (c.status.state == s) return c.status;
  return c.statuses
      .firstWhere((x) => x.state == s)
      .timeout(const Duration(seconds: 5));
}

void main() {
  test('地址顺序：直连在前，relay 按链接里的顺序', () async {
    final (c, _) = await _setup();
    final eps = endpointsOf(c.host);
    expect(eps.map((e) => e.url.toString()), [
      'ws://192.168.1.10:8080/remote/v1/stream',
      'wss://relay-a.example.com/v1/client/$_hostId',
      'wss://relay-b.example.com/v1/client/$_hostId',
    ]);
    expect(eps.first.via, Via.direct);
  });

  test('直连连不上时用 relay；连上以后状态是 online（经 relay）', () async {
    final (c, w) = await _setup();
    c.start();
    final st = await _until(c, LinkState.online);
    expect(st.via, Via.relay);
    expect(st.endpoint, 'wss://relay-a.example.com');
    expect(w.dialed.first.path, '/remote/v1/stream');
    expect(c.session!.hostVersion, 'v-test');
    c.dispose();
  });

  test('直连连得上就用直连', () async {
    final (c, w) = await _setup();
    w.directUp = true;
    c.start();
    final st = await _until(c, LinkState.online);
    expect(st.via, Via.direct);
    c.dispose();
  });

  test('权限改了（scope_changed）：马上重连', () async {
    final (c, w) = await _setup();
    c.start();
    await _until(c, LinkState.online);
    await w.hosts.last.ready.future;
    final first = c.session!;
    await w.hosts.last.goAway('scope_changed');
    await first.done;
    await _until(c, LinkState.online);
    expect(c.session, isNot(same(first)));
    expect(w.hosts.length, 2);
    c.dispose();
  });

  test('撤销（revoked）：停下来，不再重连；ensure 直接失败', () async {
    final (c, w) = await _setup();
    c.start();
    await _until(c, LinkState.online);
    await w.hosts.last.ready.future;
    await w.hosts.last.goAway('revoked');
    final st = await _until(c, LinkState.revoked);
    expect(st.goAway, 'revoked');
    await Future<void>.delayed(const Duration(milliseconds: 100));
    expect(w.hosts.length, 1, reason: '没有再连');
    await expectLater(c.ensure(), throwsA(isA<HandshakeException>()));
    c.dispose();
  });

  test('握手时主机说 not_paired：状态是 revoked', () async {
    final (c, w) = await _setup();
    w.handshakeError = 'not_paired';
    c.start();
    final st = await _until(c, LinkState.revoked);
    expect(st.message, contains('撤销'));
    c.dispose();
  });

  test('断线以后按退避重连；stop 以后不再连', () async {
    final (c, w) = await _setup();
    c.start();
    await _until(c, LinkState.online);
    await w.hosts.last.ready.future;
    final first = c.session!;
    await w.hosts.last.ch.close();
    await first.done;
    await _until(c, LinkState.online);
    expect(w.hosts.length, 2);
    c.stop();
    expect(c.status.state, LinkState.offline);
    final n = w.dialed.length;
    await Future<void>.delayed(const Duration(milliseconds: 100));
    expect(w.dialed.length, n);
    c.dispose();
  });

  test('到了后台（stop）：请求直接失败、不去连；回到前台（start）重新连上', () async {
    final (c, w) = await _setup();
    c.start();
    await _until(c, LinkState.online);
    c.stop();
    expect(c.paused, isTrue);
    final n = w.dialed.length;
    await expectLater(
      c.client.get(Uri.parse('https://pt-tools/api/app/v1/meta')),
      throwsA(isA<SessionClosedException>()),
    );
    expect(w.dialed.length, n, reason: '后台不连接');
    c.start();
    expect(c.paused, isFalse);
    await _until(c, LinkState.online);
    final resp = await c.client.get(
      Uri.parse('https://pt-tools/api/app/v1/meta'),
    );
    expect(resp.statusCode, 200);
    c.dispose();
  });

  test('握手期间到了后台：连上的会话关掉，不算在线', () async {
    final (c, w) = await _setup();
    w.directUp = true;
    final dialing = Completer<void>();
    final gate = Completer<void>();
    final slow = HostConnection(
      c.host,
      channel: (url, timeout) async {
        if (!dialing.isCompleted) dialing.complete();
        await gate.future;
        return w.dial(url, timeout);
      },
      backoff: (_) => const Duration(milliseconds: 10),
    );
    slow.start();
    await dialing.future;
    slow.stop();
    gate.complete();
    await Future<void>.delayed(const Duration(milliseconds: 100));
    expect(slow.status.state, LinkState.offline);
    expect(slow.session, isNull);
    expect(w.hosts.length, 1, reason: '直连握手完成了');
    expect(w.hosts.single.ch.isClosed, isTrue, reason: '连上的会话关掉了');
    slow.dispose();
    c.dispose();
  });

  test('经隧道的 http.Client：请求前自动连上', () async {
    final (c, _) = await _setup();
    final resp = await c.client.get(
      Uri.parse('https://pt-tools/api/app/v1/meta'),
    );
    expect(resp.statusCode, 200);
    expect(resp.body, contains('/api/app/v1/meta'));
    c.dispose();
  });
}
