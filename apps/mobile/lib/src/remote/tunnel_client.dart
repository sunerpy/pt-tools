// package:http 的 Client，请求经远程会话的隧道发给主机（App API 的生成代码与图片都用它）。
import 'package:http/http.dart' as http;

import 'session.dart';

class TunnelClient extends http.BaseClient {
  TunnelClient(this._session);

  final RemoteSession _session;

  /// 支持 [http.Abortable]：abortTrigger 完成时取消隧道上的请求（主机收到 CANCEL），抛 [http.RequestAbortedException]。
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    final body = await request.finalize().toBytes();
    final path = request.url.path.isEmpty ? '/' : request.url.path;
    final target = request.url.hasQuery ? '$path?${request.url.query}' : path;
    final abort = request is http.Abortable ? request.abortTrigger : null;
    final TunnelResponse resp;
    try {
      resp = await _session.request(
        request.method,
        target,
        headers: request.headers,
        body: body.isEmpty ? null : body,
        abort: abort,
      );
    } on RequestCanceledException {
      throw http.RequestAbortedException(request.url);
    }
    return http.StreamedResponse(
      resp.body.handleError(
        (Object _) => throw http.RequestAbortedException(request.url),
        test: (e) => e is RequestCanceledException,
      ),
      resp.status,
      headers: resp.headers,
      request: request,
    );
  }
}
