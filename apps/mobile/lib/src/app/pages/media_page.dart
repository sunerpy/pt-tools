import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../icons.dart';
import '../providers.dart';
import '../theme.dart';
import '../widgets/common.dart';
import 'mobile_editor_page.dart';

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
          actions: [
            if (ref.watch(canWriteProvider))
              IconButton(
                icon: const PtIcon(PtGlyph.plus),
                tooltip: s.mobileAdd,
                onPressed: () async {
                  if (await openMobileEditor(context, ref, 'subscription')) {
                    ref.invalidate(subscriptionsProvider);
                  }
                },
              ),
          ],
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
            ? EmptyState(icon: PtGlyph.bookmark, message: s.noSubscriptions)
            : ListView.separated(
                padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
                itemCount: items.length,
                separatorBuilder: (_, _) => const SizedBox(height: 10),
                itemBuilder: (context, i) {
                  final sub = items[i];
                  final p = sub.progress;
                  return Card(
                    clipBehavior: Clip.antiAlias,
                    child: ListTile(
                      contentPadding: const EdgeInsets.fromLTRB(12, 8, 14, 8),
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
                              child: const PtIcon(PtGlyph.sparkles, size: 20),
                            )
                          : null,
                    ),
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
            ? EmptyState(icon: PtGlyph.library, message: s.noHistory)
            : ListView.separated(
                padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
                itemCount: p.items.length,
                separatorBuilder: (_, _) => const SizedBox(height: 10),
                itemBuilder: (context, i) {
                  final h = p.items[i];
                  final ep = h.season != null && h.season! > 0
                      ? ' S${h.season.toString().padLeft(2, '0')}${h.episode != null && h.episode! > 0 ? 'E${h.episode.toString().padLeft(2, '0')}' : ''}'
                      : '';
                  final (icon, color) = switch (h.status) {
                    'done' => (PtGlyph.checkCircle, context.pt.ok),
                    'failed' => (
                      PtGlyph.errorCircle,
                      Theme.of(context).colorScheme.error,
                    ),
                    _ => (
                      PtGlyph.minusCircle,
                      Theme.of(context).colorScheme.onSurfaceVariant,
                    ),
                  };
                  return Card(
                    child: ListTile(
                      contentPadding: const EdgeInsets.fromLTRB(14, 6, 14, 6),
                      leading: PtIcon(icon, size: 22, color: color),
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

  /// 上一次是不是显示着。底部换到别的页、上面盖了别的页面（详情、编辑页）时 TickerMode 关掉；
  /// 回来时重新取一次，别处刚删掉或刚建的订阅就不会还按旧的显示。
  /// App 回到前台时连接重连，探索本来就会重新取（exploreProvider 看着 onlineEpochProvider）。
  bool _shown = true;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final shown = TickerMode.valuesOf(context).enabled;
    if (shown && !_shown) {
      // 不在构建中改 provider：等这一帧画完
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) ref.invalidate(exploreProvider);
      });
    }
    _shown = shown;
  }

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  Future<void> _subscribe(AppExploreItem it) async {
    final s = S.of(context);
    final messenger = ScaffoldMessenger.of(context);
    try {
      final meta = await ref.read(metaProvider.future);
      if (!mounted) return;
      if (meta.features.contains('mobile_editors')) {
        final saved = await openMobileEditor(
          context,
          ref,
          'subscription',
          values: {'tmdb_id': '${it.id}', 'media_type': _kind},
        );
        if (!saved) return;
      } else {
        // Older hosts retain their existing quick subscription flow.
        await ref
            .read(apiProvider)!
            .createSubscription(
              AppSubscriptionCreate(
                mediaType: AppSubscriptionCreateMediaTypeEnum.fromJson(_kind)!,
                tmdbId: it.id,
              ),
            );
      }
      messenger.showSnackBar(SnackBar(content: Text(s.subscribeOk(it.title))));
      ref.invalidate(exploreProvider);
      ref.invalidate(subscriptionsProvider);
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
            leading: const PtIcon(PtGlyph.search, size: 20),
            elevation: const WidgetStatePropertyAll(0),
            onSubmitted: (v) =>
                setState(() => _q = v.trim().isEmpty ? null : v.trim()),
            trailing: [
              if (_q != null)
                IconButton(
                  icon: const PtIcon(PtGlyph.x, size: 20),
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
                showSelectedIcon: false,
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
          child: RefreshIndicator(
            onRefresh: () async {
              try {
                ref.invalidate(exploreProvider(query));
                await ref.read(exploreProvider(query).future);
              } on Object {
                // 页面显示错误
              }
            },
            child: AsyncBody<AppExplorePage>(
              value: page,
              onRetry: () => ref.invalidate(exploreProvider(query)),
              data: (p) => p.items.isEmpty
                  ? EmptyState(message: s.searchNoResults)
                  : _ExploreGrid(
                      items: p.items,
                      canWrite: canWrite,
                      onSubscribe: (it) => unawaited(_subscribe(it)),
                    ),
            ),
          ),
        ),
      ],
    );
  }
}

/// 探索的结果按行排：每张卡片最宽 170、间距 10（和以前的格子一样多列）。
/// 每一行按卡片的内容定高，字号放大或标题占两行时卡片跟着变高，不会溢出；
/// 同一行的卡片一样高，「订阅」与「已订阅」对齐在底部。
class _ExploreGrid extends StatelessWidget {
  const _ExploreGrid({
    required this.items,
    required this.canWrite,
    required this.onSubscribe,
  });
  final List<AppExploreItem> items;
  final bool canWrite;
  final void Function(AppExploreItem item) onSubscribe;

  static const _maxCard = 170.0;
  static const _gap = 10.0;
  static const _padding = EdgeInsets.fromLTRB(16, 12, 16, 24);

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, c) {
      // 列数照 SliverGridDelegateWithMaxCrossAxisExtent 的算法
      final columns = math.max(
        1,
        ((c.maxWidth - _padding.horizontal) / (_maxCard + _gap)).ceil(),
      );
      final rows = (items.length + columns - 1) ~/ columns;
      return ListView.separated(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: _padding,
        itemCount: rows,
        separatorBuilder: (_, _) => const SizedBox(height: _gap),
        itemBuilder: (context, r) => IntrinsicHeight(
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (var i = 0; i < columns; i++) ...[
                if (i > 0) const SizedBox(width: _gap),
                Expanded(
                  child: r * columns + i < items.length
                      ? _ExploreCard(
                          item: items[r * columns + i],
                          canWrite: canWrite,
                          onSubscribe: () =>
                              onSubscribe(items[r * columns + i]),
                        )
                      : const SizedBox.shrink(),
                ),
              ],
            ],
          ),
        ),
      );
    },
  );
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
        Poster(item.posterPath, width: double.infinity),
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
          Align(
            alignment: Alignment.centerLeft,
            child: StatusPill(text: s.inLibrary, color: context.pt.ok),
          )
        else if (item.subscribed)
          Align(
            alignment: Alignment.centerLeft,
            child: StatusPill(
              text: s.subscribed,
              color: t.colorScheme.onSurfaceVariant,
            ),
          )
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
