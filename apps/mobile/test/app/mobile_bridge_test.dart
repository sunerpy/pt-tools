import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:pt_tools_app/src/app/hybrid/mobile_bridge.dart';

void main() {
  test('cancelled save is not a success and sharing never claims delivery', () {
    expect(mobileImageResult('save', false), {
      'saved': false,
      'canceled': true,
    });
    expect(mobileImageResult('save', true), {'saved': true, 'canceled': false});
    expect(mobileImageResult('share', false), {'launched': true});
  });
  test(
    'bundled editors cannot reach session APIs or arbitrary destinations',
    () {
      for (final path in [
        '/api/global',
        'https://evil/api/app/v1/subscriptions',
        '//evil/api/app/v1/subscriptions',
        '/api/app/v1/config/global',
        '/api/app/v1/../config',
        '/api/app/v1/subscriptions%2f1',
        '/api/app/v1/subscriptions/1#x',
      ]) {
        expect(allowedMobileRequest('GET', path), isFalse, reason: path);
      }
      expect(
        allowedMobileRequest('GET', '/api/app/v1/subscriptions/1'),
        isTrue,
      );
      expect(
        allowedMobileRequest('GET', '/api/app/v1/stats/summary?range=7d'),
        isTrue,
      );
      expect(
        allowedMobileRequest('DELETE', '/api/app/v1/subscriptions/1'),
        isFalse,
      );
    },
  );
  test(
    'read-only preview cannot mutate and duplicate writes do not run twice',
    () async {
      var calls = 0;
      final client = MockClient((req) async {
        calls++;
        return http.Response(
          jsonEncode({'id': 1}),
          200,
          headers: {'content-type': 'application/json'},
        );
      });
      final readonly = MobileBridge(client, canWrite: () => false);
      await expectLater(
        readonly.request('a', 'PUT', '/api/app/v1/subscriptions/1', {}),
        throwsA(isA<MobileBridgeError>()),
      );
      expect(calls, 0);
      final bridge = MobileBridge(client, canWrite: () => true);
      expect(
        await bridge.request('a', 'PUT', '/api/app/v1/subscriptions/1', {}),
        {'id': 1},
      );
      await expectLater(
        bridge.request('a', 'PUT', '/api/app/v1/subscriptions/1', {}),
        throwsA(isA<MobileBridgeError>()),
      );
      expect(calls, 1);
      bridge.dispose();
      await expectLater(
        bridge.request('b', 'GET', '/api/app/v1/meta', null),
        throwsA(isA<MobileBridgeError>()),
      );
    },
  );

  // 追剧方案 F3（3.3.3）：编辑页多两个只读接口（草稿、试搜索），相近的路径照样拒绝。
  test('the subscription editor reaches draft and search preview, nothing near them', () {
    expect(
      allowedMobileRequest(
        'GET',
        '/api/app/v1/subscriptions/draft?media_type=tv&tmdb_id=1399&season=0',
      ),
      isTrue,
    );
    expect(
      allowedMobileRequest('POST', '/api/app/v1/subscriptions/search/preview'),
      isTrue,
    );
    for (final (verb, path) in const [
      ('GET', '/api/app/v1/subscriptions/draft/1'),
      ('GET', '/api/app/v1/subscriptions/draftx'),
      ('GET', '/api/app/v1/subscriptions/draft?tmdb_id=%31'),
      ('GET', '/api/app/v1/subscriptions/1/search/preview'),
      ('GET', '/api/app/v1/subscriptions/search/preview'),
      ('POST', '/api/app/v1/subscriptions/draft'),
      ('POST', '/api/app/v1/subscriptions/1/search/preview'),
      ('POST', '/api/app/v1/subscriptions/search/preview/1'),
      ('POST', '/api/app/v1/subscriptions/search'),
      ('PUT', '/api/app/v1/subscriptions/search/preview'),
    ]) {
      expect(allowedMobileRequest(verb, path), isFalse, reason: '$verb $path');
    }
  });

  test('a read-only device cannot run a search preview', () async {
    var calls = 0;
    final client = MockClient((req) async {
      calls++;
      return http.Response('{}', 200);
    });
    final readonly = MobileBridge(client, canWrite: () => false);
    await expectLater(
      readonly.request(
        'p',
        'POST',
        '/api/app/v1/subscriptions/search/preview',
        {'tmdb_id': 1},
      ),
      throwsA(isA<MobileBridgeError>().having((e) => e.status, 'status', 403)),
    );
    expect(calls, 0);
  });

  test(
    'copy: only the subscription editor, non-empty, at most 4096 characters',
    () {
      Matcher status(int code) => throwsA(
        isA<MobileBridgeError>().having((e) => e.status, 'status', code),
      );
      expect(
        mobileCopyText('subscription', 'Dream.S01.1080p'),
        'Dream.S01.1080p',
      );
      expect(() => mobileCopyText('brush', 'Dream'), status(403));
      expect(() => mobileCopyText('poster', 'Dream'), status(403));
      expect(() => mobileCopyText('', 'Dream'), status(403));
      expect(() => mobileCopyText('subscription', ''), status(400));
      expect(() => mobileCopyText('subscription', '  '), status(400));
      expect(() => mobileCopyText('subscription', 42), status(400));
      // 按字数算，不按字节：4096 个汉字可以，4097 个不行
      final most = List.filled(maxCopyChars, '字').join();
      expect(mobileCopyText('subscription', most), most);
      expect(() => mobileCopyText('subscription', '$most字'), status(413));
    },
  );

  test(
    'cancel(id) aborts only that request; the other one still returns',
    () async {
      final gates = <String, Completer<void>>{};
      final client = MockClient.streaming((req, body) async {
        final gate = gates[req.url.path] = Completer<void>();
        final abort = req is http.Abortable ? req.abortTrigger : null;
        await Future.any<void>([
          gate.future,
          if (abort != null)
            abort.then<void>(
              (_) => throw http.RequestAbortedException(req.url),
            ),
        ]);
        return http.StreamedResponse(
          Stream.value(utf8.encode('{"ok":true}')),
          200,
          headers: {'content-type': 'application/json'},
        );
      });
      final bridge = MobileBridge(client, canWrite: () => true);
      final preview = bridge.request(
        'a',
        'POST',
        '/api/app/v1/subscriptions/search/preview',
        {'tmdb_id': 1},
      );
      final options = bridge.request(
        'b',
        'GET',
        '/api/app/v1/mobile/options',
        null,
      );
      final failed = expectLater(preview, throwsA(anything));
      await pumpEventQueue();
      bridge.cancel('a');
      await failed;
      gates['/api/app/v1/mobile/options']!.complete();
      expect(await options, {'ok': true});
    },
  );
}
