//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSubscriptionCreate {
  /// Returns a new [AppSubscriptionCreate] instance.
  AppSubscriptionCreate({
    required this.mediaType,
    required this.tmdbId,
    this.season,
    this.profileId,
    this.upgrade,
  });

  AppSubscriptionCreateMediaTypeEnum mediaType;

  /// Minimum value: 1
  int tmdbId;

  /// Minimum value: 0
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? season;

  /// Minimum value: 0
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? profileId;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? upgrade;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSubscriptionCreate &&
    other.mediaType == mediaType &&
    other.tmdbId == tmdbId &&
    other.season == season &&
    other.profileId == profileId &&
    other.upgrade == upgrade;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (mediaType.hashCode) +
    (tmdbId.hashCode) +
    (season == null ? 0 : season!.hashCode) +
    (profileId == null ? 0 : profileId!.hashCode) +
    (upgrade == null ? 0 : upgrade!.hashCode);

  @override
  String toString() => 'AppSubscriptionCreate[mediaType=$mediaType, tmdbId=$tmdbId, season=$season, profileId=$profileId, upgrade=$upgrade]';

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
    if (this.upgrade != null) {
      json[r'upgrade'] = this.upgrade;
    } else {
      json[r'upgrade'] = null;
    }
    return json;
  }

  /// Returns a new [AppSubscriptionCreate] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSubscriptionCreate? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'media_type'), 'Required key "AppSubscriptionCreate[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppSubscriptionCreate[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'tmdb_id'), 'Required key "AppSubscriptionCreate[tmdb_id]" is missing from JSON.');
        assert(json[r'tmdb_id'] != null, 'Required key "AppSubscriptionCreate[tmdb_id]" has a null value in JSON.');
        return true;
      }());

      return AppSubscriptionCreate(
        mediaType: AppSubscriptionCreateMediaTypeEnum.fromJson(json[r'media_type'])!,
        tmdbId: mapValueOfType<int>(json, r'tmdb_id')!,
        season: mapValueOfType<int>(json, r'season'),
        profileId: mapValueOfType<int>(json, r'profile_id'),
        upgrade: mapValueOfType<bool>(json, r'upgrade'),
      );
    }
    return null;
  }

  static List<AppSubscriptionCreate> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionCreate>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionCreate.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSubscriptionCreate> mapFromJson(dynamic json) {
    final map = <String, AppSubscriptionCreate>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSubscriptionCreate.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSubscriptionCreate-objects as value to a dart map
  static Map<String, List<AppSubscriptionCreate>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSubscriptionCreate>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSubscriptionCreate.listFromJson(entry.value, growable: growable,);
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


enum AppSubscriptionCreateMediaTypeEnum {
  movie._(r'movie'),
  tv._(r'tv'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppSubscriptionCreateMediaTypeEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppSubscriptionCreateMediaTypeEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppSubscriptionCreateMediaTypeEnum? fromJson(dynamic value) => AppSubscriptionCreateMediaTypeEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppSubscriptionCreateMediaTypeEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppSubscriptionCreateMediaTypeEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionCreateMediaTypeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionCreateMediaTypeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppSubscriptionCreateMediaTypeEnum] to String,
/// and [decode] dynamic data back to [AppSubscriptionCreateMediaTypeEnum].
class AppSubscriptionCreateMediaTypeEnumTypeTransformer {
  factory AppSubscriptionCreateMediaTypeEnumTypeTransformer() => _instance ??= const AppSubscriptionCreateMediaTypeEnumTypeTransformer._();

  const AppSubscriptionCreateMediaTypeEnumTypeTransformer._();

  String encode(AppSubscriptionCreateMediaTypeEnum data) => data._value;

  /// Returns the instance of [AppSubscriptionCreateMediaTypeEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppSubscriptionCreateMediaTypeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppSubscriptionCreateMediaTypeEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'movie': return AppSubscriptionCreateMediaTypeEnum.movie;
        case r'tv': return AppSubscriptionCreateMediaTypeEnum.tv;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppSubscriptionCreateMediaTypeEnumTypeTransformer? _instance;
}


