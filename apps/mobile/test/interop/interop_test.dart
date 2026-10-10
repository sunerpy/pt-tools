// 跟 Go 的主机互通：配对、直连与经 relay 的会话、经生成的 App API 客户端调用、改权限以后重连、撤销以后 not_paired。
// 要先起测试主机（internal/remote/testhost），再带上它写的地址文件跑：
//
//   go run ./internal/remote/testhost -info /tmp/testhost.json &
//   PTT_TESTHOST=/tmp/testhost.json flutter test test/interop
//
// 没有 PTT_TESTHOST 时跳过（CI 的 mobile.yml 会起测试主机）。
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:ptt_api/api.dart';
import 'package:pt_tools_app/src/remote/connection.dart';
import 'package:pt_tools_app/src/remote/host_record.dart';
import 'package:pt_tools_app/src/remote/link.dart';
import 'package:pt_tools_app/src/remote/pairing.dart';
import 'package:pt_tools_app/src/remote/session.dart';
import 'package:pt_tools_app/src/remote/transport.dart';

Future<Map<String, dynamic>> _control(
  String control,
  String path, {
  String method = 'POST',
}) async {
  final c = HttpClient();
  try {
    final req = await c.openUrl(method, Uri.parse('$control$path'));
    final resp = await req.close();
    final body = await resp.transform(utf8.decoder).join();
    if (resp.statusCode != 200) {
      throw StateError('控制接口 $path 回 ${resp.statusCode}: $body');
    }
    final v = jsonDecode(body);
    return v is Map<String, dynamic> ? v : {'items': v};
  } finally {
    c.close();
  }
}

Future<ConnectionStatus> _until(HostConnection c, LinkState s) async {
  if (c.status.state == s) return c.status;
  return c.statuses
      .firstWhere((x) => x.state == s)
      .timeout(const Duration(seconds: 15));
}

void main() {
  final infoPath = Platform.environment['PTT_TESTHOST'];
  if (infoPath == null || infoPath.isEmpty) {
    test(
      '跟 Go 的主机互通',
      () {},
      skip: '没有 PTT_TESTHOST（先起 internal/remote/testhost）',
    );
    return;
  }
  final info =
      jsonDecode(File(infoPath).readAsStringSync()) as Map<String, dynamic>;
  final control = info['control'] as String;

  test('配对（直连），直连与经 relay 的会话，App API 客户端，改权限重连，撤销以后 not_paired', () async {
    final ticket = await _control(control, '/pair?scopes=full');
    final link = PairingLink.parse(ticket['link'] as String);
    expect(link.direct, info['direct']);
    expect(link.relays, [info['relay']]);

    final host = await pairWithLink(link, 'Dart 互通');
    expect(host.deviceId, greaterThan(0));
    expect(host.scopes, containsAll(['app:read', 'app:write']));

    // 只用直连
    final direct = HostConnection(
      HostRecord.fromJson({...host.toJson(), 'relays': <String>[]}),
    );
    addTearDown(direct.dispose);
    direct.start();
    expect((await _until(direct, LinkState.online)).via, Via.direct);
    final api = AppApi(
      ApiClient(basePath: 'https://pt-tools/api/app/v1')
        ..client = direct.client,
    );
    final meta = (await api.getMeta())!;
    expect(meta.principal.kind, 'remote_device');
    expect(meta.principal.name, 'Dart 互通');
    expect(meta.principal.scopes, containsAll(['app:read', 'app:write']));

    // 只用 relay：大请求体分片发过去、原样回来
    final relayed = HostConnection(
      HostRecord.fromJson({...host.toJson(), 'direct': null}),
    );
    addTearDown(relayed.dispose);
    relayed.start();
    expect((await _until(relayed, LinkState.online)).via, Via.relay);
    final big = Uint8List.fromList(List.generate(300000, (i) => (i * 7) % 256));
    final echo = await relayed.client.post(
      Uri.parse('https://pt-tools/api/app/v1/echo'),
      body: big,
    );
    expect(echo.statusCode, 200);
    expect(echo.bodyBytes, big);
    final other = jsonDecode(
      (await relayed.client.get(
        Uri.parse('https://pt-tools/api/app/v1/sites?x=1'),
      )).body,
    );
    expect(other['path'], '/api/app/v1/sites?x=1');
    expect(other['via'], 'relay');
    // 隧道外的路径：403
    final denied = await relayed.client.get(
      Uri.parse('https://pt-tools/api/sites'),
    );
    expect(denied.statusCode, 403);

    // 改成只读：两条会话都收到 scope_changed，自动重连，按新权限
    final devices =
        (await _control(control, '/devices', method: 'GET'))['items'] as List;
    final id =
        (devices.firstWhere((d) => d['name'] == 'Dart 互通') as Map)['id'] as int;
    final first = direct.session!;
    await _control(control, '/devices/$id/scopes?scopes=read');
    await first.done;
    expect(first.goAway, 'scope_changed');
    await _until(direct, LinkState.online);
    expect((await api.getMeta())!.principal.scopes, ['app:read']);

    // 撤销：会话收到 revoked，状态停在 revoked，之后握手 not_paired
    await _control(control, '/devices/$id/revoke');
    expect((await _until(direct, LinkState.revoked)).goAway, 'revoked');
    expect((await _until(relayed, LinkState.revoked)).goAway, 'revoked');
    final again = HostConnection(host);
    addTearDown(again.dispose);
    await expectLater(
      again.ensure(),
      throwsA(
        isA<HandshakeException>().having((e) => e.notPaired, 'notPaired', true),
      ),
    );
  });

  test('只有 relay 的链接也能配对；配对窗口关了以后 not_paired', () async {
    final ticket = await _control(control, '/pair?scopes=read');
    final full = PairingLink.parse(ticket['link'] as String);
    final viaRelay = PairingLink(
      hostId: full.hostId,
      hostKey: full.hostKey,
      secret: full.secret,
      relays: full.relays,
    );
    final host = await pairWithLink(viaRelay, 'Dart relay 配对');
    expect(host.scopes, ['app:read']);
    expect(host.direct, isNull);
    // 窗口用过了：再用同一个链接配对，主机说 not_paired
    await expectLater(
      pairWithLink(viaRelay, '再来一次'),
      throwsA(isA<PairingFailure>()),
    );
  });
}
