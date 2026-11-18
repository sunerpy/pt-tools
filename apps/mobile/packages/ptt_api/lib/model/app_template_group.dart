//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTemplateGroup {
  /// Returns a new [AppTemplateGroup] instance.
  AppTemplateGroup({
    required this.key,
    required this.name,
    required this.desc,
  });

  /// notify（通知）、reply（ChatOps 命令回复）或 assistant（AI 助手回复）
  String key;

  String name;

  String desc;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTemplateGroup &&
    other.key == key &&
    other.name == name &&
    other.desc == desc;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (key.hashCode) +
    (name.hashCode) +
    (desc.hashCode);

  @override
  String toString() => 'AppTemplateGroup[key=$key, name=$name, desc=$desc]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'key'] = this.key;
      json[r'name'] = this.name;
      json[r'desc'] = this.desc;
    return json;
  }

  /// Returns a new [AppTemplateGroup] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTemplateGroup? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'key'), 'Required key "AppTemplateGroup[key]" is missing from JSON.');
        assert(json[r'key'] != null, 'Required key "AppTemplateGroup[key]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppTemplateGroup[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppTemplateGroup[name]" has a null value in JSON.');
        assert(json.containsKey(r'desc'), 'Required key "AppTemplateGroup[desc]" is missing from JSON.');
        assert(json[r'desc'] != null, 'Required key "AppTemplateGroup[desc]" has a null value in JSON.');
        return true;
      }());

      return AppTemplateGroup(
        key: mapValueOfType<String>(json, r'key')!,
        name: mapValueOfType<String>(json, r'name')!,
        desc: mapValueOfType<String>(json, r'desc')!,
      );
    }
    return null;
  }

  static List<AppTemplateGroup> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTemplateGroup>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTemplateGroup.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTemplateGroup> mapFromJson(dynamic json) {
    final map = <String, AppTemplateGroup>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTemplateGroup.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTemplateGroup-objects as value to a dart map
  static Map<String, List<AppTemplateGroup>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTemplateGroup>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTemplateGroup.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'key',
    'name',
    'desc',
  };
}

