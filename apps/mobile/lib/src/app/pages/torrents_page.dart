import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
import '../widgets/common.dart';

/// 种子：下载器里的种子，按状态筛选、按标题搜；完全控制的设备可以暂停、继续、删除（删除要确认）。每 5 秒刷新一次。
class TorrentsPage extends ConsumerStatefulWidget {
  const TorrentsPage({super.key});

  @override
  ConsumerState<TorrentsPage> createState() => _TorrentsPageState();
}

class _TorrentsPageState extends ConsumerState<TorrentsPage> {
  String? _state;
  String? _q;
  final _search = TextEditingController();
  Timer? _debounce;

  static const _states = [
    'downloading',
    'seeding',
    'paused',
    'stopped',
    'queued',
    'checking',
    'error',
  ];

  @override
  void dispose() {
    _debounce?.cancel();
    _search.dispose();
    super.dispose();
  }

  String _stateLabel(S s, String st) => switch (st) {
    'downloading' => s.stateDownloading,
    'seeding' => s.stateSeeding,
    'paused' => s.statePaused,
    'stopped' => s.stateStopped,
    'queued' => s.stateQueued,
    'checking' => s.stateChecking,
    'error' => s.stateError,
    _ => st,
  };

  Future<void> _act(AppTorrent t, String action) async {
    final s = S.of(context);
    final messenger = ScaffoldMessenger.of(context);
    var a = action;
    if (action == 'delete') {
      final withFiles = await showDialog<bool>(
        context: context,
        builder: (_) => _DeleteDialog(title: t.title),
      );
      if (withFiles == null) return;
      a = withFiles ? 'delete_with_files' : 'delete';
    }
    try {
      final res = await ref
          .read(apiProvider)!
          .torrentActions(
            AppTorrentActionsRequest(
              action: AppTorrentActionsRequestActionEnum.fromJson(a)!,
              targets: [
                AppTorrentTarget(
                  downloaderId: t.downloaderId,
                  taskId: t.taskId,
                ),
              ],
            ),
          );
      final failed =
          res?.results
              .where((r) => !r.success)
              .map((r) => r.message)
              .whereType<String>()
              .join('；') ??
          '';
      messenger.showSnackBar(
        SnackBar(
          content: Text(
            failed.isEmpty
                ? s.actionDone(res?.succeeded ?? 0, res?.failed ?? 0)
                : failed,
          ),
        ),
      );
    } on Object catch (e) {
      messenger.showSnackBar(
        SnackBar(content: Text(s.errorPrefix(describeError(e)))),
      );
    }
    ref.invalidate(torrentsProvider);
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final query = TorrentQuery(state: _state, q: _q);
    // 定时刷新
    ref.listen(
      tickerProvider,
      (_, _) => ref.invalidate(torrentsProvider(query)),
    );
    final page = ref.watch(torrentsProvider(query));
    final canWrite = ref.watch(canWriteProvider);
    return Scaffold(
      appBar: AppBar(
        title: Text(s.navTorrents),
        bottom: PreferredSize(
          preferredSize: const Size.fromHeight(104),
          child: Column(
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                child: SearchBar(
                  controller: _search,
                  hintText: s.searchTorrentsHint,
                  leading: const Icon(Icons.filter_list),
                  elevation: const WidgetStatePropertyAll(0),
                  onChanged: (v) {
                    _debounce?.cancel();
                    _debounce = Timer(
                      const Duration(milliseconds: 400),
                      () => setState(
                        () => _q = v.trim().isEmpty ? null : v.trim(),
                      ),
                    );
                  },
                ),
              ),
              SizedBox(
                height: 44,
                child: ListView(
                  scrollDirection: Axis.horizontal,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  children: [
                    for (final st in [null, ..._states])
                      Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 4),
                        child: ChoiceChip(
                          label: Text(
                            st == null ? s.torrentsAll : _stateLabel(s, st),
                          ),
                          selected: _state == st,
                          onSelected: (_) => setState(() => _state = st),
                        ),
                      ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          try {
            ref.invalidate(torrentsProvider(query));
            await ref.read(torrentsProvider(query).future);
          } on Object {
            // 页面显示错误
          }
        },
        child: AsyncBody<AppTorrentPage>(
          value: page,
          onRetry: () => ref.invalidate(torrentsProvider(query)),
          data: (p) {
            if (p.items.isEmpty && p.failures.isEmpty) {
              return EmptyState(icon: Icons.swap_vert, message: s.noTorrents);
            }
            return ListView.separated(
              padding: const EdgeInsets.only(bottom: 24),
              itemCount: p.items.length + (p.failures.isEmpty ? 0 : 1),
              separatorBuilder: (_, _) => const Divider(height: 1, indent: 16),
              itemBuilder: (context, i) {
                if (p.failures.isNotEmpty && i == 0) {
                  return ListTile(
                    leading: Icon(
                      Icons.warning_amber,
                      color: Theme.of(context).colorScheme.error,
                    ),
                    title: Text(s.torrentFailures(p.failures.length)),
                    subtitle: Text(
                      p.failures
                          .map((f) => '${f.downloader}：${f.error}')
                          .join('\n'),
                    ),
                  );
                }
                final t = p.items[i - (p.failures.isEmpty ? 0 : 1)];
                return _TorrentTile(
                  t: t,
                  stateLabel: _stateLabel(s, t.state),
                  canWrite: canWrite,
                  onAction: (a) => _act(t, a),
                );
              },
            );
          },
        ),
      ),
    );
  }
}

class _TorrentTile extends StatelessWidget {
  const _TorrentTile({
    required this.t,
    required this.stateLabel,
    required this.canWrite,
    required this.onAction,
  });
  final AppTorrent t;
  final String stateLabel;
  final bool canWrite;
  final void Function(String action) onAction;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final theme = Theme.of(context);
    final paused = t.state == 'paused' || t.state == 'stopped';
    final color = switch (t.state) {
      'downloading' => const Color(0xFF3B82F6),
      'seeding' => const Color(0xFF10B981),
      'error' => theme.colorScheme.error,
      _ => theme.colorScheme.outline,
    };
    return ListTile(
      title: Text(t.title, maxLines: 2, overflow: TextOverflow.ellipsis),
      subtitle: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const SizedBox(height: 6),
          LinearProgressIndicator(
            value: (t.progress / 100).clamp(0, 1),
            color: color,
            minHeight: 4,
            borderRadius: BorderRadius.circular(2),
          ),
          const SizedBox(height: 6),
          Wrap(
            spacing: 10,
            runSpacing: 2,
            children: [
              _Pill(text: stateLabel, color: color),
              Text(
                '${t.progress.toStringAsFixed(t.progress >= 100 ? 0 : 1)}% · ${formatBytes(t.size)}',
              ),
              if (t.uploadSpeed > 0) Text('↑${formatSpeed(t.uploadSpeed)}'),
              if (t.downloadSpeed > 0) Text('↓${formatSpeed(t.downloadSpeed)}'),
              if (t.state == 'downloading') Text(s.etaLabel(formatEta(t.eta))),
              Text('${s.kpiRatio} ${formatRatio(t.ratio)}'),
              Text(t.downloader, style: theme.textTheme.bodySmall),
            ],
          ),
        ],
      ),
      trailing: canWrite
          ? PopupMenuButton<String>(
              tooltip: s.actions,
              onSelected: onAction,
              itemBuilder: (_) => [
                if (paused)
                  PopupMenuItem(value: 'resume', child: Text(s.actionResume))
                else
                  PopupMenuItem(value: 'pause', child: Text(s.actionPause)),
                PopupMenuItem(
                  value: 'delete',
                  child: Text(
                    s.actionDelete,
                    style: TextStyle(color: theme.colorScheme.error),
                  ),
                ),
              ],
            )
          : null,
    );
  }
}

class _Pill extends StatelessWidget {
  const _Pill({required this.text, required this.color});
  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 1),
    decoration: BoxDecoration(
      color: color.withValues(alpha: 0.12),
      borderRadius: BorderRadius.circular(4),
    ),
    child: Text(
      text,
      style: TextStyle(color: color, fontSize: 12, fontWeight: FontWeight.w600),
    ),
  );
}

/// 删除确认：返回 true 表示连同数据一起删。
class _DeleteDialog extends StatefulWidget {
  const _DeleteDialog({required this.title});
  final String title;

  @override
  State<_DeleteDialog> createState() => _DeleteDialogState();
}

class _DeleteDialogState extends State<_DeleteDialog> {
  bool _withFiles = false;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return AlertDialog(
      title: Text(s.deleteTitle),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(widget.title, maxLines: 3, overflow: TextOverflow.ellipsis),
          const SizedBox(height: 8),
          CheckboxListTile(
            contentPadding: EdgeInsets.zero,
            value: _withFiles,
            onChanged: (v) => setState(() => _withFiles = v ?? false),
            title: Text(s.deleteWithFiles),
            controlAffinity: ListTileControlAffinity.leading,
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.cancel),
        ),
        FilledButton(
          style: FilledButton.styleFrom(
            backgroundColor: Theme.of(context).colorScheme.error,
          ),
          onPressed: () => Navigator.pop(context, _withFiles),
          child: Text(s.actionDelete),
        ),
      ],
    );
  }
}
