//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMediaHistory {
  /// Returns a new [AppMediaHistory] instance.
  AppMediaHistory({
    required this.id,
    required this.mediaType,
    required this.tmdbId,
    required this.title,
    this.year,
    this.season,
    this.episode,
    this.episodeEnd,
    required this.torrentName,
    this.library_,
    this.targetPath,
    required this.mode,
    required this.size,
    required this.status,
    this.message,
    required this.createdAt,
  });

  int id;

  String mediaType;

  int tmdbId;

  String title;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? year;

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

  String torrentName;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? library_;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? targetPath;

  String mode;

  int size;

  String status;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? message;

  int createdAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMediaHistory &&
    other.id == id &&
    other.mediaType == mediaType &&
    other.tmdbId == tmdbId &&
    other.title == title &&
    other.year == year &&
    other.season == season &&
    other.episode == episode &&
    other.episodeEnd == episodeEnd &&
    other.torrentName == torrentName &&
    other.library_ == library_ &&
    other.targetPath == targetPath &&
    other.mode == mode &&
    other.size == size &&
    other.status == status &&
    other.message == message &&
    other.createdAt == createdAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (mediaType.hashCode) +
    (tmdbId.hashCode) +
    (title.hashCode) +
    (year == null ? 0 : year!.hashCode) +
    (season == null ? 0 : season!.hashCode) +
    (episode == null ? 0 : episode!.hashCode) +
    (episodeEnd == null ? 0 : episodeEnd!.hashCode) +
    (torrentName.hashCode) +
    (library_ == null ? 0 : library_!.hashCode) +
    (targetPath == null ? 0 : targetPath!.hashCode) +
    (mode.hashCode) +
    (size.hashCode) +
    (status.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (createdAt.hashCode);

  @override
  String toString() => 'AppMediaHistory[id=$id, mediaType=$mediaType, tmdbId=$tmdbId, title=$title, year=$year, season=$season, episode=$episode, episodeEnd=$episodeEnd, torrentName=$torrentName, library_=$library_, targetPath=$targetPath, mode=$mode, size=$size, status=$status, message=$message, createdAt=$createdAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'media_type'] = this.mediaType;
      json[r'tmdb_id'] = this.tmdbId;
      json[r'title'] = this.title;
    if (this.year != null) {
      json[r'year'] = this.year;
    } else {
      json[r'year'] = null;
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
      json[r'torrent_name'] = this.torrentName;
    if (this.library_ != null) {
      json[r'library'] = this.library_;
    } else {
      json[r'library'] = null;
    }
    if (this.targetPath != null) {
      json[r'target_path'] = this.targetPath;
    } else {
      json[r'target_path'] = null;
    }
      json[r'mode'] = this.mode;
      json[r'size'] = this.size;
      json[r'status'] = this.status;
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
    }
      json[r'created_at'] = this.createdAt;
    return json;
  }

  /// Returns a new [AppMediaHistory] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMediaHistory? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppMediaHistory[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppMediaHistory[id]" has a null value in JSON.');
        assert(json.containsKey(r'media_type'), 'Required key "AppMediaHistory[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppMediaHistory[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'tmdb_id'), 'Required key "AppMediaHistory[tmdb_id]" is missing from JSON.');
        assert(json[r'tmdb_id'] != null, 'Required key "AppMediaHistory[tmdb_id]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppMediaHistory[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppMediaHistory[title]" has a null value in JSON.');
        assert(json.containsKey(r'torrent_name'), 'Required key "AppMediaHistory[torrent_name]" is missing from JSON.');
        assert(json[r'torrent_name'] != null, 'Required key "AppMediaHistory[torrent_name]" has a null value in JSON.');
        assert(json.containsKey(r'mode'), 'Required key "AppMediaHistory[mode]" is missing from JSON.');
        assert(json[r'mode'] != null, 'Required key "AppMediaHistory[mode]" has a null value in JSON.');
        assert(json.containsKey(r'size'), 'Required key "AppMediaHistory[size]" is missing from JSON.');
        assert(json[r'size'] != null, 'Required key "AppMediaHistory[size]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppMediaHistory[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppMediaHistory[status]" has a null value in JSON.');
        assert(json.containsKey(r'created_at'), 'Required key "AppMediaHistory[created_at]" is missing from JSON.');
        assert(json[r'created_at'] != null, 'Required key "AppMediaHistory[created_at]" has a null value in JSON.');
        return true;
      }());

      return AppMediaHistory(
        id: mapValueOfType<int>(json, r'id')!,
        mediaType: mapValueOfType<String>(json, r'media_type')!,
        tmdbId: mapValueOfType<int>(json, r'tmdb_id')!,
        title: mapValueOfType<String>(json, r'title')!,
        year: mapValueOfType<int>(json, r'year'),
        season: mapValueOfType<int>(json, r'season'),
        episode: mapValueOfType<int>(json, r'episode'),
        episodeEnd: mapValueOfType<int>(json, r'episode_end'),
        torrentName: mapValueOfType<String>(json, r'torrent_name')!,
        library_: mapValueOfType<String>(json, r'library'),
        targetPath: mapValueOfType<String>(json, r'target_path'),
        mode: mapValueOfType<String>(json, r'mode')!,
        size: mapValueOfType<int>(json, r'size')!,
        status: mapValueOfType<String>(json, r'status')!,
        message: mapValueOfType<String>(json, r'message'),
        createdAt: mapValueOfType<int>(json, r'created_at')!,
      );
    }
    return null;
  }

  static List<AppMediaHistory> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMediaHistory>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMediaHistory.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMediaHistory> mapFromJson(dynamic json) {
    final map = <String, AppMediaHistory>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMediaHistory.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMediaHistory-objects as value to a dart map
  static Map<String, List<AppMediaHistory>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMediaHistory>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMediaHistory.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'media_type',
    'tmdb_id',
    'title',
    'torrent_name',
    'mode',
    'size',
    'status',
    'created_at',
  };
}

