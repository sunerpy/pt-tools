//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppReleaseAsset {
  /// Returns a new [AppReleaseAsset] instance.
  AppReleaseAsset({
    required this.name,
    required this.downloadUrl,
    required this.size,
  });

  String name;

  String downloadUrl;

  int size;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppReleaseAsset &&
    other.name == name &&
    other.downloadUrl == downloadUrl &&
    other.size == size;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (name.hashCode) +
    (downloadUrl.hashCode) +
    (size.hashCode);

  @override
  String toString() => 'AppReleaseAsset[name=$name, downloadUrl=$downloadUrl, size=$size]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'name'] = this.name;
      json[r'download_url'] = this.downloadUrl;
      json[r'size'] = this.size;
    return json;
  }

  /// Returns a new [AppReleaseAsset] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppReleaseAsset? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'name'), 'Required key "AppReleaseAsset[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppReleaseAsset[name]" has a null value in JSON.');
        assert(json.containsKey(r'download_url'), 'Required key "AppReleaseAsset[download_url]" is missing from JSON.');
        assert(json[r'download_url'] != null, 'Required key "AppReleaseAsset[download_url]" has a null value in JSON.');
        assert(json.containsKey(r'size'), 'Required key "AppReleaseAsset[size]" is missing from JSON.');
        assert(json[r'size'] != null, 'Required key "AppReleaseAsset[size]" has a null value in JSON.');
        return true;
      }());

      return AppReleaseAsset(
        name: mapValueOfType<String>(json, r'name')!,
        downloadUrl: mapValueOfType<String>(json, r'download_url')!,
        size: mapValueOfType<int>(json, r'size')!,
      );
    }
    return null;
  }

  static List<AppReleaseAsset> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppReleaseAsset>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppReleaseAsset.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppReleaseAsset> mapFromJson(dynamic json) {
    final map = <String, AppReleaseAsset>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppReleaseAsset.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppReleaseAsset-objects as value to a dart map
  static Map<String, List<AppReleaseAsset>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppReleaseAsset>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppReleaseAsset.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'name',
    'download_url',
    'size',
  };
}

