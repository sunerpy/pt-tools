import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
import '../widgets/common.dart';

/// 概览：已启用站点的合计（上传、下载、分享率、魔力、做种、站内信）、今天的增量、下载器的速度。
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
                child: Text(
                  s.hostVersion(meta.version),
                  style: Theme.of(context).textTheme.labelMedium,
                ),
              ),
            ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          ref.invalidate(downloadersProvider);
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
              const SizedBox(height: 16),
              const _Downloaders(),
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
    final tiles = [
      MetricTile(
        label: s.kpiUploaded,
        value: formatBytes(t.uploaded),
        color: const Color(0xFF10B981),
        icon: Icons.arrow_upward,
      ),
      MetricTile(
        label: s.kpiDownloaded,
        value: formatBytes(t.downloaded),
        color: const Color(0xFF3B82F6),
        icon: Icons.arrow_downward,
      ),
      MetricTile(
        label: s.kpiRatio,
        value: formatRatio(t.ratio),
        color: const Color(0xFF8B5CF6),
        icon: Icons.balance,
      ),
      MetricTile(
        label: s.kpiBonus,
        value: formatNumber(t.bonus),
        color: const Color(0xFFF59E0B),
        icon: Icons.stars_outlined,
        note: t.bonusPerHour > 0
            ? s.perHour(formatNumber(t.bonusPerHour))
            : null,
      ),
      MetricTile(
        label: s.kpiSeeding,
        value: '${t.seeding}',
        color: const Color(0xFF14B8A6),
        icon: Icons.cloud_upload_outlined,
        note: formatBytes(t.seedingSize),
      ),
      MetricTile(
        label: s.kpiUnread,
        value: '${t.unreadMessages}',
        color: t.unreadMessages > 0
            ? const Color(0xFFEF4444)
            : const Color(0xFF94A3B8),
        icon: Icons.mail_outline,
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
                      icon: Icons.arrow_upward,
                      color: const Color(0xFF10B981),
                      text: formatBytes(d.uploaded),
                    ),
                  ),
                  Expanded(
                    child: _Delta(
                      icon: Icons.arrow_downward,
                      color: const Color(0xFF3B82F6),
                      text: formatBytes(d.downloaded),
                    ),
                  ),
                  Expanded(
                    child: _Delta(
                      icon: Icons.stars_outlined,
                      color: const Color(0xFFF59E0B),
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
          ],
        ),
      ),
    );
  }
}

class _Delta extends StatelessWidget {
  const _Delta({required this.icon, required this.color, required this.text});
  final IconData icon;
  final Color color;
  final String text;

  @override
  Widget build(BuildContext context) => Row(
    children: [
      Icon(icon, size: 16, color: color),
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
    return Card(
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
                leading: Icon(
                  d.reachable ? Icons.dns_outlined : Icons.portable_wifi_off,
                  color: d.reachable
                      ? t.colorScheme.primary
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
    );
  }
}
