//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppConfigChange {
  /// Returns a new [AppConfigChange] instance.
  AppConfigChange({
    required this.op,
    this.id,
    this.values = const {},
  });

  /// set、add、update 或 delete
  String op;

  /// update、delete 要给：id_field 的值
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? id;

  /// 只写要改的字段；密钥给新值才改，交回占位表示不改，给空串表示清除
  Map<String, Object> values;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppConfigChange &&
    other.op == op &&
    other.id == id &&
    _deepEquality.equals(other.values, values);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (op.hashCode) +
    (id == null ? 0 : id!.hashCode) +
    (values.hashCode);

  @override
  String toString() => 'AppConfigChange[op=$op, id=$id, values=$values]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'op'] = this.op;
    if (this.id != null) {
      json[r'id'] = this.id;
    } else {
      json[r'id'] = null;
    }
      json[r'values'] = this.values;
    return json;
  }

  /// Returns a new [AppConfigChange] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppConfigChange? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'op'), 'Required key "AppConfigChange[op]" is missing from JSON.');
        assert(json[r'op'] != null, 'Required key "AppConfigChange[op]" has a null value in JSON.');
        return true;
      }());

      return AppConfigChange(
        op: mapValueOfType<String>(json, r'op')!,
        id: mapValueOfType<String>(json, r'id'),
        values: mapCastOfType<String, Object>(json, r'values') ?? const {},
      );
    }
    return null;
  }

  static List<AppConfigChange> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppConfigChange>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppConfigChange.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppConfigChange> mapFromJson(dynamic json) {
    final map = <String, AppConfigChange>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppConfigChange.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppConfigChange-objects as value to a dart map
  static Map<String, List<AppConfigChange>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppConfigChange>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppConfigChange.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'op',
  };
}

