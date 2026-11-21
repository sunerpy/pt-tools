// 推送的面板：选下载器，再选保存目录、分类与标签。目录是网页「下载器设置」里给这台下载器预设的（GET /mobile/options，
// 标了默认的排在最前），分类与标签是这台下载器里现有的（GET /downloaders/{id}/labels），也都可以自己填；
// 什么都不选时照旧用下载器自己的默认目录（和网页的推送弹窗一样，默认不选预设目录）。主机的版本没有这两个接口时只有输入框。Transmission 没有分类，分类那一栏只写说明。
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../icons.dart';
import '../providers.dart';
import 'common.dart';

/// 推送时选的：下载器（null 是主机的默认下载器）、保存目录、分类与标签（空的是不指定）。
class PushOptions {
  const PushOptions({
    this.downloaderId,
    this.savePath = '',
    this.category = '',
    this.tags = const [],
  });
  final int? downloaderId;
  final String savePath;
  final String category;
  final List<String> tags;

  /// 推送这个搜索结果的 POST /push 请求。
  AppPushRequest request(AppSearchItem item) => AppPushRequest(
    site: item.site,
    torrentId: item.torrentId,
    downloaderId: downloaderId,
    title: item.title,
    savePath: savePath.isEmpty ? null : savePath,
    category: category.isEmpty ? null : category,
    tags: tags.isEmpty ? null : tags.join(','),
  );
}

/// 把输入的标签拆开：中英文逗号都算分隔，去掉空的与重复的。
List<String> splitTags(String text) {
  final out = <String>[];
  for (final part in text.split(RegExp('[,，]'))) {
    final tag = part.trim();
    if (tag.isNotEmpty && !out.contains(tag)) out.add(tag);
  }
  return out;
}

/// 打开推送面板：点「推送」返回选的，关掉面板返回 null（不推送）。面板开在最外层的导航里，
/// 盖住底部导航条与上一次推送的提示条（提示条压在「推送」上时点不到它）。
Future<PushOptions?> showPushSheet(
  BuildContext context, {
  required String title,
  required List<AppDownloader> downloaders,
}) => showModalBottomSheet<PushOptions>(
  context: context,
  isScrollControlled: true,
  useSafeArea: true,
  useRootNavigator: true,
  builder: (_) => PushSheet(title: title, downloaders: downloaders),
);

class PushSheet extends ConsumerStatefulWidget {
  const PushSheet({super.key, required this.title, required this.downloaders});

  /// 要推送的种子的标题
  final String title;

  /// 启用的下载器（GET /downloaders）；空的时候只能交给主机选默认下载器
  final List<AppDownloader> downloaders;

  @override
  ConsumerState<PushSheet> createState() => _PushSheetState();
}

class _PushSheetState extends ConsumerState<PushSheet> {
  int? _downloader;
  final _path = TextEditingController();
  final _category = TextEditingController();
  final _tagInput = TextEditingController();

  /// 选中的标签（按选的先后）与自己加的标签（下载器里可能还没有）
  final _tags = <String>[];
  final _added = <String>[];

  Set<String> _features = const {};

  /// 各台下载器的预设目录；null 是还没读到或主机没有这个接口
  List<AppMobileDirectory>? _dirs;
  String? _dirsError;

  final _labels = <int, AppDownloaderLabels>{};
  final _labelErrors = <int, String>{};
  int? _loadingLabels;
  Completer<void>? _abortLabels;

  @override
  void initState() {
    super.initState();
    final list = widget.downloaders;
    _downloader =
        (list.where((d) => d.default_).firstOrNull ?? list.firstOrNull)?.id;
    unawaited(_load());
  }

  @override
  void dispose() {
    _cancelLabels();
    _path.dispose();
    _category.dispose();
    _tagInput.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      _features = (await ref.read(metaProvider.future)).features.toSet();
    } on Object {
      // 读不到功能组时当作旧主机：只有输入框
    }
    if (!mounted) return;
    setState(() {});
    unawaited(_loadDirs());
    unawaited(_loadLabels());
  }

  Future<void> _loadDirs() async {
    final api = ref.read(apiProvider);
    if (api == null || !_features.contains('mobile_editors')) return;
    try {
      final o = await api.getMobileOptions();
      if (!mounted) return;
      setState(() {
        _dirs = o?.directories ?? const [];
        _dirsError = o?.errors['directories'];
      });
    } on Object catch (e) {
      if (!mounted) return;
      setState(() => _dirsError = describeError(e, S.of(context)));
    }
  }

  void _cancelLabels() {
    final abort = _abortLabels;
    if (abort != null && !abort.isCompleted) abort.complete();
  }

  /// 读所选下载器的分类与标签（读过的不再读）；换了下载器时取消还没回来的那次。
  Future<void> _loadLabels() async {
    final id = _downloader;
    final api = ref.read(apiProvider);
    if (id == null ||
        api == null ||
        !_features.contains('downloader_labels') ||
        _labels.containsKey(id)) {
      return;
    }
    _cancelLabels();
    final abort = _abortLabels = Completer<void>();
    setState(() {
      _loadingLabels = id;
      _labelErrors.remove(id);
    });
    try {
      final got = await api.getDownloaderLabels(id, abortTrigger: abort.future);
      if (!mounted || abort.isCompleted) return;
      if (got != null) setState(() => _labels[id] = got);
    } on Object catch (e) {
      if (!mounted || abort.isCompleted) return;
      setState(() => _labelErrors[id] = describeError(e, S.of(context)));
    } finally {
      if (mounted && identical(_abortLabels, abort)) {
        setState(() => _loadingLabels = null);
      }
    }
  }

  /// 这台下载器的预设目录：标了默认的排在最前，其余照网页里配置的顺序。
  List<AppMobileDirectory> _dirsOf(int? id) => [
    if (id != null) ...[
      for (final d in _dirs ?? const <AppMobileDirectory>[])
        if (d.downloaderId == id && (d.isDefault ?? false)) d,
      for (final d in _dirs ?? const <AppMobileDirectory>[])
        if (d.downloaderId == id && !(d.isDefault ?? false)) d,
    ],
  ];

  /// 这台下载器有没有分类：读到了以主机说的为准，否则按类型（Transmission 没有）。
  bool get _supportsCategory {
    final id = _downloader;
    if (id == null) return true;
    final labels = _labels[id];
    if (labels != null) return labels.supportsCategory;
    final d = widget.downloaders.where((d) => d.id == id).firstOrNull;
    return d?.type != 'transmission';
  }

  /// 换下载器：目录回到「下载器默认」（上一台的预设目录不一定适用）；从上一台的标签里选的不再算（自己加的留着），
  /// 读那台的分类与标签。
  void _selectDownloader(int id) {
    if (id == _downloader) return;
    setState(() {
      _downloader = id;
      _path.clear();
      _tags.retainWhere(_added.contains);
    });
    unawaited(_loadLabels());
  }

  void _addTags() {
    final add = splitTags(_tagInput.text);
    if (add.isEmpty) return;
    setState(() {
      for (final tag in add) {
        if (!_added.contains(tag)) _added.add(tag);
        if (!_tags.contains(tag)) _tags.add(tag);
      }
      _tagInput.clear();
    });
  }

  void _submit() {
    final tags = [..._tags];
    for (final tag in splitTags(_tagInput.text)) {
      if (!tags.contains(tag)) tags.add(tag);
    }
    Navigator.pop(
      context,
      PushOptions(
        downloaderId: _downloader,
        savePath: _path.text.trim(),
        category: _supportsCategory ? _category.text.trim() : '',
        tags: tags,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final t = Theme.of(context);
    final muted = t.textTheme.bodySmall?.copyWith(
      color: t.colorScheme.onSurfaceVariant,
      fontSize: 12.5,
    );
    final size = MediaQuery.sizeOf(context);
    // 一个目录或标签很长时块里的字截断，不撑出屏幕
    Widget chipText(String text) => ConstrainedBox(
      constraints: BoxConstraints(maxWidth: size.width - 96),
      child: Text(text, maxLines: 1, overflow: TextOverflow.ellipsis),
    );
    final id = _downloader;
    final labels = id == null ? null : _labels[id];
    final dirs = _dirsOf(id);
    final path = _path.text.trim();
    final category = _category.text.trim();
    final tagChoices = [
      ...?labels?.tags,
      for (final tag in _added)
        if (!(labels?.tags.contains(tag) ?? false)) tag,
    ];
    final labelError = id == null ? null : _labelErrors[id];
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      // 按实际能用的高度留出上面一截遮罩，点它关上面板
      child: LayoutBuilder(
        builder: (context, c) => ConstrainedBox(
          constraints: BoxConstraints(maxHeight: c.maxHeight * 0.88),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Flexible(
                child: ListView(
                  shrinkWrap: true,
                  padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
                  children: [
                    Text(s.pushTitle, style: t.textTheme.titleMedium),
                    const SizedBox(height: 4),
                    Text(
                      widget.title,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: muted,
                    ),
                    const SizedBox(height: 16),
                    RowGroup(
                      title: s.pushDownloader,
                      children: [
                        for (final d in widget.downloaders)
                          ListTile(
                            key: ValueKey('push-downloader-${d.id}'),
                            minTileHeight: 56,
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
                            selected: d.id == id,
                            trailing: d.id == id
                                ? const PtIcon(PtGlyph.check, size: 20)
                                : null,
                            onTap: () => _selectDownloader(d.id),
                          ),
                        if (widget.downloaders.isEmpty)
                          ListTile(
                            minTileHeight: 56,
                            title: Text(s.pushDefault),
                            selected: true,
                            trailing: const PtIcon(PtGlyph.check, size: 20),
                          ),
                      ],
                    ),
                    _SectionTitle(s.pushSavePath),
                    if (dirs.isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 8),
                        child: Wrap(
                          spacing: 8,
                          runSpacing: 4,
                          children: [
                            ChoiceChip(
                              key: const ValueKey('push-dir-default'),
                              label: chipText(s.pushPathDefault),
                              selected: path.isEmpty,
                              onSelected: (_) => setState(_path.clear),
                            ),
                            for (final d in dirs)
                              ChoiceChip(
                                key: ValueKey('push-dir-${d.path}'),
                                label: chipText(
                                  [
                                    if (d.alias?.isNotEmpty ?? false)
                                      d.alias!
                                    else
                                      d.path,
                                    if (d.isDefault ?? false) s.defaultBadge,
                                  ].join(' · '),
                                ),
                                selected: path == d.path,
                                onSelected: (_) =>
                                    setState(() => _path.text = d.path),
                              ),
                          ],
                        ),
                      ),
                    TextField(
                      key: const Key('push-save-path'),
                      controller: _path,
                      decoration: InputDecoration(hintText: s.pushSavePathHint),
                      onChanged: (_) => setState(() {}),
                    ),
                    if (_dirsError != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Text(
                          s.pushDirsFailed(_dirsError!),
                          style: muted,
                        ),
                      ),
                    _SectionTitle(s.pushCategory),
                    if (_supportsCategory) ...[
                      if (labels != null && labels.categories.isNotEmpty)
                        Padding(
                          padding: const EdgeInsets.only(bottom: 8),
                          child: Wrap(
                            spacing: 8,
                            runSpacing: 4,
                            children: [
                              for (final c in labels.categories)
                                ChoiceChip(
                                  key: ValueKey('push-category-$c'),
                                  label: chipText(c),
                                  selected: category == c,
                                  onSelected: (on) => setState(
                                    () => _category.text = on ? c : '',
                                  ),
                                ),
                            ],
                          ),
                        ),
                      TextField(
                        key: const Key('push-category'),
                        controller: _category,
                        decoration: InputDecoration(
                          hintText: s.pushCategoryHint,
                        ),
                        onChanged: (_) => setState(() {}),
                      ),
                    ] else
                      Text(s.pushNoCategory, style: muted),
                    _SectionTitle(s.pushTags),
                    if (tagChoices.isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 8),
                        child: Wrap(
                          spacing: 8,
                          runSpacing: 4,
                          children: [
                            for (final tag in tagChoices)
                              FilterChip(
                                key: ValueKey('push-tag-$tag'),
                                label: chipText(tag),
                                selected: _tags.contains(tag),
                                onSelected: (on) => setState(
                                  () => on ? _tags.add(tag) : _tags.remove(tag),
                                ),
                              ),
                          ],
                        ),
                      ),
                    TextField(
                      key: const Key('push-tag-input'),
                      controller: _tagInput,
                      decoration: InputDecoration(
                        hintText: s.pushTagsHint,
                        suffixIcon: IconButton(
                          tooltip: s.pushTagAdd,
                          icon: const PtIcon(PtGlyph.plus, size: 20),
                          onPressed: _addTags,
                        ),
                      ),
                      onSubmitted: (_) => _addTags(),
                    ),
                    if (_loadingLabels != null && _loadingLabels == id)
                      Padding(
                        padding: const EdgeInsets.only(top: 8),
                        child: Row(
                          children: [
                            const SizedBox(
                              width: 14,
                              height: 14,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: Text(s.pushLabelsLoading, style: muted),
                            ),
                          ],
                        ),
                      )
                    else if (labelError != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 4),
                        child: Row(
                          children: [
                            Expanded(
                              child: Text(
                                s.pushLabelsFailed(labelError),
                                style: muted,
                              ),
                            ),
                            TextButton(
                              onPressed: () => unawaited(_loadLabels()),
                              child: Text(s.retry),
                            ),
                          ],
                        ),
                      ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
                child: FilledButton(
                  key: const Key('push-submit'),
                  onPressed: _submit,
                  style: FilledButton.styleFrom(
                    minimumSize: const Size.fromHeight(50),
                  ),
                  child: Text(s.pushSubmit),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// 一栏的小标题（和 [RowGroup] 的标题一样）。
class _SectionTitle extends StatelessWidget {
  const _SectionTitle(this.text);
  final String text;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 18, 4, 8),
      child: Text(
        text,
        style: t.textTheme.labelLarge?.copyWith(
          fontSize: 13,
          fontWeight: FontWeight.w600,
          color: t.colorScheme.onSurfaceVariant,
        ),
      ),
    );
  }
}
