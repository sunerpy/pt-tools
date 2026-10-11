//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantSend {
  /// Returns a new [AppAssistantSend] instance.
  AppAssistantSend({
    required this.text,
  });

  String text;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantSend &&
    other.text == text;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (text.hashCode);

  @override
  String toString() => 'AppAssistantSend[text=$text]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'text'] = this.text;
    return json;
  }

  /// Returns a new [AppAssistantSend] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantSend? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'text'), 'Required key "AppAssistantSend[text]" is missing from JSON.');
        assert(json[r'text'] != null, 'Required key "AppAssistantSend[text]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantSend(
        text: mapValueOfType<String>(json, r'text')!,
      );
    }
    return null;
  }

  static List<AppAssistantSend> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantSend>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantSend.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantSend> mapFromJson(dynamic json) {
    final map = <String, AppAssistantSend>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantSend.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantSend-objects as value to a dart map
  static Map<String, List<AppAssistantSend>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantSend>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantSend.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'text',
  };
}

