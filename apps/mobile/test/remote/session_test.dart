// 会话的行为测试：设备端（RemoteSession）对着一个 Dart 写的假主机（Noise 响应方 + 隧道帧），走内存里的消息通道。
// 跨语言的互通（对 Go 的主机）在 test/interop/。
import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:pt_tools_app/src/remote/noise.dart';
import 'package:pt_tools_app/src/remote/session.dart';
import 'package:pt_tools_app/src/remote/tunnel_client.dart';

import 'fake_host.dart';

const _hostId = 'ph5tr32bj27nk7zphjn2qf4z5v';

Future<(RemoteSession, FakeHost)> _connect({
  String mode = 'device',
  Duration ping = pingInterval,
  Duration idle = idleTimeout,
}) async {
  final (dev, host) = Pipe.pair();
  final hostKey = await DhKey.generate();
  final fake = FakeHost(host, hostKey, _hostId, mode: mode);
  unawaited(fake.run());
  final s = await RemoteSession.connect(
    dev,
    hostId: _hostId,
    hostKey: hostKey.publicKey,
    device: await DhKey.generate(),
    client: 'test',
    ping: ping,
    idle: idle,
  );
  return (s, fake);
}

void main() {
  test('握手以后经隧道发请求：路径、方法、只转发白名单里的头', () async {
    final (s, fake) = await _connect();
    expect(s.mode, SessionMode.device);
    expect(s.hostVersion, 'v-test');
    final client = TunnelClient(s);
    final resp = await client.get(
      Uri.parse('https://tunnel/api/app/v1/meta?x=1'),
      headers: {
        'Accept': 'application/json',
        'Cookie': 'a=b',
        'Authorization': 'Bearer x',
      },
    );
    expect(resp.statusCode, 200);
    expect(resp.headers['content-type'], 'application/json');
    final body = jsonDecode(resp.body) as Map<String, dynamic>;
    expect(body['path'], '/api/app/v1/meta?x=1');
    expect(body['method'], 'GET');
    expect(body['headers'], {'Accept': 'application/json'});
    s.close();
  });

  test('大的请求体拆成多个 REQ_BODY，大的回应体按序拼回来', () async {
    final (s, _) = await _connect();
    final big = Uint8List.fromList(List.generate(200000, (i) => i % 251));
    final resp = await s.request('POST', '/echo', body: big);
    final got = await resp.body.fold<BytesBuilder>(
      BytesBuilder(),
      (b, c) => b..add(c),
    );
    expect(got.takeBytes(), big);
    s.close();
  });

  test('同时进行的请求超过 16 个时排队，回应完一个再发下一个', () async {
    final (s, fake) = await _connect();
    final hangs = [
      for (var i = 0; i < 16; i++) await s.request('GET', '/hang'),
    ];
    final waiting = s.request('GET', '/x');
    await Future<void>.delayed(const Duration(milliseconds: 50));
    expect(fake.requests.length, 16, reason: '第 17 个还在排队');
    // 取消一个（读者不要了）：主机收到 CANCEL，排队的那个发出去
    final sub = hangs.first.body.listen(null);
    await sub.cancel();
    final r = await waiting;
    expect(r.status, 200);
    expect(fake.cancelled, [fake.requests.first['id']]);
    s.close();
  });

  test('主机中止回应：回应体出错', () async {
    final (s, _) = await _connect();
    final resp = await s.request('GET', '/abort');
    await expectLater(
      resp.body.toList(),
      throwsA(isA<ResponseAbortedException>()),
    );
    s.close();
  });

  test('GOAWAY：会话结束，记下原因，进行中的请求失败', () async {
    final (s, _) = await _connect();
    final pending = await s.request('GET', '/hang');
    // 先挂上处理：会话一结束，回应体就出错
    final bodyDone = expectLater(
      pending.body.toList(),
      throwsA(isA<SessionClosedException>()),
    );
    await expectLater(
      s.request('GET', '/goaway'),
      throwsA(isA<SessionClosedException>()),
    );
    await s.done;
    expect(s.goAway, 'revoked');
    await bodyDone;
    expect(
      () => s.request('GET', '/x'),
      throwsA(isA<SessionClosedException>()),
    );
  });

  test('主机拒绝握手：not_paired', () async {
    final (dev, host) = Pipe.pair();
    final hostKey = await DhKey.generate();
    unawaited(FakeHost(host, hostKey, _hostId, error: 'not_paired').run());
    await expectLater(
      RemoteSession.connect(
        dev,
        hostId: _hostId,
        hostKey: hostKey.publicKey,
        device: await DhKey.generate(),
        client: 'test',
      ),
      throwsA(
        isA<HandshakeException>().having((e) => e.notPaired, 'notPaired', true),
      ),
    );
  });

  test('记错主机公钥：握手失败（主机解不开第一条，断开）', () async {
    final (dev, host) = Pipe.pair();
    final hostKey = await DhKey.generate();
    final wrong = await DhKey.generate();
    unawaited(() async {
      final hs = NoiseIK.responderOf(
        s: hostKey,
        prologue: utf8.encode('pt-tools-remote-v1$_hostId'),
      );
      final it = StreamIterator(host.messages);
      await it.moveNext();
      try {
        await hs.readMessage(it.current);
      } on NoiseException {
        await host.close();
      }
    }());
    await expectLater(
      RemoteSession.connect(
        dev,
        hostId: _hostId,
        hostKey: wrong.publicKey,
        device: await DhKey.generate(),
        client: 'test',
      ),
      throwsA(isA<HandshakeException>()),
    );
  });

  test('配对会话：提交密钥与设备名，拿到设备，随后 GOAWAY paired', () async {
    final (s, _) = await _connect(mode: 'pairing');
    expect(s.mode, SessionMode.pairing);
    final device = await s.pair(Uint8List(32), '测试手机');
    expect(device['name'], '测试手机');
    await s.done;
    expect(s.goAway, 'paired');
  });

  test('保活：按间隔发 PING，主机一直回 PONG 时会话不断', () async {
    final (s, fake) = await _connect(
      ping: const Duration(milliseconds: 20),
      idle: const Duration(milliseconds: 70),
    );
    await Future<void>.delayed(const Duration(milliseconds: 300));
    expect(s.closed, isFalse);
    expect(fake.pings, greaterThanOrEqualTo(5));
    s.close();
  });

  test('半开的连接：主机很久没有任何回应时会话结束，进行中的请求失败', () async {
    final (s, fake) = await _connect(
      ping: const Duration(milliseconds: 20),
      idle: const Duration(milliseconds: 70),
    );
    final ok = await s.request('GET', '/x');
    await ok.body.drain<void>();
    fake.silent = true;
    final failed = expectLater(
      s.request('GET', '/y'),
      throwsA(isA<SessionClosedException>()),
    );
    await s.done.timeout(const Duration(seconds: 2));
    await failed;
    expect(s.error, isA<SessionClosedException>());
    expect((s.error! as SessionClosedException).message, contains('没有回应'));
    expect(s.goAway, isNull);
  });

  test('包了 TunnelClient 的 http 请求在会话关闭以后失败', () async {
    final (s, _) = await _connect();
    s.close();
    await expectLater(
      TunnelClient(s).get(Uri.parse('https://tunnel/api/app/v1/meta')),
      throwsA(isA<SessionClosedException>()),
    );
    expect(http.Client, isNotNull);
  });
}
