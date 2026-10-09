//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSubscription {
  /// Returns a new [AppSubscription] instance.
  AppSubscription({
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

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSubscription &&
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
    other.createdAt == createdAt;

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
    (createdAt.hashCode);

  @override
  String toString() => 'AppSubscription[id=$id, mediaType=$mediaType, tmdbId=$tmdbId, season=$season, title=$title, originalTitle=$originalTitle, year=$year, posterPath=$posterPath, status=$status, upgrade=$upgrade, source_=$source_, totalEpisodes=$totalEpisodes, progress=$progress, message=$message, lastSearchAt=$lastSearchAt, nextSearchAt=$nextSearchAt, createdAt=$createdAt]';

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
    return json;
  }

  /// Returns a new [AppSubscription] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSubscription? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppSubscription[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppSubscription[id]" has a null value in JSON.');
        assert(json.containsKey(r'media_type'), 'Required key "AppSubscription[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppSubscription[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'tmdb_id'), 'Required key "AppSubscription[tmdb_id]" is missing from JSON.');
        assert(json[r'tmdb_id'] != null, 'Required key "AppSubscription[tmdb_id]" has a null value in JSON.');
        assert(json.containsKey(r'season'), 'Required key "AppSubscription[season]" is missing from JSON.');
        assert(json[r'season'] != null, 'Required key "AppSubscription[season]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppSubscription[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppSubscription[title]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppSubscription[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppSubscription[status]" has a null value in JSON.');
        assert(json.containsKey(r'upgrade'), 'Required key "AppSubscription[upgrade]" is missing from JSON.');
        assert(json[r'upgrade'] != null, 'Required key "AppSubscription[upgrade]" has a null value in JSON.');
        assert(json.containsKey(r'source'), 'Required key "AppSubscription[source]" is missing from JSON.');
        assert(json[r'source'] != null, 'Required key "AppSubscription[source]" has a null value in JSON.');
        assert(json.containsKey(r'progress'), 'Required key "AppSubscription[progress]" is missing from JSON.');
        assert(json[r'progress'] != null, 'Required key "AppSubscription[progress]" has a null value in JSON.');
        assert(json.containsKey(r'created_at'), 'Required key "AppSubscription[created_at]" is missing from JSON.');
        assert(json[r'created_at'] != null, 'Required key "AppSubscription[created_at]" has a null value in JSON.');
        return true;
      }());

      return AppSubscription(
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
      );
    }
    return null;
  }

  static List<AppSubscription> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscription>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscription.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSubscription> mapFromJson(dynamic json) {
    final map = <String, AppSubscription>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSubscription.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSubscription-objects as value to a dart map
  static Map<String, List<AppSubscription>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSubscription>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSubscription.listFromJson(entry.value, growable: growable,);
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
  };
}

