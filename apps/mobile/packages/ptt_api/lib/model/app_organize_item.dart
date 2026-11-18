//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppOrganizeItem {
  /// Returns a new [AppOrganizeItem] instance.
  AppOrganizeItem({
    required this.rel,
    required this.size,
    this.target,
    this.season,
    this.episode,
    this.episodeEnd,
    required this.status,
    this.message,
    this.historyId,
    this.priorTarget,
    this.priorMediaType,
    this.priorTmdbId,
    this.priorTitle,
  });

  /// 种子里的相对路径
  String rel;

  int size;

  /// 放进库里的位置
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? target;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? season;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? episode;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? episodeEnd;

  /// pending 要整理，done 库里已经有了，exists 目标被别的文件占着（不覆盖），replace 换掉 pt-tools 之前放进库的旧版本，change 之前整理过、这次条目变了，换到新位置（prior_* 是之前的），failed 整理不了。条目变了、这个文件不换时是 done，message 写明原因
  AppOrganizeItemStatusEnum status;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? message;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? historyId;

  /// 之前整理时放进库里的位置（这次条目变了时才有）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? priorTarget;

  /// 之前整理时的条目类型
  AppOrganizeItemPriorMediaTypeEnum? priorMediaType;

  /// 之前整理时的 TMDB 编号
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? priorTmdbId;

  /// 之前整理时的条目名
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? priorTitle;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppOrganizeItem &&
    other.rel == rel &&
    other.size == size &&
    other.target == target &&
    other.season == season &&
    other.episode == episode &&
    other.episodeEnd == episodeEnd &&
    other.status == status &&
    other.message == message &&
    other.historyId == historyId &&
    other.priorTarget == priorTarget &&
    other.priorMediaType == priorMediaType &&
    other.priorTmdbId == priorTmdbId &&
    other.priorTitle == priorTitle;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (rel.hashCode) +
    (size.hashCode) +
    (target == null ? 0 : target!.hashCode) +
    (season == null ? 0 : season!.hashCode) +
    (episode == null ? 0 : episode!.hashCode) +
    (episodeEnd == null ? 0 : episodeEnd!.hashCode) +
    (status.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (historyId == null ? 0 : historyId!.hashCode) +
    (priorTarget == null ? 0 : priorTarget!.hashCode) +
    (priorMediaType == null ? 0 : priorMediaType!.hashCode) +
    (priorTmdbId == null ? 0 : priorTmdbId!.hashCode) +
    (priorTitle == null ? 0 : priorTitle!.hashCode);

  @override
  String toString() => 'AppOrganizeItem[rel=$rel, size=$size, target=$target, season=$season, episode=$episode, episodeEnd=$episodeEnd, status=$status, message=$message, historyId=$historyId, priorTarget=$priorTarget, priorMediaType=$priorMediaType, priorTmdbId=$priorTmdbId, priorTitle=$priorTitle]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'rel'] = this.rel;
      json[r'size'] = this.size;
    if (this.target != null) {
      json[r'target'] = this.target;
    } else {
      json[r'target'] = null;
    }
    if (this.season != null) {
      json[r'season'] = this.season;
    } else {
      json[r'season'] = null;
    }
    if (this.episode != null) {
      json[r'episode'] = this.episode;
    } else {
      json[r'episode'] = null;
    }
    if (this.episodeEnd != null) {
      json[r'episode_end'] = this.episodeEnd;
    } else {
      json[r'episode_end'] = null;
    }
      json[r'status'] = this.status;
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
    }
    if (this.historyId != null) {
      json[r'history_id'] = this.historyId;
    } else {
      json[r'history_id'] = null;
    }
    if (this.priorTarget != null) {
      json[r'prior_target'] = this.priorTarget;
    } else {
      json[r'prior_target'] = null;
    }
    if (this.priorMediaType != null) {
      json[r'prior_media_type'] = this.priorMediaType;
    } else {
      json[r'prior_media_type'] = null;
    }
    if (this.priorTmdbId != null) {
      json[r'prior_tmdb_id'] = this.priorTmdbId;
    } else {
      json[r'prior_tmdb_id'] = null;
    }
    if (this.priorTitle != null) {
      json[r'prior_title'] = this.priorTitle;
    } else {
      json[r'prior_title'] = null;
    }
    return json;
  }

  /// Returns a new [AppOrganizeItem] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppOrganizeItem? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'rel'), 'Required key "AppOrganizeItem[rel]" is missing from JSON.');
        assert(json[r'rel'] != null, 'Required key "AppOrganizeItem[rel]" has a null value in JSON.');
        assert(json.containsKey(r'size'), 'Required key "AppOrganizeItem[size]" is missing from JSON.');
        assert(json[r'size'] != null, 'Required key "AppOrganizeItem[size]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppOrganizeItem[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppOrganizeItem[status]" has a null value in JSON.');
        return true;
      }());

      return AppOrganizeItem(
        rel: mapValueOfType<String>(json, r'rel')!,
        size: mapValueOfType<int>(json, r'size')!,
        target: mapValueOfType<String>(json, r'target'),
        season: mapValueOfType<int>(json, r'season'),
        episode: mapValueOfType<int>(json, r'episode'),
        episodeEnd: mapValueOfType<int>(json, r'episode_end'),
        status: AppOrganizeItemStatusEnum.fromJson(json[r'status'])!,
        message: mapValueOfType<String>(json, r'message'),
        historyId: mapValueOfType<int>(json, r'history_id'),
        priorTarget: mapValueOfType<String>(json, r'prior_target'),
        priorMediaType: AppOrganizeItemPriorMediaTypeEnum.fromJson(json[r'prior_media_type']),
        priorTmdbId: mapValueOfType<int>(json, r'prior_tmdb_id'),
        priorTitle: mapValueOfType<String>(json, r'prior_title'),
      );
    }
    return null;
  }

  static List<AppOrganizeItem> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppOrganizeItem>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppOrganizeItem.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppOrganizeItem> mapFromJson(dynamic json) {
    final map = <String, AppOrganizeItem>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppOrganizeItem.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppOrganizeItem-objects as value to a dart map
  static Map<String, List<AppOrganizeItem>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppOrganizeItem>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppOrganizeItem.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'rel',
    'size',
    'status',
  };
}

/// pending 要整理，done 库里已经有了，exists 目标被别的文件占着（不覆盖），replace 换掉 pt-tools 之前放进库的旧版本，change 之前整理过、这次条目变了，换到新位置（prior_* 是之前的），failed 整理不了。条目变了、这个文件不换时是 done，message 写明原因
enum AppOrganizeItemStatusEnum {
  pending._(r'pending'),
  done._(r'done'),
  exists._(r'exists'),
  replace._(r'replace'),
  change._(r'change'),
  failed._(r'failed'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppOrganizeItemStatusEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppOrganizeItemStatusEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppOrganizeItemStatusEnum? fromJson(dynamic value) => AppOrganizeItemStatusEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppOrganizeItemStatusEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppOrganizeItemStatusEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppOrganizeItemStatusEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppOrganizeItemStatusEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppOrganizeItemStatusEnum] to String,
/// and [decode] dynamic data back to [AppOrganizeItemStatusEnum].
class AppOrganizeItemStatusEnumTypeTransformer {
  factory AppOrganizeItemStatusEnumTypeTransformer() => _instance ??= const AppOrganizeItemStatusEnumTypeTransformer._();

  const AppOrganizeItemStatusEnumTypeTransformer._();

  String encode(AppOrganizeItemStatusEnum data) => data._value;

  /// Returns the instance of [AppOrganizeItemStatusEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppOrganizeItemStatusEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppOrganizeItemStatusEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'pending': return AppOrganizeItemStatusEnum.pending;
        case r'done': return AppOrganizeItemStatusEnum.done;
        case r'exists': return AppOrganizeItemStatusEnum.exists;
        case r'replace': return AppOrganizeItemStatusEnum.replace;
        case r'change': return AppOrganizeItemStatusEnum.change;
        case r'failed': return AppOrganizeItemStatusEnum.failed;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppOrganizeItemStatusEnumTypeTransformer? _instance;
}


/// 之前整理时的条目类型
enum AppOrganizeItemPriorMediaTypeEnum {
  movie._(r'movie'),
  tv._(r'tv'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppOrganizeItemPriorMediaTypeEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppOrganizeItemPriorMediaTypeEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppOrganizeItemPriorMediaTypeEnum? fromJson(dynamic value) => AppOrganizeItemPriorMediaTypeEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppOrganizeItemPriorMediaTypeEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppOrganizeItemPriorMediaTypeEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppOrganizeItemPriorMediaTypeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppOrganizeItemPriorMediaTypeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppOrganizeItemPriorMediaTypeEnum] to String,
/// and [decode] dynamic data back to [AppOrganizeItemPriorMediaTypeEnum].
class AppOrganizeItemPriorMediaTypeEnumTypeTransformer {
  factory AppOrganizeItemPriorMediaTypeEnumTypeTransformer() => _instance ??= const AppOrganizeItemPriorMediaTypeEnumTypeTransformer._();

  const AppOrganizeItemPriorMediaTypeEnumTypeTransformer._();

  String encode(AppOrganizeItemPriorMediaTypeEnum data) => data._value;

  /// Returns the instance of [AppOrganizeItemPriorMediaTypeEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppOrganizeItemPriorMediaTypeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppOrganizeItemPriorMediaTypeEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'movie': return AppOrganizeItemPriorMediaTypeEnum.movie;
        case r'tv': return AppOrganizeItemPriorMediaTypeEnum.tv;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppOrganizeItemPriorMediaTypeEnumTypeTransformer? _instance;
}


