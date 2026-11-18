//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMobileDownloaderOption {
  /// Returns a new [AppMobileDownloaderOption] instance.
  AppMobileDownloaderOption({
    required this.id,
    required this.name,
    required this.enabled,
  });

  int id;

  String name;

  bool enabled;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMobileDownloaderOption &&
    other.id == id &&
    other.name == name &&
    other.enabled == enabled;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (name.hashCode) +
    (enabled.hashCode);

  @override
  String toString() => 'AppMobileDownloaderOption[id=$id, name=$name, enabled=$enabled]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'name'] = this.name;
      json[r'enabled'] = this.enabled;
    return json;
  }

  /// Returns a new [AppMobileDownloaderOption] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMobileDownloaderOption? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppMobileDownloaderOption[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppMobileDownloaderOption[id]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppMobileDownloaderOption[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppMobileDownloaderOption[name]" has a null value in JSON.');
        assert(json.containsKey(r'enabled'), 'Required key "AppMobileDownloaderOption[enabled]" is missing from JSON.');
        assert(json[r'enabled'] != null, 'Required key "AppMobileDownloaderOption[enabled]" has a null value in JSON.');
        return true;
      }());

      return AppMobileDownloaderOption(
        id: mapValueOfType<int>(json, r'id')!,
        name: mapValueOfType<String>(json, r'name')!,
        enabled: mapValueOfType<bool>(json, r'enabled')!,
      );
    }
    return null;
  }

  static List<AppMobileDownloaderOption> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMobileDownloaderOption>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMobileDownloaderOption.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMobileDownloaderOption> mapFromJson(dynamic json) {
    final map = <String, AppMobileDownloaderOption>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMobileDownloaderOption.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMobileDownloaderOption-objects as value to a dart map
  static Map<String, List<AppMobileDownloaderOption>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMobileDownloaderOption>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMobileDownloaderOption.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'name',
    'enabled',
  };
}

