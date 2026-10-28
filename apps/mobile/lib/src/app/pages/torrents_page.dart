import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../icons.dart';
import '../providers.dart';
import '../theme.dart';
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
        SnackBar(content: Text(s.errorPrefix(describeError(e, s)))),
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
                  leading: const PtIcon(PtGlyph.filter, size: 20),
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
              return EmptyState(icon: PtGlyph.torrents, message: s.noTorrents);
            }
            return ListView.separated(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
              itemCount: p.items.length + (p.failures.isEmpty ? 0 : 1),
              separatorBuilder: (_, _) => const SizedBox(height: 10),
              itemBuilder: (context, i) {
                if (p.failures.isNotEmpty && i == 0) {
                  return Card(
                    child: ListTile(
                      leading: PtIcon(
                        PtGlyph.alert,
                        size: 20,
                        color: Theme.of(context).colorScheme.error,
                      ),
                      title: Text(s.torrentFailures(p.failures.length)),
                      subtitle: Text(
                        p.failures
                            .map((f) => '${f.downloader}：${f.error}')
                            .join('\n'),
                      ),
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
      'downloading' => context.pt.info,
      'seeding' => context.pt.ok,
      'error' => theme.colorScheme.error,
      _ => theme.colorScheme.onSurfaceVariant,
    };
    final muted = theme.textTheme.bodySmall?.copyWith(
      color: theme.colorScheme.onSurfaceVariant,
      fontSize: 12.5,
    );
    return RowCard(
      padding: EdgeInsets.fromLTRB(14, 12, canWrite ? 2 : 14, 12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  t.title,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: theme.textTheme.bodyLarge?.copyWith(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                  ),
                ),
                const SizedBox(height: 10),
                LinearProgressIndicator(
                  value: (t.progress / 100).clamp(0, 1),
                  color: color,
                  minHeight: 4,
                  borderRadius: BorderRadius.circular(2),
                ),
                const SizedBox(height: 10),
                Wrap(
                  spacing: 10,
                  runSpacing: 4,
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: [
                    StatusPill(text: stateLabel, color: color),
                    Text(
                      '${t.progress.toStringAsFixed(t.progress >= 100 ? 0 : 1)}% · ${formatBytes(t.size)}',
                      style: muted,
                    ),
                    if (t.uploadSpeed > 0)
                      Text('↑${formatSpeed(t.uploadSpeed)}', style: muted),
                    if (t.downloadSpeed > 0)
                      Text('↓${formatSpeed(t.downloadSpeed)}', style: muted),
                    if (t.state == 'downloading')
                      Text(s.etaLabel(formatEta(t.eta)), style: muted),
                    Text('${s.kpiRatio} ${formatRatio(t.ratio)}', style: muted),
                    Text(t.downloader, style: muted),
                  ],
                ),
              ],
            ),
          ),
          if (canWrite)
            PopupMenuButton<String>(
              tooltip: s.actions,
              icon: const PtIcon(PtGlyph.moreVertical, size: 20),
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
            ),
        ],
      ),
    );
  }
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
            foregroundColor: Theme.of(context).colorScheme.onError,
          ),
          onPressed: () => Navigator.pop(context, _withFiles),
          child: Text(s.actionDelete),
        ),
      ],
    );
  }
}
