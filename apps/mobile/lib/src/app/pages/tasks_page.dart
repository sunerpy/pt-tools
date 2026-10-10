import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
import '../widgets/common.dart';

/// 任务：推送记录（RSS、手动、App、刷流、订阅……），新的在前。
class TasksPage extends ConsumerWidget {
  const TasksPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final page = ref.watch(tasksProvider);
    return Scaffold(
      appBar: AppBar(title: Text(s.moreTasks)),
      body: RefreshIndicator(
        onRefresh: () async {
          try {
            ref.invalidate(tasksProvider);
            await ref.read(tasksProvider.future);
          } on Object {
            // 页面显示错误
          }
        },
        child: AsyncBody<AppTaskPage>(
          value: page,
          onRetry: () => ref.invalidate(tasksProvider),
          data: (p) => p.items.isEmpty
              ? EmptyState(icon: Icons.rss_feed, message: s.noTasks)
              : ListView.separated(
                  itemCount: p.items.length,
                  separatorBuilder: (_, _) =>
                      const Divider(height: 1, indent: 64),
                  itemBuilder: (context, i) {
                    final t = p.items[i];
                    final status = t.completed
                        ? s.taskCompleted
                        : (t.pushed ? s.taskPushed : s.taskNotPushed);
                    return ListTile(
                      leading: SiteIcon(t.site, size: 32),
                      title: Text(
                        t.title,
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                      subtitle: Text(
                        [
                          status,
                          formatBytes(t.size),
                          if (t.free) s.free,
                          if (t.hasHr) s.hr,
                          if (t.pushed && !t.completed)
                            '${t.progress.toStringAsFixed(0)}%',
                          formatTime(t.createdAt),
                          if (t.error != null && t.error!.isNotEmpty) t.error!,
                        ].join(' · '),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                    );
                  },
                ),
        ),
      ),
    );
  }
}
