//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppBrushPeriod {
  /// Returns a new [AppBrushPeriod] instance.
  AppBrushPeriod({
    required this.start,
    required this.end,
    required this.uploaded,
    required this.downloaded,
    required this.added,
    required this.removed,
  });

  /// 这一段的第一天（yyyy-mm-dd）
  String start;

  /// 这一段的最后一天；最后一段可能在今天以后
  String end;

  int uploaded;

  int downloaded;

  int added;

  int removed;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppBrushPeriod &&
    other.start == start &&
    other.end == end &&
    other.uploaded == uploaded &&
    other.downloaded == downloaded &&
    other.added == added &&
    other.removed == removed;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (start.hashCode) +
    (end.hashCode) +
    (uploaded.hashCode) +
    (downloaded.hashCode) +
    (added.hashCode) +
    (removed.hashCode);

  @override
  String toString() => 'AppBrushPeriod[start=$start, end=$end, uploaded=$uploaded, downloaded=$downloaded, added=$added, removed=$removed]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'start'] = this.start;
      json[r'end'] = this.end;
      json[r'uploaded'] = this.uploaded;
      json[r'downloaded'] = this.downloaded;
      json[r'added'] = this.added;
      json[r'removed'] = this.removed;
    return json;
  }

  /// Returns a new [AppBrushPeriod] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppBrushPeriod? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'start'), 'Required key "AppBrushPeriod[start]" is missing from JSON.');
        assert(json[r'start'] != null, 'Required key "AppBrushPeriod[start]" has a null value in JSON.');
        assert(json.containsKey(r'end'), 'Required key "AppBrushPeriod[end]" is missing from JSON.');
        assert(json[r'end'] != null, 'Required key "AppBrushPeriod[end]" has a null value in JSON.');
        assert(json.containsKey(r'uploaded'), 'Required key "AppBrushPeriod[uploaded]" is missing from JSON.');
        assert(json[r'uploaded'] != null, 'Required key "AppBrushPeriod[uploaded]" has a null value in JSON.');
        assert(json.containsKey(r'downloaded'), 'Required key "AppBrushPeriod[downloaded]" is missing from JSON.');
        assert(json[r'downloaded'] != null, 'Required key "AppBrushPeriod[downloaded]" has a null value in JSON.');
        assert(json.containsKey(r'added'), 'Required key "AppBrushPeriod[added]" is missing from JSON.');
        assert(json[r'added'] != null, 'Required key "AppBrushPeriod[added]" has a null value in JSON.');
        assert(json.containsKey(r'removed'), 'Required key "AppBrushPeriod[removed]" is missing from JSON.');
        assert(json[r'removed'] != null, 'Required key "AppBrushPeriod[removed]" has a null value in JSON.');
        return true;
      }());

      return AppBrushPeriod(
        start: mapValueOfType<String>(json, r'start')!,
        end: mapValueOfType<String>(json, r'end')!,
        uploaded: mapValueOfType<int>(json, r'uploaded')!,
        downloaded: mapValueOfType<int>(json, r'downloaded')!,
        added: mapValueOfType<int>(json, r'added')!,
        removed: mapValueOfType<int>(json, r'removed')!,
      );
    }
    return null;
  }

  static List<AppBrushPeriod> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppBrushPeriod>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppBrushPeriod.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppBrushPeriod> mapFromJson(dynamic json) {
    final map = <String, AppBrushPeriod>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppBrushPeriod.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppBrushPeriod-objects as value to a dart map
  static Map<String, List<AppBrushPeriod>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppBrushPeriod>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppBrushPeriod.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'start',
    'end',
    'uploaded',
    'downloaded',
    'added',
    'removed',
  };
}

