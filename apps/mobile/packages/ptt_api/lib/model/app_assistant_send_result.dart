//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantSendResult {
  /// Returns a new [AppAssistantSendResult] instance.
  AppAssistantSendResult({
    required this.runId,
    required this.message,
  });

  String runId;

  AppAssistantMessage message;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantSendResult &&
    other.runId == runId &&
    other.message == message;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (runId.hashCode) +
    (message.hashCode);

  @override
  String toString() => 'AppAssistantSendResult[runId=$runId, message=$message]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'run_id'] = this.runId;
      json[r'message'] = this.message;
    return json;
  }

  /// Returns a new [AppAssistantSendResult] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantSendResult? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'run_id'), 'Required key "AppAssistantSendResult[run_id]" is missing from JSON.');
        assert(json[r'run_id'] != null, 'Required key "AppAssistantSendResult[run_id]" has a null value in JSON.');
        assert(json.containsKey(r'message'), 'Required key "AppAssistantSendResult[message]" is missing from JSON.');
        assert(json[r'message'] != null, 'Required key "AppAssistantSendResult[message]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantSendResult(
        runId: mapValueOfType<String>(json, r'run_id')!,
        message: AppAssistantMessage.fromJson(json[r'message'])!,
      );
    }
    return null;
  }

  static List<AppAssistantSendResult> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantSendResult>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantSendResult.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantSendResult> mapFromJson(dynamic json) {
    final map = <String, AppAssistantSendResult>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantSendResult.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantSendResult-objects as value to a dart map
  static Map<String, List<AppAssistantSendResult>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantSendResult>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantSendResult.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'run_id',
    'message',
  };
}

