//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchError {
  /// Returns a new [AppSearchError] instance.
  AppSearchError({
    required this.site,
    required this.error,
  });

  String site;

  String error;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchError &&
    other.site == site &&
    other.error == error;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (error.hashCode);

  @override
  String toString() => 'AppSearchError[site=$site, error=$error]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'error'] = this.error;
    return json;
  }

  /// Returns a new [AppSearchError] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchError? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppSearchError[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppSearchError[site]" has a null value in JSON.');
        assert(json.containsKey(r'error'), 'Required key "AppSearchError[error]" is missing from JSON.');
        assert(json[r'error'] != null, 'Required key "AppSearchError[error]" has a null value in JSON.');
        return true;
      }());

      return AppSearchError(
        site: mapValueOfType<String>(json, r'site')!,
        error: mapValueOfType<String>(json, r'error')!,
      );
    }
    return null;
  }

  static List<AppSearchError> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchError>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchError.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchError> mapFromJson(dynamic json) {
    final map = <String, AppSearchError>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchError.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchError-objects as value to a dart map
  static Map<String, List<AppSearchError>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchError>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchError.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'error',
  };
}

