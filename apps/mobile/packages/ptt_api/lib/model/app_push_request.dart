//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppPushRequest {
  /// Returns a new [AppPushRequest] instance.
  AppPushRequest({
    required this.site,
    required this.torrentId,
    this.downloaderId,
    this.title,
    this.category,
    this.tags,
    this.savePath,
  });

  String site;

  String torrentId;

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
  String? title;

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
  bool operator ==(Object other) => identical(this, other) || other is AppPushRequest &&
    other.site == site &&
    other.torrentId == torrentId &&
    other.downloaderId == downloaderId &&
    other.title == title &&
    other.category == category &&
    other.tags == tags &&
    other.savePath == savePath;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (torrentId.hashCode) +
    (downloaderId == null ? 0 : downloaderId!.hashCode) +
    (title == null ? 0 : title!.hashCode) +
    (category == null ? 0 : category!.hashCode) +
    (tags == null ? 0 : tags!.hashCode) +
    (savePath == null ? 0 : savePath!.hashCode);

  @override
  String toString() => 'AppPushRequest[site=$site, torrentId=$torrentId, downloaderId=$downloaderId, title=$title, category=$category, tags=$tags, savePath=$savePath]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'torrent_id'] = this.torrentId;
    if (this.downloaderId != null) {
      json[r'downloader_id'] = this.downloaderId;
    } else {
      json[r'downloader_id'] = null;
    }
    if (this.title != null) {
      json[r'title'] = this.title;
    } else {
      json[r'title'] = null;
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
    return json;
  }

  /// Returns a new [AppPushRequest] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppPushRequest? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppPushRequest[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppPushRequest[site]" has a null value in JSON.');
        assert(json.containsKey(r'torrent_id'), 'Required key "AppPushRequest[torrent_id]" is missing from JSON.');
        assert(json[r'torrent_id'] != null, 'Required key "AppPushRequest[torrent_id]" has a null value in JSON.');
        return true;
      }());

      return AppPushRequest(
        site: mapValueOfType<String>(json, r'site')!,
        torrentId: mapValueOfType<String>(json, r'torrent_id')!,
        downloaderId: mapValueOfType<int>(json, r'downloader_id'),
        title: mapValueOfType<String>(json, r'title'),
        category: mapValueOfType<String>(json, r'category'),
        tags: mapValueOfType<String>(json, r'tags'),
        savePath: mapValueOfType<String>(json, r'save_path'),
      );
    }
    return null;
  }

  static List<AppPushRequest> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppPushRequest>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppPushRequest.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppPushRequest> mapFromJson(dynamic json) {
    final map = <String, AppPushRequest>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppPushRequest.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppPushRequest-objects as value to a dart map
  static Map<String, List<AppPushRequest>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppPushRequest>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppPushRequest.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'torrent_id',
  };
}

