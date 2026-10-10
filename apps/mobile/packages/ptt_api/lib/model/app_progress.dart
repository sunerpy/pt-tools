//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppProgress {
  /// Returns a new [AppProgress] instance.
  AppProgress({
    required this.total,
    required this.aired,
    required this.inLibrary,
    required this.downloading,
    this.missing = const [],
  });

  int total;

  int aired;

  int inLibrary;

  int downloading;

  List<int> missing;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppProgress &&
    other.total == total &&
    other.aired == aired &&
    other.inLibrary == inLibrary &&
    other.downloading == downloading &&
    _deepEquality.equals(other.missing, missing);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (total.hashCode) +
    (aired.hashCode) +
    (inLibrary.hashCode) +
    (downloading.hashCode) +
    (missing.hashCode);

  @override
  String toString() => 'AppProgress[total=$total, aired=$aired, inLibrary=$inLibrary, downloading=$downloading, missing=$missing]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'total'] = this.total;
      json[r'aired'] = this.aired;
      json[r'in_library'] = this.inLibrary;
      json[r'downloading'] = this.downloading;
      json[r'missing'] = this.missing;
    return json;
  }

  /// Returns a new [AppProgress] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppProgress? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'total'), 'Required key "AppProgress[total]" is missing from JSON.');
        assert(json[r'total'] != null, 'Required key "AppProgress[total]" has a null value in JSON.');
        assert(json.containsKey(r'aired'), 'Required key "AppProgress[aired]" is missing from JSON.');
        assert(json[r'aired'] != null, 'Required key "AppProgress[aired]" has a null value in JSON.');
        assert(json.containsKey(r'in_library'), 'Required key "AppProgress[in_library]" is missing from JSON.');
        assert(json[r'in_library'] != null, 'Required key "AppProgress[in_library]" has a null value in JSON.');
        assert(json.containsKey(r'downloading'), 'Required key "AppProgress[downloading]" is missing from JSON.');
        assert(json[r'downloading'] != null, 'Required key "AppProgress[downloading]" has a null value in JSON.');
        assert(json.containsKey(r'missing'), 'Required key "AppProgress[missing]" is missing from JSON.');
        assert(json[r'missing'] != null, 'Required key "AppProgress[missing]" has a null value in JSON.');
        return true;
      }());

      return AppProgress(
        total: mapValueOfType<int>(json, r'total')!,
        aired: mapValueOfType<int>(json, r'aired')!,
        inLibrary: mapValueOfType<int>(json, r'in_library')!,
        downloading: mapValueOfType<int>(json, r'downloading')!,
        missing: json[r'missing'] is Iterable
            ? (json[r'missing'] as Iterable).cast<int>().toList(growable: false)
            : const [],
      );
    }
    return null;
  }

  static List<AppProgress> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppProgress>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppProgress.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppProgress> mapFromJson(dynamic json) {
    final map = <String, AppProgress>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppProgress.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppProgress-objects as value to a dart map
  static Map<String, List<AppProgress>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppProgress>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppProgress.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'total',
    'aired',
    'in_library',
    'downloading',
    'missing',
  };
}

