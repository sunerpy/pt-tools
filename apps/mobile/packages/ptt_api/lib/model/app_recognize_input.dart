//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppRecognizeInput {
  /// Returns a new [AppRecognizeInput] instance.
  AppRecognizeInput({
    this.title,
    this.subtitle,
    this.imdbId,
  });

  /// 种子标题；与 subtitle 至少给一个
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? title;

  /// 站点给的副标题（常是中文名）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? subtitle;

  /// 站点给的 IMDb 编号（tt1234567），可选
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? imdbId;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppRecognizeInput &&
    other.title == title &&
    other.subtitle == subtitle &&
    other.imdbId == imdbId;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (title == null ? 0 : title!.hashCode) +
    (subtitle == null ? 0 : subtitle!.hashCode) +
    (imdbId == null ? 0 : imdbId!.hashCode);

  @override
  String toString() => 'AppRecognizeInput[title=$title, subtitle=$subtitle, imdbId=$imdbId]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
    if (this.title != null) {
      json[r'title'] = this.title;
    } else {
      json[r'title'] = null;
    }
    if (this.subtitle != null) {
      json[r'subtitle'] = this.subtitle;
    } else {
      json[r'subtitle'] = null;
    }
    if (this.imdbId != null) {
      json[r'imdb_id'] = this.imdbId;
    } else {
      json[r'imdb_id'] = null;
    }
    return json;
  }

  /// Returns a new [AppRecognizeInput] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppRecognizeInput? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        return true;
      }());

      return AppRecognizeInput(
        title: mapValueOfType<String>(json, r'title'),
        subtitle: mapValueOfType<String>(json, r'subtitle'),
        imdbId: mapValueOfType<String>(json, r'imdb_id'),
      );
    }
    return null;
  }

  static List<AppRecognizeInput> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppRecognizeInput>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppRecognizeInput.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppRecognizeInput> mapFromJson(dynamic json) {
    final map = <String, AppRecognizeInput>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppRecognizeInput.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppRecognizeInput-objects as value to a dart map
  static Map<String, List<AppRecognizeInput>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppRecognizeInput>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppRecognizeInput.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
  };
}

