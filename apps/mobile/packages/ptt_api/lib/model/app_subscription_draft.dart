//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSubscriptionDraft {
  /// Returns a new [AppSubscriptionDraft] instance.
  AppSubscriptionDraft({
    required this.mediaType,
    required this.tmdbId,
    required this.season,
    required this.seasons,
    required this.totalEpisodes,
    required this.title,
    required this.originalTitle,
    required this.year,
    required this.posterPath,
    required this.anime,
    required this.autoLibraryId,
    required this.subscriptionId,
  });

  AppSubscriptionDraftMediaTypeEnum mediaType;

  int tmdbId;

  /// 这一季：请求的 0 换成最新一季；电影是 0
  int season;

  /// 剧集一共几季（电影是 0）
  int seasons;

  /// 这一季几集（电影是 0）
  int totalEpisodes;

  String title;

  String originalTitle;

  /// 年份（剧集是这一季的）
  int year;

  /// 海报经 GET /images/tmdb/{size}/{file} 取
  String posterPath;

  /// TMDB 类型里有「动画」
  bool anime;

  /// 媒体库选「自动」时整理会选到的库；没有能用的库时是 0
  int autoLibraryId;

  /// 这一季已有的订阅；没有时是 0
  int subscriptionId;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSubscriptionDraft &&
    other.mediaType == mediaType &&
    other.tmdbId == tmdbId &&
    other.season == season &&
    other.seasons == seasons &&
    other.totalEpisodes == totalEpisodes &&
    other.title == title &&
    other.originalTitle == originalTitle &&
    other.year == year &&
    other.posterPath == posterPath &&
    other.anime == anime &&
    other.autoLibraryId == autoLibraryId &&
    other.subscriptionId == subscriptionId;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (mediaType.hashCode) +
    (tmdbId.hashCode) +
    (season.hashCode) +
    (seasons.hashCode) +
    (totalEpisodes.hashCode) +
    (title.hashCode) +
    (originalTitle.hashCode) +
    (year.hashCode) +
    (posterPath.hashCode) +
    (anime.hashCode) +
    (autoLibraryId.hashCode) +
    (subscriptionId.hashCode);

  @override
  String toString() => 'AppSubscriptionDraft[mediaType=$mediaType, tmdbId=$tmdbId, season=$season, seasons=$seasons, totalEpisodes=$totalEpisodes, title=$title, originalTitle=$originalTitle, year=$year, posterPath=$posterPath, anime=$anime, autoLibraryId=$autoLibraryId, subscriptionId=$subscriptionId]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'media_type'] = this.mediaType;
      json[r'tmdb_id'] = this.tmdbId;
      json[r'season'] = this.season;
      json[r'seasons'] = this.seasons;
      json[r'total_episodes'] = this.totalEpisodes;
      json[r'title'] = this.title;
      json[r'original_title'] = this.originalTitle;
      json[r'year'] = this.year;
      json[r'poster_path'] = this.posterPath;
      json[r'anime'] = this.anime;
      json[r'auto_library_id'] = this.autoLibraryId;
      json[r'subscription_id'] = this.subscriptionId;
    return json;
  }

  /// Returns a new [AppSubscriptionDraft] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSubscriptionDraft? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'media_type'), 'Required key "AppSubscriptionDraft[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppSubscriptionDraft[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'tmdb_id'), 'Required key "AppSubscriptionDraft[tmdb_id]" is missing from JSON.');
        assert(json[r'tmdb_id'] != null, 'Required key "AppSubscriptionDraft[tmdb_id]" has a null value in JSON.');
        assert(json.containsKey(r'season'), 'Required key "AppSubscriptionDraft[season]" is missing from JSON.');
        assert(json[r'season'] != null, 'Required key "AppSubscriptionDraft[season]" has a null value in JSON.');
        assert(json.containsKey(r'seasons'), 'Required key "AppSubscriptionDraft[seasons]" is missing from JSON.');
        assert(json[r'seasons'] != null, 'Required key "AppSubscriptionDraft[seasons]" has a null value in JSON.');
        assert(json.containsKey(r'total_episodes'), 'Required key "AppSubscriptionDraft[total_episodes]" is missing from JSON.');
        assert(json[r'total_episodes'] != null, 'Required key "AppSubscriptionDraft[total_episodes]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppSubscriptionDraft[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppSubscriptionDraft[title]" has a null value in JSON.');
        assert(json.containsKey(r'original_title'), 'Required key "AppSubscriptionDraft[original_title]" is missing from JSON.');
        assert(json[r'original_title'] != null, 'Required key "AppSubscriptionDraft[original_title]" has a null value in JSON.');
        assert(json.containsKey(r'year'), 'Required key "AppSubscriptionDraft[year]" is missing from JSON.');
        assert(json[r'year'] != null, 'Required key "AppSubscriptionDraft[year]" has a null value in JSON.');
        assert(json.containsKey(r'poster_path'), 'Required key "AppSubscriptionDraft[poster_path]" is missing from JSON.');
        assert(json[r'poster_path'] != null, 'Required key "AppSubscriptionDraft[poster_path]" has a null value in JSON.');
        assert(json.containsKey(r'anime'), 'Required key "AppSubscriptionDraft[anime]" is missing from JSON.');
        assert(json[r'anime'] != null, 'Required key "AppSubscriptionDraft[anime]" has a null value in JSON.');
        assert(json.containsKey(r'auto_library_id'), 'Required key "AppSubscriptionDraft[auto_library_id]" is missing from JSON.');
        assert(json[r'auto_library_id'] != null, 'Required key "AppSubscriptionDraft[auto_library_id]" has a null value in JSON.');
        assert(json.containsKey(r'subscription_id'), 'Required key "AppSubscriptionDraft[subscription_id]" is missing from JSON.');
        assert(json[r'subscription_id'] != null, 'Required key "AppSubscriptionDraft[subscription_id]" has a null value in JSON.');
        return true;
      }());

      return AppSubscriptionDraft(
        mediaType: AppSubscriptionDraftMediaTypeEnum.fromJson(json[r'media_type'])!,
        tmdbId: mapValueOfType<int>(json, r'tmdb_id')!,
        season: mapValueOfType<int>(json, r'season')!,
        seasons: mapValueOfType<int>(json, r'seasons')!,
        totalEpisodes: mapValueOfType<int>(json, r'total_episodes')!,
        title: mapValueOfType<String>(json, r'title')!,
        originalTitle: mapValueOfType<String>(json, r'original_title')!,
        year: mapValueOfType<int>(json, r'year')!,
        posterPath: mapValueOfType<String>(json, r'poster_path')!,
        anime: mapValueOfType<bool>(json, r'anime')!,
        autoLibraryId: mapValueOfType<int>(json, r'auto_library_id')!,
        subscriptionId: mapValueOfType<int>(json, r'subscription_id')!,
      );
    }
    return null;
  }

  static List<AppSubscriptionDraft> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionDraft>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionDraft.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSubscriptionDraft> mapFromJson(dynamic json) {
    final map = <String, AppSubscriptionDraft>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSubscriptionDraft.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSubscriptionDraft-objects as value to a dart map
  static Map<String, List<AppSubscriptionDraft>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSubscriptionDraft>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSubscriptionDraft.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'media_type',
    'tmdb_id',
    'season',
    'seasons',
    'total_episodes',
    'title',
    'original_title',
    'year',
    'poster_path',
    'anime',
    'auto_library_id',
    'subscription_id',
  };
}


enum AppSubscriptionDraftMediaTypeEnum {
  movie._(r'movie'),
  tv._(r'tv'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppSubscriptionDraftMediaTypeEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppSubscriptionDraftMediaTypeEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppSubscriptionDraftMediaTypeEnum? fromJson(dynamic value) => AppSubscriptionDraftMediaTypeEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppSubscriptionDraftMediaTypeEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppSubscriptionDraftMediaTypeEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionDraftMediaTypeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionDraftMediaTypeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppSubscriptionDraftMediaTypeEnum] to String,
/// and [decode] dynamic data back to [AppSubscriptionDraftMediaTypeEnum].
class AppSubscriptionDraftMediaTypeEnumTypeTransformer {
  factory AppSubscriptionDraftMediaTypeEnumTypeTransformer() => _instance ??= const AppSubscriptionDraftMediaTypeEnumTypeTransformer._();

  const AppSubscriptionDraftMediaTypeEnumTypeTransformer._();

  String encode(AppSubscriptionDraftMediaTypeEnum data) => data._value;

  /// Returns the instance of [AppSubscriptionDraftMediaTypeEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppSubscriptionDraftMediaTypeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppSubscriptionDraftMediaTypeEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'movie': return AppSubscriptionDraftMediaTypeEnum.movie;
        case r'tv': return AppSubscriptionDraftMediaTypeEnum.tv;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppSubscriptionDraftMediaTypeEnumTypeTransformer? _instance;
}


