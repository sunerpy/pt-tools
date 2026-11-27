//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMediaLibraryItem {
  /// Returns a new [AppMediaLibraryItem] instance.
  AppMediaLibraryItem({
    required this.mediaType,
    required this.tmdbId,
    required this.title,
    required this.year,
    required this.season,
    this.episodes = const [],
    required this.files,
    required this.size,
    this.posterPath,
    required this.latestAt,
  });

  /// movie 或 tv
  String mediaType;

  int tmdbId;

  /// 整理时记下的片名；没记下时用 TMDB 的
  String title;

  /// 0 是不知道
  int year;

  /// 剧集的季；电影是 0
  int season;

  /// 库里有的集号（升序、去重）；电影是空数组
  List<int> episodes;

  /// 整理进库的文件数（一个视频一个）
  int files;

  /// 这些文件的大小合计（字节）
  int size;

  /// TMDB 海报的相对路径（例如 /abc.jpg），用 GET /images/tmdb/{size}/{file} 取图；取不到时没有这个字段
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? posterPath;

  /// 最近一次入库的时间（Unix 秒）
  int latestAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMediaLibraryItem &&
    other.mediaType == mediaType &&
    other.tmdbId == tmdbId &&
    other.title == title &&
    other.year == year &&
    other.season == season &&
    _deepEquality.equals(other.episodes, episodes) &&
    other.files == files &&
    other.size == size &&
    other.posterPath == posterPath &&
    other.latestAt == latestAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (mediaType.hashCode) +
    (tmdbId.hashCode) +
    (title.hashCode) +
    (year.hashCode) +
    (season.hashCode) +
    (episodes.hashCode) +
    (files.hashCode) +
    (size.hashCode) +
    (posterPath == null ? 0 : posterPath!.hashCode) +
    (latestAt.hashCode);

  @override
  String toString() => 'AppMediaLibraryItem[mediaType=$mediaType, tmdbId=$tmdbId, title=$title, year=$year, season=$season, episodes=$episodes, files=$files, size=$size, posterPath=$posterPath, latestAt=$latestAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'media_type'] = this.mediaType;
      json[r'tmdb_id'] = this.tmdbId;
      json[r'title'] = this.title;
      json[r'year'] = this.year;
      json[r'season'] = this.season;
      json[r'episodes'] = this.episodes;
      json[r'files'] = this.files;
      json[r'size'] = this.size;
    if (this.posterPath != null) {
      json[r'poster_path'] = this.posterPath;
    } else {
      json[r'poster_path'] = null;
    }
      json[r'latest_at'] = this.latestAt;
    return json;
  }

  /// Returns a new [AppMediaLibraryItem] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMediaLibraryItem? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'media_type'), 'Required key "AppMediaLibraryItem[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppMediaLibraryItem[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'tmdb_id'), 'Required key "AppMediaLibraryItem[tmdb_id]" is missing from JSON.');
        assert(json[r'tmdb_id'] != null, 'Required key "AppMediaLibraryItem[tmdb_id]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppMediaLibraryItem[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppMediaLibraryItem[title]" has a null value in JSON.');
        assert(json.containsKey(r'year'), 'Required key "AppMediaLibraryItem[year]" is missing from JSON.');
        assert(json[r'year'] != null, 'Required key "AppMediaLibraryItem[year]" has a null value in JSON.');
        assert(json.containsKey(r'season'), 'Required key "AppMediaLibraryItem[season]" is missing from JSON.');
        assert(json[r'season'] != null, 'Required key "AppMediaLibraryItem[season]" has a null value in JSON.');
        assert(json.containsKey(r'episodes'), 'Required key "AppMediaLibraryItem[episodes]" is missing from JSON.');
        assert(json[r'episodes'] != null, 'Required key "AppMediaLibraryItem[episodes]" has a null value in JSON.');
        assert(json.containsKey(r'files'), 'Required key "AppMediaLibraryItem[files]" is missing from JSON.');
        assert(json[r'files'] != null, 'Required key "AppMediaLibraryItem[files]" has a null value in JSON.');
        assert(json.containsKey(r'size'), 'Required key "AppMediaLibraryItem[size]" is missing from JSON.');
        assert(json[r'size'] != null, 'Required key "AppMediaLibraryItem[size]" has a null value in JSON.');
        assert(json.containsKey(r'latest_at'), 'Required key "AppMediaLibraryItem[latest_at]" is missing from JSON.');
        assert(json[r'latest_at'] != null, 'Required key "AppMediaLibraryItem[latest_at]" has a null value in JSON.');
        return true;
      }());

      return AppMediaLibraryItem(
        mediaType: mapValueOfType<String>(json, r'media_type')!,
        tmdbId: mapValueOfType<int>(json, r'tmdb_id')!,
        title: mapValueOfType<String>(json, r'title')!,
        year: mapValueOfType<int>(json, r'year')!,
        season: mapValueOfType<int>(json, r'season')!,
        episodes: json[r'episodes'] is Iterable
            ? (json[r'episodes'] as Iterable).cast<int>().toList(growable: false)
            : const [],
        files: mapValueOfType<int>(json, r'files')!,
        size: mapValueOfType<int>(json, r'size')!,
        posterPath: mapValueOfType<String>(json, r'poster_path'),
        latestAt: mapValueOfType<int>(json, r'latest_at')!,
      );
    }
    return null;
  }

  static List<AppMediaLibraryItem> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMediaLibraryItem>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMediaLibraryItem.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMediaLibraryItem> mapFromJson(dynamic json) {
    final map = <String, AppMediaLibraryItem>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMediaLibraryItem.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMediaLibraryItem-objects as value to a dart map
  static Map<String, List<AppMediaLibraryItem>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMediaLibraryItem>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMediaLibraryItem.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'media_type',
    'tmdb_id',
    'title',
    'year',
    'season',
    'episodes',
    'files',
    'size',
    'latest_at',
  };
}

