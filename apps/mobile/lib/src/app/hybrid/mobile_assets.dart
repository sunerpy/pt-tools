import 'dart:async';
import 'dart:io';

import 'package:flutter/services.dart';

/// Serves only APK assets. There is deliberately no HTTP API proxy here.
/// Other applications can read static code but cannot call the WebView channel.
class MobileAssets {
  MobileAssets._(this._server);
  final HttpServer _server;
  Uri get origin => Uri(
    scheme: 'http',
    host: InternetAddress.loopbackIPv4.address,
    port: _server.port,
  );

  static Future<MobileAssets> start() async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((request) async {
      request.response.headers
        ..set('X-Content-Type-Options', 'nosniff')
        ..set('Cache-Control', 'no-store')
        ..set(
          'Content-Security-Policy',
          "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
        );
      final path = request.uri.path;
      final valid =
          request.method == 'GET' &&
          (path == '/mobile.html' ||
              RegExp(r'^/assets/[A-Za-z0-9_.-]+$').hasMatch(path));
      if (!valid) {
        request.response.statusCode = HttpStatus.notFound;
      } else {
        try {
          final data = await rootBundle.load('assets/mobile$path');
          final ext = path.split('.').last;
          const types = {
            'html': 'text/html; charset=utf-8',
            'js': 'text/javascript; charset=utf-8',
            'css': 'text/css; charset=utf-8',
            'woff2': 'font/woff2',
            'woff': 'font/woff',
            'png': 'image/png',
            'svg': 'image/svg+xml',
            'webp': 'image/webp',
            'jpg': 'image/jpeg',
          };
          request.response.headers.set(
            'Content-Type',
            types[ext] ?? 'application/octet-stream',
          );
          request.response.add(
            data.buffer.asUint8List(data.offsetInBytes, data.lengthInBytes),
          );
        } on Object {
          request.response.statusCode = HttpStatus.notFound;
        }
      }
      await request.response.close();
    });
    return MobileAssets._(server);
  }

  Future<void> close() => _server.close(force: true);
}
