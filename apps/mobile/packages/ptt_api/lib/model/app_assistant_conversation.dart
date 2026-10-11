//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantConversation {
  /// Returns a new [AppAssistantConversation] instance.
  AppAssistantConversation({
    required this.id,
    required this.title,
    required this.entry,
    required this.running,
    required this.createdAt,
    required this.updatedAt,
  });

  int id;

  String title;

  /// web、app 或 chatops：在哪里建的
  String entry;

  bool running;

  DateTime createdAt;

  DateTime updatedAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantConversation &&
    other.id == id &&
    other.title == title &&
    other.entry == entry &&
    other.running == running &&
    other.createdAt == createdAt &&
    other.updatedAt == updatedAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (title.hashCode) +
    (entry.hashCode) +
    (running.hashCode) +
    (createdAt.hashCode) +
    (updatedAt.hashCode);

  @override
  String toString() => 'AppAssistantConversation[id=$id, title=$title, entry=$entry, running=$running, createdAt=$createdAt, updatedAt=$updatedAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'title'] = this.title;
      json[r'entry'] = this.entry;
      json[r'running'] = this.running;
      json[r'created_at'] = this.createdAt.toUtc().toIso8601String();
      json[r'updated_at'] = this.updatedAt.toUtc().toIso8601String();
    return json;
  }

  /// Returns a new [AppAssistantConversation] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantConversation? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppAssistantConversation[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppAssistantConversation[id]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppAssistantConversation[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppAssistantConversation[title]" has a null value in JSON.');
        assert(json.containsKey(r'entry'), 'Required key "AppAssistantConversation[entry]" is missing from JSON.');
        assert(json[r'entry'] != null, 'Required key "AppAssistantConversation[entry]" has a null value in JSON.');
        assert(json.containsKey(r'running'), 'Required key "AppAssistantConversation[running]" is missing from JSON.');
        assert(json[r'running'] != null, 'Required key "AppAssistantConversation[running]" has a null value in JSON.');
        assert(json.containsKey(r'created_at'), 'Required key "AppAssistantConversation[created_at]" is missing from JSON.');
        assert(json[r'created_at'] != null, 'Required key "AppAssistantConversation[created_at]" has a null value in JSON.');
        assert(json.containsKey(r'updated_at'), 'Required key "AppAssistantConversation[updated_at]" is missing from JSON.');
        assert(json[r'updated_at'] != null, 'Required key "AppAssistantConversation[updated_at]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantConversation(
        id: mapValueOfType<int>(json, r'id')!,
        title: mapValueOfType<String>(json, r'title')!,
        entry: mapValueOfType<String>(json, r'entry')!,
        running: mapValueOfType<bool>(json, r'running')!,
        createdAt: mapDateTime(json, r'created_at', r'')!,
        updatedAt: mapDateTime(json, r'updated_at', r'')!,
      );
    }
    return null;
  }

  static List<AppAssistantConversation> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantConversation>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantConversation.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantConversation> mapFromJson(dynamic json) {
    final map = <String, AppAssistantConversation>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantConversation.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantConversation-objects as value to a dart map
  static Map<String, List<AppAssistantConversation>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantConversation>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantConversation.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'title',
    'entry',
    'running',
    'created_at',
    'updated_at',
  };
}

