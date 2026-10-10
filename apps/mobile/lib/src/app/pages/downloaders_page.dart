import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
import '../widgets/common.dart';

/// 下载器：速度、剩余空间、版本；连不上时给原因。每 5 秒刷新一次。
class DownloadersPage extends ConsumerWidget {
  const DownloadersPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    ref.listen(tickerProvider, (_, _) => ref.invalidate(downloadersProvider));
    final list = ref.watch(downloadersProvider);
    return Scaffold(
      appBar: AppBar(title: Text(s.moreDownloaders)),
      body: RefreshIndicator(
        onRefresh: () async {
          try {
            ref.invalidate(downloadersProvider);
            await ref.read(downloadersProvider.future);
          } on Object {
            // 页面显示错误
          }
        },
        child: AsyncBody<AppDownloaderList>(
          value: list,
          onRetry: () => ref.invalidate(downloadersProvider),
          data: (l) => l.items.isEmpty
              ? EmptyState(icon: Icons.dns_outlined, message: s.noDownloaders)
              : ListView(
                  padding: const EdgeInsets.all(12),
                  children: [
                    for (final d in l.items)
                      Card(
                        margin: const EdgeInsets.only(bottom: 8),
                        child: Padding(
                          padding: const EdgeInsets.all(14),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Row(
                                children: [
                                  Expanded(
                                    child: Text(
                                      d.name,
                                      style: Theme.of(context)
                                          .textTheme
                                          .titleMedium,
                                    ),
                                  ),
                                  if (d.default_)
                                    Chip(
                                      label: Text(s.defaultBadge),
                                      visualDensity: VisualDensity.compact,
                                    ),
                                ],
                              ),
                              Text(
                                '${d.type}${d.version != null && d.version!.isNotEmpty ? ' ${d.version}' : ''}',
                                style: Theme.of(context).textTheme.bodySmall,
                              ),
                              const SizedBox(height: 8),
                              if (d.reachable)
                                Wrap(
                                  spacing: 16,
                                  children: [
                                    Text('↑ ${formatSpeed(d.uploadSpeed)}'),
                                    Text('↓ ${formatSpeed(d.downloadSpeed)}'),
                                    Text(s.freeSpace(formatBytes(d.freeSpace))),
                                  ],
                                )
                              else
                                Text(
                                  d.error ?? s.downloaderUnreachable,
                                  style: TextStyle(
                                    color: Theme.of(context).colorScheme.error,
                                  ),
                                ),
                            ],
                          ),
                        ),
                      ),
                  ],
                ),
        ),
      ),
    );
  }
}
