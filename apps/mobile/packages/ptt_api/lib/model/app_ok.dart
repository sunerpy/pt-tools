//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppOK {
  /// Returns a new [AppOK] instance.
  AppOK({
    required this.ok,
  });

  bool ok;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppOK &&
    other.ok == ok;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (ok.hashCode);

  @override
  String toString() => 'AppOK[ok=$ok]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'ok'] = this.ok;
    return json;
  }

  /// Returns a new [AppOK] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppOK? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'ok'), 'Required key "AppOK[ok]" is missing from JSON.');
        assert(json[r'ok'] != null, 'Required key "AppOK[ok]" has a null value in JSON.');
        return true;
      }());

      return AppOK(
        ok: mapValueOfType<bool>(json, r'ok')!,
      );
    }
    return null;
  }

  static List<AppOK> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppOK>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppOK.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppOK> mapFromJson(dynamic json) {
    final map = <String, AppOK>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppOK.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppOK-objects as value to a dart map
  static Map<String, List<AppOK>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppOK>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppOK.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'ok',
  };
}

