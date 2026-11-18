//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewInput {
  /// Returns a new [AppSearchPreviewInput] instance.
  AppSearchPreviewInput({
    required this.mediaType,
    required this.tmdbId,
    this.season,
    this.profileId,
    this.sites = const [],
    this.includeKeywords,
    this.excludeKeywords,
    this.includeUnmatched,
  });

  AppSearchPreviewInputMediaTypeEnum mediaType;

  /// Minimum value: 1
  int tmdbId;

  /// 剧集的季，0 是最新一季；电影不用
  ///
  /// Minimum value: 0
  /// Maximum value: 200
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? season;

  /// 表单里的质量档案；0 是设置里的默认档案（和保存以后一样）
  ///
  /// Minimum value: 0
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? profileId;

  /// 只搜这些站点（没启用的写进 errors，一个都没启用时不搜）；不给或空的是全部启用的站点
  List<String> sites;

  /// 表单里的标题包含（原样，首尾空白不算）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? includeKeywords;

  /// 表单里的标题排除
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? excludeKeywords;

  /// 带上对不上这个条目的种子（最多 50 条）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? includeUnmatched;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewInput &&
    other.mediaType == mediaType &&
    other.tmdbId == tmdbId &&
    other.season == season &&
    other.profileId == profileId &&
    _deepEquality.equals(other.sites, sites) &&
    other.includeKeywords == includeKeywords &&
    other.excludeKeywords == excludeKeywords &&
    other.includeUnmatched == includeUnmatched;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (mediaType.hashCode) +
    (tmdbId.hashCode) +
    (season == null ? 0 : season!.hashCode) +
    (profileId == null ? 0 : profileId!.hashCode) +
    (sites.hashCode) +
    (includeKeywords == null ? 0 : includeKeywords!.hashCode) +
    (excludeKeywords == null ? 0 : excludeKeywords!.hashCode) +
    (includeUnmatched == null ? 0 : includeUnmatched!.hashCode);

  @override
  String toString() => 'AppSearchPreviewInput[mediaType=$mediaType, tmdbId=$tmdbId, season=$season, profileId=$profileId, sites=$sites, includeKeywords=$includeKeywords, excludeKeywords=$excludeKeywords, includeUnmatched=$includeUnmatched]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'media_type'] = this.mediaType;
      json[r'tmdb_id'] = this.tmdbId;
    if (this.season != null) {
      json[r'season'] = this.season;
    } else {
      json[r'season'] = null;
    }
    if (this.profileId != null) {
      json[r'profile_id'] = this.profileId;
    } else {
      json[r'profile_id'] = null;
    }
      json[r'sites'] = this.sites;
    if (this.includeKeywords != null) {
      json[r'include_keywords'] = this.includeKeywords;
    } else {
      json[r'include_keywords'] = null;
    }
    if (this.excludeKeywords != null) {
      json[r'exclude_keywords'] = this.excludeKeywords;
    } else {
      json[r'exclude_keywords'] = null;
    }
    if (this.includeUnmatched != null) {
      json[r'include_unmatched'] = this.includeUnmatched;
    } else {
      json[r'include_unmatched'] = null;
    }
    return json;
  }

  /// Returns a new [AppSearchPreviewInput] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewInput? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'media_type'), 'Required key "AppSearchPreviewInput[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppSearchPreviewInput[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'tmdb_id'), 'Required key "AppSearchPreviewInput[tmdb_id]" is missing from JSON.');
        assert(json[r'tmdb_id'] != null, 'Required key "AppSearchPreviewInput[tmdb_id]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewInput(
        mediaType: AppSearchPreviewInputMediaTypeEnum.fromJson(json[r'media_type'])!,
        tmdbId: mapValueOfType<int>(json, r'tmdb_id')!,
        season: mapValueOfType<int>(json, r'season'),
        profileId: mapValueOfType<int>(json, r'profile_id'),
        sites: json[r'sites'] is Iterable
            ? (json[r'sites'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        includeKeywords: mapValueOfType<String>(json, r'include_keywords'),
        excludeKeywords: mapValueOfType<String>(json, r'exclude_keywords'),
        includeUnmatched: mapValueOfType<bool>(json, r'include_unmatched'),
      );
    }
    return null;
  }

  static List<AppSearchPreviewInput> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewInput>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewInput.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewInput> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewInput>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewInput.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewInput-objects as value to a dart map
  static Map<String, List<AppSearchPreviewInput>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewInput>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewInput.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'media_type',
    'tmdb_id',
  };
}


enum AppSearchPreviewInputMediaTypeEnum {
  movie._(r'movie'),
  tv._(r'tv'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppSearchPreviewInputMediaTypeEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppSearchPreviewInputMediaTypeEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppSearchPreviewInputMediaTypeEnum? fromJson(dynamic value) => AppSearchPreviewInputMediaTypeEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppSearchPreviewInputMediaTypeEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppSearchPreviewInputMediaTypeEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewInputMediaTypeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewInputMediaTypeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppSearchPreviewInputMediaTypeEnum] to String,
/// and [decode] dynamic data back to [AppSearchPreviewInputMediaTypeEnum].
class AppSearchPreviewInputMediaTypeEnumTypeTransformer {
  factory AppSearchPreviewInputMediaTypeEnumTypeTransformer() => _instance ??= const AppSearchPreviewInputMediaTypeEnumTypeTransformer._();

  const AppSearchPreviewInputMediaTypeEnumTypeTransformer._();

  String encode(AppSearchPreviewInputMediaTypeEnum data) => data._value;

  /// Returns the instance of [AppSearchPreviewInputMediaTypeEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppSearchPreviewInputMediaTypeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppSearchPreviewInputMediaTypeEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'movie': return AppSearchPreviewInputMediaTypeEnum.movie;
        case r'tv': return AppSearchPreviewInputMediaTypeEnum.tv;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppSearchPreviewInputMediaTypeEnumTypeTransformer? _instance;
}


