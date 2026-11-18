// 连接管理：先试直连（配了而且连得上），再按顺序试 relay；会话断了自动重连（只在前台）。
// 主机发的 GOAWAY 决定怎么办：撤销、换了密钥就停下来提示重新配对，关掉了远程访问就隔一会儿再试，改了权限马上重连。
import 'dart:async';
import 'dart:math';

import 'package:http/http.dart' as http;

import 'frames.dart';
import 'host_record.dart';
import 'noise.dart';
import 'session.dart';
import 'transport.dart';
import 'tunnel_client.dart';

/// 连上一个地址的通道（测试里换成内存的）。
typedef ChannelFactory = Future<MsgChannel> Function(Uri url, Duration timeout);

Future<MsgChannel> _webSocket(Uri url, Duration timeout) =>
    WebSocketMsgChannel.connect(url, timeout: timeout);

/// 连接的状态。
enum LinkState {
  /// 还没连，或者 App 到了后台
  offline,
  connecting,
  online,

  /// 这台设备被撤销、主机换了密钥：要重新配对
  revoked,

  /// 主机关掉了远程访问
  disabled,

  /// 所有地址都连不上（稍后自动再试）
  failed,
}

class Endpoint {
  const Endpoint(this.via, this.url, this.label);
  final Via via;
  final Uri url;
  final String label;
}

/// 这台主机的连接地址：直连在前，relay 按配对链接里的顺序。
List<Endpoint> endpointsOf(HostRecord h) => [
  if (h.direct != null)
    Endpoint(Via.direct, directStreamUrl(h.direct!), h.direct!),
  for (final r in h.relays) Endpoint(Via.relay, relayClientUrl(r, h.hostId), r),
];

class ConnectionStatus {
  const ConnectionStatus(
    this.state, {
    this.via,
    this.endpoint,
    this.message,
    this.goAway,
  });
  final LinkState state;
  final Via? via;
  final String? endpoint;
  final String? message;
  final String? goAway;

  @override
  String toString() =>
      'ConnectionStatus($state, via=$via, $endpoint, $message)';
}

class HostConnection {
  HostConnection(
    this.host, {
    this.clientName = 'pt-tools-app/1.0.0',
    ChannelFactory? channel,
    this.directTimeout = const Duration(seconds: 3),
    this.relayTimeout = const Duration(seconds: 8),
    Duration Function(int attempt)? backoff,
  }) : _channel = channel ?? _webSocket,
       _backoff = backoff ?? _defaultBackoff;

  HostRecord host;

  /// 握手时告诉主机的 App 名字与版本
  final String clientName;
  final ChannelFactory _channel;
  final Duration directTimeout;
  final Duration relayTimeout;
  final Duration Function(int attempt) _backoff;

  final _statusCtl = StreamController<ConnectionStatus>.broadcast();
  ConnectionStatus _status = const ConnectionStatus(LinkState.offline);
  RemoteSession? _session;
  Future<RemoteSession>? _connecting;
  Timer? _retry;
  int _attempt = 0;
  bool _running = false;
  bool _paused = false;
  DhKey? _device;

  /// 当前状态与变化。
  ConnectionStatus get status => _status;
  Stream<ConnectionStatus> get statuses => _statusCtl.stream;

  /// stop 以后、下一次 start 以前（App 在后台）：不连接，定时刷新也停下来。
  bool get paused => _paused;

  /// 现在的会话（没连上时为 null）。
  RemoteSession? get session => _session;

  /// 经隧道发请求的 http.Client：每个请求都先拿到（必要时建立）会话。
  late final http.Client client = _ConnectionClient(this);

  static Duration _defaultBackoff(int attempt) {
    final s = min(30, pow(2, min(attempt, 5)).toInt());
    return Duration(milliseconds: s * 1000 + Random().nextInt(500));
  }

  void _set(ConnectionStatus s) {
    _status = s;
    if (!_statusCtl.isClosed) _statusCtl.add(s);
  }

  /// 开始连接并保持（App 到了前台）。
  void start() {
    _paused = false;
    if (_running) return;
    _running = true;
    if (_status.state == LinkState.revoked) return;
    unawaited(ensure().then((_) {}, onError: (_) {}));
  }

  /// 断开，不再自动重连，之后的请求直接失败（App 到了后台）。
  void stop() {
    _running = false;
    _paused = true;
    _retry?.cancel();
    _session?.close();
    _session = null;
    if (_status.state != LinkState.revoked) {
      _set(const ConnectionStatus(LinkState.offline));
    }
  }

  void dispose() {
    stop();
    unawaited(_statusCtl.close());
  }

  /// 拿到一个能用的会话：已经连着就直接用，正在连就等它，否则现在连。
  Future<RemoteSession> ensure() {
    final s = _session;
    if (s != null && !s.closed) return Future.value(s);
    if (_paused) {
      return Future.error(const SessionClosedException('App 在后台，不连接'));
    }
    if (_status.state == LinkState.revoked) {
      return Future.error(
        const HandshakeException('这台设备已经被撤销，请重新配对', reason: 'not_paired'),
      );
    }
    return _connecting ??= _connect().whenComplete(() => _connecting = null);
  }

  Future<RemoteSession> _connect() async {
    _retry?.cancel();
    _set(
      ConnectionStatus(
        LinkState.connecting,
        via: _status.via,
        endpoint: _status.endpoint,
      ),
    );
    _device ??= await DhKey.fromPrivate(host.devicePrivate);
    final errors = <String>[];
    for (final ep in endpointsOf(host)) {
      if (_paused) break;
      MsgChannel? ch;
      try {
        ch = await _channel(
          ep.url,
          ep.via == Via.direct ? directTimeout : relayTimeout,
        );
        final s = await RemoteSession.connect(
          ch,
          hostId: host.hostId,
          hostKey: host.hostKey,
          device: _device!,
          client: clientName,
        );
        if (s.mode != SessionMode.device) {
          s.close();
          throw const HandshakeException('主机把这台设备当成了没有配对的设备');
        }
        if (_paused) {
          // 握手期间 App 到了后台
          s.close();
          break;
        }
        _session = s;
        _attempt = 0;
        host = host.copyWith(hostVersion: s.hostVersion);
        _set(
          ConnectionStatus(LinkState.online, via: ep.via, endpoint: ep.label),
        );
        final opened = DateTime.now();
        unawaited(s.done.then((_) => _onClosed(s, opened)));
        return s;
      } on HandshakeException catch (e) {
        unawaited(ch?.close());
        if (e.notPaired) {
          // 主机明确说不认识这台设备：别的地址也一样，停下来
          _set(
            ConnectionStatus(
              LinkState.revoked,
              message: e.message,
              goAway: GoAwayReason.revoked,
            ),
          );
          rethrow;
        }
        errors.add('${ep.label}：${e.message}');
      } on Object catch (e) {
        unawaited(ch?.close());
        errors.add('${ep.label}：$e');
      }
    }
    if (_paused) throw const SessionClosedException('App 在后台，不连接');
    final msg = errors.isEmpty ? '没有可用的连接地址' : errors.join('；');
    _set(ConnectionStatus(LinkState.failed, message: msg));
    _scheduleRetry();
    throw SessionClosedException(msg);
  }

  void _onClosed(RemoteSession s, DateTime opened) {
    if (!identical(_session, s)) return;
    _session = null;
    if (DateTime.now().difference(opened) > const Duration(seconds: 60)) {
      _attempt = 0;
    }
    switch (s.goAway) {
      case GoAwayReason.revoked || GoAwayReason.keyRotated:
        _set(
          ConnectionStatus(
            LinkState.revoked,
            goAway: s.goAway,
            message: s.goAway == GoAwayReason.revoked
                ? '这台设备被撤销了'
                : '主机的密钥换了，所有设备都要重新配对',
          ),
        );
        return;
      case GoAwayReason.disabled:
        _set(
          ConnectionStatus(
            LinkState.disabled,
            goAway: s.goAway,
            message: '主机关掉了远程访问',
          ),
        );
        if (_running) _retry = Timer(const Duration(seconds: 60), _reconnect);
        return;
      case GoAwayReason.scopeChanged:
        // 权限改了：马上重连，按新权限显示
        if (_running) _reconnect();
        return;
      default:
        _set(
          ConnectionStatus(
            LinkState.connecting,
            goAway: s.goAway,
            message: '连接断开，正在重连',
          ),
        );
        _scheduleRetry();
    }
  }

  void _scheduleRetry() {
    if (!_running) return;
    _retry?.cancel();
    _retry = Timer(_backoff(_attempt++), _reconnect);
  }

  void _reconnect() {
    if (!_running) return;
    unawaited(ensure().then((_) {}, onError: (_) {}));
  }
}

class _ConnectionClient extends http.BaseClient {
  _ConnectionClient(this._conn);
  final HostConnection _conn;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final s = await _conn.ensure();
    return TunnelClient(s).send(request);
  }
}
