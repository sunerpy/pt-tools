// AI 助手（路线图 M19）：和 pt-tools 内置的 AI 助手对话。助手在主机上运行，这里只发消息、长轮询事件、确认改动类操作。
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../providers.dart';
import '../widgets/common.dart';

/// 对话页：打开最近的会话（没有就从空白开始）；进度经 GET /assistant/conversations/{id}/events 长轮询。
/// 改动类操作出现确认卡片，设备有 app:write 才有「确认执行 / 拒绝」。
class AssistantPage extends ConsumerStatefulWidget {
  const AssistantPage({super.key});

  @override
  ConsumerState<AssistantPage> createState() => _AssistantPageState();
}

class _AssistantPageState extends ConsumerState<AssistantPage> {
  final _input = TextEditingController();
  final _scroll = ScrollController();

  int? _conv;
  String _title = '';
  List<AppAssistantMessage> _messages = [];
  List<AppAssistantConfirmation> _confirmations = [];
  bool _running = false;
  String _runId = '';
  String _partial = '';
  String? _tool;
  String? _error;
  int _lastSeq = 0;
  bool _loading = true;
  bool _sending = false;
  int _deciding = 0;

  /// 每打开一个会话加一：旧会话的长轮询看到对不上就退出。
  int _generation = 0;
  Completer<void>? _abort;

  /// 长轮询两次之间的等待（页面关掉时取消，不留定时器）。
  Timer? _sleepTimer;
  Completer<void>? _sleepDone;

  Future<void> _sleep(Duration d) {
    final done = _sleepDone = Completer<void>();
    _sleepTimer = Timer(d, () {
      if (!done.isCompleted) done.complete();
    });
    return done.future;
  }

  void _wake() {
    _sleepTimer?.cancel();
    final done = _sleepDone;
    if (done != null && !done.isCompleted) done.complete();
  }

  AppApi get _api => ref.read(apiProvider)!;

  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  @override
  void dispose() {
    _generation++;
    final abort = _abort;
    if (abort != null && !abort.isCompleted) abort.complete();
    _wake();
    _input.dispose();
    _scroll.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final list = await _api.listAssistantConversations() ?? [];
      if (!mounted) return;
      if (list.isEmpty) {
        setState(() => _loading = false);
        return;
      }
      await _open(list.first.id);
    } on Object catch (e) {
      if (!mounted) return;
      setState(() {
        _loading = false;
        _error = describeError(e, S.of(context));
      });
    }
  }

  void _stopPolling() {
    _generation++;
    final abort = _abort;
    if (abort != null && !abort.isCompleted) abort.complete();
    _abort = null;
    _wake();
  }

  Future<void> _open(int id) async {
    _stopPolling();
    final gen = _generation;
    final AppAssistantConversationDetail? got;
    try {
      got = await _api.getAssistantConversation(id);
    } on Object catch (e) {
      if (!mounted || gen != _generation) return;
      if (e is ApiException && e.code == 404) {
        // 会话已经删了（网页上删的，或者过了保留期）：回到空白
        _newChat();
        setState(() => _loading = false);
        return;
      }
      // 隧道断了一下、主机在重启：显示错误，过一会儿再读，长轮询跟着接上
      setState(() {
        _loading = false;
        _error = describeError(e, S.of(context));
      });
      await _sleep(const Duration(seconds: 3));
      if (mounted && gen == _generation) unawaited(_open(id));
      return;
    }
    if (!mounted || gen != _generation || got == null) return;
    final d = got;
    setState(() {
      _conv = d.id;
      _title = d.title;
      _messages = [...d.messages];
      _confirmations = [...d.confirmations];
      _running = d.running;
      _runId = d.runId ?? '';
      _partial = d.running ? (d.partial ?? '') : '';
      _tool = null;
      _error = null;
      _lastSeq = d.lastSeq;
      _loading = false;
    });
    _scrollToEnd();
    unawaited(_poll(id, gen));
  }

  /// 长轮询：主机最多等 25 秒；回得太快（没有事件）时自己隔一会儿再问，免得空转。
  Future<void> _poll(int id, int gen) async {
    while (mounted && gen == _generation) {
      final abort = _abort = Completer<void>();
      final started = DateTime.now();
      try {
        final page = await _api.getAssistantEvents(
          id,
          after: _lastSeq,
          wait: 25,
          abortTrigger: abort.future,
        );
        if (!mounted || gen != _generation || page == null) return;
        if (page.reset) {
          await _open(id);
          return;
        }
        setState(() {
          for (final ev in page.events) {
            _apply(ev);
          }
          _lastSeq = page.last > _lastSeq ? page.last : _lastSeq;
          if (!page.running && page.events.isEmpty) _running = false;
        });
        if (page.events.isNotEmpty) _scrollToEnd();
        if (page.events.isEmpty &&
            DateTime.now().difference(started) < const Duration(seconds: 2)) {
          await _sleep(const Duration(seconds: 2));
        }
      } on Object {
        if (!mounted || gen != _generation) return;
        await _sleep(const Duration(seconds: 3));
      }
    }
  }

  void _apply(AppAssistantEvent ev) {
    if (ev.seq <= _lastSeq) return;
    _lastSeq = ev.seq;
    switch (ev.type) {
      case AppAssistantEventTypeEnum.runStarted:
        _running = true;
        _runId = ev.runId ?? '';
        _partial = '';
        _tool = null;
        _error = null;
      case AppAssistantEventTypeEnum.delta:
        _partial = (ev.replace ?? false)
            ? (ev.text ?? '')
            : _partial + (ev.text ?? '');
      case AppAssistantEventTypeEnum.message:
        final m = ev.message;
        if (m == null) break;
        _upsertMessage(m);
        if (m.role != AppAssistantMessageRoleEnum.user &&
            m.role != AppAssistantMessageRoleEnum.tool) {
          _partial = '';
        }
        if (m.role == AppAssistantMessageRoleEnum.tool) _tool = null;
      case AppAssistantEventTypeEnum.toolStarted:
        _tool = ev.tool?.name;
      case AppAssistantEventTypeEnum.confirmation:
        final c = ev.confirmation;
        if (c != null) _upsertConfirmation(c);
      case AppAssistantEventTypeEnum.runFinished:
        _running = false;
        _partial = '';
        _tool = null;
        _error = ev.status == AppAssistantEventStatusEnum.failed
            ? ev.error
            : null;
    }
  }

  void _upsertMessage(AppAssistantMessage m) {
    _messages = [..._messages.where((x) => x.id != m.id), m]
      ..sort((a, b) => a.id.compareTo(b.id));
  }

  void _upsertConfirmation(AppAssistantConfirmation c) {
    _confirmations = [..._confirmations.where((x) => x.id != c.id), c]
      ..sort((a, b) => a.id.compareTo(b.id));
  }

  void _scrollToEnd() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scroll.hasClients) {
        _scroll.jumpTo(_scroll.position.maxScrollExtent);
      }
    });
  }

  void _newChat() {
    _stopPolling();
    setState(() {
      _conv = null;
      _title = '';
      _messages = [];
      _confirmations = [];
      _running = false;
      _runId = '';
      _partial = '';
      _tool = null;
      _error = null;
      _lastSeq = 0;
    });
  }

  void _snack(String text) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(text)));
  }

  Future<void> _send() async {
    final text = _input.text.trim();
    if (text.isEmpty || _sending || _running) return;
    final s = S.of(context);
    setState(() => _sending = true);
    try {
      final conv = _conv;
      if (conv == null) {
        final res = await _api.createAssistantConversation(
          AppAssistantCreate(message: text),
        );
        _input.clear();
        if (res != null && mounted) await _open(res.conversation.id);
      } else {
        final res = await _api.sendAssistantMessage(
          conv,
          AppAssistantSend(text: text),
        );
        _input.clear();
        // 回应到的时候已经不在这个会话上了（例如会话被删、回到了空白）：不再写进页面
        if (res != null && mounted && _conv == conv) {
          setState(() {
            _upsertMessage(res.message);
            // 事件可能先到、这次运行都结束了：见过这次运行就不再改状态
            if (_runId != res.runId) {
              _running = true;
              _runId = res.runId;
            }
          });
          _scrollToEnd();
        }
      }
    } on Object catch (e) {
      if (mounted) _snack(s.errorPrefix(describeError(e, s)));
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  Future<void> _cancel() async {
    final conv = _conv;
    if (conv == null) return;
    final s = S.of(context);
    try {
      await _api.cancelAssistantRun(conv);
    } on Object catch (e) {
      if (mounted) _snack(s.errorPrefix(describeError(e, s)));
    }
  }

  Future<void> _decide(AppAssistantConfirmation c, bool approve) async {
    final s = S.of(context);
    setState(() => _deciding = c.id);
    try {
      final got = await _api.decideAssistantConfirmation(
        c.id,
        AppAssistantDecide(approve: approve),
      );
      if (got != null && mounted) setState(() => _upsertConfirmation(got));
    } on ApiException catch (e) {
      if (!mounted) return;
      _snack(e.code == 409 ? s.assistantDecided : describeError(e, s));
    } on Object catch (e) {
      if (mounted) _snack(s.errorPrefix(describeError(e, s)));
    } finally {
      if (mounted) setState(() => _deciding = 0);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final status = ref.watch(assistantStatusProvider);
    final st = status.value;
    if (status.hasValue && (st == null || !st.enabled)) {
      return Scaffold(
        appBar: AppBar(title: Text(s.assistantTitle)),
        body: EmptyState(
          message: s.assistantDisabled,
          icon: Icons.smart_toy_outlined,
        ),
      );
    }
    final canConfirm = st?.canConfirm ?? false;
    return Scaffold(
      appBar: AppBar(
        title: Text(_title.isEmpty ? s.assistantTitle : _title),
        actions: [
          IconButton(
            tooltip: s.assistantNew,
            icon: const Icon(Icons.add_comment_outlined),
            // 发送中也不能换：回应回来时要落在发出它的会话上
            onPressed: _running || _sending ? null : _newChat,
          ),
        ],
      ),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: _loading
                  ? const Center(child: CircularProgressIndicator())
                  : _buildMessages(context, canConfirm),
            ),
            if (!canConfirm)
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                child: Text(
                  s.assistantReadOnly,
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ),
            _buildComposer(context),
          ],
        ),
      ),
    );
  }

  Widget _buildMessages(BuildContext context, bool canConfirm) {
    final s = S.of(context);
    final theme = Theme.of(context);
    final rows = <Widget>[];
    for (final m in _messages) {
      switch (m.role) {
        case AppAssistantMessageRoleEnum.user:
          rows.add(_bubble(context, m.content, mine: true));
        case AppAssistantMessageRoleEnum.notice:
          rows.add(
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Text(
                m.content,
                textAlign: TextAlign.center,
                style: theme.textTheme.bodySmall?.copyWith(
                  color: (m.isError ?? false)
                      ? theme.colorScheme.error
                      : theme.colorScheme.outline,
                ),
              ),
            ),
          );
        case AppAssistantMessageRoleEnum.assistant:
          if (m.content.trim().isNotEmpty) {
            rows.add(_bubble(context, plainAnswer(m.content), mine: false));
          }
          for (final call in m.toolCalls) {
            final c = _confirmations
                .where((x) => x.runId == m.runId && x.toolCallId == call.id)
                .firstOrNull;
            rows.add(_callRow(context, call, c, canConfirm));
          }
        default:
          // 工具结果不单独显示
          break;
      }
    }
    if (_running) {
      if (_partial.isNotEmpty) {
        rows.add(_bubble(context, plainAnswer(_partial), mine: false));
      }
      final pending = _confirmations.any(
        (c) =>
            c.status == AppAssistantConfirmationStatusEnum.pending &&
            c.runId == _runId,
      );
      rows.add(
        Padding(
          padding: const EdgeInsets.symmetric(vertical: 8),
          child: Row(
            children: [
              const SizedBox(
                width: 14,
                height: 14,
                child: CircularProgressIndicator(strokeWidth: 2),
              ),
              const SizedBox(width: 8),
              Text(
                pending
                    ? s.assistantWaitConfirm
                    : (_tool != null
                          ? s.assistantTool(_tool!)
                          : s.assistantThinking),
                style: theme.textTheme.bodySmall,
              ),
            ],
          ),
        ),
      );
    }
    if (_error != null && _error!.isNotEmpty) {
      rows.add(Text(_error!, style: TextStyle(color: theme.colorScheme.error)));
    }
    if (rows.isEmpty) {
      return Padding(
        padding: const EdgeInsets.all(24),
        child: Text(s.assistantEmpty, style: theme.textTheme.bodyMedium),
      );
    }
    return ListView(
      key: const Key('assistant-messages'),
      controller: _scroll,
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
      children: rows,
    );
  }

  Widget _bubble(BuildContext context, String text, {required bool mine}) {
    final theme = Theme.of(context);
    return Align(
      alignment: mine ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.symmetric(vertical: 4),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        constraints: const BoxConstraints(maxWidth: 320),
        decoration: BoxDecoration(
          color: mine
              ? theme.colorScheme.primaryContainer
              : theme.colorScheme.surfaceContainerHighest,
          borderRadius: BorderRadius.circular(10),
        ),
        child: SelectableText(text),
      ),
    );
  }

  Widget _callRow(
    BuildContext context,
    AppAssistantToolCall call,
    AppAssistantConfirmation? c,
    bool canConfirm,
  ) {
    final s = S.of(context);
    final theme = Theme.of(context);
    final status = c?.status;
    final label = switch (status) {
      AppAssistantConfirmationStatusEnum.pending => s.assistantPending,
      AppAssistantConfirmationStatusEnum.approved => s.assistantApproved,
      AppAssistantConfirmationStatusEnum.denied => s.assistantDenied,
      AppAssistantConfirmationStatusEnum.expired => s.assistantExpired,
      _ => null,
    };
    return Card(
      key: c == null ? null : Key('assistant-confirm-${c.id}'),
      margin: const EdgeInsets.symmetric(vertical: 4),
      child: Padding(
        padding: const EdgeInsets.all(10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(Icons.build_outlined, size: 14),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    c?.summary ?? call.name,
                    style: theme.textTheme.bodyMedium,
                  ),
                ),
                if (label != null)
                  Text(label, style: theme.textTheme.labelSmall),
              ],
            ),
            if (status == AppAssistantConfirmationStatusEnum.pending &&
                canConfirm)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.end,
                  children: [
                    OutlinedButton(
                      onPressed: _deciding == c!.id
                          ? null
                          : () => _decide(c, false),
                      child: Text(s.assistantDeny),
                    ),
                    const SizedBox(width: 8),
                    FilledButton(
                      onPressed: _deciding == c.id
                          ? null
                          : () => _decide(c, true),
                      child: Text(s.assistantApprove),
                    ),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _buildComposer(BuildContext context) {
    final s = S.of(context);
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 6, 12, 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Expanded(
                child: TextField(
                  key: const Key('assistant-input'),
                  controller: _input,
                  minLines: 1,
                  maxLines: 4,
                  maxLength: 4000,
                  decoration: InputDecoration(
                    hintText: s.assistantHint,
                    counterText: '',
                    border: const OutlineInputBorder(),
                    isDense: true,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              _running
                  ? IconButton.outlined(
                      key: const Key('assistant-stop'),
                      tooltip: s.assistantStop,
                      icon: const Icon(Icons.stop),
                      onPressed: _cancel,
                    )
                  : IconButton.filled(
                      key: const Key('assistant-send'),
                      tooltip: s.assistantSend,
                      icon: const Icon(Icons.send),
                      onPressed: _sending ? null : _send,
                    ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            s.assistantPrivacy,
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ],
      ),
    );
  }
}

/// 回答里常见的 Markdown 记号去掉（App 按纯文本显示）。
String plainAnswer(String s) => s
    .replaceAll('**', '')
    .replaceAll('__', '')
    .split('\n')
    .map(
      (l) => l.trimLeft().startsWith('#')
          ? l.trimLeft().replaceFirst(RegExp(r'^#+\s*'), '')
          : l,
    )
    .join('\n');
