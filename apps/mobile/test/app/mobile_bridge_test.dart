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
}
