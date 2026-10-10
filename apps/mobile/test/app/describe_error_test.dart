import 'dart:ui';

import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/l10n/app_localizations.dart';
import 'package:pt_tools_app/src/app/widgets/common.dart';
import 'package:pt_tools_app/src/remote/frames.dart';
import 'package:pt_tools_app/src/remote/session.dart';
import 'package:ptt_api/api.dart';

void main() {
  final s = lookupS(const Locale('zh'));

  /// 生成的客户端把连接上的异常包成这样。
  ApiException wrapped(Exception inner) => ApiException.withInner(
    400,
    'Exception occurred: GET /torrents',
    inner,
    null,
  );

  test('撤销以后的请求：说要重新配对，不给生成的客户端那句', () {
    final e = wrapped(
      const HandshakeException('这台设备已经被撤销，请重新配对', reason: 'not_paired'),
    );
    expect(describeError(e, s), s.connRevoked);
  });

  test('主机发来的 GOAWAY：按原因说', () {
    String of(String reason) => describeError(
      wrapped(SessionClosedException('主机关闭了会话：$reason', goAway: reason)),
      s,
    );
    expect(of(GoAwayReason.revoked), s.connRevoked);
    expect(of(GoAwayReason.keyRotated), s.connKeyRotated);
    expect(of(GoAwayReason.disabled), s.connDisabled);
  });

  test('其他连接上的错误：给内层异常的说明', () {
    expect(
      describeError(wrapped(const SessionClosedException('连接断开（1006）')), s),
      '连接断开（1006）',
    );
    expect(
      describeError(wrapped(const HandshakeException('主机把这台设备当成了没有配对的设备')), s),
      '主机把这台设备当成了没有配对的设备',
    );
    expect(
      describeError(wrapped(const ResponseAbortedException('internal')), s),
      'internal',
    );
  });

  test('App API 回的错误：取 message，没有时原样给', () {
    expect(
      describeError(
        ApiException(403, '{"error":"forbidden","message":"这台设备只读"}'),
        s,
      ),
      '这台设备只读',
    );
    expect(describeError(ApiException(502, 'bad gateway'), s), 'bad gateway');
    expect(describeError(ApiException(500, ''), s), 'HTTP 500');
  });
}
