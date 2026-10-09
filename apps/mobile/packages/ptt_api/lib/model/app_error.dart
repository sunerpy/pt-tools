//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppError {
  /// Returns a new [AppError] instance.
  AppError({
    required this.error,
    required this.message,
  });

  /// invalid_body、invalid_argument、unauthorized、forbidden、not_found、method_not_allowed、busy、rate_limited、internal、search_failed、download_failed、attend_failed、upstream、unavailable 等
  String error;

  String message;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppError &&
    other.error == error &&
    other.message == message;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (error.hashCode) +
    (message.hashCode);

  @override
  String toString() => 'AppError[error=$error, message=$message]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'error'] = this.error;
      json[r'message'] = this.message;
    return json;
  }

  /// Returns a new [AppError] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppError? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'error'), 'Required key "AppError[error]" is missing from JSON.');
        assert(json[r'error'] != null, 'Required key "AppError[error]" has a null value in JSON.');
        assert(json.containsKey(r'message'), 'Required key "AppError[message]" is missing from JSON.');
        assert(json[r'message'] != null, 'Required key "AppError[message]" has a null value in JSON.');
        return true;
      }());

      return AppError(
        error: mapValueOfType<String>(json, r'error')!,
        message: mapValueOfType<String>(json, r'message')!,
      );
    }
    return null;
  }

  static List<AppError> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppError>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppError.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppError> mapFromJson(dynamic json) {
    final map = <String, AppError>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppError.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppError-objects as value to a dart map
  static Map<String, List<AppError>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppError>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppError.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'error',
    'message',
  };
}

