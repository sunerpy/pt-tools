// 排序的选择怎么存、怎么读，搜索结果怎么在手机上排；推送选项怎么变成 POST /push 的请求。
import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/app/sorting.dart';
import 'package:pt_tools_app/src/app/widgets/push_sheet.dart';
import 'package:ptt_api/api.dart';

AppSearchItem _item(
  String id, {
  int size = 0,
  int seeders = 0,
  int leechers = 0,
  int? snatched,
  int? uploadedAt,
}) => AppSearchItem(
  site: 'hdsky',
  torrentId: id,
  title: '标题 $id',
  size: size,
  seeders: seeders,
  leechers: leechers,
  snatched: snatched,
  uploadedAt: uploadedAt,
  free: false,
  hasHr: false,
);

List<String> _ids(List<AppSearchItem> items) => [
  for (final it in items) it.torrentId,
];

/// 存不进去也读不出来（例如 Keystore 出错）。
class _BrokenSortStore implements SortStore {
  @override
  Future<SortChoice?> load(SortList list) async => throw StateError('坏了');

  @override
  Future<void> save(SortList list, SortChoice choice) async =>
      throw StateError('坏了');
}

/// 一直读不完。
class _HangingSortStore implements SortStore {
  @override
  Future<SortChoice?> load(SortList list) => Completer<SortChoice?>().future;

  @override
  Future<void> save(SortList list, SortChoice choice) async {}
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('排序存成「字段:方向」；读不懂的、字段不在这个列表里的当作没有存过', () {
    expect(const SortChoice('size').encode(), 'size:desc');
    expect(const SortChoice('title', descending: false).encode(), 'title:asc');
    expect(
      SortChoice.decode('size:asc', SortList.torrents.fields),
      const SortChoice('size', descending: false),
    );
    expect(
      SortChoice.decode('default:desc', SortList.search.fields),
      const SortChoice(searchSortDefault),
    );
    for (final raw in [
      null,
      '',
      'size',
      'size:up',
      'size:desc:x',
      'passkey:desc',
      // 搜索结果才有的字段，种子页不认
      'seeders:desc',
    ]) {
      expect(
        SortChoice.decode(raw, SortList.torrents.fields),
        isNull,
        reason: '$raw',
      );
    }
    expect(SortChoice.decode('eta:asc', SortList.search.fields), isNull);
  });

  test('安全存储里每个列表一个键，存了读得回来', () async {
    FlutterSecureStorage.setMockInitialValues({
      'pt-tools.sort.search.v1': 'snatched:asc',
    });
    final store = SecureSortStore();
    expect(await store.load(SortList.torrents), isNull);
    expect(
      await store.load(SortList.search),
      const SortChoice('snatched', descending: false),
    );
    await store.save(SortList.torrents, const SortChoice('ratio'));
    expect(await store.load(SortList.torrents), const SortChoice('ratio'));
    expect(
      await const FlutterSecureStorage().read(key: 'pt-tools.sort.torrents.v1'),
      'ratio:desc',
    );
  });

  test('启动时读排序：存过的照存的，没存过、读不出来的用默认', () async {
    expect(
      await loadInitialSorts(
        MemorySortStore({SortList.search: const SortChoice('size')}),
      ),
      {SortList.search: const SortChoice('size')},
    );
    expect(await loadInitialSorts(_BrokenSortStore()), isEmpty);
  });

  testWidgets('启动时读排序太慢：2 秒后用默认，不挡着启动', (tester) async {
    Map<SortList, SortChoice>? got;
    unawaited(loadInitialSorts(_HangingSortStore()).then((v) => got = v));
    await tester.pump(const Duration(seconds: 1));
    expect(got, isNull);
    await tester.pump(const Duration(seconds: 2));
    expect(got, isEmpty);
  });

  test('换排序马上生效并存下来；存不进去也不退回', () async {
    final store = MemorySortStore();
    final c = ProviderContainer(
      overrides: [
        sortStoreProvider.overrideWithValue(store),
        initialSortsProvider.overrideWithValue({
          SortList.torrents: const SortChoice('size'),
        }),
      ],
    );
    addTearDown(c.dispose);
    expect(c.read(torrentSortProvider), const SortChoice('size'));
    expect(c.read(searchSortProvider), const SortChoice(searchSortDefault));
    await c.read(searchSortProvider.notifier).set(const SortChoice('seeders'));
    expect(c.read(searchSortProvider), const SortChoice('seeders'));
    expect(store.saved, {SortList.search: const SortChoice('seeders')});

    final broken = ProviderContainer(
      overrides: [sortStoreProvider.overrideWithValue(_BrokenSortStore())],
    );
    addTearDown(broken.dispose);
    expect(broken.read(torrentSortProvider), defaultTorrentSort);
    await broken
        .read(torrentSortProvider.notifier)
        .set(const SortChoice('eta', descending: false));
    expect(
      broken.read(torrentSortProvider),
      const SortChoice('eta', descending: false),
    );
  });

  test('搜索结果：默认顺序原样；一样的保持主机给的先后；不改原来的列表', () {
    final items = [
      _item('a', seeders: 5, size: 30, leechers: 2),
      _item('b', seeders: 9, size: 10, leechers: 2, snatched: 7),
      _item('c', seeders: 5, size: 20, leechers: 8, snatched: 3),
      _item('d', seeders: 1, size: 40, leechers: 0),
    ];
    expect(
      identical(
        sortSearchItems(items, const SortChoice(searchSortDefault)),
        items,
      ),
      isTrue,
    );
    expect(_ids(sortSearchItems(items, const SortChoice('seeders'))), [
      'b',
      'a',
      'c',
      'd',
    ]);
    expect(
      _ids(
        sortSearchItems(items, const SortChoice('seeders', descending: false)),
      ),
      ['d', 'a', 'c', 'b'],
    );
    expect(_ids(sortSearchItems(items, const SortChoice('size'))), [
      'd',
      'a',
      'c',
      'b',
    ]);
    expect(_ids(sortSearchItems(items, const SortChoice('leechers'))), [
      'c',
      'a',
      'b',
      'd',
    ]);
    // 没有完成数的按 0 算
    expect(_ids(sortSearchItems(items, const SortChoice('snatched'))), [
      'b',
      'c',
      'a',
      'd',
    ]);
    expect(_ids(items), ['a', 'b', 'c', 'd']);
  });

  test('搜索结果按发布时间：没有发布时间的不管哪个方向都放在最后', () {
    final items = [
      _item('a', uploadedAt: 200),
      _item('b'),
      _item('c', uploadedAt: 100),
      _item('d', uploadedAt: 0),
    ];
    expect(_ids(sortSearchItems(items, const SortChoice('uploaded_at'))), [
      'a',
      'c',
      'b',
      'd',
    ]);
    expect(
      _ids(
        sortSearchItems(
          items,
          const SortChoice('uploaded_at', descending: false),
        ),
      ),
      ['c', 'a', 'b', 'd'],
    );
  });

  test('推送选项：空的不带，标签用逗号连起来；输入的标签按中英文逗号拆开', () {
    final it = _item('101');
    final empty = const PushOptions().request(it).toJson();
    expect(empty, {
      'site': 'hdsky',
      'torrent_id': '101',
      'downloader_id': null,
      'title': '标题 101',
      'category': null,
      'tags': null,
      'save_path': null,
    });
    final full = const PushOptions(
      downloaderId: 2,
      savePath: '/data/tv',
      category: 'tv',
      tags: ['hd', '手机'],
    ).request(it).toJson();
    expect(full, {
      'site': 'hdsky',
      'torrent_id': '101',
      'downloader_id': 2,
      'title': '标题 101',
      'category': 'tv',
      'tags': 'hd,手机',
      'save_path': '/data/tv',
    });
    expect(splitTags(' hd, 手机，,hd ,pt '), ['hd', '手机', 'pt']);
    expect(splitTags(' , ，'), isEmpty);
  });
}
