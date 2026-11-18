//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTorrent {
  /// Returns a new [AppTorrent] instance.
  AppTorrent({
    required this.downloaderId,
    required this.downloader,
    required this.taskId,
    required this.infoHash,
    required this.title,
    required this.progress,
    required this.size,
    required this.state,
    required this.ratio,
    required this.seeds,
    required this.peers,
    required this.uploadSpeed,
    required this.downloadSpeed,
    required this.eta,
    required this.addedAt,
    required this.completedAt,
    this.category,
    this.tags,
    this.savePath,
  });

  int downloaderId;

  String downloader;

  String taskId;

  String infoHash;

  String title;

  double progress;

  int size;

  String state;

  double ratio;

  int seeds;

  int peers;

  int uploadSpeed;

  int downloadSpeed;

  /// 预计剩余秒数，-1 表示无法估计
  int eta;

  int addedAt;

  int completedAt;

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

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? savePath;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTorrent &&
    other.downloaderId == downloaderId &&
    other.downloader == downloader &&
    other.taskId == taskId &&
    other.infoHash == infoHash &&
    other.title == title &&
    other.progress == progress &&
    other.size == size &&
    other.state == state &&
    other.ratio == ratio &&
    other.seeds == seeds &&
    other.peers == peers &&
    other.uploadSpeed == uploadSpeed &&
    other.downloadSpeed == downloadSpeed &&
    other.eta == eta &&
    other.addedAt == addedAt &&
    other.completedAt == completedAt &&
    other.category == category &&
    other.tags == tags &&
    other.savePath == savePath;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (downloaderId.hashCode) +
    (downloader.hashCode) +
    (taskId.hashCode) +
    (infoHash.hashCode) +
    (title.hashCode) +
    (progress.hashCode) +
    (size.hashCode) +
    (state.hashCode) +
    (ratio.hashCode) +
    (seeds.hashCode) +
    (peers.hashCode) +
    (uploadSpeed.hashCode) +
    (downloadSpeed.hashCode) +
    (eta.hashCode) +
    (addedAt.hashCode) +
    (completedAt.hashCode) +
    (category == null ? 0 : category!.hashCode) +
    (tags == null ? 0 : tags!.hashCode) +
    (savePath == null ? 0 : savePath!.hashCode);

  @override
  String toString() => 'AppTorrent[downloaderId=$downloaderId, downloader=$downloader, taskId=$taskId, infoHash=$infoHash, title=$title, progress=$progress, size=$size, state=$state, ratio=$ratio, seeds=$seeds, peers=$peers, uploadSpeed=$uploadSpeed, downloadSpeed=$downloadSpeed, eta=$eta, addedAt=$addedAt, completedAt=$completedAt, category=$category, tags=$tags, savePath=$savePath]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'downloader_id'] = this.downloaderId;
      json[r'downloader'] = this.downloader;
      json[r'task_id'] = this.taskId;
      json[r'info_hash'] = this.infoHash;
      json[r'title'] = this.title;
      json[r'progress'] = this.progress;
      json[r'size'] = this.size;
      json[r'state'] = this.state;
      json[r'ratio'] = this.ratio;
      json[r'seeds'] = this.seeds;
      json[r'peers'] = this.peers;
      json[r'upload_speed'] = this.uploadSpeed;
      json[r'download_speed'] = this.downloadSpeed;
      json[r'eta'] = this.eta;
      json[r'added_at'] = this.addedAt;
      json[r'completed_at'] = this.completedAt;
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
    return json;
  }

  /// Returns a new [AppTorrent] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTorrent? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'downloader_id'), 'Required key "AppTorrent[downloader_id]" is missing from JSON.');
        assert(json[r'downloader_id'] != null, 'Required key "AppTorrent[downloader_id]" has a null value in JSON.');
        assert(json.containsKey(r'downloader'), 'Required key "AppTorrent[downloader]" is missing from JSON.');
        assert(json[r'downloader'] != null, 'Required key "AppTorrent[downloader]" has a null value in JSON.');
        assert(json.containsKey(r'task_id'), 'Required key "AppTorrent[task_id]" is missing from JSON.');
        assert(json[r'task_id'] != null, 'Required key "AppTorrent[task_id]" has a null value in JSON.');
        assert(json.containsKey(r'info_hash'), 'Required key "AppTorrent[info_hash]" is missing from JSON.');
        assert(json[r'info_hash'] != null, 'Required key "AppTorrent[info_hash]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppTorrent[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppTorrent[title]" has a null value in JSON.');
        assert(json.containsKey(r'progress'), 'Required key "AppTorrent[progress]" is missing from JSON.');
        assert(json[r'progress'] != null, 'Required key "AppTorrent[progress]" has a null value in JSON.');
        assert(json.containsKey(r'size'), 'Required key "AppTorrent[size]" is missing from JSON.');
        assert(json[r'size'] != null, 'Required key "AppTorrent[size]" has a null value in JSON.');
        assert(json.containsKey(r'state'), 'Required key "AppTorrent[state]" is missing from JSON.');
        assert(json[r'state'] != null, 'Required key "AppTorrent[state]" has a null value in JSON.');
        assert(json.containsKey(r'ratio'), 'Required key "AppTorrent[ratio]" is missing from JSON.');
        assert(json[r'ratio'] != null, 'Required key "AppTorrent[ratio]" has a null value in JSON.');
        assert(json.containsKey(r'seeds'), 'Required key "AppTorrent[seeds]" is missing from JSON.');
        assert(json[r'seeds'] != null, 'Required key "AppTorrent[seeds]" has a null value in JSON.');
        assert(json.containsKey(r'peers'), 'Required key "AppTorrent[peers]" is missing from JSON.');
        assert(json[r'peers'] != null, 'Required key "AppTorrent[peers]" has a null value in JSON.');
        assert(json.containsKey(r'upload_speed'), 'Required key "AppTorrent[upload_speed]" is missing from JSON.');
        assert(json[r'upload_speed'] != null, 'Required key "AppTorrent[upload_speed]" has a null value in JSON.');
        assert(json.containsKey(r'download_speed'), 'Required key "AppTorrent[download_speed]" is missing from JSON.');
        assert(json[r'download_speed'] != null, 'Required key "AppTorrent[download_speed]" has a null value in JSON.');
        assert(json.containsKey(r'eta'), 'Required key "AppTorrent[eta]" is missing from JSON.');
        assert(json[r'eta'] != null, 'Required key "AppTorrent[eta]" has a null value in JSON.');
        assert(json.containsKey(r'added_at'), 'Required key "AppTorrent[added_at]" is missing from JSON.');
        assert(json[r'added_at'] != null, 'Required key "AppTorrent[added_at]" has a null value in JSON.');
        assert(json.containsKey(r'completed_at'), 'Required key "AppTorrent[completed_at]" is missing from JSON.');
        assert(json[r'completed_at'] != null, 'Required key "AppTorrent[completed_at]" has a null value in JSON.');
        return true;
      }());

      return AppTorrent(
        downloaderId: mapValueOfType<int>(json, r'downloader_id')!,
        downloader: mapValueOfType<String>(json, r'downloader')!,
        taskId: mapValueOfType<String>(json, r'task_id')!,
        infoHash: mapValueOfType<String>(json, r'info_hash')!,
        title: mapValueOfType<String>(json, r'title')!,
        progress: mapValueOfType<double>(json, r'progress')!,
        size: mapValueOfType<int>(json, r'size')!,
        state: mapValueOfType<String>(json, r'state')!,
        ratio: mapValueOfType<double>(json, r'ratio')!,
        seeds: mapValueOfType<int>(json, r'seeds')!,
        peers: mapValueOfType<int>(json, r'peers')!,
        uploadSpeed: mapValueOfType<int>(json, r'upload_speed')!,
        downloadSpeed: mapValueOfType<int>(json, r'download_speed')!,
        eta: mapValueOfType<int>(json, r'eta')!,
        addedAt: mapValueOfType<int>(json, r'added_at')!,
        completedAt: mapValueOfType<int>(json, r'completed_at')!,
        category: mapValueOfType<String>(json, r'category'),
        tags: mapValueOfType<String>(json, r'tags'),
        savePath: mapValueOfType<String>(json, r'save_path'),
      );
    }
    return null;
  }

  static List<AppTorrent> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTorrent>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTorrent.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTorrent> mapFromJson(dynamic json) {
    final map = <String, AppTorrent>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTorrent.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTorrent-objects as value to a dart map
  static Map<String, List<AppTorrent>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTorrent>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTorrent.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'downloader_id',
    'downloader',
    'task_id',
    'info_hash',
    'title',
    'progress',
    'size',
    'state',
    'ratio',
    'seeds',
    'peers',
    'upload_speed',
    'download_speed',
    'eta',
    'added_at',
    'completed_at',
  };
}

