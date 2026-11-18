// 刷流收益的统计周期：按日、周、月、季度、年分段（App API GET /brush/stats；主机时区，周从周一开始，
// 最后一段是当前这一段、只算到今天）。与网页端 web/frontend/src/utils/brushPeriods.ts 同一套叫法与段数。
import '../../l10n/app_localizations.dart';

enum BrushPeriod {
  day('day', 30),
  week('week', 12),
  month('month', 12),
  quarter('quarter', 8),
  year('year', 5);

  const BrushPeriod(this.api, this.periods);

  /// 接口的 granularity
  final String api;

  /// 请求几段
  final int periods;

  static BrushPeriod fromApi(String v) =>
      values.firstWhere((p) => p.api == v, orElse: () => day);

  /// 分段按钮上的字
  String label(S s) => switch (this) {
    day => s.brushPeriodDay,
    week => s.brushPeriodWeek,
    month => s.brushPeriodMonth,
    quarter => s.brushPeriodQuarter,
    year => s.brushPeriodYear,
  };

  /// 当前这一段：今日、本周、本月、本季度、今年
  String current(S s) => switch (this) {
    day => s.brushCurDay,
    week => s.brushCurWeek,
    month => s.brushCurMonth,
    quarter => s.brushCurQuarter,
    year => s.brushCurYear,
  };

  /// 上一段：昨日、上周、上月、上季度、去年
  String previous(S s) => switch (this) {
    day => s.brushPrevDay,
    week => s.brushPrevWeek,
    month => s.brushPrevMonth,
    quarter => s.brushPrevQuarter,
    year => s.brushPrevYear,
  };

  /// 整个窗口：最近 30 天、最近 12 周…（[n] 是实际的段数）
  String window(S s, [int? n]) {
    final c = n ?? periods;
    return switch (this) {
      day => s.brushWindowDay(c),
      week => s.brushWindowWeek(c),
      month => s.brushWindowMonth(c),
      quarter => s.brushWindowQuarter(c),
      year => s.brushWindowYear(c),
    };
  }
}

/// 「2026-10-29」→ 本地的那一天（不按 UTC 解析）；不是这个格式时为 null。
DateTime? parseBrushDay(String v) {
  final m = RegExp(r'^(\d{4})-(\d{2})-(\d{2})$').firstMatch(v);
  if (m == null) return null;
  return DateTime(int.parse(m[1]!), int.parse(m[2]!), int.parse(m[3]!));
}

const _enMonths = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
];

int _quarter(DateTime d) => (d.month - 1) ~/ 3 + 1;

/// 柱子下面的短标签：日与周「10/29」（周写周一），月「10月」/「Oct」（一月带年份），
/// 季度「Q4」（第一季度带年份），年「2026」。
///
/// 日期的写法放在代码里而不是文案表：中英文的语序不同，而文案表的占位符只认整段替换。
String brushPeriodShort(BrushPeriod p, String start, {required bool zh}) {
  final d = parseBrushDay(start);
  if (d == null) return start;
  return switch (p) {
    BrushPeriod.day || BrushPeriod.week => '${d.month}/${d.day}',
    BrushPeriod.month =>
      zh
          ? (d.month == 1 ? '${d.year}年1月' : '${d.month}月')
          : (d.month == 1 ? 'Jan ${d.year}' : _enMonths[d.month - 1]),
    BrushPeriod.quarter =>
      _quarter(d) == 1 ? '${d.year} Q1' : 'Q${_quarter(d)}',
    BrushPeriod.year => '${d.year}',
  };
}

/// 读数里的完整写法：「10月29日」「10月26日–11月1日」「2026年10月」「2026年第4季度」「2026年」；
/// 英文「Oct 29」「Oct 26 – Nov 1」「Oct 2026」「Q4 2026」「2026」。
String brushPeriodLong(
  BrushPeriod p,
  String start,
  String end, {
  required bool zh,
}) {
  final d = parseBrushDay(start);
  if (d == null) return start;
  String md(DateTime x) =>
      zh ? '${x.month}月${x.day}日' : '${_enMonths[x.month - 1]} ${x.day}';
  switch (p) {
    case BrushPeriod.day:
      return md(d);
    case BrushPeriod.week:
      final e = parseBrushDay(end);
      if (e == null) return md(d);
      return zh ? '${md(d)}–${md(e)}' : '${md(d)} – ${md(e)}';
    case BrushPeriod.month:
      return zh
          ? '${d.year}年${d.month}月'
          : '${_enMonths[d.month - 1]} ${d.year}';
    case BrushPeriod.quarter:
      return zh ? '${d.year}年第${_quarter(d)}季度' : 'Q${_quarter(d)} ${d.year}';
    case BrushPeriod.year:
      return zh ? '${d.year}年' : '${d.year}';
  }
}
