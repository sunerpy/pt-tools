//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppBrushStat {
  /// Returns a new [AppBrushStat] instance.
  AppBrushStat({
    required this.uploaded,
    required this.downloaded,
    required this.added,
    required this.removed,
  });

  int uploaded;

  int downloaded;

  int added;

  int removed;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppBrushStat &&
    other.uploaded == uploaded &&
    other.downloaded == downloaded &&
    other.added == added &&
    other.removed == removed;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (uploaded.hashCode) +
    (downloaded.hashCode) +
    (added.hashCode) +
    (removed.hashCode);

  @override
  String toString() => 'AppBrushStat[uploaded=$uploaded, downloaded=$downloaded, added=$added, removed=$removed]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'uploaded'] = this.uploaded;
      json[r'downloaded'] = this.downloaded;
      json[r'added'] = this.added;
      json[r'removed'] = this.removed;
    return json;
  }

  /// Returns a new [AppBrushStat] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppBrushStat? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'uploaded'), 'Required key "AppBrushStat[uploaded]" is missing from JSON.');
        assert(json[r'uploaded'] != null, 'Required key "AppBrushStat[uploaded]" has a null value in JSON.');
        assert(json.containsKey(r'downloaded'), 'Required key "AppBrushStat[downloaded]" is missing from JSON.');
        assert(json[r'downloaded'] != null, 'Required key "AppBrushStat[downloaded]" has a null value in JSON.');
        assert(json.containsKey(r'added'), 'Required key "AppBrushStat[added]" is missing from JSON.');
        assert(json[r'added'] != null, 'Required key "AppBrushStat[added]" has a null value in JSON.');
        assert(json.containsKey(r'removed'), 'Required key "AppBrushStat[removed]" is missing from JSON.');
        assert(json[r'removed'] != null, 'Required key "AppBrushStat[removed]" has a null value in JSON.');
        return true;
      }());

      return AppBrushStat(
        uploaded: mapValueOfType<int>(json, r'uploaded')!,
        downloaded: mapValueOfType<int>(json, r'downloaded')!,
        added: mapValueOfType<int>(json, r'added')!,
        removed: mapValueOfType<int>(json, r'removed')!,
      );
    }
    return null;
  }

  static List<AppBrushStat> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppBrushStat>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppBrushStat.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppBrushStat> mapFromJson(dynamic json) {
    final map = <String, AppBrushStat>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppBrushStat.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppBrushStat-objects as value to a dart map
  static Map<String, List<AppBrushStat>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppBrushStat>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppBrushStat.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'uploaded',
    'downloaded',
    'added',
    'removed',
  };
}

