// 生成的 App API 客户端改订阅时只发给了的字段：没设 sites 时发 null（服务端当成不改），不是空数组（空数组是「全部站点」，
// 以前只改定时也会把站点限制清掉）。契约 docs/reference/app-api-v1.yaml 的 AppSubscriptionUpdate；生成脚本 tool/gen_api.sh。
// 只改定时时发出的 JSON 与 web/testdata/app_subscription_update_schedule_only.json 逐字一样：Go 那边拿同一个文件打 App API，
// 确认站点限制、下载器、保存路径都不变（web/api_app_v1_follow_test.go 的 TestAppSubscriptionUpdateFromGeneratedClient）。
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:ptt_api/api.dart';

void main() {
  test('只改定时：sites 发 null，发出的 JSON 和 Go 测试用的那份逐字一样', () {
    final update = AppSubscriptionUpdate(
      schedule: AppSchedule(kind: 'cron', cron: '0 22 * * 5'),
    );
    expect(update.sites, isNull);
    final body = jsonDecode(jsonEncode(update)) as Map<String, dynamic>;
    expect(body.containsKey('sites'), isTrue);
    expect(body['sites'], isNull);
    final golden = File(
      '../../web/testdata/app_subscription_update_schedule_only.json',
    ).readAsStringSync().trim();
    expect(jsonEncode(update), golden);
  });

  test('给了空数组才是全部站点，给了站点就只在这些站点找', () {
    expect(
      jsonDecode(jsonEncode(AppSubscriptionUpdate(sites: const [])))['sites'],
      isEmpty,
    );
    expect(
      jsonDecode(jsonEncode(AppSubscriptionUpdate(sites: ['hdsky'])))['sites'],
      ['hdsky'],
    );
  });
}
