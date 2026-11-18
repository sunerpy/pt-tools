//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTemplatePreview {
  /// Returns a new [AppTemplatePreview] instance.
  AppTemplatePreview({
    required this.title,
    required this.text,
  });

  String title;

  String text;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTemplatePreview &&
    other.title == title &&
    other.text == text;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (title.hashCode) +
    (text.hashCode);

  @override
  String toString() => 'AppTemplatePreview[title=$title, text=$text]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'title'] = this.title;
      json[r'text'] = this.text;
    return json;
  }

  /// Returns a new [AppTemplatePreview] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTemplatePreview? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'title'), 'Required key "AppTemplatePreview[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppTemplatePreview[title]" has a null value in JSON.');
        assert(json.containsKey(r'text'), 'Required key "AppTemplatePreview[text]" is missing from JSON.');
        assert(json[r'text'] != null, 'Required key "AppTemplatePreview[text]" has a null value in JSON.');
        return true;
      }());

      return AppTemplatePreview(
        title: mapValueOfType<String>(json, r'title')!,
        text: mapValueOfType<String>(json, r'text')!,
      );
    }
    return null;
  }

  static List<AppTemplatePreview> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTemplatePreview>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTemplatePreview.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTemplatePreview> mapFromJson(dynamic json) {
    final map = <String, AppTemplatePreview>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTemplatePreview.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTemplatePreview-objects as value to a dart map
  static Map<String, List<AppTemplatePreview>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTemplatePreview>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTemplatePreview.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'title',
    'text',
  };
}

