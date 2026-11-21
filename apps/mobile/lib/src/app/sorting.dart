// 列表的排序：种子页由主机按字段排（GET /torrents 的 sort、order），搜索结果在手机上排。
// 选过的排序存在本机（与配对记录、主题同一个安全存储，只是一个短字符串），启动时和主题一起读出来，重开 App 沿用。
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:ptt_api/api.dart';

/// 一种排序：字段与方向。
class SortChoice {
  const SortChoice(this.field, {this.descending = true});

  /// 字段（种子页是 App API 的 sort 值）
  final String field;
  final bool descending;

  /// App API 的 order。
  String get order => descending ? 'desc' : 'asc';

  /// 存下来的写法：`字段:desc` 或 `字段:asc`。
  String encode() => '$field:$order';

  /// 读存下来的写法；读不懂或字段不在 [fields] 里时为 null。
  static SortChoice? decode(String? raw, List<String> fields) {
    final parts = raw?.split(':');
    if (parts == null || parts.length != 2 || !fields.contains(parts[0])) {
      return null;
    }
    return switch (parts[1]) {
      'desc' => SortChoice(parts[0]),
      'asc' => SortChoice(parts[0], descending: false),
      _ => null,
    };
  }

  @override
  bool operator ==(Object other) =>
      other is SortChoice &&
      other.field == field &&
      other.descending == descending;

  @override
  int get hashCode => Object.hash(field, descending);

  @override
  String toString() => encode();
}

/// 种子页默认的排序：按添加时间从新到旧（和 App API 不带 sort 时一样）。
const defaultTorrentSort = SortChoice('added_at');

/// 搜索结果的「默认顺序」：主机给的先后，没有方向。
const searchSortDefault = 'default';

/// 记得住排序的两个列表。
enum SortList {
  /// 种子页：主机按这个字段排
  torrents('pt-tools.sort.torrents.v1', [
    'added_at',
    'completed_at',
    'title',
    'size',
    'progress',
    'ratio',
    'upload_speed',
    'download_speed',
    'eta',
    'seeds',
    'state',
  ], defaultTorrentSort),

  /// 搜索结果：在手机上排，默认是主机给的顺序
  search('pt-tools.sort.search.v1', [
    searchSortDefault,
    'uploaded_at',
    'seeders',
    'leechers',
    'snatched',
    'size',
  ], SortChoice(searchSortDefault));

  const SortList(this.key, this.fields, this.fallback);

  /// 安全存储里的键
  final String key;

  /// 能选的字段（顺序就是面板里列出的顺序）
  final List<String> fields;

  /// 没有选过时用的排序
  final SortChoice fallback;
}

/// 排序选择的读写（测试里换成内存的）。
abstract interface class SortStore {
  Future<SortChoice?> load(SortList list);
  Future<void> save(SortList list, SortChoice choice);
}

class SecureSortStore implements SortStore {
  SecureSortStore([FlutterSecureStorage? storage])
    : _s = storage ?? const FlutterSecureStorage();

  final FlutterSecureStorage _s;

  @override
  Future<SortChoice?> load(SortList list) async =>
      SortChoice.decode(await _s.read(key: list.key), list.fields);

  @override
  Future<void> save(SortList list, SortChoice choice) =>
      _s.write(key: list.key, value: choice.encode());
}

class MemorySortStore implements SortStore {
  MemorySortStore([Map<SortList, SortChoice>? saved]) : saved = {...?saved};
  final Map<SortList, SortChoice> saved;

  @override
  Future<SortChoice?> load(SortList list) async => saved[list];

  @override
  Future<void> save(SortList list, SortChoice choice) async =>
      saved[list] = choice;
}

/// 启动时读出来的排序（main 里覆盖）；读不到、读不懂或者太慢的用各列表的默认，不挡着启动。
Future<Map<SortList, SortChoice>> loadInitialSorts(SortStore store) async {
  final loaded = await Future.wait([
    for (final list in SortList.values)
      store
          .load(list)
          .timeout(const Duration(seconds: 2))
          .then<SortChoice?>((c) => c, onError: (Object _) => null),
  ]);
  return {for (final (i, list) in SortList.values.indexed) list: ?loaded[i]};
}

final sortStoreProvider = Provider<SortStore>((ref) => SecureSortStore());

final initialSortsProvider = Provider<Map<SortList, SortChoice>>(
  (ref) => const {},
);

/// 种子页的排序。
final torrentSortProvider = NotifierProvider<SortNotifier, SortChoice>(
  () => SortNotifier(SortList.torrents),
);

/// 搜索结果的排序。
final searchSortProvider = NotifierProvider<SortNotifier, SortChoice>(
  () => SortNotifier(SortList.search),
);

class SortNotifier extends Notifier<SortChoice> {
  SortNotifier(this.list);
  final SortList list;

  @override
  SortChoice build() => ref.read(initialSortsProvider)[list] ?? list.fallback;

  /// 换排序：列表马上按新的排，再存下来（存不进去也不退回，下次启动按默认）。
  Future<void> set(SortChoice choice) async {
    if (choice == state) return;
    state = choice;
    try {
      await ref.read(sortStoreProvider).save(list, choice);
    } on Object {
      // 只影响下次启动
    }
  }
}

/// 搜索结果按 [choice] 排（在手机上排）：默认顺序原样返回；一样的保持主机给的先后；
/// 没有发布时间的不管哪个方向都放在最后。
List<AppSearchItem> sortSearchItems(
  List<AppSearchItem> items,
  SortChoice choice,
) {
  if (choice.field == searchSortDefault) return items;
  int? key(AppSearchItem it) => switch (choice.field) {
    'uploaded_at' => (it.uploadedAt ?? 0) > 0 ? it.uploadedAt : null,
    'seeders' => it.seeders,
    'leechers' => it.leechers,
    'snatched' => it.snatched ?? 0,
    'size' => it.size,
    _ => null,
  };
  final keyed = [for (final (i, it) in items.indexed) (i, it, key(it))];
  keyed.sort((a, b) {
    final (ka, kb) = (a.$3, b.$3);
    if (ka == null || kb == null) {
      if (ka != kb) return ka == null ? 1 : -1;
      return a.$1 - b.$1;
    }
    final c = choice.descending ? kb.compareTo(ka) : ka.compareTo(kb);
    return c != 0 ? c : a.$1 - b.$1;
  });
  return [for (final (_, it, _) in keyed) it];
}
