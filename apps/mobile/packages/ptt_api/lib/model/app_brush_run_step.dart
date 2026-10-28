//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppBrushRunStep {
  /// Returns a new [AppBrushRunStep] instance.
  AppBrushRunStep({
    required this.kind,
    required this.text,
  });

  String kind;

  String text;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppBrushRunStep &&
    other.kind == kind &&
    other.text == text;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (kind.hashCode) +
    (text.hashCode);

  @override
  String toString() => 'AppBrushRunStep[kind=$kind, text=$text]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'kind'] = this.kind;
      json[r'text'] = this.text;
    return json;
  }

  /// Returns a new [AppBrushRunStep] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppBrushRunStep? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'kind'), 'Required key "AppBrushRunStep[kind]" is missing from JSON.');
        assert(json[r'kind'] != null, 'Required key "AppBrushRunStep[kind]" has a null value in JSON.');
        assert(json.containsKey(r'text'), 'Required key "AppBrushRunStep[text]" is missing from JSON.');
        assert(json[r'text'] != null, 'Required key "AppBrushRunStep[text]" has a null value in JSON.');
        return true;
      }());

      return AppBrushRunStep(
        kind: mapValueOfType<String>(json, r'kind')!,
        text: mapValueOfType<String>(json, r'text')!,
      );
    }
    return null;
  }

  static List<AppBrushRunStep> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppBrushRunStep>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppBrushRunStep.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppBrushRunStep> mapFromJson(dynamic json) {
    final map = <String, AppBrushRunStep>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppBrushRunStep.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppBrushRunStep-objects as value to a dart map
  static Map<String, List<AppBrushRunStep>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppBrushRunStep>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppBrushRunStep.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'kind',
    'text',
  };
}

