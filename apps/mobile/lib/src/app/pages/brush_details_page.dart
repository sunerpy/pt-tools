import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../hybrid/mobile_api.dart';
import '../icons.dart';
import '../providers.dart';
import '../theme.dart';
import '../widgets/common.dart';
import 'mobile_editor_page.dart';

class BrushDetailsPage extends ConsumerStatefulWidget {
  const BrushDetailsPage({super.key, required this.id, required this.name});
  final int id;
  final String name;
  @override
  ConsumerState<BrushDetailsPage> createState() => _BrushDetailsPageState();
}

class _BrushDetailsPageState extends ConsumerState<BrushDetailsPage> {
  Map<String, dynamic>? _config;
  List<dynamic> _runs = [];
  String? _error;
  bool _loading = true;
  bool _busy = false;
  bool _more = false;
  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load({bool more = false}) async {
    setState(() => _loading = true);
    try {
      final config = await mobileApi(ref, '/brush/tasks/${widget.id}');
      final page = await mobileApi(
        ref,
        '/brush/tasks/${widget.id}/runs?limit=30&offset=${more ? _runs.length : 0}',
      ) as Map<String, dynamic>;
      if (!mounted) return;
      setState(() {
        _config = config as Map<String, dynamic>;
        _runs = [if (more) ..._runs, ...page['items'] as List];
        _more = _runs.length < (page['total'] as num);
        _error = null;
      });
    } on Object catch (e) {
      if (mounted) setState(() => _error = describeError(e, S.of(context)));
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _change(String action) async {
    if (_busy || !ref.read(canWriteProvider)) return;
    final s = S.of(context);
    if (action == 'delete') {
      final confirm = await showDialog<bool>(
        context: context,
        builder: (c) => AlertDialog(
          title: Text(s.mobileDeleteBrush),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(c, false),
              child: Text(s.cancel),
            ),
            FilledButton(
              style: FilledButton.styleFrom(
                backgroundColor: Theme.of(c).colorScheme.error,
                foregroundColor: Theme.of(c).colorScheme.onError,
              ),
              onPressed: () => Navigator.pop(c, true),
              child: Text(s.actionDelete),
            ),
          ],
        ),
      );
      if (confirm != true || !mounted) return;
    }
    setState(() => _busy = true);
    try {
      if (action == 'edit') {
        if (await openMobileEditor(context, ref, 'brush', id: widget.id)) {
          await _load();
        }
      } else if (action == 'delete') {
        await mobileApi(ref, '/brush/tasks/${widget.id}', verb: 'DELETE');
        ref.invalidate(brushTasksProvider);
        if (mounted) Navigator.pop(context);
      } else {
        await mobileApi(
          ref,
          '/brush/tasks/${widget.id}/status',
          verb: 'POST',
          body: {'enabled': _config?['enabled'] != true},
        );
        await _load();
      }
      ref.invalidate(brushTasksProvider);
    } on Object catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(describeError(e, s))));
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final writable = ref.watch(canWriteProvider);
    return Scaffold(
      appBar: AppBar(
        title: Text(widget.name),
        actions: [
          if (writable && _config != null)
            PopupMenuButton<String>(
              enabled: !_busy,
              icon: const PtIcon(PtGlyph.moreVertical),
              onSelected: _change,
              itemBuilder: (_) => [
                PopupMenuItem(value: 'edit', child: Text(s.mobileEditor)),
                PopupMenuItem(
                  value: 'toggle',
                  child: Text(
                    _config?['enabled'] == true
                        ? s.mobileDisable
                        : s.mobileEnable,
                  ),
                ),
                PopupMenuItem(
                  value: 'delete',
                  child: Text(
                    s.actionDelete,
                    style: TextStyle(
                      color: Theme.of(context).colorScheme.error,
                    ),
                  ),
                ),
              ],
            ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _load,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            if (_busy || _loading) const LinearProgressIndicator(),
            if (_error != null) ...[
              Text(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
              TextButton(onPressed: () => _load(), child: Text(s.retry)),
            ],
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 4),
              child: Text(
                s.mobileRuns,
                style: Theme.of(context).textTheme.labelLarge?.copyWith(
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  color: Theme.of(context).colorScheme.onSurfaceVariant,
                ),
              ),
            ),
            if (!_loading && _runs.isEmpty && _error == null)
              Padding(padding: const EdgeInsets.all(24), child: Text(s.empty)),
            for (final item in _runs)
              Builder(
                builder: (context) {
                  final r = item as Map;
                  final (label, color) = _runStatus(
                    context,
                    '${r['status'] ?? ''}',
                  );
                  final error = '${r['error'] ?? ''}';
                  return Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: Card(
                      clipBehavior: Clip.antiAlias,
                      child: ExpansionTile(
                        title: Text('${r['summary'] ?? ''}'),
                        subtitle: Padding(
                          padding: const EdgeInsets.only(top: 6),
                          child: Wrap(
                            spacing: 8,
                            runSpacing: 4,
                            crossAxisAlignment: WrapCrossAlignment.center,
                            children: [
                              StatusPill(text: label, color: color),
                              Text(_startedAt(r['started_at'])),
                              if (error.isNotEmpty)
                                Text(
                                  error,
                                  style: TextStyle(
                                    color: Theme.of(context).colorScheme.error,
                                  ),
                                ),
                            ],
                          ),
                        ),
                        children: [
                          for (final step in (r['steps'] as List? ?? []))
                            ListTile(
                              dense: true,
                              title: Text(
                                '${step['text'] ?? step['message'] ?? step['step'] ?? step['name'] ?? ''}',
                              ),
                              subtitle: Text(
                                _stepKind(
                                  s,
                                  '${step['kind'] ?? step['error'] ?? step['status'] ?? ''}',
                                ),
                              ),
                            ),
                        ],
                      ),
                    ),
                  );
                },
              ),
            if (_more)
              TextButton(
                onPressed: _loading ? null : () => _load(more: true),
                child: Text(s.navMore),
              ),
          ],
        ),
      ),
    );
  }
}

/// 一轮运行的结果：名字与颜色（与网页的刷流运行记录一致）。
(String, Color) _runStatus(BuildContext context, String status) {
  final s = S.of(context);
  final pt = context.pt;
  return switch (status) {
    'running' => (s.brushRunRunning, Theme.of(context).colorScheme.primary),
    'ok' => (s.brushRunOk, pt.ok),
    'partial' => (s.brushRunPartial, pt.warn),
    'failed' => (s.brushRunFailed, pt.danger),
    'interrupted' => (s.brushRunInterrupted, pt.warn),
    _ => (status, Theme.of(context).colorScheme.onSurfaceVariant),
  };
}

/// 一步的种类（list、add……）的名字；不认识的原样显示。
String _stepKind(S s, String kind) => switch (kind) {
  'list' => s.brushStepList,
  'reject' => s.brushStepReject,
  'add' => s.brushStepAdd,
  'remove' => s.brushStepRemove,
  'gone' => s.brushStepGone,
  'skip' => s.brushStepSkip,
  'stop' => s.brushStepStop,
  'error' => s.brushStepError,
  'info' => s.brushStepInfo,
  _ => kind,
};

/// 开始时间（接口给的是 RFC 3339）按本地时间显示；读不出来时原样显示。
String _startedAt(Object? v) {
  final t = DateTime.tryParse('${v ?? ''}');
  if (t == null) return '${v ?? ''}';
  return formatTime(t.millisecondsSinceEpoch ~/ 1000);
}
