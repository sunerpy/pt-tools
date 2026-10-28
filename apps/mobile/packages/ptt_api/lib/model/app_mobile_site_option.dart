//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMobileSiteOption {
  /// Returns a new [AppMobileSiteOption] instance.
  AppMobileSiteOption({
    required this.name,
    required this.displayName,
  });

  String name;

  String displayName;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMobileSiteOption &&
    other.name == name &&
    other.displayName == displayName;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (name.hashCode) +
    (displayName.hashCode);

  @override
  String toString() => 'AppMobileSiteOption[name=$name, displayName=$displayName]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'name'] = this.name;
      json[r'display_name'] = this.displayName;
    return json;
  }

  /// Returns a new [AppMobileSiteOption] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMobileSiteOption? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'name'), 'Required key "AppMobileSiteOption[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppMobileSiteOption[name]" has a null value in JSON.');
        assert(json.containsKey(r'display_name'), 'Required key "AppMobileSiteOption[display_name]" is missing from JSON.');
        assert(json[r'display_name'] != null, 'Required key "AppMobileSiteOption[display_name]" has a null value in JSON.');
        return true;
      }());

      return AppMobileSiteOption(
        name: mapValueOfType<String>(json, r'name')!,
        displayName: mapValueOfType<String>(json, r'display_name')!,
      );
    }
    return null;
  }

  static List<AppMobileSiteOption> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMobileSiteOption>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMobileSiteOption.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMobileSiteOption> mapFromJson(dynamic json) {
    final map = <String, AppMobileSiteOption>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMobileSiteOption.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMobileSiteOption-objects as value to a dart map
  static Map<String, List<AppMobileSiteOption>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMobileSiteOption>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMobileSiteOption.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'name',
    'display_name',
  };
}

