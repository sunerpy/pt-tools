//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSubscriptionDetail {
  /// Returns a new [AppSubscriptionDetail] instance.
  AppSubscriptionDetail({
    required this.id,
    required this.mediaType,
    required this.tmdbId,
    required this.season,
    required this.title,
    this.originalTitle,
    this.year,
    this.posterPath,
    required this.status,
    required this.upgrade,
    required this.source_,
    this.totalEpisodes,
    required this.progress,
    this.message,
    this.lastSearchAt,
    this.nextSearchAt,
    required this.createdAt,
    this.profileId,
    this.sites = const [],
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
    this.nextSeasonId,
    this.libraryId,
    this.completedAt,
    this.episodes = const [],
    this.torrents = const [],
  });

  int id;

  /// movie 或 tv
  String mediaType;

  int tmdbId;

  int season;

  String title;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? originalTitle;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? year;

  /// TMDB 的海报路径，经 /images/tmdb/{size}/{file} 取图
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? posterPath;

  String status;

  bool upgrade;

  String source_;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? totalEpisodes;

  AppProgress progress;

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
  int? lastSearchAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? nextSearchAt;

  int createdAt;

  /// 质量档案（0 用设置里的默认）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? profileId;

  /// 只在这些站点找（空的是全部站点）
  List<String> sites;

  /// 推送到哪个下载器（0 用设置里的）
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

  /// 推送时的标签，逗号分开
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? tags;

  /// 推送时的保存目录（空的按下载器的默认）
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

  /// 版本锁：第一集选定以后只要同一个制作组、同一个分辨率
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? lockVersion;

  /// 锁住的制作组（还没定时为空）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? lockGroup;

  /// 锁住的分辨率（还没定时为空）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? lockResolution;

  /// 从第几集开始追（0 与 1 是整季）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? startEpisode;

  /// 追完以后自动追下一季
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? followNextSeason;

  /// 自动建的下一季的订阅
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? nextSeasonId;

  /// 整理进哪个媒体库（0 按类型自动选）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? libraryId;

  /// 追完的时间
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? completedAt;

  List<AppEpisode> episodes;

  List<AppSubscriptionTorrent> torrents;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSubscriptionDetail &&
    other.id == id &&
    other.mediaType == mediaType &&
    other.tmdbId == tmdbId &&
    other.season == season &&
    other.title == title &&
    other.originalTitle == originalTitle &&
    other.year == year &&
    other.posterPath == posterPath &&
    other.status == status &&
    other.upgrade == upgrade &&
    other.source_ == source_ &&
    other.totalEpisodes == totalEpisodes &&
    other.progress == progress &&
    other.message == message &&
    other.lastSearchAt == lastSearchAt &&
    other.nextSearchAt == nextSearchAt &&
    other.createdAt == createdAt &&
    other.profileId == profileId &&
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
    other.nextSeasonId == nextSeasonId &&
    other.libraryId == libraryId &&
    other.completedAt == completedAt &&
    _deepEquality.equals(other.episodes, episodes) &&
    _deepEquality.equals(other.torrents, torrents);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (mediaType.hashCode) +
    (tmdbId.hashCode) +
    (season.hashCode) +
    (title.hashCode) +
    (originalTitle == null ? 0 : originalTitle!.hashCode) +
    (year == null ? 0 : year!.hashCode) +
    (posterPath == null ? 0 : posterPath!.hashCode) +
    (status.hashCode) +
    (upgrade.hashCode) +
    (source_.hashCode) +
    (totalEpisodes == null ? 0 : totalEpisodes!.hashCode) +
    (progress.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (lastSearchAt == null ? 0 : lastSearchAt!.hashCode) +
    (nextSearchAt == null ? 0 : nextSearchAt!.hashCode) +
    (createdAt.hashCode) +
    (profileId == null ? 0 : profileId!.hashCode) +
    (sites.hashCode) +
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
    (nextSeasonId == null ? 0 : nextSeasonId!.hashCode) +
    (libraryId == null ? 0 : libraryId!.hashCode) +
    (completedAt == null ? 0 : completedAt!.hashCode) +
    (episodes.hashCode) +
    (torrents.hashCode);

  @override
  String toString() => 'AppSubscriptionDetail[id=$id, mediaType=$mediaType, tmdbId=$tmdbId, season=$season, title=$title, originalTitle=$originalTitle, year=$year, posterPath=$posterPath, status=$status, upgrade=$upgrade, source_=$source_, totalEpisodes=$totalEpisodes, progress=$progress, message=$message, lastSearchAt=$lastSearchAt, nextSearchAt=$nextSearchAt, createdAt=$createdAt, profileId=$profileId, sites=$sites, downloaderId=$downloaderId, category=$category, tags=$tags, savePath=$savePath, schedule=$schedule, lockVersion=$lockVersion, lockGroup=$lockGroup, lockResolution=$lockResolution, startEpisode=$startEpisode, followNextSeason=$followNextSeason, nextSeasonId=$nextSeasonId, libraryId=$libraryId, completedAt=$completedAt, episodes=$episodes, torrents=$torrents]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'media_type'] = this.mediaType;
      json[r'tmdb_id'] = this.tmdbId;
      json[r'season'] = this.season;
      json[r'title'] = this.title;
    if (this.originalTitle != null) {
      json[r'original_title'] = this.originalTitle;
    } else {
      json[r'original_title'] = null;
    }
    if (this.year != null) {
      json[r'year'] = this.year;
    } else {
      json[r'year'] = null;
    }
    if (this.posterPath != null) {
      json[r'poster_path'] = this.posterPath;
    } else {
      json[r'poster_path'] = null;
    }
      json[r'status'] = this.status;
      json[r'upgrade'] = this.upgrade;
      json[r'source'] = this.source_;
    if (this.totalEpisodes != null) {
      json[r'total_episodes'] = this.totalEpisodes;
    } else {
      json[r'total_episodes'] = null;
    }
      json[r'progress'] = this.progress;
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
    }
    if (this.lastSearchAt != null) {
      json[r'last_search_at'] = this.lastSearchAt;
    } else {
      json[r'last_search_at'] = null;
    }
    if (this.nextSearchAt != null) {
      json[r'next_search_at'] = this.nextSearchAt;
    } else {
      json[r'next_search_at'] = null;
    }
      json[r'created_at'] = this.createdAt;
    if (this.profileId != null) {
      json[r'profile_id'] = this.profileId;
    } else {
      json[r'profile_id'] = null;
    }
      json[r'sites'] = this.sites;
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
    if (this.nextSeasonId != null) {
      json[r'next_season_id'] = this.nextSeasonId;
    } else {
      json[r'next_season_id'] = null;
    }
    if (this.libraryId != null) {
      json[r'library_id'] = this.libraryId;
    } else {
      json[r'library_id'] = null;
    }
    if (this.completedAt != null) {
      json[r'completed_at'] = this.completedAt;
    } else {
      json[r'completed_at'] = null;
    }
      json[r'episodes'] = this.episodes;
      json[r'torrents'] = this.torrents;
    return json;
  }

  /// Returns a new [AppSubscriptionDetail] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSubscriptionDetail? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppSubscriptionDetail[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppSubscriptionDetail[id]" has a null value in JSON.');
        assert(json.containsKey(r'media_type'), 'Required key "AppSubscriptionDetail[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppSubscriptionDetail[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'tmdb_id'), 'Required key "AppSubscriptionDetail[tmdb_id]" is missing from JSON.');
        assert(json[r'tmdb_id'] != null, 'Required key "AppSubscriptionDetail[tmdb_id]" has a null value in JSON.');
        assert(json.containsKey(r'season'), 'Required key "AppSubscriptionDetail[season]" is missing from JSON.');
        assert(json[r'season'] != null, 'Required key "AppSubscriptionDetail[season]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppSubscriptionDetail[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppSubscriptionDetail[title]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppSubscriptionDetail[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppSubscriptionDetail[status]" has a null value in JSON.');
        assert(json.containsKey(r'upgrade'), 'Required key "AppSubscriptionDetail[upgrade]" is missing from JSON.');
        assert(json[r'upgrade'] != null, 'Required key "AppSubscriptionDetail[upgrade]" has a null value in JSON.');
        assert(json.containsKey(r'source'), 'Required key "AppSubscriptionDetail[source]" is missing from JSON.');
        assert(json[r'source'] != null, 'Required key "AppSubscriptionDetail[source]" has a null value in JSON.');
        assert(json.containsKey(r'progress'), 'Required key "AppSubscriptionDetail[progress]" is missing from JSON.');
        assert(json[r'progress'] != null, 'Required key "AppSubscriptionDetail[progress]" has a null value in JSON.');
        assert(json.containsKey(r'created_at'), 'Required key "AppSubscriptionDetail[created_at]" is missing from JSON.');
        assert(json[r'created_at'] != null, 'Required key "AppSubscriptionDetail[created_at]" has a null value in JSON.');
        assert(json.containsKey(r'episodes'), 'Required key "AppSubscriptionDetail[episodes]" is missing from JSON.');
        assert(json[r'episodes'] != null, 'Required key "AppSubscriptionDetail[episodes]" has a null value in JSON.');
        assert(json.containsKey(r'torrents'), 'Required key "AppSubscriptionDetail[torrents]" is missing from JSON.');
        assert(json[r'torrents'] != null, 'Required key "AppSubscriptionDetail[torrents]" has a null value in JSON.');
        return true;
      }());

      return AppSubscriptionDetail(
        id: mapValueOfType<int>(json, r'id')!,
        mediaType: mapValueOfType<String>(json, r'media_type')!,
        tmdbId: mapValueOfType<int>(json, r'tmdb_id')!,
        season: mapValueOfType<int>(json, r'season')!,
        title: mapValueOfType<String>(json, r'title')!,
        originalTitle: mapValueOfType<String>(json, r'original_title'),
        year: mapValueOfType<int>(json, r'year'),
        posterPath: mapValueOfType<String>(json, r'poster_path'),
        status: mapValueOfType<String>(json, r'status')!,
        upgrade: mapValueOfType<bool>(json, r'upgrade')!,
        source_: mapValueOfType<String>(json, r'source')!,
        totalEpisodes: mapValueOfType<int>(json, r'total_episodes'),
        progress: AppProgress.fromJson(json[r'progress'])!,
        message: mapValueOfType<String>(json, r'message'),
        lastSearchAt: mapValueOfType<int>(json, r'last_search_at'),
        nextSearchAt: mapValueOfType<int>(json, r'next_search_at'),
        createdAt: mapValueOfType<int>(json, r'created_at')!,
        profileId: mapValueOfType<int>(json, r'profile_id'),
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
        nextSeasonId: mapValueOfType<int>(json, r'next_season_id'),
        libraryId: mapValueOfType<int>(json, r'library_id'),
        completedAt: mapValueOfType<int>(json, r'completed_at'),
        episodes: AppEpisode.listFromJson(json[r'episodes']),
        torrents: AppSubscriptionTorrent.listFromJson(json[r'torrents']),
      );
    }
    return null;
  }

  static List<AppSubscriptionDetail> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionDetail>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionDetail.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSubscriptionDetail> mapFromJson(dynamic json) {
    final map = <String, AppSubscriptionDetail>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSubscriptionDetail.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSubscriptionDetail-objects as value to a dart map
  static Map<String, List<AppSubscriptionDetail>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSubscriptionDetail>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSubscriptionDetail.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'media_type',
    'tmdb_id',
    'season',
    'title',
    'status',
    'upgrade',
    'source',
    'progress',
    'created_at',
    'episodes',
    'torrents',
  };
}

