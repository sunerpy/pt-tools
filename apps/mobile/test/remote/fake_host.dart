// 测试用的假主机：Noise 响应方加隧道帧，走内存里的消息通道。
import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/remote/frames.dart';
import 'package:pt_tools_app/src/remote/noise.dart';
import 'package:pt_tools_app/src/remote/session.dart';

/// 内存里的一对消息通道。
class Pipe implements MsgChannel {
  Pipe(this._in, this._out);
  final StreamController<Uint8List> _in;
  final StreamController<Uint8List> _out;
  int? code;
  Pipe? peer;

  @override
  Stream<Uint8List> get messages => _in.stream;

  @override
  void send(Uint8List message) {
    if (!_out.isClosed) _out.add(message);
  }

  @override
  Future<void> close([int code = 1000, String reason = '']) async {
    peer?.code ??= code;
    if (!_out.isClosed) await _out.close();
    if (!_in.isClosed) await _in.close();
  }

  @override
  int? get closeCode => code;

  bool get isClosed => _out.isClosed;

  static (Pipe, Pipe) pair() {
    final a = StreamController<Uint8List>();
    final b = StreamController<Uint8List>();
    final x = Pipe(a, b);
    final y = Pipe(b, a);
    x.peer = y;
    y.peer = x;
    return (x, y);
  }
}

/// 假主机：握手以后按请求路径回应。
class FakeHost {
  FakeHost(
    this.ch,
    this.key,
    this.hostId, {
    this.mode = 'device',
    this.error,
    this.api,
  });
  final Pipe ch;
  final DhKey key;
  final String hostId;
  final String mode;
  final String? error;

  /// App API 的罐头回应：返回 null 时按默认（回显路径）处理
  final Object? Function(String method, String path)? api;
  late CipherState send;
  late CipherState recv;
  final requests = <Map<String, dynamic>>[];
  final bodies = <int, BytesBuilder>{};
  final cancelled = <int>[];
  int pings = 0;

  /// 不再回应任何帧（PING 也不回），模拟半开的连接
  bool silent = false;

  /// 握手完成（第二条消息发出）时完成。
  final ready = Completer<void>();

  /// 发 GOAWAY，然后断开。
  Future<void> goAway(String reason) async {
    write(Frame.json(FrameType.goAway, 0, {'reason': reason}));
    await ch.close();
  }

  void write(Frame f) => ch.send(send.encryptWithAd(const [], f.encode()));

  Future<void> run() async {
    final hs = NoiseIK.responderOf(
      s: key,
      prologue: utf8.encode('pt-tools-remote-v1$hostId'),
    );
    final it = StreamIterator(ch.messages);
    await it.moveNext();
    final hello = jsonDecode(utf8.decode(await hs.readMessage(it.current)));
    expect(hello['v'], 1);
    final reply = error != null
        ? {'v': 1, 'error': error}
        : {'v': 1, 'mode': mode, 'host': 'v-test'};
    ch.send(await hs.writeMessage(utf8.encode(jsonEncode(reply))));
    if (error != null) {
      await ch.close();
      return;
    }
    send = hs.send;
    recv = hs.recv;
    ready.complete();
    while (await it.moveNext()) {
      final f = Frame.parse(recv.decryptWithAd(const [], it.current));
      if (silent) continue;
      switch (f.type) {
        case FrameType.reqHead:
          requests.add({
            'id': f.id,
            ...jsonDecode(utf8.decode(f.payload)) as Map<String, dynamic>,
          });
          bodies[f.id] = BytesBuilder();
        case FrameType.reqBody:
          bodies[f.id]!.add(f.payload);
        case FrameType.reqEnd:
          _respond(f.id);
        case FrameType.cancel:
          cancelled.add(f.id);
        case FrameType.ping:
          pings++;
          write(Frame(FrameType.pong, 0, f.payload));
      }
    }
  }

  void _respond(int id) {
    final req = requests.firstWhere((r) => r['id'] == id);
    final path = req['path'] as String;
    final canned = api?.call(req['method'] as String, path);
    if (canned != null) {
      write(
        Frame.json(FrameType.respHead, id, {
          'status': 200,
          'headers': {'Content-Type': 'application/json'},
        }),
      );
      write(Frame.json(FrameType.respBody, id, canned));
      write(Frame(FrameType.respEnd, id, Uint8List(0)));
      return;
    }
    if (path == '/echo') {
      final body = bodies[id]!.takeBytes();
      write(
        Frame.json(FrameType.respHead, id, {
          'status': 200,
          'headers': {'Content-Type': 'application/octet-stream'},
        }),
      );
      for (var off = 0; off < body.length; off += maxFramePayload) {
        final end = off + maxFramePayload < body.length
            ? off + maxFramePayload
            : body.length;
        write(
          Frame(FrameType.respBody, id, Uint8List.sublistView(body, off, end)),
        );
      }
      write(Frame(FrameType.respEnd, id, Uint8List(0)));
    } else if (path == '/hang') {
      write(Frame.json(FrameType.respHead, id, {'status': 200}));
      write(Frame(FrameType.respBody, id, Uint8List.fromList([1, 2, 3])));
    } else if (path == '/abort') {
      write(Frame.json(FrameType.respHead, id, {'status': 200}));
      write(
        Frame(
          FrameType.cancel,
          id,
          Uint8List.fromList(utf8.encode('internal error')),
        ),
      );
    } else if (path == '/goaway') {
      write(Frame.json(FrameType.goAway, 0, {'reason': 'revoked'}));
    } else if (path == '/remote/v1/pair') {
      write(
        Frame.json(FrameType.respHead, id, {
          'status': 200,
          'headers': {'Content-Type': 'application/json'},
        }),
      );
      write(
        Frame.json(FrameType.respBody, id, {
          'device': {
            'id': 7,
            'name': jsonDecode(utf8.decode(bodies[id]!.takeBytes()))['name'],
          },
        }),
      );
      write(Frame(FrameType.respEnd, id, Uint8List(0)));
      write(Frame.json(FrameType.goAway, 0, {'reason': 'paired'}));
    } else {
      write(
        Frame.json(FrameType.respHead, id, {
          'status': 200,
          'headers': {'Content-Type': 'application/json'},
        }),
      );
      write(
        Frame.json(FrameType.respBody, id, {
          'path': path,
          'method': req['method'],
          'headers': req['headers'],
        }),
      );
      write(Frame(FrameType.respEnd, id, Uint8List(0)));
    }
  }
}
