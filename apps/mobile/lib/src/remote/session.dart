// 设备端的远程会话：在一条消息通道（直连或 relay 的 WebSocket）上做 Noise IK 握手，之后用隧道帧收发 HTTP 请求。
// 协议见 docs/design/remote-access.md；Go 端的对应实现是 internal/remote/client.go。
import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'bytes.dart';
import 'frames.dart';
import 'noise.dart';

/// 一条消息通道：每条二进制消息恰好是一条 Noise 消息。
abstract interface class MsgChannel {
  /// 收到的消息（只能监听一次）；通道关闭时结束。
  Stream<Uint8List> get messages;

  void send(Uint8List message);

  Future<void> close([int code = 1000, String reason = '']);

  /// 对端关闭时的关闭码（还没关、或者没有关闭帧时为 null）。
  int? get closeCode;
}

/// 会话种类。
enum SessionMode { device, pairing }

/// 握手失败。[reason] 是主机给的原因：`not_paired`、`unsupported_version`、`busy`；连接断开、超时、主机公钥不对时为 null。
class HandshakeException implements Exception {
  const HandshakeException(this.message, {this.reason});
  final String message;
  final String? reason;

  bool get notPaired => reason == 'not_paired';

  @override
  String toString() => 'HandshakeException: $message';
}

/// 会话结束了（[goAway] 是主机给的原因，没有时为 null）。
class SessionClosedException implements Exception {
  const SessionClosedException(this.message, {this.goAway});
  final String message;
  final String? goAway;
  @override
  String toString() => 'SessionClosedException: $message';
}

/// 主机中止了一个回应（回应头发出以后出错）。
class ResponseAbortedException implements Exception {
  const ResponseAbortedException(this.reason);
  final String reason;
  @override
  String toString() => 'ResponseAbortedException: $reason';
}

/// 隧道上的一个回应：状态、回应头（小写名字）与回应体。
class TunnelResponse {
  TunnelResponse(this.status, this.headers, this.body);
  final int status;
  final Map<String, String> headers;
  final Stream<List<int>> body;
}

/// 每条会话同时进行的请求上限（主机超过时回 429）。
const maxConcurrentRequests = 16;

/// 发 PING 的间隔（主机 90 秒收不到任何帧就断开）。
const pingInterval = Duration(seconds: 30);

/// 这么久没收到主机的任何帧（PONG 也算）就当连接已经断了（例如手机换了网络，旧连接半开着）。
const idleTimeout = Duration(seconds: 90);

/// 每条握手消息最多等这么久。
const handshakeTimeout = Duration(seconds: 10);

class _Call {
  _Call(this.id);
  final int id;
  final head = Completer<TunnelResponse>();
  StreamController<List<int>>? body;
  bool cancelled = false;
}

class RemoteSession {
  RemoteSession._(
    this._ch,
    this._send,
    this._recv,
    this.mode,
    this.hostVersion,
    this._pingEvery,
    this._idle,
  );

  final MsgChannel _ch;
  final CipherState _send;
  final CipherState _recv;
  final SessionMode mode;
  final String hostVersion;
  final Duration _pingEvery;
  final Duration _idle;

  final _calls = <int, _Call>{};
  int _nextId = 0;
  int _active = 0;
  final _waiters = <Completer<void>>[];
  Timer? _ping;
  final _sinceRecv = Stopwatch();
  final _done = Completer<void>();
  Object? _error;
  String? _goAway;

  /// 会话结束时完成。
  Future<void> get done => _done.future;

  /// 会话已经结束。
  bool get closed => _done.isCompleted;

  /// 主机关会话时给的原因（没有时为 null）。
  String? get goAway => _goAway;

  /// 会话结束的原因。
  Object? get error => _error;

  /// 在 [ch] 上握手。[hostKey] 是配对时记下的主机 X25519 公钥，[device] 是这台设备的密钥。
  static Future<RemoteSession> connect(
    MsgChannel ch, {
    required String hostId,
    required Uint8List hostKey,
    required DhKey device,
    required String client,
    DhKey? ephemeral,
    Duration timeout = handshakeTimeout,
    Duration ping = pingInterval,
    Duration idle = idleTimeout,
  }) async {
    final hs = NoiseIK.initiatorOf(
      s: device,
      rs: hostKey,
      prologue: utf8.encode('pt-tools-remote-v1$hostId'),
      e: ephemeral,
    );
    final it = StreamIterator(ch.messages);
    try {
      ch.send(
        await hs.writeMessage(
          utf8.encode(jsonEncode({'v': 1, 'client': client})),
        ),
      );
      final bool got;
      try {
        got = await it.moveNext().timeout(timeout);
      } on TimeoutException {
        throw const HandshakeException('等主机回话超时');
      }
      if (!got) throw HandshakeException('连接断开（${ch.closeCode ?? '无关闭码'}）');
      final msg = it.current;
      if (msg.length > maxNoiseMessage) {
        throw const HandshakeException('第二条握手消息太长');
      }
      final Uint8List payload;
      try {
        payload = await hs.readMessage(msg);
      } on NoiseException {
        throw const HandshakeException('解不开主机的回话（主机公钥不对）');
      }
      final Object? hello;
      try {
        hello = payload.length > 1024 ? null : jsonDecode(utf8.decode(payload));
      } on FormatException {
        throw const HandshakeException('不认识主机的握手内容');
      }
      if (hello is! Map || hello['v'] != 1) {
        throw const HandshakeException('不认识主机的握手内容');
      }
      final err = hello['error'];
      if (err is String && err.isNotEmpty) {
        throw HandshakeException(
          err == 'not_paired' ? '这台设备没有配对，或者已经被撤销' : '主机拒绝了连接：$err',
          reason: err,
        );
      }
      final mode = switch (hello['mode']) {
        'device' => SessionMode.device,
        'pairing' => SessionMode.pairing,
        _ => throw const HandshakeException('不认识的会话种类'),
      };
      final s = RemoteSession._(
        ch,
        hs.send,
        hs.recv,
        mode,
        hello['host'] is String ? hello['host'] as String : '',
        ping,
        idle,
      );
      s._start(it);
      return s;
    } catch (_) {
      await it.cancel();
      unawaited(ch.close());
      rethrow;
    }
  }

  void _start(StreamIterator<Uint8List> it) {
    _sinceRecv.start();
    // 握手以后的消息接着从同一个迭代器读
    unawaited(() async {
      try {
        while (await it.moveNext()) {
          _sinceRecv.reset();
          _onMessage(it.current);
          if (closed) break;
        }
        _fail(SessionClosedException('连接断开（${_ch.closeCode ?? '无关闭码'}）'));
      } catch (e) {
        _fail(e);
      } finally {
        await it.cancel();
      }
    }());
    _ping = Timer.periodic(_pingEvery, (_) {
      if (_sinceRecv.elapsed >= _idle) {
        _fail(const SessionClosedException('主机很久没有回应，连接可能已经断了'));
        return;
      }
      try {
        _write(Frame(FrameType.ping, 0, Uint8List(0)));
      } on Object catch (e) {
        _fail(e);
      }
    });
  }

  void _write(Frame f) {
    if (closed) throw _error ?? const SessionClosedException('会话已经结束');
    // 加密是同步的：nonce 顺序就是发送顺序
    _ch.send(_send.encryptWithAd(const [], f.encode()));
  }

  void _onMessage(Uint8List msg) {
    if (msg.length < tagLen + frameHeaderLen || msg.length > maxNoiseMessage) {
      throw FrameException('消息长度 ${msg.length} 字节');
    }
    final Frame f;
    try {
      f = Frame.parse(_recv.decryptWithAd(const [], msg));
    } on NoiseException {
      throw const FrameException('解密失败');
    }
    switch (f.type) {
      case FrameType.respHead:
        final c = _calls[f.id];
        if (c == null || c.head.isCompleted) return;
        final head = ResponseHead.parse(f.payload);
        final body = StreamController<List<int>>(
          onCancel: () => _cancel(c, '读者不要了'),
        );
        c.body = body;
        c.head.complete(TunnelResponse(head.status, head.headers, body.stream));
      case FrameType.respBody:
        _calls[f.id]?.body?.add(Uint8List.fromList(f.payload));
      case FrameType.respEnd:
        final c = _calls[f.id];
        if (c != null) {
          _finish(c);
          c.body?.close();
        }
      case FrameType.cancel:
        final c = _calls[f.id];
        if (c != null) {
          _finish(c);
          final err = ResponseAbortedException(
            utf8.decode(f.payload, allowMalformed: true),
          );
          if (!c.head.isCompleted) c.head.completeError(err);
          c.body?.addError(err);
          c.body?.close();
        }
      case FrameType.ping:
        _write(Frame(FrameType.pong, 0, f.payload));
      case FrameType.pong:
        break;
      case FrameType.goAway:
        // 主机随后会断开；这边也不再等，直接结束会话（经 relay 时主机的关闭不一定传得过来）
        String? reason;
        try {
          final v = jsonDecode(utf8.decode(f.payload));
          if (v is Map && v['reason'] is String) reason = v['reason'] as String;
        } on FormatException {
          // 原因读不出来也照样结束
        }
        _goAway = reason ?? GoAwayReason.protocolError;
        _fail(SessionClosedException('主机关闭了会话：$_goAway', goAway: _goAway));
      default:
        throw FrameException('主机不该发类型 0x${f.type.toRadixString(16)}');
    }
  }

  void _finish(_Call c) {
    if (_calls.remove(c.id) == null) return;
    _active--;
    if (_waiters.isNotEmpty) _waiters.removeAt(0).complete();
  }

  void _cancel(_Call c, String reason) {
    if (!_calls.containsKey(c.id) || c.cancelled) return;
    c.cancelled = true;
    _finish(c);
    if (!closed) {
      try {
        _write(
          Frame(
            FrameType.cancel,
            c.id,
            Uint8List.fromList(utf8.encode(reason)),
          ),
        );
      } on Object {
        // 会话已经结束
      }
    }
  }

  int _allocId() {
    for (;;) {
      _nextId = _nextId >= 0xffffffff ? 1 : _nextId + 1;
      if (!_calls.containsKey(_nextId)) return _nextId;
    }
  }

  /// 发一个请求，等到回应头。[path] 是以 / 开头的路径，可以带查询串。回应体要读完或者取消，否则一直占着一个名额。
  Future<TunnelResponse> request(
    String method,
    String path, {
    Map<String, String> headers = const {},
    List<int>? body,
  }) async {
    if (closed) throw _error ?? const SessionClosedException('会话已经结束');
    if (body != null && body.length > maxRequestBody) {
      throw ArgumentError('请求体超过 16 MiB');
    }
    while (_active >= maxConcurrentRequests) {
      final w = Completer<void>();
      _waiters.add(w);
      await w.future;
      if (closed) throw _error ?? const SessionClosedException('会话已经结束');
    }
    final c = _Call(_allocId());
    _calls[c.id] = c;
    _active++;
    final h = <String, String>{};
    for (final e in headers.entries) {
      final allowed = requestHeaderAllow.where(
        (k) => k.toLowerCase() == e.key.toLowerCase(),
      );
      if (allowed.isNotEmpty) h[allowed.first] = e.value;
    }
    try {
      _write(
        Frame.json(
          FrameType.reqHead,
          c.id,
          RequestHead(method, path, h).toJson(),
        ),
      );
      final b = body ?? const <int>[];
      for (var off = 0; off < b.length; off += maxFramePayload) {
        final end = off + maxFramePayload < b.length
            ? off + maxFramePayload
            : b.length;
        _write(
          Frame(
            FrameType.reqBody,
            c.id,
            Uint8List.fromList(b.sublist(off, end)),
          ),
        );
      }
      _write(Frame(FrameType.reqEnd, c.id, Uint8List(0)));
    } catch (e) {
      _finish(c);
      rethrow;
    }
    return c.head.future;
  }

  /// 配对会话里提交配对密钥与设备名。成功时返回主机记下的设备（JSON），之后主机会以 GOAWAY paired 关掉这条会话。
  Future<Map<String, dynamic>> pair(Uint8List secret, String name) async {
    final resp = await request(
      'POST',
      '/remote/v1/pair',
      headers: const {'Content-Type': 'application/json'},
      body: utf8.encode(
        jsonEncode({'secret': encodeKey(secret), 'name': name}),
      ),
    );
    final data = await resp.body.fold<List<int>>(
      <int>[],
      (a, b) => a..addAll(b),
    );
    final Object? v = data.isEmpty ? null : jsonDecode(utf8.decode(data));
    if (resp.status != 200) {
      final code = v is Map ? v['error'] as String? : null;
      final message = v is Map ? v['message'] as String? : null;
      throw PairException(resp.status, code ?? '', message ?? '');
    }
    if (v is! Map || v['device'] is! Map) {
      throw const FormatException('配对回应格式不对');
    }
    return (v['device'] as Map).cast<String, dynamic>();
  }

  void _fail(Object err) {
    if (closed) return;
    _error = err;
    _ping?.cancel();
    final calls = _calls.values.toList();
    _calls.clear();
    _active = 0;
    for (final c in calls) {
      if (!c.head.isCompleted) c.head.completeError(err);
      if (c.body != null && !c.body!.isClosed) {
        c.body!.addError(err);
        c.body!.close();
      }
    }
    for (final w in _waiters) {
      w.complete();
    }
    _waiters.clear();
    _done.complete();
    unawaited(_ch.close());
  }

  /// 关掉会话。
  void close() => _fail(const SessionClosedException('会话已经关闭'));
}

/// 配对被拒绝（401 密钥不对、410 窗口关了、409 拒绝等）。
class PairException implements Exception {
  const PairException(this.status, this.code, this.message);
  final int status;
  final String code;
  final String message;
  @override
  String toString() => 'PairException($status $code): $message';
}
