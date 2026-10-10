import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
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
      setState(() => _error = describeError(e));
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
                leading: const Icon(Icons.dns_outlined),
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
        SnackBar(content: Text(s.errorPrefix(describeError(e)))),
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
                  leading: const Icon(Icons.search),
                  elevation: const WidgetStatePropertyAll(0),
                  onSubmitted: (_) => _search(),
                  trailing: [
                    IconButton(
                      icon: const Icon(Icons.arrow_forward),
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
          ? EmptyState(icon: Icons.travel_explore, message: s.searchIntro)
          : r.items.isEmpty
          ? EmptyState(message: s.searchNoResults)
          : ListView.separated(
              padding: const EdgeInsets.only(bottom: 24),
              itemCount: r.items.length + (r.errors.isEmpty ? 0 : 1),
              separatorBuilder: (_, _) => const Divider(height: 1, indent: 64),
              itemBuilder: (context, i) {
                if (r.errors.isNotEmpty && i == 0) {
                  return ListTile(
                    leading: Icon(
                      Icons.warning_amber,
                      color: Theme.of(context).colorScheme.error,
                    ),
                    title: Text(s.searchErrors(r.errors.length)),
                    subtitle: Text(
                      r.errors.map((e) => '${e.site}：${e.error}').join('\n'),
                      maxLines: 3,
                      overflow: TextOverflow.ellipsis,
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
    return ListTile(
      leading: SiteIcon(item.site, size: 32),
      title: Text(item.title, maxLines: 2, overflow: TextOverflow.ellipsis),
      subtitle: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (item.subtitle != null && item.subtitle!.isNotEmpty)
            Text(item.subtitle!, maxLines: 1, overflow: TextOverflow.ellipsis),
          const SizedBox(height: 2),
          Wrap(
            spacing: 8,
            children: [
              Text(item.site, style: t.textTheme.labelSmall),
              Text(formatBytes(item.size)),
              Text('↑${item.seeders} ↓${item.leechers}'),
              if (item.free)
                Text(
                  s.free,
                  style: const TextStyle(
                    color: Color(0xFF10B981),
                    fontWeight: FontWeight.w600,
                  ),
                ),
              if (item.hasHr)
                Text(
                  s.hr,
                  style: TextStyle(
                    color: t.colorScheme.error,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              if (item.uploadedAt != null && item.uploadedAt! > 0)
                Text(
                  formatTime(item.uploadedAt!),
                  style: t.textTheme.bodySmall,
                ),
            ],
          ),
        ],
      ),
      trailing: canWrite
          ? IconButton(
              icon: const Icon(Icons.download_for_offline_outlined),
              tooltip: s.pushTitle,
              onPressed: onPush,
            )
          : null,
    );
  }
}
