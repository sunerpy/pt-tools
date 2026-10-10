// 两种传输：直连 <d>/remote/v1/stream，经 relay <r>/v1/client/<hostId>。每条 WebSocket 二进制消息是一条 Noise 消息。
import 'dart:async';
import 'dart:typed_data';

import 'package:web_socket_channel/web_socket_channel.dart';

import 'frames.dart';
import 'session.dart';

/// 连接方式。
enum Via { direct, relay }

/// 直连地址（http/https）换成 WebSocket 地址。
Uri directStreamUrl(String direct) {
  final u = Uri.parse(direct);
  final scheme = u.scheme == 'https' ? 'wss' : 'ws';
  return u.replace(scheme: scheme, path: '${u.path}/remote/v1/stream');
}

/// relay 上这台主机的客户端地址。
Uri relayClientUrl(String relay, String hostId) =>
    Uri.parse('$relay/v1/client/$hostId');

class WebSocketMsgChannel implements MsgChannel {
  WebSocketMsgChannel._(this._ws);

  final WebSocketChannel _ws;

  /// 连上 [url]（[timeout] 内完成 WebSocket 握手）。
  static Future<WebSocketMsgChannel> connect(
    Uri url, {
    Duration timeout = const Duration(seconds: 8),
  }) async {
    final ws = WebSocketChannel.connect(url);
    try {
      await ws.ready.timeout(timeout);
    } catch (_) {
      unawaited(ws.sink.close());
      rethrow;
    }
    return WebSocketMsgChannel._(ws);
  }

  @override
  Stream<Uint8List> get messages => _ws.stream.map((m) {
    if (m is Uint8List) return m;
    if (m is ByteBuffer) return m.asUint8List();
    if (m is List<int>) return Uint8List.fromList(m);
    throw const FrameException('收到了文本消息');
  });

  @override
  void send(Uint8List message) => _ws.sink.add(message);

  @override
  Future<void> close([int code = 1000, String reason = '']) async {
    try {
      await _ws.sink.close(code, reason);
    } on Object {
      // 已经关了
    }
  }

  @override
  int? get closeCode => _ws.closeCode;
}
