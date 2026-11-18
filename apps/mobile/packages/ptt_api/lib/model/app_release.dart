//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppRelease {
  /// Returns a new [AppRelease] instance.
  AppRelease({
    required this.version,
    required this.name,
    required this.changelog,
    required this.url,
    required this.publishedAt,
    this.assets = const [],
    this.prerelease,
    this.prereleaseLabel,
  });

  String version;

  String name;

  String changelog;

  String url;

  int publishedAt;

  List<AppReleaseAsset> assets;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? prerelease;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? prereleaseLabel;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppRelease &&
    other.version == version &&
    other.name == name &&
    other.changelog == changelog &&
    other.url == url &&
    other.publishedAt == publishedAt &&
    _deepEquality.equals(other.assets, assets) &&
    other.prerelease == prerelease &&
    other.prereleaseLabel == prereleaseLabel;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (version.hashCode) +
    (name.hashCode) +
    (changelog.hashCode) +
    (url.hashCode) +
    (publishedAt.hashCode) +
    (assets.hashCode) +
    (prerelease == null ? 0 : prerelease!.hashCode) +
    (prereleaseLabel == null ? 0 : prereleaseLabel!.hashCode);

  @override
  String toString() => 'AppRelease[version=$version, name=$name, changelog=$changelog, url=$url, publishedAt=$publishedAt, assets=$assets, prerelease=$prerelease, prereleaseLabel=$prereleaseLabel]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'version'] = this.version;
      json[r'name'] = this.name;
      json[r'changelog'] = this.changelog;
      json[r'url'] = this.url;
      json[r'published_at'] = this.publishedAt;
      json[r'assets'] = this.assets;
    if (this.prerelease != null) {
      json[r'prerelease'] = this.prerelease;
    } else {
      json[r'prerelease'] = null;
    }
    if (this.prereleaseLabel != null) {
      json[r'prerelease_label'] = this.prereleaseLabel;
    } else {
      json[r'prerelease_label'] = null;
    }
    return json;
  }

  /// Returns a new [AppRelease] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppRelease? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'version'), 'Required key "AppRelease[version]" is missing from JSON.');
        assert(json[r'version'] != null, 'Required key "AppRelease[version]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppRelease[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppRelease[name]" has a null value in JSON.');
        assert(json.containsKey(r'changelog'), 'Required key "AppRelease[changelog]" is missing from JSON.');
        assert(json[r'changelog'] != null, 'Required key "AppRelease[changelog]" has a null value in JSON.');
        assert(json.containsKey(r'url'), 'Required key "AppRelease[url]" is missing from JSON.');
        assert(json[r'url'] != null, 'Required key "AppRelease[url]" has a null value in JSON.');
        assert(json.containsKey(r'published_at'), 'Required key "AppRelease[published_at]" is missing from JSON.');
        assert(json[r'published_at'] != null, 'Required key "AppRelease[published_at]" has a null value in JSON.');
        return true;
      }());

      return AppRelease(
        version: mapValueOfType<String>(json, r'version')!,
        name: mapValueOfType<String>(json, r'name')!,
        changelog: mapValueOfType<String>(json, r'changelog')!,
        url: mapValueOfType<String>(json, r'url')!,
        publishedAt: mapValueOfType<int>(json, r'published_at')!,
        assets: AppReleaseAsset.listFromJson(json[r'assets']),
        prerelease: mapValueOfType<bool>(json, r'prerelease'),
        prereleaseLabel: mapValueOfType<String>(json, r'prerelease_label'),
      );
    }
    return null;
  }

  static List<AppRelease> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppRelease>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppRelease.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppRelease> mapFromJson(dynamic json) {
    final map = <String, AppRelease>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppRelease.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppRelease-objects as value to a dart map
  static Map<String, List<AppRelease>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppRelease>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppRelease.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'version',
    'name',
    'changelog',
    'url',
    'published_at',
  };
}

