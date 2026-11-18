//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTemplatePreviewInput {
  /// Returns a new [AppTemplatePreviewInput] instance.
  AppTemplatePreviewInput({
    required this.type,
    this.title,
    this.text,
  });

  String type;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? title;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? text;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTemplatePreviewInput &&
    other.type == type &&
    other.title == title &&
    other.text == text;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (type.hashCode) +
    (title == null ? 0 : title!.hashCode) +
    (text == null ? 0 : text!.hashCode);

  @override
  String toString() => 'AppTemplatePreviewInput[type=$type, title=$title, text=$text]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'type'] = this.type;
    if (this.title != null) {
      json[r'title'] = this.title;
    } else {
      json[r'title'] = null;
    }
    if (this.text != null) {
      json[r'text'] = this.text;
    } else {
      json[r'text'] = null;
    }
    return json;
  }

  /// Returns a new [AppTemplatePreviewInput] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTemplatePreviewInput? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'type'), 'Required key "AppTemplatePreviewInput[type]" is missing from JSON.');
        assert(json[r'type'] != null, 'Required key "AppTemplatePreviewInput[type]" has a null value in JSON.');
        return true;
      }());

      return AppTemplatePreviewInput(
        type: mapValueOfType<String>(json, r'type')!,
        title: mapValueOfType<String>(json, r'title'),
        text: mapValueOfType<String>(json, r'text'),
      );
    }
    return null;
  }

  static List<AppTemplatePreviewInput> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTemplatePreviewInput>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTemplatePreviewInput.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTemplatePreviewInput> mapFromJson(dynamic json) {
    final map = <String, AppTemplatePreviewInput>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTemplatePreviewInput.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTemplatePreviewInput-objects as value to a dart map
  static Map<String, List<AppTemplatePreviewInput>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTemplatePreviewInput>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTemplatePreviewInput.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'type',
  };
}

