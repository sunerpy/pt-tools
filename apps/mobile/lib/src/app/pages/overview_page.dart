import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../icons.dart';
import '../providers.dart';
import '../theme.dart';
import '../widgets/common.dart';

/// 概览：已启用站点的合计（上传、计费下载、分享率、魔力、做种、站内信）、今天的增量、下载器的速度，
/// 以及每台下载器正在做种的个数与体积（主机有这个接口时）。
///
/// 站点报的下载量是计费口径（按免费、折扣折算以后计入分享率），不是实际传输量；App API 没有实际传输量。
class OverviewPage extends ConsumerWidget {
  const OverviewPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final ov = ref.watch(overviewProvider);
    final meta = ref.watch(metaProvider).value;
    return Scaffold(
      appBar: AppBar(
        title: Text(s.navOverview),
        actions: [
          if (meta != null)
            Padding(
              padding: const EdgeInsets.only(right: 16),
              child: Center(
                // 大字号时版本号最多占四成宽，放不下就省略，不把顶栏挤出界
                child: ConstrainedBox(
                  constraints: BoxConstraints(
                    maxWidth: MediaQuery.sizeOf(context).width * 0.4,
                  ),
                  child: Text(
                    s.hostVersion(meta.version),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context).textTheme.labelMedium,
                  ),
                ),
              ),
            ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          ref.invalidate(downloadersProvider);
          ref.invalidate(downloaderSeedingProvider);
          try {
            ref.invalidate(overviewProvider);
            await ref.read(overviewProvider.future);
          } on Object {
            // 页面显示错误
          }
        },
        child: AsyncBody<AppOverview>(
          value: ov,
          onRetry: () => ref.invalidate(overviewProvider),
          data: (o) => ListView(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
            children: [
              if (meta != null && meta.remoteApiLevel != supportedApiLevel)
                Card(
                  color: Theme.of(context).colorScheme.errorContainer,
                  child: Padding(
                    padding: const EdgeInsets.all(12),
                    child: Text(
                      s.apiLevelMismatch(
                        meta.remoteApiLevel,
                        supportedApiLevel,
                      ),
                    ),
                  ),
                ),
              _Totals(o.totals),
              const SizedBox(height: 16),
              _Today(o.today),
              const _Downloaders(),
              const _DownloaderSeeding(),
              if (o.updatedAt > 0) ...[
                const SizedBox(height: 12),
                Text(
                  s.updatedAt(formatTime(o.updatedAt)),
                  style: Theme.of(context).textTheme.bodySmall,
                  textAlign: TextAlign.center,
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _Totals extends StatelessWidget {
  const _Totals(this.t);
  final AppTotals t;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final pt = context.pt;
    final tiles = [
      MetricTile(
        label: s.kpiUploaded,
        value: formatBytes(t.uploaded),
        color: pt.ok,
        icon: PtGlyph.arrowUp,
      ),
      MetricTile(
        label: s.kpiDownloaded,
        value: formatBytes(t.downloaded),
        color: pt.info,
        icon: PtGlyph.arrowDown,
      ),
      MetricTile(
        label: s.kpiRatio,
        value: formatRatio(t.ratio),
        color: pt.violet,
        icon: PtGlyph.scale,
      ),
      MetricTile(
        label: s.kpiBonus,
        value: formatNumber(t.bonus),
        color: pt.warn,
        icon: PtGlyph.star,
        note: t.bonusPerHour > 0
            ? s.perHour(formatNumber(t.bonusPerHour))
            : null,
      ),
      MetricTile(
        label: s.kpiSeeding,
        value: '${t.seeding}',
        color: pt.pink,
        icon: PtGlyph.cloudUp,
        note: formatBytes(t.seedingSize),
      ),
      MetricTile(
        label: s.kpiUnread,
        value: '${t.unreadMessages}',
        color: t.unreadMessages > 0 ? pt.danger : pt.subtle,
        icon: PtGlyph.mail,
        note: s.siteCount(t.siteCount),
      ),
    ];
    return LayoutBuilder(
      builder: (context, c) {
        final cols = c.maxWidth >= 600 ? 3 : 2;
        const gap = 10.0;
        final w = (c.maxWidth - gap * (cols - 1)) / cols;
        return Wrap(
          spacing: gap,
          runSpacing: gap,
          children: [for (final tile in tiles) SizedBox(width: w, child: tile)],
        );
      },
    );
  }
}

class _Today extends StatelessWidget {
  const _Today(this.d);
  final AppDelta d;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final t = Theme.of(context);
    final sites = [...d.sites]
      ..sort((a, b) => b.uploaded.compareTo(a.uploaded));
    return Card(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(s.todayTitle, style: t.textTheme.titleMedium),
            const SizedBox(height: 8),
            if (d.error != null && d.error!.isNotEmpty)
              Text(
                s.todayError(d.error!),
                style: TextStyle(color: t.colorScheme.error),
              )
            else
              Row(
                children: [
                  Expanded(
                    child: _Delta(
                      icon: PtGlyph.arrowUp,
                      color: context.pt.ok,
                      text: formatBytes(d.uploaded),
                    ),
                  ),
                  Expanded(
                    child: _Delta(
                      icon: PtGlyph.arrowDown,
                      color: context.pt.info,
                      text: formatBytes(d.downloaded),
                    ),
                  ),
                  Expanded(
                    child: _Delta(
                      icon: PtGlyph.star,
                      color: context.pt.warn,
                      text: '+${formatNumber(d.bonus)}',
                    ),
                  ),
                ],
              ),
            if (sites.isNotEmpty) ...[
              const Divider(height: 20),
              Text(s.todayBySite, style: t.textTheme.labelLarge),
              for (final x in sites.take(8))
                ListTile(
                  dense: true,
                  contentPadding: EdgeInsets.zero,
                  leading: SiteIcon(x.site, size: 24),
                  title: Text(x.site),
                  trailing: Text(
                    '↑${formatBytes(x.uploaded)}  ↓${formatBytes(x.downloaded)}',
                    style: t.textTheme.bodySmall,
                  ),
                ),
            ],
            // 上面的 ↓ 都是计费下载量，增量来自每日快照，快照只存这个口径
            if (d.error == null || d.error!.isEmpty) ...[
              const SizedBox(height: 8),
              Text(s.todayBilledNote, style: t.textTheme.bodySmall),
            ],
          ],
        ),
      ),
    );
  }
}

class _Delta extends StatelessWidget {
  const _Delta({required this.icon, required this.color, required this.text});
  final PtGlyph icon;
  final Color color;
  final String text;

  @override
  Widget build(BuildContext context) => Row(
    children: [
      PtIcon(icon, size: 16, weight: 2, color: color),
      const SizedBox(width: 4),
      Flexible(
        child: Text(
          text,
          style: Theme.of(context).textTheme.titleSmall,
          overflow: TextOverflow.ellipsis,
        ),
      ),
    ],
  );
}

class _Downloaders extends ConsumerWidget {
  const _Downloaders();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final t = Theme.of(context);
    final list =
        ref.watch(downloadersProvider).value?.items ?? const <AppDownloader>[];
    if (list.isEmpty) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(top: 16),
      child: Card(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(s.downloadersTitle, style: t.textTheme.titleMedium),
              for (final d in list)
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  dense: true,
                  leading: PtIcon(
                    d.reachable ? PtGlyph.server : PtGlyph.serverOff,
                    size: 20,
                    color: d.reachable
                        ? t.colorScheme.onSurfaceVariant
                        : t.colorScheme.error,
                  ),
                  title: Text(d.name),
                  subtitle: Text(
                    d.reachable
                        ? s.freeSpace(formatBytes(d.freeSpace))
                        : s.downloaderUnreachable,
                  ),
                  trailing: Text(
                    '↑${formatSpeed(d.uploadSpeed)}\n↓${formatSpeed(d.downloadSpeed)}',
                    textAlign: TextAlign.right,
                    style: t.textTheme.bodySmall,
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// 下载器做种：每台下载器现在正在做种的个数与体积，和上面「做种」里各站上报的对着看。
/// 主机的版本没有这个接口、还在读或者没有启用的下载器时不占位置；读取失败时写一句，能重试。
class _DownloaderSeeding extends ConsumerWidget {
  const _DownloaderSeeding();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final t = Theme.of(context);
    final value = ref.watch(downloaderSeedingProvider);
    if (value.hasError && !value.hasValue) {
      return Padding(
        padding: const EdgeInsets.only(top: 16),
        child: Card(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 8, 8),
            child: Row(
              children: [
                Expanded(child: Text(s.seedingFailed)),
                TextButton(
                  onPressed: () => ref.invalidate(downloaderSeedingProvider),
                  child: Text(s.retry),
                ),
              ],
            ),
          ),
        ),
      );
    }
    final data = value.value;
    if (data == null || data.items.isEmpty) return const SizedBox.shrink();
    final strong = t.textTheme.bodyMedium?.copyWith(
      fontWeight: FontWeight.w600,
    );
    return Padding(
      padding: const EdgeInsets.only(top: 16),
      child: Card(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // 字号大、屏幕窄时合计换到下一行，不挤出界
              Wrap(
                alignment: WrapAlignment.spaceBetween,
                crossAxisAlignment: WrapCrossAlignment.center,
                spacing: 12,
                children: [
                  Text(s.seedingTitle, style: t.textTheme.titleMedium),
                  Text(
                    s.seedingCountSize(
                      data.totalSeedingCount,
                      formatBytes(data.totalSeedingSize),
                    ),
                    style: t.textTheme.labelLarge,
                  ),
                ],
              ),
              for (final d in data.items)
                Padding(
                  padding: const EdgeInsets.only(top: 12),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      PtIcon(
                        d.reachable ? PtGlyph.server : PtGlyph.serverOff,
                        size: 20,
                        color: d.reachable
                            ? t.colorScheme.onSurfaceVariant
                            : t.colorScheme.error,
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Wrap(
                              alignment: WrapAlignment.spaceBetween,
                              spacing: 12,
                              children: [
                                Text(d.name, style: t.textTheme.bodyMedium),
                                Text(
                                  d.reachable
                                      ? s.seedingCountSize(
                                          d.seedingCount,
                                          formatBytes(d.seedingSize),
                                        )
                                      : s.downloaderUnreachable,
                                  style: d.reachable
                                      ? strong
                                      : strong?.copyWith(
                                          color: t.colorScheme.error,
                                        ),
                                ),
                              ],
                            ),
                            // 下载完了没在做种的；连不上时是原因（可能很长，只留两行，完整的在「下载器」页）
                            if (!d.reachable &&
                                d.error != null &&
                                d.error!.isNotEmpty)
                              Text(
                                d.error!,
                                maxLines: 2,
                                overflow: TextOverflow.ellipsis,
                                style: t.textTheme.bodySmall,
                              )
                            else if (d.reachable && d.idleCount > 0)
                              Text(
                                s.seedingIdle(
                                  d.idleCount,
                                  formatBytes(d.idleSize),
                                ),
                                style: t.textTheme.bodySmall,
                              ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              const SizedBox(height: 12),
              Text(s.seedingNote, style: t.textTheme.bodySmall),
            ],
          ),
        ),
      ),
    );
  }
}
