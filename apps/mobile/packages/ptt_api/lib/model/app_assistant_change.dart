//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantChange {
  /// Returns a new [AppAssistantChange] instance.
  AppAssistantChange({
    required this.label,
    this.before,
    this.after,
    this.secret,
  });

  String label;

  /// 新增的没有
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? before;

  /// 删掉的没有
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? after;

  /// 密钥：两边都不写原文
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? secret;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantChange &&
    other.label == label &&
    other.before == before &&
    other.after == after &&
    other.secret == secret;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (label.hashCode) +
    (before == null ? 0 : before!.hashCode) +
    (after == null ? 0 : after!.hashCode) +
    (secret == null ? 0 : secret!.hashCode);

  @override
  String toString() => 'AppAssistantChange[label=$label, before=$before, after=$after, secret=$secret]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'label'] = this.label;
    if (this.before != null) {
      json[r'before'] = this.before;
    } else {
      json[r'before'] = null;
    }
    if (this.after != null) {
      json[r'after'] = this.after;
    } else {
      json[r'after'] = null;
    }
    if (this.secret != null) {
      json[r'secret'] = this.secret;
    } else {
      json[r'secret'] = null;
    }
    return json;
  }

  /// Returns a new [AppAssistantChange] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantChange? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'label'), 'Required key "AppAssistantChange[label]" is missing from JSON.');
        assert(json[r'label'] != null, 'Required key "AppAssistantChange[label]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantChange(
        label: mapValueOfType<String>(json, r'label')!,
        before: mapValueOfType<String>(json, r'before'),
        after: mapValueOfType<String>(json, r'after'),
        secret: mapValueOfType<bool>(json, r'secret'),
      );
    }
    return null;
  }

  static List<AppAssistantChange> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantChange>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantChange.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantChange> mapFromJson(dynamic json) {
    final map = <String, AppAssistantChange>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantChange.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantChange-objects as value to a dart map
  static Map<String, List<AppAssistantChange>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantChange>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantChange.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'label',
  };
}

