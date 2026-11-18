//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTemplateCatalog {
  /// Returns a new [AppTemplateCatalog] instance.
  AppTemplateCatalog({
    this.groups = const [],
    this.commonVars = const [],
    this.events = const [],
  });

  List<AppTemplateGroup> groups;

  /// 每种事件都有的变量
  List<AppTemplateVar> commonVars;

  List<AppTemplate> events;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTemplateCatalog &&
    _deepEquality.equals(other.groups, groups) &&
    _deepEquality.equals(other.commonVars, commonVars) &&
    _deepEquality.equals(other.events, events);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (groups.hashCode) +
    (commonVars.hashCode) +
    (events.hashCode);

  @override
  String toString() => 'AppTemplateCatalog[groups=$groups, commonVars=$commonVars, events=$events]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'groups'] = this.groups;
      json[r'common_vars'] = this.commonVars;
      json[r'events'] = this.events;
    return json;
  }

  /// Returns a new [AppTemplateCatalog] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTemplateCatalog? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'groups'), 'Required key "AppTemplateCatalog[groups]" is missing from JSON.');
        assert(json[r'groups'] != null, 'Required key "AppTemplateCatalog[groups]" has a null value in JSON.');
        assert(json.containsKey(r'common_vars'), 'Required key "AppTemplateCatalog[common_vars]" is missing from JSON.');
        assert(json[r'common_vars'] != null, 'Required key "AppTemplateCatalog[common_vars]" has a null value in JSON.');
        assert(json.containsKey(r'events'), 'Required key "AppTemplateCatalog[events]" is missing from JSON.');
        assert(json[r'events'] != null, 'Required key "AppTemplateCatalog[events]" has a null value in JSON.');
        return true;
      }());

      return AppTemplateCatalog(
        groups: AppTemplateGroup.listFromJson(json[r'groups']),
        commonVars: AppTemplateVar.listFromJson(json[r'common_vars']),
        events: AppTemplate.listFromJson(json[r'events']),
      );
    }
    return null;
  }

  static List<AppTemplateCatalog> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTemplateCatalog>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTemplateCatalog.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTemplateCatalog> mapFromJson(dynamic json) {
    final map = <String, AppTemplateCatalog>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTemplateCatalog.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTemplateCatalog-objects as value to a dart map
  static Map<String, List<AppTemplateCatalog>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTemplateCatalog>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTemplateCatalog.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'groups',
    'common_vars',
    'events',
  };
}

