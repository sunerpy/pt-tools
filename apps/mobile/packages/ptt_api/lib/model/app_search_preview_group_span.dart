//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewGroupSpan {
  /// Returns a new [AppSearchPreviewGroupSpan] instance.
  AppSearchPreviewGroupSpan({
    required this.start,
    required this.end,
    required this.whole,
  });

  int start;

  int end;

  bool whole;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewGroupSpan &&
    other.start == start &&
    other.end == end &&
    other.whole == whole;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (start.hashCode) +
    (end.hashCode) +
    (whole.hashCode);

  @override
  String toString() => 'AppSearchPreviewGroupSpan[start=$start, end=$end, whole=$whole]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'start'] = this.start;
      json[r'end'] = this.end;
      json[r'whole'] = this.whole;
    return json;
  }

  /// Returns a new [AppSearchPreviewGroupSpan] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewGroupSpan? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'start'), 'Required key "AppSearchPreviewGroupSpan[start]" is missing from JSON.');
        assert(json[r'start'] != null, 'Required key "AppSearchPreviewGroupSpan[start]" has a null value in JSON.');
        assert(json.containsKey(r'end'), 'Required key "AppSearchPreviewGroupSpan[end]" is missing from JSON.');
        assert(json[r'end'] != null, 'Required key "AppSearchPreviewGroupSpan[end]" has a null value in JSON.');
        assert(json.containsKey(r'whole'), 'Required key "AppSearchPreviewGroupSpan[whole]" is missing from JSON.');
        assert(json[r'whole'] != null, 'Required key "AppSearchPreviewGroupSpan[whole]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewGroupSpan(
        start: mapValueOfType<int>(json, r'start')!,
        end: mapValueOfType<int>(json, r'end')!,
        whole: mapValueOfType<bool>(json, r'whole')!,
      );
    }
    return null;
  }

  static List<AppSearchPreviewGroupSpan> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewGroupSpan>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewGroupSpan.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewGroupSpan> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewGroupSpan>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewGroupSpan.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewGroupSpan-objects as value to a dart map
  static Map<String, List<AppSearchPreviewGroupSpan>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewGroupSpan>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewGroupSpan.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'start',
    'end',
    'whole',
  };
}

