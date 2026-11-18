// 刷流收益的统计周期：段数与柱子、读数上的日期写法（中英文）。
import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/app/brush_periods.dart';

void main() {
  test('每种周期请求的段数与接口值', () {
    expect(BrushPeriod.values.map((p) => '${p.api}:${p.periods}'), [
      'day:30',
      'week:12',
      'month:12',
      'quarter:8',
      'year:5',
    ]);
    expect(BrushPeriod.fromApi('quarter'), BrushPeriod.quarter);
    expect(BrushPeriod.fromApi('hour'), BrushPeriod.day);
  });

  test('柱子下面的短标签', () {
    expect(brushPeriodShort(BrushPeriod.day, '2026-10-29', zh: true), '10/29');
    expect(
      brushPeriodShort(BrushPeriod.week, '2026-10-26', zh: false),
      '10/26',
    );
    expect(brushPeriodShort(BrushPeriod.month, '2026-10-01', zh: true), '10月');
    expect(
      brushPeriodShort(BrushPeriod.month, '2026-01-01', zh: true),
      '2026年1月',
    );
    expect(brushPeriodShort(BrushPeriod.month, '2026-10-01', zh: false), 'Oct');
    expect(
      brushPeriodShort(BrushPeriod.month, '2026-01-01', zh: false),
      'Jan 2026',
    );
    expect(brushPeriodShort(BrushPeriod.quarter, '2026-10-01', zh: true), 'Q4');
    expect(
      brushPeriodShort(BrushPeriod.quarter, '2026-01-01', zh: true),
      '2026 Q1',
    );
    expect(brushPeriodShort(BrushPeriod.year, '2026-01-01', zh: true), '2026');
    expect(brushPeriodShort(BrushPeriod.day, 'bad', zh: true), 'bad');
  });

  test('读数里的完整写法', () {
    expect(
      brushPeriodLong(BrushPeriod.day, '2026-10-29', '2026-10-29', zh: true),
      '10月29日',
    );
    expect(
      brushPeriodLong(BrushPeriod.week, '2026-10-26', '2026-11-01', zh: true),
      '10月26日–11月1日',
    );
    expect(
      brushPeriodLong(BrushPeriod.week, '2026-10-26', '2026-11-01', zh: false),
      'Oct 26 – Nov 1',
    );
    expect(
      brushPeriodLong(BrushPeriod.month, '2026-10-01', '2026-10-31', zh: true),
      '2026年10月',
    );
    expect(
      brushPeriodLong(
        BrushPeriod.quarter,
        '2026-07-01',
        '2026-09-30',
        zh: true,
      ),
      '2026年第3季度',
    );
    expect(
      brushPeriodLong(
        BrushPeriod.quarter,
        '2026-07-01',
        '2026-09-30',
        zh: false,
      ),
      'Q3 2026',
    );
    expect(
      brushPeriodLong(BrushPeriod.year, '2026-01-01', '2026-12-31', zh: true),
      '2026年',
    );
  });
}
