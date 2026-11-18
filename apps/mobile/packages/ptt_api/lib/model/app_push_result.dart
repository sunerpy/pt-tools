//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppPushResult {
  /// Returns a new [AppPushResult] instance.
  AppPushResult({
    required this.success,
    required this.skipped,
    this.message,
    this.infoHash,
    required this.downloaderId,
    required this.downloader,
  });

  bool success;

  bool skipped;

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
  String? infoHash;

  int downloaderId;

  String downloader;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppPushResult &&
    other.success == success &&
    other.skipped == skipped &&
    other.message == message &&
    other.infoHash == infoHash &&
    other.downloaderId == downloaderId &&
    other.downloader == downloader;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (success.hashCode) +
    (skipped.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (infoHash == null ? 0 : infoHash!.hashCode) +
    (downloaderId.hashCode) +
    (downloader.hashCode);

  @override
  String toString() => 'AppPushResult[success=$success, skipped=$skipped, message=$message, infoHash=$infoHash, downloaderId=$downloaderId, downloader=$downloader]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'success'] = this.success;
      json[r'skipped'] = this.skipped;
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
    }
    if (this.infoHash != null) {
      json[r'info_hash'] = this.infoHash;
    } else {
      json[r'info_hash'] = null;
    }
      json[r'downloader_id'] = this.downloaderId;
      json[r'downloader'] = this.downloader;
    return json;
  }

  /// Returns a new [AppPushResult] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppPushResult? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'success'), 'Required key "AppPushResult[success]" is missing from JSON.');
        assert(json[r'success'] != null, 'Required key "AppPushResult[success]" has a null value in JSON.');
        assert(json.containsKey(r'skipped'), 'Required key "AppPushResult[skipped]" is missing from JSON.');
        assert(json[r'skipped'] != null, 'Required key "AppPushResult[skipped]" has a null value in JSON.');
        assert(json.containsKey(r'downloader_id'), 'Required key "AppPushResult[downloader_id]" is missing from JSON.');
        assert(json[r'downloader_id'] != null, 'Required key "AppPushResult[downloader_id]" has a null value in JSON.');
        assert(json.containsKey(r'downloader'), 'Required key "AppPushResult[downloader]" is missing from JSON.');
        assert(json[r'downloader'] != null, 'Required key "AppPushResult[downloader]" has a null value in JSON.');
        return true;
      }());

      return AppPushResult(
        success: mapValueOfType<bool>(json, r'success')!,
        skipped: mapValueOfType<bool>(json, r'skipped')!,
        message: mapValueOfType<String>(json, r'message'),
        infoHash: mapValueOfType<String>(json, r'info_hash'),
        downloaderId: mapValueOfType<int>(json, r'downloader_id')!,
        downloader: mapValueOfType<String>(json, r'downloader')!,
      );
    }
    return null;
  }

  static List<AppPushResult> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppPushResult>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppPushResult.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppPushResult> mapFromJson(dynamic json) {
    final map = <String, AppPushResult>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppPushResult.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppPushResult-objects as value to a dart map
  static Map<String, List<AppPushResult>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppPushResult>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppPushResult.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'success',
    'skipped',
    'downloader_id',
    'downloader',
  };
}

