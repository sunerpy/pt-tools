import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../icons.dart';
import '../providers.dart';
import '../theme.dart';
import '../widgets/common.dart';

/// 搜索：多站搜索种子；完全控制的设备可以把结果推送到下载器（经磁盘空间与站点做种容量的检查）。
class SearchPage extends ConsumerStatefulWidget {
  const SearchPage({super.key});

  @override
  ConsumerState<SearchPage> createState() => _SearchPageState();
}

class _SearchPageState extends ConsumerState<SearchPage> {
  final _q = TextEditingController();
  bool _freeOnly = false;
  bool _busy = false;
  AppSearchResult? _result;
  String? _error;

  @override
  void dispose() {
    _q.dispose();
    super.dispose();
  }

  Future<void> _search() async {
    final kw = _q.text.trim();
    if (kw.isEmpty) return;
    final s = S.of(context);
    FocusScope.of(context).unfocus();
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final r = await ref
          .read(apiProvider)!
          .search(AppSearchRequest(keyword: kw, freeOnly: _freeOnly));
      setState(() => _result = r);
    } on Object catch (e) {
      setState(() => _error = describeError(e, s));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _push(AppSearchItem item) async {
    final s = S.of(context);
    final messenger = ScaffoldMessenger.of(context);
    final downloaders = await ref
        .read(downloadersProvider.future)
        .then((d) => d.items, onError: (_) => <AppDownloader>[]);
    if (!mounted) return;
    final target = await showModalBottomSheet<AppDownloader?>(
      context: context,
      showDragHandle: true,
      builder: (context) => SafeArea(
        child: ListView(
          shrinkWrap: true,
          children: [
            ListTile(
              title: Text(
                s.pushTitle,
                style: Theme.of(context).textTheme.titleMedium,
              ),
              subtitle: Text(
                item.title,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
            ),
            for (final d in downloaders)
              ListTile(
                leading: PtIcon(
                  d.reachable ? PtGlyph.server : PtGlyph.serverOff,
                  size: 20,
                ),
                title: Text(d.name),
                subtitle: Text(
                  [
                    if (d.default_) s.defaultBadge,
                    if (d.reachable)
                      s.freeSpace(formatBytes(d.freeSpace))
                    else
                      s.downloaderUnreachable,
                  ].join(' · '),
                ),
                onTap: () => Navigator.pop(context, d),
              ),
            if (downloaders.isEmpty)
              ListTile(
                title: Text(s.pushDefault),
                onTap: () => Navigator.pop(context),
              ),
          ],
        ),
      ),
    );
    if (!mounted) return;
    try {
      final r = await ref
          .read(apiProvider)!
          .push(
            AppPushRequest(
              site: item.site,
              torrentId: item.torrentId,
              downloaderId: target?.id,
              title: item.title,
            ),
          );
      final msg = r == null
          ? s.errorPrefix('-')
          : r.skipped
          ? s.pushSkipped
          : r.success
          ? s.pushOk(r.downloader)
          : s.pushBlocked(r.message ?? '');
      messenger.showSnackBar(SnackBar(content: Text(msg)));
    } on Object catch (e) {
      messenger.showSnackBar(
        SnackBar(content: Text(s.errorPrefix(describeError(e, s)))),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final canWrite = ref.watch(canWriteProvider);
    final r = _result;
    return Scaffold(
      appBar: AppBar(
        title: Text(s.navSearch),
        bottom: PreferredSize(
          preferredSize: const Size.fromHeight(108),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
            child: Column(
              children: [
                SearchBar(
                  key: const Key('search-input'),
                  controller: _q,
                  hintText: s.searchHint,
                  leading: const PtIcon(PtGlyph.search, size: 20),
                  elevation: const WidgetStatePropertyAll(0),
                  onSubmitted: (_) => _search(),
                  trailing: [
                    IconButton(
                      icon: const PtIcon(PtGlyph.arrowRight, size: 20),
                      tooltip: s.navSearch,
                      onPressed: _busy ? null : _search,
                    ),
                  ],
                ),
                Row(
                  children: [
                    FilterChip(
                      label: Text(s.searchFreeOnly),
                      selected: _freeOnly,
                      onSelected: (v) => setState(() => _freeOnly = v),
                    ),
                    const Spacer(),
                    if (r != null)
                      Text(
                        s.searchSites(r.sites.length, r.durationMs),
                        style: Theme.of(context).textTheme.bodySmall,
                      ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
      body: _busy
          ? const Center(child: CircularProgressIndicator())
          : _error != null
          ? ErrorState(message: _error!, onRetry: _search)
          : r == null
          ? EmptyState(icon: PtGlyph.search, message: s.searchIntro)
          : r.items.isEmpty
          ? EmptyState(message: s.searchNoResults)
          : ListView.separated(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
              itemCount: r.items.length + (r.errors.isEmpty ? 0 : 1),
              separatorBuilder: (_, _) => const SizedBox(height: 10),
              itemBuilder: (context, i) {
                if (r.errors.isNotEmpty && i == 0) {
                  return Card(
                    child: ListTile(
                      leading: PtIcon(
                        PtGlyph.alert,
                        size: 20,
                        color: Theme.of(context).colorScheme.error,
                      ),
                      title: Text(s.searchErrors(r.errors.length)),
                      subtitle: Text(
                        r.errors.map((e) => '${e.site}：${e.error}').join('\n'),
                        maxLines: 3,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                  );
                }
                final it = r.items[i - (r.errors.isEmpty ? 0 : 1)];
                return _ResultTile(
                  item: it,
                  canWrite: canWrite,
                  onPush: () => _push(it),
                );
              },
            ),
    );
  }
}

class _ResultTile extends StatelessWidget {
  const _ResultTile({
    required this.item,
    required this.canWrite,
    required this.onPush,
  });
  final AppSearchItem item;
  final bool canWrite;
  final VoidCallback onPush;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final t = Theme.of(context);
    final muted = t.textTheme.bodySmall?.copyWith(
      color: t.colorScheme.onSurfaceVariant,
      fontSize: 12.5,
    );
    return RowCard(
      padding: EdgeInsets.fromLTRB(14, 12, canWrite ? 4 : 14, 12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SiteIcon(item.site, size: 32),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  item.title,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: t.textTheme.bodyLarge?.copyWith(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                  ),
                ),
                if (item.subtitle != null && item.subtitle!.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 2),
                    child: Text(
                      item.subtitle!,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: muted,
                    ),
                  ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 8,
                  runSpacing: 4,
                  crossAxisAlignment: WrapCrossAlignment.center,
                  children: [
                    if (item.free)
                      StatusPill(text: s.free, color: context.pt.ok),
                    if (item.hasHr)
                      StatusPill(text: s.hr, color: t.colorScheme.error),
                    Text(item.site, style: muted),
                    Text(formatBytes(item.size), style: muted),
                    Text('↑${item.seeders} ↓${item.leechers}', style: muted),
                    if (item.uploadedAt != null && item.uploadedAt! > 0)
                      Text(formatTime(item.uploadedAt!), style: muted),
                  ],
                ),
              ],
            ),
          ),
          if (canWrite)
            IconButton(
              icon: const PtIcon(PtGlyph.download, size: 22),
              tooltip: s.pushTitle,
              onPressed: onPush,
            ),
        ],
      ),
    );
  }
}
