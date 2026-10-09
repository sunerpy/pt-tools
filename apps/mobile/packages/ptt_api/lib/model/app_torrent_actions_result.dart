//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTorrentActionsResult {
  /// Returns a new [AppTorrentActionsResult] instance.
  AppTorrentActionsResult({
    required this.succeeded,
    required this.failed,
    this.results = const [],
  });

  int succeeded;

  int failed;

  List<AppTorrentActionResult> results;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTorrentActionsResult &&
    other.succeeded == succeeded &&
    other.failed == failed &&
    _deepEquality.equals(other.results, results);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (succeeded.hashCode) +
    (failed.hashCode) +
    (results.hashCode);

  @override
  String toString() => 'AppTorrentActionsResult[succeeded=$succeeded, failed=$failed, results=$results]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'succeeded'] = this.succeeded;
      json[r'failed'] = this.failed;
      json[r'results'] = this.results;
    return json;
  }

  /// Returns a new [AppTorrentActionsResult] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTorrentActionsResult? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'succeeded'), 'Required key "AppTorrentActionsResult[succeeded]" is missing from JSON.');
        assert(json[r'succeeded'] != null, 'Required key "AppTorrentActionsResult[succeeded]" has a null value in JSON.');
        assert(json.containsKey(r'failed'), 'Required key "AppTorrentActionsResult[failed]" is missing from JSON.');
        assert(json[r'failed'] != null, 'Required key "AppTorrentActionsResult[failed]" has a null value in JSON.');
        assert(json.containsKey(r'results'), 'Required key "AppTorrentActionsResult[results]" is missing from JSON.');
        assert(json[r'results'] != null, 'Required key "AppTorrentActionsResult[results]" has a null value in JSON.');
        return true;
      }());

      return AppTorrentActionsResult(
        succeeded: mapValueOfType<int>(json, r'succeeded')!,
        failed: mapValueOfType<int>(json, r'failed')!,
        results: AppTorrentActionResult.listFromJson(json[r'results']),
      );
    }
    return null;
  }

  static List<AppTorrentActionsResult> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTorrentActionsResult>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTorrentActionsResult.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTorrentActionsResult> mapFromJson(dynamic json) {
    final map = <String, AppTorrentActionsResult>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTorrentActionsResult.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTorrentActionsResult-objects as value to a dart map
  static Map<String, List<AppTorrentActionsResult>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTorrentActionsResult>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTorrentActionsResult.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'succeeded',
    'failed',
    'results',
  };
}

