//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppDownloaderFailure {
  /// Returns a new [AppDownloaderFailure] instance.
  AppDownloaderFailure({
    required this.downloaderId,
    required this.downloader,
    required this.error,
  });

  int downloaderId;

  String downloader;

  String error;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppDownloaderFailure &&
    other.downloaderId == downloaderId &&
    other.downloader == downloader &&
    other.error == error;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (downloaderId.hashCode) +
    (downloader.hashCode) +
    (error.hashCode);

  @override
  String toString() => 'AppDownloaderFailure[downloaderId=$downloaderId, downloader=$downloader, error=$error]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'downloader_id'] = this.downloaderId;
      json[r'downloader'] = this.downloader;
      json[r'error'] = this.error;
    return json;
  }

  /// Returns a new [AppDownloaderFailure] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppDownloaderFailure? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'downloader_id'), 'Required key "AppDownloaderFailure[downloader_id]" is missing from JSON.');
        assert(json[r'downloader_id'] != null, 'Required key "AppDownloaderFailure[downloader_id]" has a null value in JSON.');
        assert(json.containsKey(r'downloader'), 'Required key "AppDownloaderFailure[downloader]" is missing from JSON.');
        assert(json[r'downloader'] != null, 'Required key "AppDownloaderFailure[downloader]" has a null value in JSON.');
        assert(json.containsKey(r'error'), 'Required key "AppDownloaderFailure[error]" is missing from JSON.');
        assert(json[r'error'] != null, 'Required key "AppDownloaderFailure[error]" has a null value in JSON.');
        return true;
      }());

      return AppDownloaderFailure(
        downloaderId: mapValueOfType<int>(json, r'downloader_id')!,
        downloader: mapValueOfType<String>(json, r'downloader')!,
        error: mapValueOfType<String>(json, r'error')!,
      );
    }
    return null;
  }

  static List<AppDownloaderFailure> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppDownloaderFailure>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppDownloaderFailure.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppDownloaderFailure> mapFromJson(dynamic json) {
    final map = <String, AppDownloaderFailure>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppDownloaderFailure.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppDownloaderFailure-objects as value to a dart map
  static Map<String, List<AppDownloaderFailure>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppDownloaderFailure>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppDownloaderFailure.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'downloader_id',
    'downloader',
    'error',
  };
}

