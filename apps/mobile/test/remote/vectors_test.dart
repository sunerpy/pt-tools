// 用 Go 那边的测试向量逐字节核对 Dart 实现：Noise 公开向量、本协议的帧、配对链接与一次完整会话的线上字节。
// 向量文件在仓库的 internal/remote/testdata/（两端共用）。
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/remote/bytes.dart';
import 'package:pt_tools_app/src/remote/frames.dart';
import 'package:pt_tools_app/src/remote/link.dart';
import 'package:pt_tools_app/src/remote/noise.dart';

Map<String, dynamic> _load(String name) =>
    jsonDecode(File('../../internal/remote/testdata/$name').readAsStringSync())
        as Map<String, dynamic>;

void main() {
  final vectors = _load('vectors.json');
  final noise = _load('noise_ik_vectors.json');

  group('Noise 公开向量', () {
    for (final v in (noise['vectors'] as List).cast<Map<String, dynamic>>()) {
      test('${v['source']}', () async {
        expect(v['protocol_name'], protocolName);
        final init = NoiseIK.initiatorOf(
          s: await DhKey.fromPrivate(fromHex(v['init_static'] as String)),
          rs: fromHex(v['init_remote_static'] as String),
          prologue: fromHex(v['init_prologue'] as String),
          e: await DhKey.fromPrivate(fromHex(v['init_ephemeral'] as String)),
        );
        final resp = NoiseIK.responderOf(
          s: await DhKey.fromPrivate(fromHex(v['resp_static'] as String)),
          prologue: fromHex(v['resp_prologue'] as String),
          e: await DhKey.fromPrivate(fromHex(v['resp_ephemeral'] as String)),
        );
        final msgs = (v['messages'] as List).cast<Map<String, dynamic>>();
        for (var i = 0; i < msgs.length; i++) {
          final payload = fromHex(msgs[i]['payload'] as String);
          final want = msgs[i]['ciphertext'] as String;
          final fromInit = i.isEven;
          final (w, r) = fromInit ? (init, resp) : (resp, init);
          Uint8List ct;
          Uint8List pt;
          if (i < 2) {
            ct = await w.writeMessage(payload);
            pt = await r.readMessage(ct);
          } else {
            ct = w.send.encryptWithAd(const [], payload);
            pt = r.recv.decryptWithAd(const [], ct);
          }
          expect(toHex(ct), want, reason: '第 $i 条消息');
          expect(toHex(pt), toHex(payload));
          if (i == 1 && (v['handshake_hash'] as String).isNotEmpty) {
            expect(toHex(init.handshakeHash), v['handshake_hash']);
            expect(toHex(resp.handshakeHash), v['handshake_hash']);
          }
        }
      });
    }
  });

  test('隧道帧：编出来一字不差，不合规的解不开', () {
    for (final f in (vectors['frames'] as List).cast<Map<String, dynamic>>()) {
      final frame = Frame(
        f['type'] as int,
        f['id'] as int,
        fromHex(f['payload'] as String),
      );
      expect(toHex(frame.encode()), f['encoded']);
      final back = Frame.parse(fromHex(f['encoded'] as String));
      expect(back.type, f['type']);
      expect(back.id, f['id']);
      expect(toHex(back.payload), f['payload']);
    }
    for (final b in (vectors['invalid_frames'] as List).cast<String>()) {
      expect(
        () => Frame.parse(fromHex(b)),
        throwsA(isA<FrameException>()),
        reason: b,
      );
    }
  });

  test('配对链接', () {
    for (final l in (vectors['links'] as List).cast<Map<String, dynamic>>()) {
      final p = PairingLink.parse(l['link'] as String);
      expect(p.hostId, l['host_id']);
      expect(toHex(p.hostKey), l['host_key']);
      expect(toHex(p.secret), l['secret']);
      expect(
        p.relays,
        (l['relays'] as List?)?.cast<String>() ?? const <String>[],
      );
      expect(p.direct, l['direct'] == '' ? null : l['direct']);
    }
    for (final raw in (vectors['invalid_links'] as List).cast<String>()) {
      expect(
        () => PairingLink.parse(raw),
        throwsA(isA<LinkException>()),
        reason: raw,
      );
    }
    expect(
      () => PairingLink.parse((vectors['invalid_links'] as List)[1] as String),
      throwsA(
        isA<LinkException>().having(
          (e) => e.unsupportedVersion,
          'unsupportedVersion',
          true,
        ),
      ),
    );
  });

  test('一次完整会话：握手与每个帧的密文和 Go 的一字不差', () async {
    final s = vectors['session'] as Map<String, dynamic>;
    final device = NoiseIK.initiatorOf(
      s: await DhKey.fromPrivate(fromHex(s['device_static'] as String)),
      rs: fromHex(s['host_static_public'] as String),
      prologue: fromHex(s['prologue'] as String),
      e: await DhKey.fromPrivate(fromHex(s['device_ephemeral'] as String)),
    );
    final host = NoiseIK.responderOf(
      s: await DhKey.fromPrivate(fromHex(s['host_static'] as String)),
      prologue: fromHex(s['prologue'] as String),
      e: await DhKey.fromPrivate(fromHex(s['host_ephemeral'] as String)),
    );
    expect(
      utf8.decode(fromHex(s['prologue'] as String)),
      'pt-tools-remote-v1${s['host_id']}',
    );
    for (final m in (s['messages'] as List).cast<Map<String, dynamic>>()) {
      final fromDevice = m['from'] == 'device';
      final want = m['ciphertext'] as String;
      if (m['kind'] == 'handshake') {
        final payload = utf8.encode(m['payload'] as String);
        final (w, r) = fromDevice ? (device, host) : (host, device);
        final ct = await w.writeMessage(payload);
        expect(toHex(ct), want);
        expect(utf8.decode(await r.readMessage(ct)), m['payload']);
        continue;
      }
      final f = m['frame'] as Map<String, dynamic>;
      final frame = Frame(
        f['type'] as int,
        f['id'] as int,
        fromHex(f['payload'] as String),
      );
      final (send, recv) = fromDevice
          ? (device.send, host.recv)
          : (host.send, device.recv);
      final ct = send.encryptWithAd(const [], frame.encode());
      expect(
        toHex(ct),
        want,
        reason: 'type 0x${(f['type'] as int).toRadixString(16)}',
      );
      final back = Frame.parse(recv.decryptWithAd(const [], ct));
      expect(back.type, f['type']);
    }
    expect(toHex(host.remoteStatic!), s['device_static_public']);
  });

  test('base64url 只认规范写法', () {
    expect(
      encodeKey(List.filled(32, 0xaa)),
      'qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqo',
    );
    expect(
      decodeKey('qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqo', length: 32),
      List.filled(32, 0xaa),
    );
    expect(
      () => decodeKey('qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqp'),
      throwsFormatException,
    );
    expect(() => decodeKey('qq=='), throwsFormatException);
    expect(() => decodeKey('a+b/'), throwsFormatException);
  });
}
