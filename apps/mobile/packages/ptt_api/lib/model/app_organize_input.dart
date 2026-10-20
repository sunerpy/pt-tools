//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppOrganizeInput {
  /// Returns a new [AppOrganizeInput] instance.
  AppOrganizeInput({
    required this.downloaderId,
    required this.hash,
    this.mediaType,
    this.tmdbId,
    this.libraryId,
  });

  /// Minimum value: 1
  int downloaderId;

  /// 种子的 info_hash（GET /torrents 的 info_hash，没有时用 task_id）
  String hash;

  /// 与 tmdb_id 一起给时按这个条目整理，不自动识别
  AppOrganizeInputMediaTypeEnum? mediaType;

  /// Minimum value: 0
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? tmdbId;

  /// 不给时按类型选媒体库
  ///
  /// Minimum value: 0
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? libraryId;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppOrganizeInput &&
    other.downloaderId == downloaderId &&
    other.hash == hash &&
    other.mediaType == mediaType &&
    other.tmdbId == tmdbId &&
    other.libraryId == libraryId;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (downloaderId.hashCode) +
    (hash.hashCode) +
    (mediaType == null ? 0 : mediaType!.hashCode) +
    (tmdbId == null ? 0 : tmdbId!.hashCode) +
    (libraryId == null ? 0 : libraryId!.hashCode);

  @override
  String toString() => 'AppOrganizeInput[downloaderId=$downloaderId, hash=$hash, mediaType=$mediaType, tmdbId=$tmdbId, libraryId=$libraryId]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'downloader_id'] = this.downloaderId;
      json[r'hash'] = this.hash;
    if (this.mediaType != null) {
      json[r'media_type'] = this.mediaType;
    } else {
      json[r'media_type'] = null;
    }
    if (this.tmdbId != null) {
      json[r'tmdb_id'] = this.tmdbId;
    } else {
      json[r'tmdb_id'] = null;
    }
    if (this.libraryId != null) {
      json[r'library_id'] = this.libraryId;
    } else {
      json[r'library_id'] = null;
    }
    return json;
  }

  /// Returns a new [AppOrganizeInput] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppOrganizeInput? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'downloader_id'), 'Required key "AppOrganizeInput[downloader_id]" is missing from JSON.');
        assert(json[r'downloader_id'] != null, 'Required key "AppOrganizeInput[downloader_id]" has a null value in JSON.');
        assert(json.containsKey(r'hash'), 'Required key "AppOrganizeInput[hash]" is missing from JSON.');
        assert(json[r'hash'] != null, 'Required key "AppOrganizeInput[hash]" has a null value in JSON.');
        return true;
      }());

      return AppOrganizeInput(
        downloaderId: mapValueOfType<int>(json, r'downloader_id')!,
        hash: mapValueOfType<String>(json, r'hash')!,
        mediaType: AppOrganizeInputMediaTypeEnum.fromJson(json[r'media_type']),
        tmdbId: mapValueOfType<int>(json, r'tmdb_id'),
        libraryId: mapValueOfType<int>(json, r'library_id'),
      );
    }
    return null;
  }

  static List<AppOrganizeInput> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppOrganizeInput>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppOrganizeInput.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppOrganizeInput> mapFromJson(dynamic json) {
    final map = <String, AppOrganizeInput>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppOrganizeInput.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppOrganizeInput-objects as value to a dart map
  static Map<String, List<AppOrganizeInput>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppOrganizeInput>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppOrganizeInput.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'downloader_id',
    'hash',
  };
}

/// 与 tmdb_id 一起给时按这个条目整理，不自动识别
enum AppOrganizeInputMediaTypeEnum {
  movie._(r'movie'),
  tv._(r'tv'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppOrganizeInputMediaTypeEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppOrganizeInputMediaTypeEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppOrganizeInputMediaTypeEnum? fromJson(dynamic value) => AppOrganizeInputMediaTypeEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppOrganizeInputMediaTypeEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppOrganizeInputMediaTypeEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppOrganizeInputMediaTypeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppOrganizeInputMediaTypeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppOrganizeInputMediaTypeEnum] to String,
/// and [decode] dynamic data back to [AppOrganizeInputMediaTypeEnum].
class AppOrganizeInputMediaTypeEnumTypeTransformer {
  factory AppOrganizeInputMediaTypeEnumTypeTransformer() => _instance ??= const AppOrganizeInputMediaTypeEnumTypeTransformer._();

  const AppOrganizeInputMediaTypeEnumTypeTransformer._();

  String encode(AppOrganizeInputMediaTypeEnum data) => data._value;

  /// Returns the instance of [AppOrganizeInputMediaTypeEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppOrganizeInputMediaTypeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppOrganizeInputMediaTypeEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'movie': return AppOrganizeInputMediaTypeEnum.movie;
        case r'tv': return AppOrganizeInputMediaTypeEnum.tv;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppOrganizeInputMediaTypeEnumTypeTransformer? _instance;
}


