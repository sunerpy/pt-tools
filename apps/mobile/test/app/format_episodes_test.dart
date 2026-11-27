// 媒体库页的集号范围：连续的合成一段，最多 3 段，超过时加省略号。
import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/app/format.dart';

void main() {
  test('连续的合成一段，单集单写，集号补两位', () {
    expect(formatEpisodeRanges([1, 2, 3, 5, 7, 8]), 'E01–E03, E05, E07–E08');
    expect(formatEpisodeRanges([1, 2, 3, 5]), 'E01–E03, E05');
    expect(formatEpisodeRanges([12]), 'E12');
    expect(formatEpisodeRanges([99, 100, 101]), 'E99–E101');
  });

  test('超过 3 段时只写前 3 段，后面加省略号', () {
    expect(formatEpisodeRanges([1, 3, 5, 7]), 'E01, E03, E05 …');
    expect(formatEpisodeRanges([1, 2, 4, 6, 8, 9]), 'E01–E02, E04, E06 …');
  });

  test('乱序、重复的先排好；空的不显示', () {
    expect(formatEpisodeRanges([3, 1, 2, 2]), 'E01–E03');
    expect(formatEpisodeRanges([]), '');
    expect(formatEpisodeRanges([0, -1]), '');
  });
}
