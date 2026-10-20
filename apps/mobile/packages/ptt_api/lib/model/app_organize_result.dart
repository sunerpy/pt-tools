//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppOrganizeResult {
  /// Returns a new [AppOrganizeResult] instance.
  AppOrganizeResult({
    this.plan,
    required this.created,
    required this.done,
    required this.skipped,
    required this.failed,
    this.messages = const [],
    required this.queued,
  });

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppOrganizePlan? plan;

  /// 这次整理出的文件数
  int created;

  /// 库里已经有了的
  int done;

  int skipped;

  int failed;

  List<String> messages;

  /// 还在后台进行（等不到结果就先回了），稍后看整理历史
  bool queued;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppOrganizeResult &&
    other.plan == plan &&
    other.created == created &&
    other.done == done &&
    other.skipped == skipped &&
    other.failed == failed &&
    _deepEquality.equals(other.messages, messages) &&
    other.queued == queued;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (plan == null ? 0 : plan!.hashCode) +
    (created.hashCode) +
    (done.hashCode) +
    (skipped.hashCode) +
    (failed.hashCode) +
    (messages.hashCode) +
    (queued.hashCode);

  @override
  String toString() => 'AppOrganizeResult[plan=$plan, created=$created, done=$done, skipped=$skipped, failed=$failed, messages=$messages, queued=$queued]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
    if (this.plan != null) {
      json[r'plan'] = this.plan;
    } else {
      json[r'plan'] = null;
    }
      json[r'created'] = this.created;
      json[r'done'] = this.done;
      json[r'skipped'] = this.skipped;
      json[r'failed'] = this.failed;
      json[r'messages'] = this.messages;
      json[r'queued'] = this.queued;
    return json;
  }

  /// Returns a new [AppOrganizeResult] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppOrganizeResult? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'created'), 'Required key "AppOrganizeResult[created]" is missing from JSON.');
        assert(json[r'created'] != null, 'Required key "AppOrganizeResult[created]" has a null value in JSON.');
        assert(json.containsKey(r'done'), 'Required key "AppOrganizeResult[done]" is missing from JSON.');
        assert(json[r'done'] != null, 'Required key "AppOrganizeResult[done]" has a null value in JSON.');
        assert(json.containsKey(r'skipped'), 'Required key "AppOrganizeResult[skipped]" is missing from JSON.');
        assert(json[r'skipped'] != null, 'Required key "AppOrganizeResult[skipped]" has a null value in JSON.');
        assert(json.containsKey(r'failed'), 'Required key "AppOrganizeResult[failed]" is missing from JSON.');
        assert(json[r'failed'] != null, 'Required key "AppOrganizeResult[failed]" has a null value in JSON.');
        assert(json.containsKey(r'messages'), 'Required key "AppOrganizeResult[messages]" is missing from JSON.');
        assert(json[r'messages'] != null, 'Required key "AppOrganizeResult[messages]" has a null value in JSON.');
        assert(json.containsKey(r'queued'), 'Required key "AppOrganizeResult[queued]" is missing from JSON.');
        assert(json[r'queued'] != null, 'Required key "AppOrganizeResult[queued]" has a null value in JSON.');
        return true;
      }());

      return AppOrganizeResult(
        plan: AppOrganizePlan.fromJson(json[r'plan']),
        created: mapValueOfType<int>(json, r'created')!,
        done: mapValueOfType<int>(json, r'done')!,
        skipped: mapValueOfType<int>(json, r'skipped')!,
        failed: mapValueOfType<int>(json, r'failed')!,
        messages: json[r'messages'] is Iterable
            ? (json[r'messages'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        queued: mapValueOfType<bool>(json, r'queued')!,
      );
    }
    return null;
  }

  static List<AppOrganizeResult> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppOrganizeResult>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppOrganizeResult.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppOrganizeResult> mapFromJson(dynamic json) {
    final map = <String, AppOrganizeResult>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppOrganizeResult.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppOrganizeResult-objects as value to a dart map
  static Map<String, List<AppOrganizeResult>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppOrganizeResult>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppOrganizeResult.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'created',
    'done',
    'skipped',
    'failed',
    'messages',
    'queued',
  };
}

