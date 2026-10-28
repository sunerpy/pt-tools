//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMobileOptions {
  /// Returns a new [AppMobileOptions] instance.
  AppMobileOptions({
    this.sites = const [],
    this.downloaders = const [],
    this.profiles = const [],
    this.libraries = const [],
    this.errors = const {},
  });

  List<AppMobileSiteOption> sites;

  List<AppMobileOption> downloaders;

  List<AppMobileOption> profiles;

  List<AppMobileOption> libraries;

  Map<String, String> errors;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMobileOptions &&
    _deepEquality.equals(other.sites, sites) &&
    _deepEquality.equals(other.downloaders, downloaders) &&
    _deepEquality.equals(other.profiles, profiles) &&
    _deepEquality.equals(other.libraries, libraries) &&
    _deepEquality.equals(other.errors, errors);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (sites.hashCode) +
    (downloaders.hashCode) +
    (profiles.hashCode) +
    (libraries.hashCode) +
    (errors.hashCode);

  @override
  String toString() => 'AppMobileOptions[sites=$sites, downloaders=$downloaders, profiles=$profiles, libraries=$libraries, errors=$errors]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'sites'] = this.sites;
      json[r'downloaders'] = this.downloaders;
      json[r'profiles'] = this.profiles;
      json[r'libraries'] = this.libraries;
      json[r'errors'] = this.errors;
    return json;
  }

  /// Returns a new [AppMobileOptions] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMobileOptions? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'sites'), 'Required key "AppMobileOptions[sites]" is missing from JSON.');
        assert(json[r'sites'] != null, 'Required key "AppMobileOptions[sites]" has a null value in JSON.');
        assert(json.containsKey(r'downloaders'), 'Required key "AppMobileOptions[downloaders]" is missing from JSON.');
        assert(json[r'downloaders'] != null, 'Required key "AppMobileOptions[downloaders]" has a null value in JSON.');
        assert(json.containsKey(r'profiles'), 'Required key "AppMobileOptions[profiles]" is missing from JSON.');
        assert(json[r'profiles'] != null, 'Required key "AppMobileOptions[profiles]" has a null value in JSON.');
        assert(json.containsKey(r'libraries'), 'Required key "AppMobileOptions[libraries]" is missing from JSON.');
        assert(json[r'libraries'] != null, 'Required key "AppMobileOptions[libraries]" has a null value in JSON.');
        return true;
      }());

      return AppMobileOptions(
        sites: AppMobileSiteOption.listFromJson(json[r'sites']),
        downloaders: AppMobileOption.listFromJson(json[r'downloaders']),
        profiles: AppMobileOption.listFromJson(json[r'profiles']),
        libraries: AppMobileOption.listFromJson(json[r'libraries']),
        errors: mapCastOfType<String, String>(json, r'errors') ?? const {},
      );
    }
    return null;
  }

  static List<AppMobileOptions> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMobileOptions>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMobileOptions.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMobileOptions> mapFromJson(dynamic json) {
    final map = <String, AppMobileOptions>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMobileOptions.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMobileOptions-objects as value to a dart map
  static Map<String, List<AppMobileOptions>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMobileOptions>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMobileOptions.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'sites',
    'downloaders',
    'profiles',
    'libraries',
  };
}

