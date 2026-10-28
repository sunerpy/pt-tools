import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/app/hybrid/mobile_assets.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'asset server exposes no HTTP APIs and denies non-asset paths',
    () async {
      final assets = await MobileAssets.start();
      final client = HttpOverrides.runWithHttpOverrides(
        () => HttpClient(),
        _RealHttpOverrides(),
      );
      addTearDown(() async {
        client.close(force: true);
        await assets.close();
      });
      expect(assets.origin.host, '127.0.0.1');
      for (final path in [
        '/api/app/v1/meta',
        '/pubspec.yaml',
        '/assets/../secret.key',
        '/assets/nested/script.js',
      ]) {
        final req = await client.getUrl(assets.origin.replace(path: path));
        final response = await req.close();
        expect(response.statusCode, 404, reason: path);
        expect(
          response.headers.value('Content-Security-Policy'),
          contains("frame-src 'none'"),
        );
        await response.drain<void>();
      }
      final request = await client.postUrl(
        assets.origin.replace(path: '/mobile.html'),
      );
      final response = await request.close();
      expect(response.statusCode, 404);
      await response.drain<void>();
    },
  );
}

class _RealHttpOverrides extends HttpOverrides {}
