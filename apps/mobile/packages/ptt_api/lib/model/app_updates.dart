//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppUpdates {
  /// Returns a new [AppUpdates] instance.
  AppUpdates({
    required this.currentVersion,
    required this.hasUpdate,
    this.newReleases = const [],
    this.changelogUrl,
    this.hasMoreReleases,
    required this.checkedAt,
    this.error,
  });

  String currentVersion;

  bool hasUpdate;

  List<AppRelease> newReleases;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? changelogUrl;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? hasMoreReleases;

  int checkedAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? error;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppUpdates &&
    other.currentVersion == currentVersion &&
    other.hasUpdate == hasUpdate &&
    _deepEquality.equals(other.newReleases, newReleases) &&
    other.changelogUrl == changelogUrl &&
    other.hasMoreReleases == hasMoreReleases &&
    other.checkedAt == checkedAt &&
    other.error == error;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (currentVersion.hashCode) +
    (hasUpdate.hashCode) +
    (newReleases.hashCode) +
    (changelogUrl == null ? 0 : changelogUrl!.hashCode) +
    (hasMoreReleases == null ? 0 : hasMoreReleases!.hashCode) +
    (checkedAt.hashCode) +
    (error == null ? 0 : error!.hashCode);

  @override
  String toString() => 'AppUpdates[currentVersion=$currentVersion, hasUpdate=$hasUpdate, newReleases=$newReleases, changelogUrl=$changelogUrl, hasMoreReleases=$hasMoreReleases, checkedAt=$checkedAt, error=$error]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'current_version'] = this.currentVersion;
      json[r'has_update'] = this.hasUpdate;
      json[r'new_releases'] = this.newReleases;
    if (this.changelogUrl != null) {
      json[r'changelog_url'] = this.changelogUrl;
    } else {
      json[r'changelog_url'] = null;
    }
    if (this.hasMoreReleases != null) {
      json[r'has_more_releases'] = this.hasMoreReleases;
    } else {
      json[r'has_more_releases'] = null;
    }
      json[r'checked_at'] = this.checkedAt;
    if (this.error != null) {
      json[r'error'] = this.error;
    } else {
      json[r'error'] = null;
    }
    return json;
  }

  /// Returns a new [AppUpdates] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppUpdates? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'current_version'), 'Required key "AppUpdates[current_version]" is missing from JSON.');
        assert(json[r'current_version'] != null, 'Required key "AppUpdates[current_version]" has a null value in JSON.');
        assert(json.containsKey(r'has_update'), 'Required key "AppUpdates[has_update]" is missing from JSON.');
        assert(json[r'has_update'] != null, 'Required key "AppUpdates[has_update]" has a null value in JSON.');
        assert(json.containsKey(r'checked_at'), 'Required key "AppUpdates[checked_at]" is missing from JSON.');
        assert(json[r'checked_at'] != null, 'Required key "AppUpdates[checked_at]" has a null value in JSON.');
        return true;
      }());

      return AppUpdates(
        currentVersion: mapValueOfType<String>(json, r'current_version')!,
        hasUpdate: mapValueOfType<bool>(json, r'has_update')!,
        newReleases: AppRelease.listFromJson(json[r'new_releases']),
        changelogUrl: mapValueOfType<String>(json, r'changelog_url'),
        hasMoreReleases: mapValueOfType<bool>(json, r'has_more_releases'),
        checkedAt: mapValueOfType<int>(json, r'checked_at')!,
        error: mapValueOfType<String>(json, r'error'),
      );
    }
    return null;
  }

  static List<AppUpdates> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppUpdates>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppUpdates.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppUpdates> mapFromJson(dynamic json) {
    final map = <String, AppUpdates>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppUpdates.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppUpdates-objects as value to a dart map
  static Map<String, List<AppUpdates>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppUpdates>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppUpdates.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'current_version',
    'has_update',
    'checked_at',
  };
}

