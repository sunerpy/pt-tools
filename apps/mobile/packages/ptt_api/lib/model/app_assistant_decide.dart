//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantDecide {
  /// Returns a new [AppAssistantDecide] instance.
  AppAssistantDecide({
    required this.approve,
  });

  /// true 确认执行，false 拒绝
  bool approve;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantDecide &&
    other.approve == approve;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (approve.hashCode);

  @override
  String toString() => 'AppAssistantDecide[approve=$approve]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'approve'] = this.approve;
    return json;
  }

  /// Returns a new [AppAssistantDecide] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantDecide? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'approve'), 'Required key "AppAssistantDecide[approve]" is missing from JSON.');
        assert(json[r'approve'] != null, 'Required key "AppAssistantDecide[approve]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantDecide(
        approve: mapValueOfType<bool>(json, r'approve')!,
      );
    }
    return null;
  }

  static List<AppAssistantDecide> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantDecide>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantDecide.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantDecide> mapFromJson(dynamic json) {
    final map = <String, AppAssistantDecide>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantDecide.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantDecide-objects as value to a dart map
  static Map<String, List<AppAssistantDecide>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantDecide>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantDecide.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'approve',
  };
}

