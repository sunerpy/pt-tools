//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSubscriptionUpdate {
  /// Returns a new [AppSubscriptionUpdate] instance.
  AppSubscriptionUpdate({
    this.includeKeywords,
    this.excludeKeywords,
    this.profileId,
    this.upgrade,
    this.sites,
    this.downloaderId,
    this.category,
    this.tags,
    this.savePath,
    this.schedule,
    this.lockVersion,
    this.lockGroup,
    this.lockResolution,
    this.startEpisode,
    this.followNextSeason,
    this.libraryId,
    this.totalEpisodes,
  });

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? includeKeywords;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? excludeKeywords;

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

  /// 只在这些站点找（空数组是全部站点；不给或 null 不变，生成的客户端没设时发 null）
  List<String>? sites;

  /// 推送到哪个下载器（0 用设置里的）
  ///
  /// Minimum value: 0
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? downloaderId;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? category;

  /// 逗号分开
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? tags;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? savePath;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppSchedule? schedule;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? lockVersion;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? lockGroup;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? lockResolution;

  /// Minimum value: 0
  /// Maximum value: 2000
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? startEpisode;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? followNextSeason;

  /// Minimum value: 0
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? libraryId;

  /// 这一季的集数：大于 0 时以它为准，0 改回按 TMDB
  ///
  /// Minimum value: 0
  /// Maximum value: 2000
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? totalEpisodes;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSubscriptionUpdate &&
    other.includeKeywords == includeKeywords &&
    other.excludeKeywords == excludeKeywords &&
    other.profileId == profileId &&
    other.upgrade == upgrade &&
    _deepEquality.equals(other.sites, sites) &&
    other.downloaderId == downloaderId &&
    other.category == category &&
    other.tags == tags &&
    other.savePath == savePath &&
    other.schedule == schedule &&
    other.lockVersion == lockVersion &&
    other.lockGroup == lockGroup &&
    other.lockResolution == lockResolution &&
    other.startEpisode == startEpisode &&
    other.followNextSeason == followNextSeason &&
    other.libraryId == libraryId &&
    other.totalEpisodes == totalEpisodes;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (includeKeywords == null ? 0 : includeKeywords!.hashCode) +
    (excludeKeywords == null ? 0 : excludeKeywords!.hashCode) +
    (profileId == null ? 0 : profileId!.hashCode) +
    (upgrade == null ? 0 : upgrade!.hashCode) +
    (sites == null ? 0 : sites!.hashCode) +
    (downloaderId == null ? 0 : downloaderId!.hashCode) +
    (category == null ? 0 : category!.hashCode) +
    (tags == null ? 0 : tags!.hashCode) +
    (savePath == null ? 0 : savePath!.hashCode) +
    (schedule == null ? 0 : schedule!.hashCode) +
    (lockVersion == null ? 0 : lockVersion!.hashCode) +
    (lockGroup == null ? 0 : lockGroup!.hashCode) +
    (lockResolution == null ? 0 : lockResolution!.hashCode) +
    (startEpisode == null ? 0 : startEpisode!.hashCode) +
    (followNextSeason == null ? 0 : followNextSeason!.hashCode) +
    (libraryId == null ? 0 : libraryId!.hashCode) +
    (totalEpisodes == null ? 0 : totalEpisodes!.hashCode);

  @override
  String toString() => 'AppSubscriptionUpdate[includeKeywords=$includeKeywords, excludeKeywords=$excludeKeywords, profileId=$profileId, upgrade=$upgrade, sites=$sites, downloaderId=$downloaderId, category=$category, tags=$tags, savePath=$savePath, schedule=$schedule, lockVersion=$lockVersion, lockGroup=$lockGroup, lockResolution=$lockResolution, startEpisode=$startEpisode, followNextSeason=$followNextSeason, libraryId=$libraryId, totalEpisodes=$totalEpisodes]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
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
    if (this.sites != null) {
      json[r'sites'] = this.sites;
    } else {
      json[r'sites'] = null;
    }
    if (this.downloaderId != null) {
      json[r'downloader_id'] = this.downloaderId;
    } else {
      json[r'downloader_id'] = null;
    }
    if (this.category != null) {
      json[r'category'] = this.category;
    } else {
      json[r'category'] = null;
    }
    if (this.tags != null) {
      json[r'tags'] = this.tags;
    } else {
      json[r'tags'] = null;
    }
    if (this.savePath != null) {
      json[r'save_path'] = this.savePath;
    } else {
      json[r'save_path'] = null;
    }
    if (this.schedule != null) {
      json[r'schedule'] = this.schedule;
    } else {
      json[r'schedule'] = null;
    }
    if (this.lockVersion != null) {
      json[r'lock_version'] = this.lockVersion;
    } else {
      json[r'lock_version'] = null;
    }
    if (this.lockGroup != null) {
      json[r'lock_group'] = this.lockGroup;
    } else {
      json[r'lock_group'] = null;
    }
    if (this.lockResolution != null) {
      json[r'lock_resolution'] = this.lockResolution;
    } else {
      json[r'lock_resolution'] = null;
    }
    if (this.startEpisode != null) {
      json[r'start_episode'] = this.startEpisode;
    } else {
      json[r'start_episode'] = null;
    }
    if (this.followNextSeason != null) {
      json[r'follow_next_season'] = this.followNextSeason;
    } else {
      json[r'follow_next_season'] = null;
    }
    if (this.libraryId != null) {
      json[r'library_id'] = this.libraryId;
    } else {
      json[r'library_id'] = null;
    }
    if (this.totalEpisodes != null) {
      json[r'total_episodes'] = this.totalEpisodes;
    } else {
      json[r'total_episodes'] = null;
    }
    return json;
  }

  /// Returns a new [AppSubscriptionUpdate] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSubscriptionUpdate? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        return true;
      }());

      return AppSubscriptionUpdate(
        includeKeywords: mapValueOfType<String>(json, r'include_keywords'),
        excludeKeywords: mapValueOfType<String>(json, r'exclude_keywords'),
        profileId: mapValueOfType<int>(json, r'profile_id'),
        upgrade: mapValueOfType<bool>(json, r'upgrade'),
        sites: json[r'sites'] is Iterable
            ? (json[r'sites'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        downloaderId: mapValueOfType<int>(json, r'downloader_id'),
        category: mapValueOfType<String>(json, r'category'),
        tags: mapValueOfType<String>(json, r'tags'),
        savePath: mapValueOfType<String>(json, r'save_path'),
        schedule: AppSchedule.fromJson(json[r'schedule']),
        lockVersion: mapValueOfType<bool>(json, r'lock_version'),
        lockGroup: mapValueOfType<String>(json, r'lock_group'),
        lockResolution: mapValueOfType<String>(json, r'lock_resolution'),
        startEpisode: mapValueOfType<int>(json, r'start_episode'),
        followNextSeason: mapValueOfType<bool>(json, r'follow_next_season'),
        libraryId: mapValueOfType<int>(json, r'library_id'),
        totalEpisodes: mapValueOfType<int>(json, r'total_episodes'),
      );
    }
    return null;
  }

  static List<AppSubscriptionUpdate> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionUpdate>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionUpdate.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSubscriptionUpdate> mapFromJson(dynamic json) {
    final map = <String, AppSubscriptionUpdate>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSubscriptionUpdate.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSubscriptionUpdate-objects as value to a dart map
  static Map<String, List<AppSubscriptionUpdate>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSubscriptionUpdate>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSubscriptionUpdate.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
  };
}

