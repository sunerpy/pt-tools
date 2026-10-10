//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTorrentActionResult {
  /// Returns a new [AppTorrentActionResult] instance.
  AppTorrentActionResult({
    required this.downloaderId,
    required this.taskId,
    required this.success,
    this.message,
  });

  int downloaderId;

  String taskId;

  bool success;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? message;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTorrentActionResult &&
    other.downloaderId == downloaderId &&
    other.taskId == taskId &&
    other.success == success &&
    other.message == message;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (downloaderId.hashCode) +
    (taskId.hashCode) +
    (success.hashCode) +
    (message == null ? 0 : message!.hashCode);

  @override
  String toString() => 'AppTorrentActionResult[downloaderId=$downloaderId, taskId=$taskId, success=$success, message=$message]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'downloader_id'] = this.downloaderId;
      json[r'task_id'] = this.taskId;
      json[r'success'] = this.success;
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
    }
    return json;
  }

  /// Returns a new [AppTorrentActionResult] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTorrentActionResult? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'downloader_id'), 'Required key "AppTorrentActionResult[downloader_id]" is missing from JSON.');
        assert(json[r'downloader_id'] != null, 'Required key "AppTorrentActionResult[downloader_id]" has a null value in JSON.');
        assert(json.containsKey(r'task_id'), 'Required key "AppTorrentActionResult[task_id]" is missing from JSON.');
        assert(json[r'task_id'] != null, 'Required key "AppTorrentActionResult[task_id]" has a null value in JSON.');
        assert(json.containsKey(r'success'), 'Required key "AppTorrentActionResult[success]" is missing from JSON.');
        assert(json[r'success'] != null, 'Required key "AppTorrentActionResult[success]" has a null value in JSON.');
        return true;
      }());

      return AppTorrentActionResult(
        downloaderId: mapValueOfType<int>(json, r'downloader_id')!,
        taskId: mapValueOfType<String>(json, r'task_id')!,
        success: mapValueOfType<bool>(json, r'success')!,
        message: mapValueOfType<String>(json, r'message'),
      );
    }
    return null;
  }

  static List<AppTorrentActionResult> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTorrentActionResult>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTorrentActionResult.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTorrentActionResult> mapFromJson(dynamic json) {
    final map = <String, AppTorrentActionResult>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTorrentActionResult.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTorrentActionResult-objects as value to a dart map
  static Map<String, List<AppTorrentActionResult>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTorrentActionResult>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTorrentActionResult.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'downloader_id',
    'task_id',
    'success',
  };
}

