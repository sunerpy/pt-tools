import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
import '../widgets/common.dart';

/// 媒体：订阅、最近入库、探索（TMDB 的热门、流行与搜索，可以直接订阅）。
class MediaPage extends ConsumerWidget {
  const MediaPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    return DefaultTabController(
      length: 3,
      child: Scaffold(
        appBar: AppBar(
          title: Text(s.navMedia),
          bottom: TabBar(
            tabs: [
              Tab(text: s.tabSubscriptions),
              Tab(text: s.tabHistory),
              Tab(text: s.tabExplore),
            ],
          ),
        ),
        body: const TabBarView(
          children: [_Subscriptions(), _History(), _Explore()],
        ),
      ),
    );
  }
}

String subStatusLabel(S s, String st) => switch (st) {
  'active' => s.subActive,
  'paused' => s.subPaused,
  'pending' => s.subPending,
  'done' => s.subDone,
  _ => st,
};

class _Subscriptions extends ConsumerWidget {
  const _Subscriptions();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final list = ref.watch(subscriptionsProvider);
    return RefreshIndicator(
      onRefresh: () async {
        try {
          ref.invalidate(subscriptionsProvider);
          await ref.read(subscriptionsProvider.future);
        } on Object {
          // 页面显示错误
        }
      },
      child: AsyncBody<List<AppSubscription>>(
        value: list,
        onRetry: () => ref.invalidate(subscriptionsProvider),
        data: (items) => items.isEmpty
            ? EmptyState(
                icon: Icons.bookmark_border,
                message: s.noSubscriptions,
              )
            : ListView.separated(
                padding: const EdgeInsets.symmetric(vertical: 8),
                itemCount: items.length,
                separatorBuilder: (_, _) => const SizedBox(height: 4),
                itemBuilder: (context, i) {
                  final sub = items[i];
                  final p = sub.progress;
                  return ListTile(
                    onTap: () => context.go('/media/subscriptions/${sub.id}'),
                    leading: Poster(sub.posterPath, width: 44, size: 'w185'),
                    title: Text(
                      sub.mediaType == 'tv' && sub.season > 0
                          ? '${sub.title} S${sub.season.toString().padLeft(2, '0')}'
                          : sub.title,
                    ),
                    subtitle: Text(
                      [
                        subStatusLabel(s, sub.status),
                        if (sub.year != null && sub.year! > 0) '${sub.year}',
                        if (sub.mediaType == 'tv') ...[
                          s.subProgress(
                            p.inLibrary,
                            p.aired == 0 ? p.total : p.aired,
                          ),
                          if (p.missing.isNotEmpty)
                            s.subMissing(p.missing.take(6).join(', ')),
                        ] else
                          p.inLibrary > 0 ? s.inLibrary : s.movieNotInLibrary,
                      ].join(' · '),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                    trailing: sub.upgrade
                        ? Tooltip(
                            message: s.subUpgrade,
                            child: const Icon(Icons.high_quality_outlined),
                          )
                        : null,
                  );
                },
              ),
      ),
    );
  }
}

class _History extends ConsumerWidget {
  const _History();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final page = ref.watch(mediaHistoryProvider);
    return RefreshIndicator(
      onRefresh: () async {
        try {
          ref.invalidate(mediaHistoryProvider);
          await ref.read(mediaHistoryProvider.future);
        } on Object {
          // 页面显示错误
        }
      },
      child: AsyncBody<AppMediaHistoryPage>(
        value: page,
        onRetry: () => ref.invalidate(mediaHistoryProvider),
        data: (p) => p.items.isEmpty
            ? EmptyState(
                icon: Icons.video_library_outlined,
                message: s.noHistory,
              )
            : ListView.separated(
                itemCount: p.items.length,
                separatorBuilder: (_, _) =>
                    const Divider(height: 1, indent: 16),
                itemBuilder: (context, i) {
                  final h = p.items[i];
                  final ep = h.season != null && h.season! > 0
                      ? ' S${h.season.toString().padLeft(2, '0')}${h.episode != null && h.episode! > 0 ? 'E${h.episode.toString().padLeft(2, '0')}' : ''}'
                      : '';
                  final (icon, color) = switch (h.status) {
                    'done' => (
                      Icons.check_circle_outline,
                      const Color(0xFF10B981),
                    ),
                    'failed' => (
                      Icons.error_outline,
                      Theme.of(context).colorScheme.error,
                    ),
                    _ => (
                      Icons.remove_circle_outline,
                      Theme.of(context).colorScheme.outline,
                    ),
                  };
                  return ListTile(
                    leading: Icon(icon, color: color),
                    title: Text(
                      '${h.title}$ep',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    subtitle: Text(
                      [
                        h.library_,
                        formatBytes(h.size),
                        formatTime(h.createdAt),
                        if (h.message != null && h.message!.isNotEmpty)
                          h.message!,
                      ].whereType<String>().join(' · '),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  );
                },
              ),
      ),
    );
  }
}

class _Explore extends ConsumerStatefulWidget {
  const _Explore();

  @override
  ConsumerState<_Explore> createState() => _ExploreState();
}

class _ExploreState extends ConsumerState<_Explore> {
  String _kind = 'movie';
  String _list = 'trending';
  String? _q;
  final _search = TextEditingController();

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  Future<void> _subscribe(AppExploreItem it) async {
    final s = S.of(context);
    final messenger = ScaffoldMessenger.of(context);
    try {
      await ref
          .read(apiProvider)!
          .createSubscription(
            AppSubscriptionCreate(
              mediaType: AppSubscriptionCreateMediaTypeEnum.fromJson(_kind)!,
              tmdbId: it.id,
            ),
          );
      messenger.showSnackBar(SnackBar(content: Text(s.subscribeOk(it.title))));
      ref.invalidate(exploreProvider);
      ref.invalidate(subscriptionsProvider);
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
    final query = ExploreQuery(
      _kind,
      list: _q == null ? _list : 'search',
      q: _q,
    );
    final page = ref.watch(exploreProvider(query));
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
          child: SearchBar(
            controller: _search,
            hintText: s.exploreSearchHint,
            leading: const Icon(Icons.search),
            elevation: const WidgetStatePropertyAll(0),
            onSubmitted: (v) =>
                setState(() => _q = v.trim().isEmpty ? null : v.trim()),
            trailing: [
              if (_q != null)
                IconButton(
                  icon: const Icon(Icons.close),
                  onPressed: () => setState(() {
                    _q = null;
                    _search.clear();
                  }),
                ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12),
          child: Row(
            children: [
              SegmentedButton<String>(
                segments: [
                  ButtonSegment(value: 'movie', label: Text(s.exploreMovie)),
                  ButtonSegment(value: 'tv', label: Text(s.exploreTv)),
                ],
                selected: {_kind},
                onSelectionChanged: (v) => setState(() => _kind = v.first),
              ),
              const Spacer(),
              if (_q == null)
                DropdownButton<String>(
                  value: _list,
                  underline: const SizedBox.shrink(),
                  items: [
                    DropdownMenuItem(
                      value: 'trending',
                      child: Text(s.exploreTrending),
                    ),
                    DropdownMenuItem(
                      value: 'popular',
                      child: Text(s.explorePopular),
                    ),
                  ],
                  onChanged: (v) => setState(() => _list = v ?? 'trending'),
                ),
            ],
          ),
        ),
        Expanded(
          child: AsyncBody<AppExplorePage>(
            value: page,
            onRetry: () => ref.invalidate(exploreProvider(query)),
            data: (p) => p.items.isEmpty
                ? EmptyState(message: s.searchNoResults)
                : GridView.builder(
                    padding: const EdgeInsets.all(12),
                    gridDelegate:
                        const SliverGridDelegateWithMaxCrossAxisExtent(
                          maxCrossAxisExtent: 170,
                          childAspectRatio: 0.52,
                          crossAxisSpacing: 10,
                          mainAxisSpacing: 10,
                        ),
                    itemCount: p.items.length,
                    itemBuilder: (context, i) => _ExploreCard(
                      item: p.items[i],
                      canWrite: canWrite,
                      onSubscribe: () => unawaited(_subscribe(p.items[i])),
                    ),
                  ),
          ),
        ),
      ],
    );
  }
}

class _ExploreCard extends StatelessWidget {
  const _ExploreCard({
    required this.item,
    required this.canWrite,
    required this.onSubscribe,
  });
  final AppExploreItem item;
  final bool canWrite;
  final VoidCallback onSubscribe;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final t = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        LayoutBuilder(
          builder: (context, c) => Poster(item.posterPath, width: c.maxWidth),
        ),
        const SizedBox(height: 6),
        Text(
          item.title,
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
          style: t.textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w600),
        ),
        Text(
          [
            if (item.year != null && item.year! > 0) '${item.year}',
            if (item.voteAverage != null && item.voteAverage! > 0)
              '★ ${item.voteAverage!.toStringAsFixed(1)}',
          ].join(' · '),
          style: t.textTheme.bodySmall,
        ),
        const Spacer(),
        if (item.inLibrary)
          Chip(label: Text(s.inLibrary), visualDensity: VisualDensity.compact)
        else if (item.subscribed)
          Chip(label: Text(s.subscribed), visualDensity: VisualDensity.compact)
        else if (canWrite)
          FilledButton.tonal(
            onPressed: onSubscribe,
            style: FilledButton.styleFrom(visualDensity: VisualDensity.compact),
            child: Text(s.subscribe),
          ),
      ],
    );
  }
}
