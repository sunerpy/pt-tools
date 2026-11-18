//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSiteAttendance {
  /// Returns a new [AppSiteAttendance] instance.
  AppSiteAttendance({
    required this.supported,
    required this.enabled,
    required this.day,
    this.status,
    this.message,
    this.lastAttemptAt,
  });

  bool supported;

  bool enabled;

  String day;

  /// pending、signed、already、failed 或 unsupported
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? status;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? message;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? lastAttemptAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSiteAttendance &&
    other.supported == supported &&
    other.enabled == enabled &&
    other.day == day &&
    other.status == status &&
    other.message == message &&
    other.lastAttemptAt == lastAttemptAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (supported.hashCode) +
    (enabled.hashCode) +
    (day.hashCode) +
    (status == null ? 0 : status!.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (lastAttemptAt == null ? 0 : lastAttemptAt!.hashCode);

  @override
  String toString() => 'AppSiteAttendance[supported=$supported, enabled=$enabled, day=$day, status=$status, message=$message, lastAttemptAt=$lastAttemptAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'supported'] = this.supported;
      json[r'enabled'] = this.enabled;
      json[r'day'] = this.day;
    if (this.status != null) {
      json[r'status'] = this.status;
    } else {
      json[r'status'] = null;
    }
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
    }
    if (this.lastAttemptAt != null) {
      json[r'last_attempt_at'] = this.lastAttemptAt;
    } else {
      json[r'last_attempt_at'] = null;
    }
    return json;
  }

  /// Returns a new [AppSiteAttendance] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSiteAttendance? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'supported'), 'Required key "AppSiteAttendance[supported]" is missing from JSON.');
        assert(json[r'supported'] != null, 'Required key "AppSiteAttendance[supported]" has a null value in JSON.');
        assert(json.containsKey(r'enabled'), 'Required key "AppSiteAttendance[enabled]" is missing from JSON.');
        assert(json[r'enabled'] != null, 'Required key "AppSiteAttendance[enabled]" has a null value in JSON.');
        assert(json.containsKey(r'day'), 'Required key "AppSiteAttendance[day]" is missing from JSON.');
        assert(json[r'day'] != null, 'Required key "AppSiteAttendance[day]" has a null value in JSON.');
        return true;
      }());

      return AppSiteAttendance(
        supported: mapValueOfType<bool>(json, r'supported')!,
        enabled: mapValueOfType<bool>(json, r'enabled')!,
        day: mapValueOfType<String>(json, r'day')!,
        status: mapValueOfType<String>(json, r'status'),
        message: mapValueOfType<String>(json, r'message'),
        lastAttemptAt: mapValueOfType<int>(json, r'last_attempt_at'),
      );
    }
    return null;
  }

  static List<AppSiteAttendance> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSiteAttendance>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSiteAttendance.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSiteAttendance> mapFromJson(dynamic json) {
    final map = <String, AppSiteAttendance>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSiteAttendance.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSiteAttendance-objects as value to a dart map
  static Map<String, List<AppSiteAttendance>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSiteAttendance>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSiteAttendance.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'supported',
    'enabled',
    'day',
  };
}

