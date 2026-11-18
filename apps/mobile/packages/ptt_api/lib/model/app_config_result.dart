//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppConfigResult {
  /// Returns a new [AppConfigResult] instance.
  AppConfigResult({
    required this.resource,
    required this.op,
    this.id,
    required this.summary,
    this.changes = const [],
    this.impact = const [],
    required this.applied,
    this.item = const {},
  });

  String resource;

  String op;

  /// 新增出来的编号也在这里
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? id;

  String summary;

  List<AppConfigDiff> changes;

  List<String> impact;

  /// 预览时为假
  bool applied;

  /// 改完的样子（删除时没有）
  Map<String, Object> item;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppConfigResult &&
    other.resource == resource &&
    other.op == op &&
    other.id == id &&
    other.summary == summary &&
    _deepEquality.equals(other.changes, changes) &&
    _deepEquality.equals(other.impact, impact) &&
    other.applied == applied &&
    _deepEquality.equals(other.item, item);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (resource.hashCode) +
    (op.hashCode) +
    (id == null ? 0 : id!.hashCode) +
    (summary.hashCode) +
    (changes.hashCode) +
    (impact.hashCode) +
    (applied.hashCode) +
    (item.hashCode);

  @override
  String toString() => 'AppConfigResult[resource=$resource, op=$op, id=$id, summary=$summary, changes=$changes, impact=$impact, applied=$applied, item=$item]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'resource'] = this.resource;
      json[r'op'] = this.op;
    if (this.id != null) {
      json[r'id'] = this.id;
    } else {
      json[r'id'] = null;
    }
      json[r'summary'] = this.summary;
      json[r'changes'] = this.changes;
      json[r'impact'] = this.impact;
      json[r'applied'] = this.applied;
      json[r'item'] = this.item;
    return json;
  }

  /// Returns a new [AppConfigResult] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppConfigResult? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'resource'), 'Required key "AppConfigResult[resource]" is missing from JSON.');
        assert(json[r'resource'] != null, 'Required key "AppConfigResult[resource]" has a null value in JSON.');
        assert(json.containsKey(r'op'), 'Required key "AppConfigResult[op]" is missing from JSON.');
        assert(json[r'op'] != null, 'Required key "AppConfigResult[op]" has a null value in JSON.');
        assert(json.containsKey(r'summary'), 'Required key "AppConfigResult[summary]" is missing from JSON.');
        assert(json[r'summary'] != null, 'Required key "AppConfigResult[summary]" has a null value in JSON.');
        assert(json.containsKey(r'changes'), 'Required key "AppConfigResult[changes]" is missing from JSON.');
        assert(json[r'changes'] != null, 'Required key "AppConfigResult[changes]" has a null value in JSON.');
        assert(json.containsKey(r'impact'), 'Required key "AppConfigResult[impact]" is missing from JSON.');
        assert(json[r'impact'] != null, 'Required key "AppConfigResult[impact]" has a null value in JSON.');
        assert(json.containsKey(r'applied'), 'Required key "AppConfigResult[applied]" is missing from JSON.');
        assert(json[r'applied'] != null, 'Required key "AppConfigResult[applied]" has a null value in JSON.');
        return true;
      }());

      return AppConfigResult(
        resource: mapValueOfType<String>(json, r'resource')!,
        op: mapValueOfType<String>(json, r'op')!,
        id: mapValueOfType<String>(json, r'id'),
        summary: mapValueOfType<String>(json, r'summary')!,
        changes: AppConfigDiff.listFromJson(json[r'changes']),
        impact: json[r'impact'] is Iterable
            ? (json[r'impact'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        applied: mapValueOfType<bool>(json, r'applied')!,
        item: mapCastOfType<String, Object>(json, r'item') ?? const {},
      );
    }
    return null;
  }

  static List<AppConfigResult> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppConfigResult>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppConfigResult.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppConfigResult> mapFromJson(dynamic json) {
    final map = <String, AppConfigResult>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppConfigResult.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppConfigResult-objects as value to a dart map
  static Map<String, List<AppConfigResult>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppConfigResult>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppConfigResult.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'resource',
    'op',
    'summary',
    'changes',
    'impact',
    'applied',
  };
}

