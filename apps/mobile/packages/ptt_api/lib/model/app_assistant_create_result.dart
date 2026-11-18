//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantCreateResult {
  /// Returns a new [AppAssistantCreateResult] instance.
  AppAssistantCreateResult({
    required this.conversation,
    this.run,
  });

  AppAssistantConversation conversation;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppAssistantSendResult? run;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantCreateResult &&
    other.conversation == conversation &&
    other.run == run;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (conversation.hashCode) +
    (run == null ? 0 : run!.hashCode);

  @override
  String toString() => 'AppAssistantCreateResult[conversation=$conversation, run=$run]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'conversation'] = this.conversation;
    if (this.run != null) {
      json[r'run'] = this.run;
    } else {
      json[r'run'] = null;
    }
    return json;
  }

  /// Returns a new [AppAssistantCreateResult] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantCreateResult? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'conversation'), 'Required key "AppAssistantCreateResult[conversation]" is missing from JSON.');
        assert(json[r'conversation'] != null, 'Required key "AppAssistantCreateResult[conversation]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantCreateResult(
        conversation: AppAssistantConversation.fromJson(json[r'conversation'])!,
        run: AppAssistantSendResult.fromJson(json[r'run']),
      );
    }
    return null;
  }

  static List<AppAssistantCreateResult> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantCreateResult>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantCreateResult.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantCreateResult> mapFromJson(dynamic json) {
    final map = <String, AppAssistantCreateResult>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantCreateResult.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantCreateResult-objects as value to a dart map
  static Map<String, List<AppAssistantCreateResult>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantCreateResult>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantCreateResult.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'conversation',
  };
}

