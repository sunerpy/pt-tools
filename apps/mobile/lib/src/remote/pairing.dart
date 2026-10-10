// 扫码配对：用链接里的地址（先直连再 relay）建配对会话，提交配对密钥与设备名，拿到主机记下的设备。
import 'dart:async';

import 'connection.dart';
import 'host_record.dart';
import 'link.dart';
import 'noise.dart';
import 'session.dart';
import 'transport.dart';

/// 配对失败，[message] 是给人看的原因。
class PairingFailure implements Exception {
  const PairingFailure(this.message, {this.retryable = false});
  final String message;

  /// 换个网络、稍后再试可能就好了（连不上、主机忙）；否则要在网页上重新生成二维码
  final bool retryable;
  @override
  String toString() => 'PairingFailure: $message';
}

/// 把配对时的错误翻译成给人看的原因。
PairingFailure describePairError(Object e) {
  if (e is PairingFailure) return e;
  if (e is PairException) {
    return switch (e.code) {
      'invalid_secret' => const PairingFailure('配对密钥不对：请重新扫码，或者在网页上重新生成二维码'),
      'pairing_closed' => const PairingFailure('二维码已经过期、用过或者被取消了：请在网页上重新生成'),
      'rejected' => const PairingFailure('主机拒绝了这台设备（设备数到了上限，或者这把密钥已经配对过）'),
      'invalid_name' => const PairingFailure('设备名不对：最多 64 个字，不能有控制字符'),
      _ => PairingFailure('配对失败（${e.status} ${e.code}）：${e.message}'),
    };
  }
  if (e is HandshakeException) {
    if (e.notPaired) {
      return const PairingFailure('主机现在没有打开配对窗口：请在网页上「添加设备」重新生成二维码');
    }
    if (e.reason == 'busy') {
      return const PairingFailure('主机正忙，稍后再试', retryable: true);
    }
    return PairingFailure('握手失败：${e.message}', retryable: true);
  }
  if (e is SessionClosedException && e.goAway == 'pairing_closed') {
    return const PairingFailure('二维码已经过期或者作废了：请在网页上重新生成');
  }
  return PairingFailure('连不上主机：$e', retryable: true);
}

/// 用 [link] 配对，设备名是 [name]。成功时返回要存下来的主机记录。
Future<HostRecord> pairWithLink(
  PairingLink link,
  String name, {
  ChannelFactory? channel,
  String client = 'pt-tools-app/1.0.0',
  DateTime Function()? now,
}) async {
  final dial =
      channel ??
      (Uri url, Duration timeout) =>
          WebSocketMsgChannel.connect(url, timeout: timeout);
  final device = await DhKey.generate();
  final draft = HostRecord(
    hostId: link.hostId,
    hostKey: link.hostKey,
    devicePrivate: device.privateKey,
    relays: link.relays,
    direct: link.direct,
    deviceId: 0,
    deviceName: name,
    scopes: const [],
    pairedAt: (now ?? DateTime.now)(),
  );
  Object? last;
  for (final ep in endpointsOf(draft)) {
    MsgChannel? ch;
    RemoteSession? s;
    try {
      ch = await dial(
        ep.url,
        ep.via == Via.direct
            ? const Duration(seconds: 3)
            : const Duration(seconds: 8),
      );
      s = await RemoteSession.connect(
        ch,
        hostId: link.hostId,
        hostKey: link.hostKey,
        device: device,
        client: client,
      );
      if (s.mode != SessionMode.pairing) {
        throw const HandshakeException('主机没有把这次连接当成配对');
      }
      final dev = await s.pair(link.secret, name);
      // 主机随后以 GOAWAY paired 关掉这条会话；不用等
      s.close();
      return HostRecord(
        hostId: link.hostId,
        hostKey: link.hostKey,
        devicePrivate: device.privateKey,
        relays: link.relays,
        direct: link.direct,
        deviceId: (dev['id'] as num?)?.toInt() ?? 0,
        deviceName: (dev['name'] as String?) ?? name,
        scopes: ((dev['scopes'] as List?) ?? const []).cast<String>(),
        pairedAt: draft.pairedAt,
      );
    } on PairException catch (e) {
      s?.close();
      // 主机已经回了配对结果：别的地址也是同一个窗口，不用再试
      throw describePairError(e);
    } on HandshakeException catch (e) {
      unawaited(ch?.close());
      if (e.notPaired) throw describePairError(e);
      last = e;
    } on Object catch (e) {
      s?.close();
      unawaited(ch?.close());
      last = e;
    }
  }
  throw describePairError(last ?? const PairingFailure('链接里没有可用的地址'));
}
