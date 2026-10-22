//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSchedule {
  /// Returns a new [AppSchedule] instance.
  AppSchedule({
    required this.kind,
    this.at,
    this.every,
    this.days = const [],
    this.airDelayHours,
    this.airRetryHours,
    this.airWindowDays,
    this.intervalMinutes,
    this.cron,
  });

  /// 空的、airtime、interval、daily、every_days、weekly、monthly 或 cron（回应里不用枚举：以后加了新的种类，旧的 App 也读得了）
  String kind;

  /// HH:MM
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? at;

  /// Minimum value: 1
  /// Maximum value: 30
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? every;

  List<int> days;

  /// Minimum value: 0
  /// Maximum value: 72
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? airDelayHours;

  /// Minimum value: 0
  /// Maximum value: 48
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? airRetryHours;

  /// Minimum value: 0
  /// Maximum value: 60
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? airWindowDays;

  /// interval：每隔几分钟
  ///
  /// Minimum value: 30
  /// Maximum value: 10080
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? intervalMinutes;

  /// cron：例如「0 20 * * *」（每天 20:00）、「0 22 * * 5; 0 9,13 * * 6」（周五 22:00 与周六 9:00、13:00）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? cron;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSchedule &&
    other.kind == kind &&
    other.at == at &&
    other.every == every &&
    _deepEquality.equals(other.days, days) &&
    other.airDelayHours == airDelayHours &&
    other.airRetryHours == airRetryHours &&
    other.airWindowDays == airWindowDays &&
    other.intervalMinutes == intervalMinutes &&
    other.cron == cron;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (kind.hashCode) +
    (at == null ? 0 : at!.hashCode) +
    (every == null ? 0 : every!.hashCode) +
    (days.hashCode) +
    (airDelayHours == null ? 0 : airDelayHours!.hashCode) +
    (airRetryHours == null ? 0 : airRetryHours!.hashCode) +
    (airWindowDays == null ? 0 : airWindowDays!.hashCode) +
    (intervalMinutes == null ? 0 : intervalMinutes!.hashCode) +
    (cron == null ? 0 : cron!.hashCode);

  @override
  String toString() => 'AppSchedule[kind=$kind, at=$at, every=$every, days=$days, airDelayHours=$airDelayHours, airRetryHours=$airRetryHours, airWindowDays=$airWindowDays, intervalMinutes=$intervalMinutes, cron=$cron]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'kind'] = this.kind;
    if (this.at != null) {
      json[r'at'] = this.at;
    } else {
      json[r'at'] = null;
    }
    if (this.every != null) {
      json[r'every'] = this.every;
    } else {
      json[r'every'] = null;
    }
      json[r'days'] = this.days;
    if (this.airDelayHours != null) {
      json[r'air_delay_hours'] = this.airDelayHours;
    } else {
      json[r'air_delay_hours'] = null;
    }
    if (this.airRetryHours != null) {
      json[r'air_retry_hours'] = this.airRetryHours;
    } else {
      json[r'air_retry_hours'] = null;
    }
    if (this.airWindowDays != null) {
      json[r'air_window_days'] = this.airWindowDays;
    } else {
      json[r'air_window_days'] = null;
    }
    if (this.intervalMinutes != null) {
      json[r'interval_minutes'] = this.intervalMinutes;
    } else {
      json[r'interval_minutes'] = null;
    }
    if (this.cron != null) {
      json[r'cron'] = this.cron;
    } else {
      json[r'cron'] = null;
    }
    return json;
  }

  /// Returns a new [AppSchedule] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSchedule? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'kind'), 'Required key "AppSchedule[kind]" is missing from JSON.');
        assert(json[r'kind'] != null, 'Required key "AppSchedule[kind]" has a null value in JSON.');
        return true;
      }());

      return AppSchedule(
        kind: mapValueOfType<String>(json, r'kind')!,
        at: mapValueOfType<String>(json, r'at'),
        every: mapValueOfType<int>(json, r'every'),
        days: json[r'days'] is Iterable
            ? (json[r'days'] as Iterable).cast<int>().toList(growable: false)
            : const [],
        airDelayHours: mapValueOfType<int>(json, r'air_delay_hours'),
        airRetryHours: mapValueOfType<int>(json, r'air_retry_hours'),
        airWindowDays: mapValueOfType<int>(json, r'air_window_days'),
        intervalMinutes: mapValueOfType<int>(json, r'interval_minutes'),
        cron: mapValueOfType<String>(json, r'cron'),
      );
    }
    return null;
  }

  static List<AppSchedule> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSchedule>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSchedule.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSchedule> mapFromJson(dynamic json) {
    final map = <String, AppSchedule>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSchedule.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSchedule-objects as value to a dart map
  static Map<String, List<AppSchedule>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSchedule>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSchedule.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'kind',
  };
}

