//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSubscriptionTorrent {
  /// Returns a new [AppSubscriptionTorrent] instance.
  AppSubscriptionTorrent({
    required this.site,
    required this.torrentId,
    required this.title,
    required this.status,
    this.episode,
    this.episodeEnd,
    required this.complete,
    required this.size,
    this.message,
    required this.createdAt,
  });

  String site;

  String torrentId;

  String title;

  String status;

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

  bool complete;

  int size;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? message;

  int createdAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSubscriptionTorrent &&
    other.site == site &&
    other.torrentId == torrentId &&
    other.title == title &&
    other.status == status &&
    other.episode == episode &&
    other.episodeEnd == episodeEnd &&
    other.complete == complete &&
    other.size == size &&
    other.message == message &&
    other.createdAt == createdAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (torrentId.hashCode) +
    (title.hashCode) +
    (status.hashCode) +
    (episode == null ? 0 : episode!.hashCode) +
    (episodeEnd == null ? 0 : episodeEnd!.hashCode) +
    (complete.hashCode) +
    (size.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (createdAt.hashCode);

  @override
  String toString() => 'AppSubscriptionTorrent[site=$site, torrentId=$torrentId, title=$title, status=$status, episode=$episode, episodeEnd=$episodeEnd, complete=$complete, size=$size, message=$message, createdAt=$createdAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'torrent_id'] = this.torrentId;
      json[r'title'] = this.title;
      json[r'status'] = this.status;
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
      json[r'complete'] = this.complete;
      json[r'size'] = this.size;
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
    }
      json[r'created_at'] = this.createdAt;
    return json;
  }

  /// Returns a new [AppSubscriptionTorrent] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSubscriptionTorrent? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppSubscriptionTorrent[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppSubscriptionTorrent[site]" has a null value in JSON.');
        assert(json.containsKey(r'torrent_id'), 'Required key "AppSubscriptionTorrent[torrent_id]" is missing from JSON.');
        assert(json[r'torrent_id'] != null, 'Required key "AppSubscriptionTorrent[torrent_id]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppSubscriptionTorrent[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppSubscriptionTorrent[title]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppSubscriptionTorrent[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppSubscriptionTorrent[status]" has a null value in JSON.');
        assert(json.containsKey(r'complete'), 'Required key "AppSubscriptionTorrent[complete]" is missing from JSON.');
        assert(json[r'complete'] != null, 'Required key "AppSubscriptionTorrent[complete]" has a null value in JSON.');
        assert(json.containsKey(r'size'), 'Required key "AppSubscriptionTorrent[size]" is missing from JSON.');
        assert(json[r'size'] != null, 'Required key "AppSubscriptionTorrent[size]" has a null value in JSON.');
        assert(json.containsKey(r'created_at'), 'Required key "AppSubscriptionTorrent[created_at]" is missing from JSON.');
        assert(json[r'created_at'] != null, 'Required key "AppSubscriptionTorrent[created_at]" has a null value in JSON.');
        return true;
      }());

      return AppSubscriptionTorrent(
        site: mapValueOfType<String>(json, r'site')!,
        torrentId: mapValueOfType<String>(json, r'torrent_id')!,
        title: mapValueOfType<String>(json, r'title')!,
        status: mapValueOfType<String>(json, r'status')!,
        episode: mapValueOfType<int>(json, r'episode'),
        episodeEnd: mapValueOfType<int>(json, r'episode_end'),
        complete: mapValueOfType<bool>(json, r'complete')!,
        size: mapValueOfType<int>(json, r'size')!,
        message: mapValueOfType<String>(json, r'message'),
        createdAt: mapValueOfType<int>(json, r'created_at')!,
      );
    }
    return null;
  }

  static List<AppSubscriptionTorrent> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionTorrent>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionTorrent.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSubscriptionTorrent> mapFromJson(dynamic json) {
    final map = <String, AppSubscriptionTorrent>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSubscriptionTorrent.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSubscriptionTorrent-objects as value to a dart map
  static Map<String, List<AppSubscriptionTorrent>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSubscriptionTorrent>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSubscriptionTorrent.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'torrent_id',
    'title',
    'status',
    'complete',
    'size',
    'created_at',
  };
}

