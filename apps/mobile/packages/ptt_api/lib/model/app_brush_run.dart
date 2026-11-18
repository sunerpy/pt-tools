//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppBrushRun {
  /// Returns a new [AppBrushRun] instance.
  AppBrushRun({
    required this.id,
    required this.taskId,
    required this.trigger,
    required this.startedAt,
    this.finishedAt,
    required this.status,
    required this.summary,
    required this.error,
    this.listed,
    this.eligible,
    this.added,
    this.removed,
    this.gone,
    this.stepsTruncated,
    this.steps = const [],
  });

  int id;

  int taskId;

  String trigger;

  DateTime startedAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  DateTime? finishedAt;

  String status;

  String summary;

  String error;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? listed;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? eligible;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? added;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? removed;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? gone;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? stepsTruncated;

  List<AppBrushRunStep> steps;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppBrushRun &&
    other.id == id &&
    other.taskId == taskId &&
    other.trigger == trigger &&
    other.startedAt == startedAt &&
    other.finishedAt == finishedAt &&
    other.status == status &&
    other.summary == summary &&
    other.error == error &&
    other.listed == listed &&
    other.eligible == eligible &&
    other.added == added &&
    other.removed == removed &&
    other.gone == gone &&
    other.stepsTruncated == stepsTruncated &&
    _deepEquality.equals(other.steps, steps);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (taskId.hashCode) +
    (trigger.hashCode) +
    (startedAt.hashCode) +
    (finishedAt == null ? 0 : finishedAt!.hashCode) +
    (status.hashCode) +
    (summary.hashCode) +
    (error.hashCode) +
    (listed == null ? 0 : listed!.hashCode) +
    (eligible == null ? 0 : eligible!.hashCode) +
    (added == null ? 0 : added!.hashCode) +
    (removed == null ? 0 : removed!.hashCode) +
    (gone == null ? 0 : gone!.hashCode) +
    (stepsTruncated == null ? 0 : stepsTruncated!.hashCode) +
    (steps.hashCode);

  @override
  String toString() => 'AppBrushRun[id=$id, taskId=$taskId, trigger=$trigger, startedAt=$startedAt, finishedAt=$finishedAt, status=$status, summary=$summary, error=$error, listed=$listed, eligible=$eligible, added=$added, removed=$removed, gone=$gone, stepsTruncated=$stepsTruncated, steps=$steps]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'task_id'] = this.taskId;
      json[r'trigger'] = this.trigger;
      json[r'started_at'] = this.startedAt.toUtc().toIso8601String();
    if (this.finishedAt != null) {
      json[r'finished_at'] = this.finishedAt!.toUtc().toIso8601String();
    } else {
      json[r'finished_at'] = null;
    }
      json[r'status'] = this.status;
      json[r'summary'] = this.summary;
      json[r'error'] = this.error;
    if (this.listed != null) {
      json[r'listed'] = this.listed;
    } else {
      json[r'listed'] = null;
    }
    if (this.eligible != null) {
      json[r'eligible'] = this.eligible;
    } else {
      json[r'eligible'] = null;
    }
    if (this.added != null) {
      json[r'added'] = this.added;
    } else {
      json[r'added'] = null;
    }
    if (this.removed != null) {
      json[r'removed'] = this.removed;
    } else {
      json[r'removed'] = null;
    }
    if (this.gone != null) {
      json[r'gone'] = this.gone;
    } else {
      json[r'gone'] = null;
    }
    if (this.stepsTruncated != null) {
      json[r'steps_truncated'] = this.stepsTruncated;
    } else {
      json[r'steps_truncated'] = null;
    }
      json[r'steps'] = this.steps;
    return json;
  }

  /// Returns a new [AppBrushRun] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppBrushRun? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppBrushRun[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppBrushRun[id]" has a null value in JSON.');
        assert(json.containsKey(r'task_id'), 'Required key "AppBrushRun[task_id]" is missing from JSON.');
        assert(json[r'task_id'] != null, 'Required key "AppBrushRun[task_id]" has a null value in JSON.');
        assert(json.containsKey(r'trigger'), 'Required key "AppBrushRun[trigger]" is missing from JSON.');
        assert(json[r'trigger'] != null, 'Required key "AppBrushRun[trigger]" has a null value in JSON.');
        assert(json.containsKey(r'started_at'), 'Required key "AppBrushRun[started_at]" is missing from JSON.');
        assert(json[r'started_at'] != null, 'Required key "AppBrushRun[started_at]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppBrushRun[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppBrushRun[status]" has a null value in JSON.');
        assert(json.containsKey(r'summary'), 'Required key "AppBrushRun[summary]" is missing from JSON.');
        assert(json[r'summary'] != null, 'Required key "AppBrushRun[summary]" has a null value in JSON.');
        assert(json.containsKey(r'error'), 'Required key "AppBrushRun[error]" is missing from JSON.');
        assert(json[r'error'] != null, 'Required key "AppBrushRun[error]" has a null value in JSON.');
        assert(json.containsKey(r'steps'), 'Required key "AppBrushRun[steps]" is missing from JSON.');
        assert(json[r'steps'] != null, 'Required key "AppBrushRun[steps]" has a null value in JSON.');
        return true;
      }());

      return AppBrushRun(
        id: mapValueOfType<int>(json, r'id')!,
        taskId: mapValueOfType<int>(json, r'task_id')!,
        trigger: mapValueOfType<String>(json, r'trigger')!,
        startedAt: mapDateTime(json, r'started_at', r'')!,
        finishedAt: mapDateTime(json, r'finished_at', r''),
        status: mapValueOfType<String>(json, r'status')!,
        summary: mapValueOfType<String>(json, r'summary')!,
        error: mapValueOfType<String>(json, r'error')!,
        listed: mapValueOfType<int>(json, r'listed'),
        eligible: mapValueOfType<int>(json, r'eligible'),
        added: mapValueOfType<int>(json, r'added'),
        removed: mapValueOfType<int>(json, r'removed'),
        gone: mapValueOfType<int>(json, r'gone'),
        stepsTruncated: mapValueOfType<bool>(json, r'steps_truncated'),
        steps: AppBrushRunStep.listFromJson(json[r'steps']),
      );
    }
    return null;
  }

  static List<AppBrushRun> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppBrushRun>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppBrushRun.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppBrushRun> mapFromJson(dynamic json) {
    final map = <String, AppBrushRun>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppBrushRun.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppBrushRun-objects as value to a dart map
  static Map<String, List<AppBrushRun>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppBrushRun>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppBrushRun.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'task_id',
    'trigger',
    'started_at',
    'status',
    'summary',
    'error',
    'steps',
  };
}

