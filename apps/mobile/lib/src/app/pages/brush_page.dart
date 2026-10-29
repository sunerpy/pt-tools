import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../brush_periods.dart';
import '../format.dart';
import '../icons.dart';
import '../providers.dart';
import '../theme.dart';
import '../widgets/brush_trend.dart';
import '../widgets/common.dart';
import 'brush_details_page.dart';
import 'mobile_editor_page.dart';

/// 刷流任务：在刷的种子数、收益与累计。主机提供按周期分段的统计（features 里有 brush_stats）时，
/// 页面最上面是「日 / 周 / 月 / 季 / 年」：本期与上一期的上传下载、每一段的柱状图，
/// 每个任务那一行也写它本期的收益（2026-10-28 用户要按这几种范围看，手机端与网页端同步）。
class BrushPage extends ConsumerWidget {
  const BrushPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final list = ref.watch(brushTasksProvider);
    final withStats =
        ref.watch(metaProvider).value?.features.contains('brush_stats') == true;
    return Scaffold(
      appBar: AppBar(
        title: Text(s.moreBrush),
        actions: [
          if (ref.watch(canWriteProvider))
            IconButton(
              icon: const PtIcon(PtGlyph.plus),
              tooltip: s.mobileAdd,
              onPressed: () async {
                if (await openMobileEditor(context, ref, 'brush')) {
                  ref.invalidate(brushTasksProvider);
                }
              },
            ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          try {
            ref.invalidate(brushTasksProvider);
            if (withStats) {
              ref.invalidate(brushStatsProvider(ref.read(brushPeriodProvider)));
            }
            await ref.read(brushTasksProvider.future);
          } on Object {
            // 页面显示错误
          }
        },
        child: AsyncBody<List<AppBrushTask>>(
          value: list,
          onRetry: () => ref.invalidate(brushTasksProvider),
          data: (items) => items.isEmpty
              ? EmptyState(icon: PtGlyph.cycle, message: s.noBrush)
              : _BrushBody(items: items, withStats: withStats),
        ),
      ),
    );
  }
}

class _BrushBody extends ConsumerStatefulWidget {
  const _BrushBody({required this.items, required this.withStats});
  final List<AppBrushTask> items;
  final bool withStats;

  @override
  ConsumerState<_BrushBody> createState() => _BrushBodyState();
}

class _BrushBodyState extends ConsumerState<_BrushBody> {
  /// 上一次读到的统计：换周期时新的一份还在路上，先接着显示这一份（淡一点），不闪成空的
  AppBrushStats? _last;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final period = ref.watch(brushPeriodProvider);
    final value = widget.withStats
        ? ref.watch(brushStatsProvider(period))
        : null;
    if (value?.value != null) _last = value!.value;
    final shown = value?.value ?? _last;
    final stale = value != null && value.isLoading && value.value == null;
    final failed = value != null && value.hasError && !value.isLoading;
    // 显示的数字按它们自己的周期写：读新周期的那一下，旧数据仍是旧周期的叫法。
    // 新周期读失败时不再显示旧数据，叫法跟着选的周期（不拿今天的冒充本周）
    final shownPeriod = shown == null || failed
        ? period
        : BrushPeriod.fromApi(shown.granularity.toJson());
    // 每个任务本期的收益；统计没读到时按日还有任务自己的「今天」，别的周期写「—」
    final perTask = <int, AppBrushPeriod>{
      if (shown != null && !failed)
        for (final t in shown.tasks)
          if (t.periods.isNotEmpty) t.id: t.periods.last,
    };
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
      children: [
        if (widget.withStats) ...[
          SegmentedButton<BrushPeriod>(
            key: const ValueKey('brush-period'),
            showSelectedIcon: false,
            segments: [
              for (final p in BrushPeriod.values)
                ButtonSegment(value: p, label: Text(p.label(s))),
            ],
            selected: {period},
            onSelectionChanged: (v) =>
                ref.read(brushPeriodProvider.notifier).set(v.first),
          ),
          if (value != null && value.isLoading)
            const Padding(
              padding: EdgeInsets.only(top: 8),
              child: LinearProgressIndicator(minHeight: 2),
            ),
          const SizedBox(height: 12),
          if (failed)
            _StatsError(
              onRetry: () => ref.invalidate(brushStatsProvider(period)),
            )
          else if (shown != null)
            AnimatedOpacity(
              opacity: stale ? 0.5 : 1,
              duration: const Duration(milliseconds: 150),
              child: _StatsSummary(stats: shown, period: shownPeriod),
            ),
          const SizedBox(height: 16),
        ],
        for (final b in widget.items)
          _TaskCard(
            task: b,
            period: widget.withStats ? shownPeriod : null,
            current: perTask[b.id],
          ),
      ],
    );
  }
}

class _StatsError extends StatelessWidget {
  const _StatsError({required this.onRetry});
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final t = Theme.of(context);
    return Card(
      margin: EdgeInsets.zero,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(14, 6, 6, 6),
        child: Row(
          children: [
            Expanded(
              child: Text(
                s.brushStatsFailed,
                style: t.textTheme.bodyMedium?.copyWith(
                  color: t.colorScheme.onSurfaceVariant,
                ),
              ),
            ),
            TextButton(onPressed: onRetry, child: Text(s.retry)),
          ],
        ),
      ),
    );
  }
}

/// 本期与上一期的上传下载，加上每一段的柱状图。
class _StatsSummary extends StatelessWidget {
  const _StatsSummary({required this.stats, required this.period});
  final AppBrushStats stats;
  final BrushPeriod period;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final t = Theme.of(context);
    final pt = context.pt;
    final zh = Localizations.localeOf(context).languageCode == 'zh';
    final totals = stats.totals;
    final cur = totals.isEmpty ? null : totals.last;
    final prev = totals.length > 1 ? totals[totals.length - 2] : null;
    final up = totals.fold<int>(0, (n, p) => n + p.uploaded);
    final down = totals.fold<int>(0, (n, p) => n + p.downloaded);
    final window = period.window(s, totals.length);
    final points = [
      for (var i = 0; i < totals.length; i++)
        BrushTrendPoint(
          short: brushPeriodShort(period, totals[i].start, zh: zh),
          long: i == totals.length - 1
              ? s.brushTrendCurrent(
                  brushPeriodLong(
                    period,
                    totals[i].start,
                    totals[i].end,
                    zh: zh,
                  ),
                  period.current(s),
                )
              : brushPeriodLong(period, totals[i].start, totals[i].end, zh: zh),
          value: totals[i].uploaded,
          valueText: s.brushTrendUp(formatBytes(totals[i].uploaded)),
          detail: s.brushTrendDetail(
            formatBytes(totals[i].downloaded),
            totals[i].added,
            totals[i].removed,
          ),
        ),
    ];
    final hasTrend = totals.any((p) => p.uploaded > 0 || p.downloaded > 0);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              child: MetricTile(
                key: const ValueKey('brush-kpi-up'),
                label: s.brushUpIn(period.current(s)),
                value: cur == null ? '—' : formatBytes(cur.uploaded),
                color: pt.ok,
                icon: PtGlyph.arrowUp,
                note: prev == null
                    ? null
                    : s.brushPrevValue(
                        period.previous(s),
                        formatBytes(prev.uploaded),
                      ),
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: MetricTile(
                key: const ValueKey('brush-kpi-down'),
                label: s.brushDownIn(period.current(s)),
                value: cur == null ? '—' : formatBytes(cur.downloaded),
                color: pt.info,
                icon: PtGlyph.arrowDown,
                note: prev == null
                    ? null
                    : s.brushPrevValue(
                        period.previous(s),
                        formatBytes(prev.downloaded),
                      ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 10),
        Card(
          margin: EdgeInsets.zero,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(14, 12, 14, 12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  s.brushTrendTitle(window),
                  style: t.textTheme.titleSmall?.copyWith(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const SizedBox(height: 10),
                if (hasTrend)
                  BrushTrendChart(points: points, color: pt.ok)
                else
                  Padding(
                    padding: const EdgeInsets.symmetric(vertical: 12),
                    child: Text(
                      s.brushTrendEmpty(window),
                      style: t.textTheme.bodyMedium?.copyWith(
                        color: t.colorScheme.onSurfaceVariant,
                      ),
                    ),
                  ),
                const SizedBox(height: 8),
                Text(
                  [
                    s.brushTrendTotal(formatBytes(up), formatBytes(down)),
                    if (hasTrend && points.length > 1) s.brushTrendHint,
                  ].join(' · '),
                  style: t.textTheme.bodySmall?.copyWith(
                    fontSize: 12,
                    color: t.colorScheme.onSurfaceVariant,
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _TaskCard extends ConsumerWidget {
  const _TaskCard({
    required this.task,
    required this.period,
    required this.current,
  });

  final AppBrushTask task;

  /// 统计周期；主机没有分段统计时为 null，那一行照旧写「今天」
  final BrushPeriod? period;

  /// 这个任务本期的收益（统计没读到时为 null）
  final AppBrushPeriod? current;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final b = task;
    final p = period;
    final String periodLine;
    if (p == null) {
      periodLine = s.brushToday(
        formatBytes(b.today.uploaded),
        formatBytes(b.today.downloaded),
      );
    } else if (current != null) {
      periodLine = s.brushPeriodRow(
        p.current(s),
        formatBytes(current!.uploaded),
        formatBytes(current!.downloaded),
      );
    } else if (p == BrushPeriod.day) {
      periodLine = s.brushPeriodRow(
        p.current(s),
        formatBytes(b.today.uploaded),
        formatBytes(b.today.downloaded),
      );
    } else {
      // 统计没读到：不拿今天的冒充本周
      periodLine = s.brushPeriodRow(p.current(s), '—', '—');
    }
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      clipBehavior: Clip.antiAlias,
      child: ListTile(
        contentPadding: const EdgeInsets.fromLTRB(14, 8, 14, 8),
        onTap: () async {
          final meta = await ref.read(metaProvider.future);
          if (!context.mounted) return;
          if (!meta.features.contains('brush_manage')) {
            ScaffoldMessenger.of(context)
                .showSnackBar(SnackBar(content: Text(s.mobileUpgrade)));
            return;
          }
          await Navigator.of(context).push(
            MaterialPageRoute<void>(
              builder: (_) => BrushDetailsPage(id: b.id, name: b.name),
            ),
          );
          ref.invalidate(brushTasksProvider);
          if (p != null) ref.invalidate(brushStatsProvider(p));
        },
        leading: SiteIcon(b.site, size: 32),
        title: Text(b.name),
        subtitle: Text(
          [
            if (!b.enabled) s.brushDisabled,
            s.brushActive(b.active),
            periodLine,
            s.brushTotal(
              formatBytes(b.total.uploaded),
              formatBytes(b.total.downloaded),
            ),
          ].join('\n'),
        ),
        isThreeLine: true,
        trailing: Text(
          b.downloader,
          style: Theme.of(context).textTheme.bodySmall,
        ),
      ),
    );
  }
}
