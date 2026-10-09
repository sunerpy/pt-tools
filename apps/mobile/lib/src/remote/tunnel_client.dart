// package:http 的 Client，请求经远程会话的隧道发给主机（App API 的生成代码与图片都用它）。
import 'package:http/http.dart' as http;

import 'session.dart';

class TunnelClient extends http.BaseClient {
  TunnelClient(this._session);

  final RemoteSession _session;

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final body = await request.finalize().toBytes();
    final path = request.url.path.isEmpty ? '/' : request.url.path;
    final target = request.url.hasQuery ? '$path?${request.url.query}' : path;
    final resp = await _session.request(
      request.method,
      target,
      headers: request.headers,
      body: body.isEmpty ? null : body,
    );
    return http.StreamedResponse(
      resp.body,
      resp.status,
      headers: resp.headers,
      request: request,
    );
  }
}
