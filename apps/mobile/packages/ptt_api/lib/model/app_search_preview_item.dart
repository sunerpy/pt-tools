//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewItem {
  /// Returns a new [AppSearchPreviewItem] instance.
  AppSearchPreviewItem({
    required this.mediaType,
    required this.tmdbId,
    required this.season,
    required this.title,
    required this.originalTitle,
    required this.year,
    required this.posterPath,
    required this.seasons,
    required this.totalEpisodes,
  });

  AppSearchPreviewItemMediaTypeEnum mediaType;

  int tmdbId;

  /// 剧集的季（请求的 0 换成了最新一季）；电影是 0
  int season;

  String title;

  String originalTitle;

  /// 年份（剧集是这一季的）
  int year;

  String posterPath;

  /// 剧集一共几季
  int seasons;

  /// 这一季几集
  int totalEpisodes;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewItem &&
    other.mediaType == mediaType &&
    other.tmdbId == tmdbId &&
    other.season == season &&
    other.title == title &&
    other.originalTitle == originalTitle &&
    other.year == year &&
    other.posterPath == posterPath &&
    other.seasons == seasons &&
    other.totalEpisodes == totalEpisodes;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (mediaType.hashCode) +
    (tmdbId.hashCode) +
    (season.hashCode) +
    (title.hashCode) +
    (originalTitle.hashCode) +
    (year.hashCode) +
    (posterPath.hashCode) +
    (seasons.hashCode) +
    (totalEpisodes.hashCode);

  @override
  String toString() => 'AppSearchPreviewItem[mediaType=$mediaType, tmdbId=$tmdbId, season=$season, title=$title, originalTitle=$originalTitle, year=$year, posterPath=$posterPath, seasons=$seasons, totalEpisodes=$totalEpisodes]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'media_type'] = this.mediaType;
      json[r'tmdb_id'] = this.tmdbId;
      json[r'season'] = this.season;
      json[r'title'] = this.title;
      json[r'original_title'] = this.originalTitle;
      json[r'year'] = this.year;
      json[r'poster_path'] = this.posterPath;
      json[r'seasons'] = this.seasons;
      json[r'total_episodes'] = this.totalEpisodes;
    return json;
  }

  /// Returns a new [AppSearchPreviewItem] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewItem? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'media_type'), 'Required key "AppSearchPreviewItem[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppSearchPreviewItem[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'tmdb_id'), 'Required key "AppSearchPreviewItem[tmdb_id]" is missing from JSON.');
        assert(json[r'tmdb_id'] != null, 'Required key "AppSearchPreviewItem[tmdb_id]" has a null value in JSON.');
        assert(json.containsKey(r'season'), 'Required key "AppSearchPreviewItem[season]" is missing from JSON.');
        assert(json[r'season'] != null, 'Required key "AppSearchPreviewItem[season]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppSearchPreviewItem[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppSearchPreviewItem[title]" has a null value in JSON.');
        assert(json.containsKey(r'original_title'), 'Required key "AppSearchPreviewItem[original_title]" is missing from JSON.');
        assert(json[r'original_title'] != null, 'Required key "AppSearchPreviewItem[original_title]" has a null value in JSON.');
        assert(json.containsKey(r'year'), 'Required key "AppSearchPreviewItem[year]" is missing from JSON.');
        assert(json[r'year'] != null, 'Required key "AppSearchPreviewItem[year]" has a null value in JSON.');
        assert(json.containsKey(r'poster_path'), 'Required key "AppSearchPreviewItem[poster_path]" is missing from JSON.');
        assert(json[r'poster_path'] != null, 'Required key "AppSearchPreviewItem[poster_path]" has a null value in JSON.');
        assert(json.containsKey(r'seasons'), 'Required key "AppSearchPreviewItem[seasons]" is missing from JSON.');
        assert(json[r'seasons'] != null, 'Required key "AppSearchPreviewItem[seasons]" has a null value in JSON.');
        assert(json.containsKey(r'total_episodes'), 'Required key "AppSearchPreviewItem[total_episodes]" is missing from JSON.');
        assert(json[r'total_episodes'] != null, 'Required key "AppSearchPreviewItem[total_episodes]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewItem(
        mediaType: AppSearchPreviewItemMediaTypeEnum.fromJson(json[r'media_type'])!,
        tmdbId: mapValueOfType<int>(json, r'tmdb_id')!,
        season: mapValueOfType<int>(json, r'season')!,
        title: mapValueOfType<String>(json, r'title')!,
        originalTitle: mapValueOfType<String>(json, r'original_title')!,
        year: mapValueOfType<int>(json, r'year')!,
        posterPath: mapValueOfType<String>(json, r'poster_path')!,
        seasons: mapValueOfType<int>(json, r'seasons')!,
        totalEpisodes: mapValueOfType<int>(json, r'total_episodes')!,
      );
    }
    return null;
  }

  static List<AppSearchPreviewItem> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewItem>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewItem.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewItem> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewItem>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewItem.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewItem-objects as value to a dart map
  static Map<String, List<AppSearchPreviewItem>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewItem>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewItem.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'media_type',
    'tmdb_id',
    'season',
    'title',
    'original_title',
    'year',
    'poster_path',
    'seasons',
    'total_episodes',
  };
}


enum AppSearchPreviewItemMediaTypeEnum {
  movie._(r'movie'),
  tv._(r'tv'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppSearchPreviewItemMediaTypeEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppSearchPreviewItemMediaTypeEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppSearchPreviewItemMediaTypeEnum? fromJson(dynamic value) => AppSearchPreviewItemMediaTypeEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppSearchPreviewItemMediaTypeEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppSearchPreviewItemMediaTypeEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewItemMediaTypeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewItemMediaTypeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppSearchPreviewItemMediaTypeEnum] to String,
/// and [decode] dynamic data back to [AppSearchPreviewItemMediaTypeEnum].
class AppSearchPreviewItemMediaTypeEnumTypeTransformer {
  factory AppSearchPreviewItemMediaTypeEnumTypeTransformer() => _instance ??= const AppSearchPreviewItemMediaTypeEnumTypeTransformer._();

  const AppSearchPreviewItemMediaTypeEnumTypeTransformer._();

  String encode(AppSearchPreviewItemMediaTypeEnum data) => data._value;

  /// Returns the instance of [AppSearchPreviewItemMediaTypeEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppSearchPreviewItemMediaTypeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppSearchPreviewItemMediaTypeEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'movie': return AppSearchPreviewItemMediaTypeEnum.movie;
        case r'tv': return AppSearchPreviewItemMediaTypeEnum.tv;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppSearchPreviewItemMediaTypeEnumTypeTransformer? _instance;
}


