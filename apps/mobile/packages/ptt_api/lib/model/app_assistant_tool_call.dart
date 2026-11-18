//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantToolCall {
  /// Returns a new [AppAssistantToolCall] instance.
  AppAssistantToolCall({
    required this.id,
    required this.name,
    required this.arguments,
    this.write,
  });

  String id;

  /// 工具名（与 MCP 的工具相同）
  String name;

  /// 规范化、脱敏以后的参数（JSON 文本）
  String arguments;

  /// 改动类操作（要确认）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? write;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantToolCall &&
    other.id == id &&
    other.name == name &&
    other.arguments == arguments &&
    other.write == write;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (name.hashCode) +
    (arguments.hashCode) +
    (write == null ? 0 : write!.hashCode);

  @override
  String toString() => 'AppAssistantToolCall[id=$id, name=$name, arguments=$arguments, write=$write]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'name'] = this.name;
      json[r'arguments'] = this.arguments;
    if (this.write != null) {
      json[r'write'] = this.write;
    } else {
      json[r'write'] = null;
    }
    return json;
  }

  /// Returns a new [AppAssistantToolCall] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantToolCall? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppAssistantToolCall[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppAssistantToolCall[id]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppAssistantToolCall[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppAssistantToolCall[name]" has a null value in JSON.');
        assert(json.containsKey(r'arguments'), 'Required key "AppAssistantToolCall[arguments]" is missing from JSON.');
        assert(json[r'arguments'] != null, 'Required key "AppAssistantToolCall[arguments]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantToolCall(
        id: mapValueOfType<String>(json, r'id')!,
        name: mapValueOfType<String>(json, r'name')!,
        arguments: mapValueOfType<String>(json, r'arguments')!,
        write: mapValueOfType<bool>(json, r'write'),
      );
    }
    return null;
  }

  static List<AppAssistantToolCall> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantToolCall>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantToolCall.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantToolCall> mapFromJson(dynamic json) {
    final map = <String, AppAssistantToolCall>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantToolCall.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantToolCall-objects as value to a dart map
  static Map<String, List<AppAssistantToolCall>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantToolCall>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantToolCall.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'name',
    'arguments',
  };
}

