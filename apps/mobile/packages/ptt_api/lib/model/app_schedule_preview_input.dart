//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSchedulePreviewInput {
  /// Returns a new [AppSchedulePreviewInput] instance.
  AppSchedulePreviewInput({
    required this.schedule,
    this.mediaType,
    this.subscriptionId,
    this.count,
  });

  AppSchedule schedule;

  /// tv 或 movie；不给时按 subscription_id 的订阅，都没有时按剧集
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? mediaType;

  /// 按这个订阅的类型与建立时间算（每隔几天从建立那天对齐）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? subscriptionId;

  /// 给几次，默认 5
  ///
  /// Minimum value: 0
  /// Maximum value: 10
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? count;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSchedulePreviewInput &&
    other.schedule == schedule &&
    other.mediaType == mediaType &&
    other.subscriptionId == subscriptionId &&
    other.count == count;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (schedule.hashCode) +
    (mediaType == null ? 0 : mediaType!.hashCode) +
    (subscriptionId == null ? 0 : subscriptionId!.hashCode) +
    (count == null ? 0 : count!.hashCode);

  @override
  String toString() => 'AppSchedulePreviewInput[schedule=$schedule, mediaType=$mediaType, subscriptionId=$subscriptionId, count=$count]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'schedule'] = this.schedule;
    if (this.mediaType != null) {
      json[r'media_type'] = this.mediaType;
    } else {
      json[r'media_type'] = null;
    }
    if (this.subscriptionId != null) {
      json[r'subscription_id'] = this.subscriptionId;
    } else {
      json[r'subscription_id'] = null;
    }
    if (this.count != null) {
      json[r'count'] = this.count;
    } else {
      json[r'count'] = null;
    }
    return json;
  }

  /// Returns a new [AppSchedulePreviewInput] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSchedulePreviewInput? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'schedule'), 'Required key "AppSchedulePreviewInput[schedule]" is missing from JSON.');
        assert(json[r'schedule'] != null, 'Required key "AppSchedulePreviewInput[schedule]" has a null value in JSON.');
        return true;
      }());

      return AppSchedulePreviewInput(
        schedule: AppSchedule.fromJson(json[r'schedule'])!,
        mediaType: mapValueOfType<String>(json, r'media_type'),
        subscriptionId: mapValueOfType<int>(json, r'subscription_id'),
        count: mapValueOfType<int>(json, r'count'),
      );
    }
    return null;
  }

  static List<AppSchedulePreviewInput> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSchedulePreviewInput>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSchedulePreviewInput.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSchedulePreviewInput> mapFromJson(dynamic json) {
    final map = <String, AppSchedulePreviewInput>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSchedulePreviewInput.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSchedulePreviewInput-objects as value to a dart map
  static Map<String, List<AppSchedulePreviewInput>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSchedulePreviewInput>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSchedulePreviewInput.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'schedule',
  };
}

