//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTask {
  /// Returns a new [AppTask] instance.
  AppTask({
    required this.id,
    required this.site,
    required this.torrentId,
    required this.title,
    required this.size,
    this.category,
    this.tags,
    required this.free,
    this.freeLevel,
    this.freeEndAt,
    required this.hasHr,
    required this.pushed,
    this.pushedAt,
    this.downloader,
    required this.progress,
    required this.completed,
    this.completedAt,
    this.source_,
    this.error,
    required this.createdAt,
  });

  int id;

  String site;

  String torrentId;

  String title;

  int size;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? category;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? tags;

  bool free;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? freeLevel;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? freeEndAt;

  bool hasHr;

  bool pushed;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? pushedAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? downloader;

  double progress;

  bool completed;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? completedAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? source_;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? error;

  int createdAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTask &&
    other.id == id &&
    other.site == site &&
    other.torrentId == torrentId &&
    other.title == title &&
    other.size == size &&
    other.category == category &&
    other.tags == tags &&
    other.free == free &&
    other.freeLevel == freeLevel &&
    other.freeEndAt == freeEndAt &&
    other.hasHr == hasHr &&
    other.pushed == pushed &&
    other.pushedAt == pushedAt &&
    other.downloader == downloader &&
    other.progress == progress &&
    other.completed == completed &&
    other.completedAt == completedAt &&
    other.source_ == source_ &&
    other.error == error &&
    other.createdAt == createdAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (site.hashCode) +
    (torrentId.hashCode) +
    (title.hashCode) +
    (size.hashCode) +
    (category == null ? 0 : category!.hashCode) +
    (tags == null ? 0 : tags!.hashCode) +
    (free.hashCode) +
    (freeLevel == null ? 0 : freeLevel!.hashCode) +
    (freeEndAt == null ? 0 : freeEndAt!.hashCode) +
    (hasHr.hashCode) +
    (pushed.hashCode) +
    (pushedAt == null ? 0 : pushedAt!.hashCode) +
    (downloader == null ? 0 : downloader!.hashCode) +
    (progress.hashCode) +
    (completed.hashCode) +
    (completedAt == null ? 0 : completedAt!.hashCode) +
    (source_ == null ? 0 : source_!.hashCode) +
    (error == null ? 0 : error!.hashCode) +
    (createdAt.hashCode);

  @override
  String toString() => 'AppTask[id=$id, site=$site, torrentId=$torrentId, title=$title, size=$size, category=$category, tags=$tags, free=$free, freeLevel=$freeLevel, freeEndAt=$freeEndAt, hasHr=$hasHr, pushed=$pushed, pushedAt=$pushedAt, downloader=$downloader, progress=$progress, completed=$completed, completedAt=$completedAt, source_=$source_, error=$error, createdAt=$createdAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'site'] = this.site;
      json[r'torrent_id'] = this.torrentId;
      json[r'title'] = this.title;
      json[r'size'] = this.size;
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
      json[r'free'] = this.free;
    if (this.freeLevel != null) {
      json[r'free_level'] = this.freeLevel;
    } else {
      json[r'free_level'] = null;
    }
    if (this.freeEndAt != null) {
      json[r'free_end_at'] = this.freeEndAt;
    } else {
      json[r'free_end_at'] = null;
    }
      json[r'has_hr'] = this.hasHr;
      json[r'pushed'] = this.pushed;
    if (this.pushedAt != null) {
      json[r'pushed_at'] = this.pushedAt;
    } else {
      json[r'pushed_at'] = null;
    }
    if (this.downloader != null) {
      json[r'downloader'] = this.downloader;
    } else {
      json[r'downloader'] = null;
    }
      json[r'progress'] = this.progress;
      json[r'completed'] = this.completed;
    if (this.completedAt != null) {
      json[r'completed_at'] = this.completedAt;
    } else {
      json[r'completed_at'] = null;
    }
    if (this.source_ != null) {
      json[r'source'] = this.source_;
    } else {
      json[r'source'] = null;
    }
    if (this.error != null) {
      json[r'error'] = this.error;
    } else {
      json[r'error'] = null;
    }
      json[r'created_at'] = this.createdAt;
    return json;
  }

  /// Returns a new [AppTask] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTask? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppTask[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppTask[id]" has a null value in JSON.');
        assert(json.containsKey(r'site'), 'Required key "AppTask[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppTask[site]" has a null value in JSON.');
        assert(json.containsKey(r'torrent_id'), 'Required key "AppTask[torrent_id]" is missing from JSON.');
        assert(json[r'torrent_id'] != null, 'Required key "AppTask[torrent_id]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppTask[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppTask[title]" has a null value in JSON.');
        assert(json.containsKey(r'size'), 'Required key "AppTask[size]" is missing from JSON.');
        assert(json[r'size'] != null, 'Required key "AppTask[size]" has a null value in JSON.');
        assert(json.containsKey(r'free'), 'Required key "AppTask[free]" is missing from JSON.');
        assert(json[r'free'] != null, 'Required key "AppTask[free]" has a null value in JSON.');
        assert(json.containsKey(r'has_hr'), 'Required key "AppTask[has_hr]" is missing from JSON.');
        assert(json[r'has_hr'] != null, 'Required key "AppTask[has_hr]" has a null value in JSON.');
        assert(json.containsKey(r'pushed'), 'Required key "AppTask[pushed]" is missing from JSON.');
        assert(json[r'pushed'] != null, 'Required key "AppTask[pushed]" has a null value in JSON.');
        assert(json.containsKey(r'progress'), 'Required key "AppTask[progress]" is missing from JSON.');
        assert(json[r'progress'] != null, 'Required key "AppTask[progress]" has a null value in JSON.');
        assert(json.containsKey(r'completed'), 'Required key "AppTask[completed]" is missing from JSON.');
        assert(json[r'completed'] != null, 'Required key "AppTask[completed]" has a null value in JSON.');
        assert(json.containsKey(r'created_at'), 'Required key "AppTask[created_at]" is missing from JSON.');
        assert(json[r'created_at'] != null, 'Required key "AppTask[created_at]" has a null value in JSON.');
        return true;
      }());

      return AppTask(
        id: mapValueOfType<int>(json, r'id')!,
        site: mapValueOfType<String>(json, r'site')!,
        torrentId: mapValueOfType<String>(json, r'torrent_id')!,
        title: mapValueOfType<String>(json, r'title')!,
        size: mapValueOfType<int>(json, r'size')!,
        category: mapValueOfType<String>(json, r'category'),
        tags: mapValueOfType<String>(json, r'tags'),
        free: mapValueOfType<bool>(json, r'free')!,
        freeLevel: mapValueOfType<String>(json, r'free_level'),
        freeEndAt: mapValueOfType<int>(json, r'free_end_at'),
        hasHr: mapValueOfType<bool>(json, r'has_hr')!,
        pushed: mapValueOfType<bool>(json, r'pushed')!,
        pushedAt: mapValueOfType<int>(json, r'pushed_at'),
        downloader: mapValueOfType<String>(json, r'downloader'),
        progress: mapValueOfType<double>(json, r'progress')!,
        completed: mapValueOfType<bool>(json, r'completed')!,
        completedAt: mapValueOfType<int>(json, r'completed_at'),
        source_: mapValueOfType<String>(json, r'source'),
        error: mapValueOfType<String>(json, r'error'),
        createdAt: mapValueOfType<int>(json, r'created_at')!,
      );
    }
    return null;
  }

  static List<AppTask> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTask>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTask.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTask> mapFromJson(dynamic json) {
    final map = <String, AppTask>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTask.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTask-objects as value to a dart map
  static Map<String, List<AppTask>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTask>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTask.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'site',
    'torrent_id',
    'title',
    'size',
    'free',
    'has_hr',
    'pushed',
    'progress',
    'completed',
    'created_at',
  };
}

