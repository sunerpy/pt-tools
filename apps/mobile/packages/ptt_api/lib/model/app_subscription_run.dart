//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSubscriptionRun {
  /// Returns a new [AppSubscriptionRun] instance.
  AppSubscriptionRun({
    required this.id,
    required this.trigger,
    required this.startedAt,
    this.finishedAt,
    required this.status,
    required this.summary,
    this.steps = const [],
    required this.stepsTruncated,
  });

  int id;

  /// create、schedule、rss、manual 或 ai
  String trigger;

  int startedAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? finishedAt;

  /// ok、nothing、waiting、failed、partial 或 changed（手动改动：改设置、跳过一集、换版本、集号规则，不是一次搜索）
  String status;

  String summary;

  List<AppRunStep> steps;

  /// 步骤太多，后面的没有记
  bool stepsTruncated;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSubscriptionRun &&
    other.id == id &&
    other.trigger == trigger &&
    other.startedAt == startedAt &&
    other.finishedAt == finishedAt &&
    other.status == status &&
    other.summary == summary &&
    _deepEquality.equals(other.steps, steps) &&
    other.stepsTruncated == stepsTruncated;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (trigger.hashCode) +
    (startedAt.hashCode) +
    (finishedAt == null ? 0 : finishedAt!.hashCode) +
    (status.hashCode) +
    (summary.hashCode) +
    (steps.hashCode) +
    (stepsTruncated.hashCode);

  @override
  String toString() => 'AppSubscriptionRun[id=$id, trigger=$trigger, startedAt=$startedAt, finishedAt=$finishedAt, status=$status, summary=$summary, steps=$steps, stepsTruncated=$stepsTruncated]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'trigger'] = this.trigger;
      json[r'started_at'] = this.startedAt;
    if (this.finishedAt != null) {
      json[r'finished_at'] = this.finishedAt;
    } else {
      json[r'finished_at'] = null;
    }
      json[r'status'] = this.status;
      json[r'summary'] = this.summary;
      json[r'steps'] = this.steps;
      json[r'steps_truncated'] = this.stepsTruncated;
    return json;
  }

  /// Returns a new [AppSubscriptionRun] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSubscriptionRun? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppSubscriptionRun[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppSubscriptionRun[id]" has a null value in JSON.');
        assert(json.containsKey(r'trigger'), 'Required key "AppSubscriptionRun[trigger]" is missing from JSON.');
        assert(json[r'trigger'] != null, 'Required key "AppSubscriptionRun[trigger]" has a null value in JSON.');
        assert(json.containsKey(r'started_at'), 'Required key "AppSubscriptionRun[started_at]" is missing from JSON.');
        assert(json[r'started_at'] != null, 'Required key "AppSubscriptionRun[started_at]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppSubscriptionRun[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppSubscriptionRun[status]" has a null value in JSON.');
        assert(json.containsKey(r'summary'), 'Required key "AppSubscriptionRun[summary]" is missing from JSON.');
        assert(json[r'summary'] != null, 'Required key "AppSubscriptionRun[summary]" has a null value in JSON.');
        assert(json.containsKey(r'steps'), 'Required key "AppSubscriptionRun[steps]" is missing from JSON.');
        assert(json[r'steps'] != null, 'Required key "AppSubscriptionRun[steps]" has a null value in JSON.');
        assert(json.containsKey(r'steps_truncated'), 'Required key "AppSubscriptionRun[steps_truncated]" is missing from JSON.');
        assert(json[r'steps_truncated'] != null, 'Required key "AppSubscriptionRun[steps_truncated]" has a null value in JSON.');
        return true;
      }());

      return AppSubscriptionRun(
        id: mapValueOfType<int>(json, r'id')!,
        trigger: mapValueOfType<String>(json, r'trigger')!,
        startedAt: mapValueOfType<int>(json, r'started_at')!,
        finishedAt: mapValueOfType<int>(json, r'finished_at'),
        status: mapValueOfType<String>(json, r'status')!,
        summary: mapValueOfType<String>(json, r'summary')!,
        steps: AppRunStep.listFromJson(json[r'steps']),
        stepsTruncated: mapValueOfType<bool>(json, r'steps_truncated')!,
      );
    }
    return null;
  }

  static List<AppSubscriptionRun> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionRun>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionRun.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSubscriptionRun> mapFromJson(dynamic json) {
    final map = <String, AppSubscriptionRun>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSubscriptionRun.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSubscriptionRun-objects as value to a dart map
  static Map<String, List<AppSubscriptionRun>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSubscriptionRun>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSubscriptionRun.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'trigger',
    'started_at',
    'status',
    'summary',
    'steps',
    'steps_truncated',
  };
}

