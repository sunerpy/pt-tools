// 排序的入口与面板（种子页、搜索结果共用）：一个小块写着现在按什么排，点开在底部面板里换字段与方向。
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../l10n/app_localizations.dart';
import '../icons.dart';
import '../sorting.dart';

/// 字段的名字。
String sortFieldLabel(S s, String field) => switch (field) {
  searchSortDefault => s.sortDefaultOrder,
  'added_at' => s.sortAddedAt,
  'completed_at' => s.sortCompletedAt,
  'title' => s.sortName,
  'size' => s.sortSize,
  'progress' => s.sortProgress,
  'ratio' => s.sortRatio,
  'upload_speed' => s.sortUploadSpeed,
  'download_speed' => s.sortDownloadSpeed,
  'eta' => s.sortEta,
  'seeds' || 'seeders' => s.sortSeeds,
  'state' => s.sortState,
  'uploaded_at' => s.sortUploadedAt,
  'leechers' => s.sortLeechers,
  'snatched' => s.sortSnatched,
  _ => field,
};

/// 现在的排序：字段名与方向的箭头（默认顺序没有方向）；点开换。
class SortChip extends ConsumerWidget {
  const SortChip({super.key, required this.list, required this.provider});
  final SortList list;
  final NotifierProvider<SortNotifier, SortChoice> provider;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final choice = ref.watch(provider);
    final directed = choice.field != searchSortDefault;
    return ActionChip(
      key: ValueKey('sort-${list.name}'),
      avatar: const PtIcon(PtGlyph.sort, size: 16),
      tooltip: s.sortTitle,
      label: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(sortFieldLabel(s, choice.field)),
          if (directed) ...[
            const SizedBox(width: 2),
            PtIcon(
              choice.descending ? PtGlyph.arrowDown : PtGlyph.arrowUp,
              size: 14,
              semanticLabel: choice.descending
                  ? s.sortDescending
                  : s.sortAscending,
            ),
          ],
        ],
      ),
      // 开在最外层的导航里：盖住底部导航条与提示条
      onPressed: () => showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        useSafeArea: true,
        useRootNavigator: true,
        builder: (_) => SortSheet(list: list, provider: provider),
      ),
    );
  }
}

/// 选排序的面板：上面是方向（默认顺序没有方向），换了马上生效；下面是字段，点一个就按它排（方向不变）并关上面板。
class SortSheet extends ConsumerWidget {
  const SortSheet({super.key, required this.list, required this.provider});
  final SortList list;
  final NotifierProvider<SortNotifier, SortChoice> provider;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final t = Theme.of(context);
    final choice = ref.watch(provider);
    final notifier = ref.read(provider.notifier);
    // 按实际能用的高度留出上面一截遮罩，点它关上面板
    return LayoutBuilder(
      builder: (context, c) => ConstrainedBox(
        constraints: BoxConstraints(maxHeight: c.maxHeight * 0.88),
        child: ListView(
          shrinkWrap: true,
          padding: const EdgeInsets.only(bottom: 16),
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
              child: Text(s.sortTitle, style: t.textTheme.titleMedium),
            ),
            if (choice.field != searchSortDefault)
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                child: SegmentedButton<bool>(
                  key: const Key('sort-direction'),
                  showSelectedIcon: false,
                  segments: [
                    ButtonSegment(
                      value: true,
                      icon: const PtIcon(PtGlyph.arrowDown, size: 16),
                      label: Text(s.sortDescending),
                    ),
                    ButtonSegment(
                      value: false,
                      icon: const PtIcon(PtGlyph.arrowUp, size: 16),
                      label: Text(s.sortAscending),
                    ),
                  ],
                  selected: {choice.descending},
                  onSelectionChanged: (v) => notifier.set(
                    SortChoice(choice.field, descending: v.first),
                  ),
                ),
              ),
            for (final f in list.fields)
              ListTile(
                key: ValueKey('sort-field-$f'),
                minTileHeight: 48,
                title: Text(sortFieldLabel(s, f)),
                selected: f == choice.field,
                trailing: f == choice.field
                    ? const PtIcon(PtGlyph.check, size: 20)
                    : null,
                onTap: () {
                  notifier.set(SortChoice(f, descending: choice.descending));
                  Navigator.pop(context);
                },
              ),
          ],
        ),
      ),
    );
  }
}
