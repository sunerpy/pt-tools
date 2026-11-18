//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMeta {
  /// Returns a new [AppMeta] instance.
  AppMeta({
    required this.name,
    required this.version,
    required this.remoteApiLevel,
    this.features = const [],
    required this.principal,
  });

  String name;

  String version;

  int remoteApiLevel;

  List<String> features;

  AppPrincipal principal;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMeta &&
    other.name == name &&
    other.version == version &&
    other.remoteApiLevel == remoteApiLevel &&
    _deepEquality.equals(other.features, features) &&
    other.principal == principal;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (name.hashCode) +
    (version.hashCode) +
    (remoteApiLevel.hashCode) +
    (features.hashCode) +
    (principal.hashCode);

  @override
  String toString() => 'AppMeta[name=$name, version=$version, remoteApiLevel=$remoteApiLevel, features=$features, principal=$principal]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'name'] = this.name;
      json[r'version'] = this.version;
      json[r'remote_api_level'] = this.remoteApiLevel;
      json[r'features'] = this.features;
      json[r'principal'] = this.principal;
    return json;
  }

  /// Returns a new [AppMeta] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMeta? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'name'), 'Required key "AppMeta[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppMeta[name]" has a null value in JSON.');
        assert(json.containsKey(r'version'), 'Required key "AppMeta[version]" is missing from JSON.');
        assert(json[r'version'] != null, 'Required key "AppMeta[version]" has a null value in JSON.');
        assert(json.containsKey(r'remote_api_level'), 'Required key "AppMeta[remote_api_level]" is missing from JSON.');
        assert(json[r'remote_api_level'] != null, 'Required key "AppMeta[remote_api_level]" has a null value in JSON.');
        assert(json.containsKey(r'features'), 'Required key "AppMeta[features]" is missing from JSON.');
        assert(json[r'features'] != null, 'Required key "AppMeta[features]" has a null value in JSON.');
        assert(json.containsKey(r'principal'), 'Required key "AppMeta[principal]" is missing from JSON.');
        assert(json[r'principal'] != null, 'Required key "AppMeta[principal]" has a null value in JSON.');
        return true;
      }());

      return AppMeta(
        name: mapValueOfType<String>(json, r'name')!,
        version: mapValueOfType<String>(json, r'version')!,
        remoteApiLevel: mapValueOfType<int>(json, r'remote_api_level')!,
        features: json[r'features'] is Iterable
            ? (json[r'features'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        principal: AppPrincipal.fromJson(json[r'principal'])!,
      );
    }
    return null;
  }

  static List<AppMeta> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMeta>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMeta.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMeta> mapFromJson(dynamic json) {
    final map = <String, AppMeta>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMeta.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMeta-objects as value to a dart map
  static Map<String, List<AppMeta>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMeta>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMeta.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'name',
    'version',
    'remote_api_level',
    'features',
    'principal',
  };
}

