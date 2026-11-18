//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewError {
  /// Returns a new [AppSearchPreviewError] instance.
  AppSearchPreviewError({
    required this.site,
    required this.error,
    required this.timedOut,
  });

  /// 站点；为空时是一次搜索
  String site;

  String error;

  /// 到了时限还没有回应
  bool timedOut;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewError &&
    other.site == site &&
    other.error == error &&
    other.timedOut == timedOut;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (error.hashCode) +
    (timedOut.hashCode);

  @override
  String toString() => 'AppSearchPreviewError[site=$site, error=$error, timedOut=$timedOut]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'error'] = this.error;
      json[r'timed_out'] = this.timedOut;
    return json;
  }

  /// Returns a new [AppSearchPreviewError] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewError? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppSearchPreviewError[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppSearchPreviewError[site]" has a null value in JSON.');
        assert(json.containsKey(r'error'), 'Required key "AppSearchPreviewError[error]" is missing from JSON.');
        assert(json[r'error'] != null, 'Required key "AppSearchPreviewError[error]" has a null value in JSON.');
        assert(json.containsKey(r'timed_out'), 'Required key "AppSearchPreviewError[timed_out]" is missing from JSON.');
        assert(json[r'timed_out'] != null, 'Required key "AppSearchPreviewError[timed_out]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewError(
        site: mapValueOfType<String>(json, r'site')!,
        error: mapValueOfType<String>(json, r'error')!,
        timedOut: mapValueOfType<bool>(json, r'timed_out')!,
      );
    }
    return null;
  }

  static List<AppSearchPreviewError> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewError>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewError.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewError> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewError>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewError.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewError-objects as value to a dart map
  static Map<String, List<AppSearchPreviewError>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewError>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewError.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'error',
    'timed_out',
  };
}

