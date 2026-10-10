import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
import '../widgets/common.dart';
import 'media_page.dart';

/// 订阅详情：海报、进度、每一集的状态、下载过的种子；完全控制的设备可以暂停/恢复、立即搜索、删除。
class SubscriptionPage extends ConsumerWidget {
  const SubscriptionPage({super.key, required this.id});
  final int id;

  Future<void> _run(
    BuildContext context,
    WidgetRef ref,
    Future<String?> Function(AppApi api) f,
  ) async {
    final s = S.of(context);
    final messenger = ScaffoldMessenger.of(context);
    try {
      final msg = await f(ref.read(apiProvider)!);
      if (msg != null) messenger.showSnackBar(SnackBar(content: Text(msg)));
    } on Object catch (e) {
      messenger.showSnackBar(
        SnackBar(content: Text(s.errorPrefix(describeError(e)))),
      );
    }
    ref.invalidate(subscriptionProvider(id));
    ref.invalidate(subscriptionsProvider);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final detail = ref.watch(subscriptionProvider(id));
    final canWrite = ref.watch(canWriteProvider);
    final d = detail.value;
    return Scaffold(
      appBar: AppBar(
        title: Text(d?.title ?? s.tabSubscriptions),
        actions: [
          if (canWrite && d != null)
            PopupMenuButton<String>(
              onSelected: (v) async {
                switch (v) {
                  case 'toggle':
                    await _run(context, ref, (api) async {
                      await api.setSubscriptionStatus(
                        id,
                        AppSubscriptionStatus(
                          status: AppSubscriptionStatusStatusEnum.fromJson(
                            d.status == 'paused' ? 'active' : 'paused',
                          )!,
                        ),
                      );
                      return null;
                    });
                  case 'search':
                    await _run(
                      context,
                      ref,
                      (api) async =>
                          (await api.searchSubscription(id))?.message,
                    );
                  case 'delete':
                    final ok = await showDialog<bool>(
                      context: context,
                      builder: (c) => AlertDialog(
                        title: Text(s.subDelete),
                        content: Text(s.subDeleteConfirm),
                        actions: [
                          TextButton(
                            onPressed: () => Navigator.pop(c, false),
                            child: Text(s.cancel),
                          ),
                          FilledButton(
                            onPressed: () => Navigator.pop(c, true),
                            child: Text(s.actionDelete),
                          ),
                        ],
                      ),
                    );
                    if (ok == true && context.mounted) {
                      await _run(context, ref, (api) async {
                        await api.deleteSubscription(id);
                        return null;
                      });
                      if (context.mounted) context.go('/media');
                    }
                }
              },
              itemBuilder: (_) => [
                PopupMenuItem(
                  value: 'toggle',
                  child: Text(d.status == 'paused' ? s.subResume : s.subPause),
                ),
                if (d.status != 'paused')
                  PopupMenuItem(value: 'search', child: Text(s.subSearchNow)),
                PopupMenuItem(value: 'delete', child: Text(s.subDelete)),
              ],
            ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          try {
            ref.invalidate(subscriptionProvider(id));
            await ref.read(subscriptionProvider(id).future);
          } on Object {
            // 页面显示错误
          }
        },
        child: AsyncBody<AppSubscriptionDetail>(
          value: detail,
          onRetry: () => ref.invalidate(subscriptionProvider(id)),
          data: (d) => ListView(
            padding: const EdgeInsets.all(16),
            children: [
              Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Poster(d.posterPath, width: 100),
                  const SizedBox(width: 16),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          d.title,
                          style: Theme.of(context).textTheme.titleLarge,
                        ),
                        if (d.originalTitle != null &&
                            d.originalTitle != d.title)
                          Text(
                            d.originalTitle!,
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                        const SizedBox(height: 8),
                        Wrap(
                          spacing: 6,
                          runSpacing: 6,
                          children: [
                            Chip(
                              label: Text(subStatusLabel(s, d.status)),
                              visualDensity: VisualDensity.compact,
                            ),
                            if (d.mediaType == 'tv' && d.season > 0)
                              Chip(
                                label: Text(
                                  'S${d.season.toString().padLeft(2, '0')}',
                                ),
                                visualDensity: VisualDensity.compact,
                              ),
                            if (d.upgrade)
                              Chip(
                                label: Text(s.subUpgrade),
                                visualDensity: VisualDensity.compact,
                              ),
                          ],
                        ),
                        if (d.mediaType == 'tv')
                          Text(
                            s.subProgress(
                              d.progress.inLibrary,
                              d.progress.aired == 0
                                  ? d.progress.total
                                  : d.progress.aired,
                            ),
                          ),
                        if (d.message != null && d.message!.isNotEmpty)
                          Text(
                            d.message!,
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                        if (d.nextSearchAt != null && d.nextSearchAt! > 0)
                          Text(
                            s.subNextSearch(formatTime(d.nextSearchAt!)),
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                      ],
                    ),
                  ),
                ],
              ),
              if (d.episodes.isNotEmpty) ...[
                const SizedBox(height: 20),
                Text(
                  s.episodesTitle,
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [for (final e in d.episodes) _EpisodeChip(e)],
                ),
              ],
              if (d.torrents.isNotEmpty) ...[
                const SizedBox(height: 20),
                Text(
                  s.subTorrentsTitle,
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                for (final t in d.torrents)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: SiteIcon(t.site, size: 28),
                    title: Text(
                      t.title,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                    subtitle: Text(
                      [
                        t.status,
                        formatBytes(t.size),
                        formatTime(t.createdAt),
                      ].join(' · '),
                    ),
                  ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _EpisodeChip extends StatelessWidget {
  const _EpisodeChip(this.e);
  final AppEpisode e;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final scheme = Theme.of(context).colorScheme;
    final (bg, label) = switch (e.state) {
      'library' => (const Color(0xFF10B981), s.epLibrary),
      'downloading' => (const Color(0xFF3B82F6), s.epDownloading),
      'missing' => (scheme.error, s.epMissing),
      _ => (scheme.outline, s.epUpcoming),
    };
    return Tooltip(
      message: '${e.name ?? ''} ${e.airDate ?? ''} $label'.trim(),
      child: Container(
        width: 40,
        height: 32,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: bg.withValues(alpha: 0.15),
          borderRadius: BorderRadius.circular(6),
          border: Border.all(color: bg.withValues(alpha: 0.6)),
        ),
        child: Text(
          '${e.number}',
          style: TextStyle(color: bg, fontWeight: FontWeight.w600),
        ),
      ),
    );
  }
}
