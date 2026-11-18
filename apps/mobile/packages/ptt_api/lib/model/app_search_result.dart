//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchResult {
  /// Returns a new [AppSearchResult] instance.
  AppSearchResult({
    this.items = const [],
    required this.total,
    this.sites = const {},
    this.errors = const [],
    required this.durationMs,
  });

  List<AppSearchItem> items;

  int total;

  Map<String, int> sites;

  List<AppSearchError> errors;

  int durationMs;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchResult &&
    _deepEquality.equals(other.items, items) &&
    other.total == total &&
    _deepEquality.equals(other.sites, sites) &&
    _deepEquality.equals(other.errors, errors) &&
    other.durationMs == durationMs;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (items.hashCode) +
    (total.hashCode) +
    (sites.hashCode) +
    (errors.hashCode) +
    (durationMs.hashCode);

  @override
  String toString() => 'AppSearchResult[items=$items, total=$total, sites=$sites, errors=$errors, durationMs=$durationMs]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'items'] = this.items;
      json[r'total'] = this.total;
      json[r'sites'] = this.sites;
      json[r'errors'] = this.errors;
      json[r'duration_ms'] = this.durationMs;
    return json;
  }

  /// Returns a new [AppSearchResult] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchResult? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'items'), 'Required key "AppSearchResult[items]" is missing from JSON.');
        assert(json[r'items'] != null, 'Required key "AppSearchResult[items]" has a null value in JSON.');
        assert(json.containsKey(r'total'), 'Required key "AppSearchResult[total]" is missing from JSON.');
        assert(json[r'total'] != null, 'Required key "AppSearchResult[total]" has a null value in JSON.');
        assert(json.containsKey(r'sites'), 'Required key "AppSearchResult[sites]" is missing from JSON.');
        assert(json[r'sites'] != null, 'Required key "AppSearchResult[sites]" has a null value in JSON.');
        assert(json.containsKey(r'errors'), 'Required key "AppSearchResult[errors]" is missing from JSON.');
        assert(json[r'errors'] != null, 'Required key "AppSearchResult[errors]" has a null value in JSON.');
        assert(json.containsKey(r'duration_ms'), 'Required key "AppSearchResult[duration_ms]" is missing from JSON.');
        assert(json[r'duration_ms'] != null, 'Required key "AppSearchResult[duration_ms]" has a null value in JSON.');
        return true;
      }());

      return AppSearchResult(
        items: AppSearchItem.listFromJson(json[r'items']),
        total: mapValueOfType<int>(json, r'total')!,
        sites: mapCastOfType<String, int>(json, r'sites')!,
        errors: AppSearchError.listFromJson(json[r'errors']),
        durationMs: mapValueOfType<int>(json, r'duration_ms')!,
      );
    }
    return null;
  }

  static List<AppSearchResult> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchResult>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchResult.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchResult> mapFromJson(dynamic json) {
    final map = <String, AppSearchResult>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchResult.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchResult-objects as value to a dart map
  static Map<String, List<AppSearchResult>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchResult>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchResult.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'items',
    'total',
    'sites',
    'errors',
    'duration_ms',
  };
}

