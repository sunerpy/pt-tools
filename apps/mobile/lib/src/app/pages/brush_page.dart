import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
import '../widgets/common.dart';

/// 刷流任务：在刷的种子数、今天与累计的收益。
class BrushPage extends ConsumerWidget {
  const BrushPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final list = ref.watch(brushTasksProvider);
    return Scaffold(
      appBar: AppBar(title: Text(s.moreBrush)),
      body: RefreshIndicator(
        onRefresh: () async {
          try {
            ref.invalidate(brushTasksProvider);
            await ref.read(brushTasksProvider.future);
          } on Object {
            // 页面显示错误
          }
        },
        child: AsyncBody<List<AppBrushTask>>(
          value: list,
          onRetry: () => ref.invalidate(brushTasksProvider),
          data: (items) => items.isEmpty
              ? EmptyState(icon: Icons.autorenew, message: s.noBrush)
              : ListView(
                  padding: const EdgeInsets.all(12),
                  children: [
                    for (final b in items)
                      Card(
                        margin: const EdgeInsets.only(bottom: 8),
                        child: ListTile(
                          leading: SiteIcon(b.site, size: 32),
                          title: Text(b.name),
                          subtitle: Text(
                            [
                              if (!b.enabled) s.brushDisabled,
                              s.brushActive(b.active),
                              s.brushToday(
                                formatBytes(b.today.uploaded),
                                formatBytes(b.today.downloaded),
                              ),
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
                      ),
                  ],
                ),
        ),
      ),
    );
  }
}
