//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppBrushTaskPeriods {
  /// Returns a new [AppBrushTaskPeriods] instance.
  AppBrushTaskPeriods({
    required this.id,
    required this.name,
    this.periods = const [],
  });

  int id;

  String name;

  List<AppBrushPeriod> periods;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppBrushTaskPeriods &&
    other.id == id &&
    other.name == name &&
    _deepEquality.equals(other.periods, periods);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (name.hashCode) +
    (periods.hashCode);

  @override
  String toString() => 'AppBrushTaskPeriods[id=$id, name=$name, periods=$periods]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'name'] = this.name;
      json[r'periods'] = this.periods;
    return json;
  }

  /// Returns a new [AppBrushTaskPeriods] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppBrushTaskPeriods? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppBrushTaskPeriods[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppBrushTaskPeriods[id]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppBrushTaskPeriods[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppBrushTaskPeriods[name]" has a null value in JSON.');
        assert(json.containsKey(r'periods'), 'Required key "AppBrushTaskPeriods[periods]" is missing from JSON.');
        assert(json[r'periods'] != null, 'Required key "AppBrushTaskPeriods[periods]" has a null value in JSON.');
        return true;
      }());

      return AppBrushTaskPeriods(
        id: mapValueOfType<int>(json, r'id')!,
        name: mapValueOfType<String>(json, r'name')!,
        periods: AppBrushPeriod.listFromJson(json[r'periods']),
      );
    }
    return null;
  }

  static List<AppBrushTaskPeriods> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppBrushTaskPeriods>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppBrushTaskPeriods.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppBrushTaskPeriods> mapFromJson(dynamic json) {
    final map = <String, AppBrushTaskPeriods>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppBrushTaskPeriods.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppBrushTaskPeriods-objects as value to a dart map
  static Map<String, List<AppBrushTaskPeriods>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppBrushTaskPeriods>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppBrushTaskPeriods.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'name',
    'periods',
  };
}

