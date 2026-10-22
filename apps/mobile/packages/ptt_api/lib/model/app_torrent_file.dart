//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTorrentFile {
  /// Returns a new [AppTorrentFile] instance.
  AppTorrentFile({
    required this.id,
    required this.name,
    this.episode,
    this.episodeEnd,
    required this.status,
    this.message,
    this.scrapeStatus,
    this.scrapeNote,
    this.nextRetryAt,
    this.retrying,
  });

  /// 整理记录的编号（/media/history 的 id）
  int id;

  /// 源文件名
  String name;

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

  /// done、failed、skipped 或 removed
  String status;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? message;

  /// ok、partial、failed；空的是没刮削过
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? scrapeStatus;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? scrapeNote;

  /// 失败后下次自动重试的时间
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? nextRetryAt;

  /// 整理队列正在重试它
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? retrying;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTorrentFile &&
    other.id == id &&
    other.name == name &&
    other.episode == episode &&
    other.episodeEnd == episodeEnd &&
    other.status == status &&
    other.message == message &&
    other.scrapeStatus == scrapeStatus &&
    other.scrapeNote == scrapeNote &&
    other.nextRetryAt == nextRetryAt &&
    other.retrying == retrying;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (name.hashCode) +
    (episode == null ? 0 : episode!.hashCode) +
    (episodeEnd == null ? 0 : episodeEnd!.hashCode) +
    (status.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (scrapeStatus == null ? 0 : scrapeStatus!.hashCode) +
    (scrapeNote == null ? 0 : scrapeNote!.hashCode) +
    (nextRetryAt == null ? 0 : nextRetryAt!.hashCode) +
    (retrying == null ? 0 : retrying!.hashCode);

  @override
  String toString() => 'AppTorrentFile[id=$id, name=$name, episode=$episode, episodeEnd=$episodeEnd, status=$status, message=$message, scrapeStatus=$scrapeStatus, scrapeNote=$scrapeNote, nextRetryAt=$nextRetryAt, retrying=$retrying]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'name'] = this.name;
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
    if (this.scrapeStatus != null) {
      json[r'scrape_status'] = this.scrapeStatus;
    } else {
      json[r'scrape_status'] = null;
    }
    if (this.scrapeNote != null) {
      json[r'scrape_note'] = this.scrapeNote;
    } else {
      json[r'scrape_note'] = null;
    }
    if (this.nextRetryAt != null) {
      json[r'next_retry_at'] = this.nextRetryAt;
    } else {
      json[r'next_retry_at'] = null;
    }
    if (this.retrying != null) {
      json[r'retrying'] = this.retrying;
    } else {
      json[r'retrying'] = null;
    }
    return json;
  }

  /// Returns a new [AppTorrentFile] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTorrentFile? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppTorrentFile[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppTorrentFile[id]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppTorrentFile[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppTorrentFile[name]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppTorrentFile[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppTorrentFile[status]" has a null value in JSON.');
        return true;
      }());

      return AppTorrentFile(
        id: mapValueOfType<int>(json, r'id')!,
        name: mapValueOfType<String>(json, r'name')!,
        episode: mapValueOfType<int>(json, r'episode'),
        episodeEnd: mapValueOfType<int>(json, r'episode_end'),
        status: mapValueOfType<String>(json, r'status')!,
        message: mapValueOfType<String>(json, r'message'),
        scrapeStatus: mapValueOfType<String>(json, r'scrape_status'),
        scrapeNote: mapValueOfType<String>(json, r'scrape_note'),
        nextRetryAt: mapValueOfType<int>(json, r'next_retry_at'),
        retrying: mapValueOfType<bool>(json, r'retrying'),
      );
    }
    return null;
  }

  static List<AppTorrentFile> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTorrentFile>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTorrentFile.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTorrentFile> mapFromJson(dynamic json) {
    final map = <String, AppTorrentFile>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTorrentFile.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTorrentFile-objects as value to a dart map
  static Map<String, List<AppTorrentFile>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTorrentFile>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTorrentFile.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'name',
    'status',
  };
}

