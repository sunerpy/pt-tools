//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantConfirmation {
  /// Returns a new [AppAssistantConfirmation] instance.
  AppAssistantConfirmation({
    required this.id,
    required this.runId,
    required this.toolCallId,
    required this.tool,
    required this.summary,
    this.arguments = const {},
    required this.status,
    this.decidedBy,
    this.decidedAt,
    required this.expiresAt,
    required this.createdAt,
    this.changes = const [],
    this.impact = const [],
  });

  int id;

  String runId;

  String toolCallId;

  String tool;

  /// 给人看的一句话，例如「暂停种子：下载器 1，任务 abc」
  String summary;

  Map<String, Object> arguments;

  AppAssistantConfirmationStatusEnum status;

  /// 属主，或者 timeout、cancel、restart、shutdown
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? decidedBy;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  DateTime? decidedAt;

  DateTime expiresAt;

  DateTime createdAt;

  /// 写工具的预览：会改什么（改以前、改以后）；密钥两边都不写原文
  List<AppAssistantChange> changes;

  /// 写工具的预览：可能的影响
  List<String> impact;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantConfirmation &&
    other.id == id &&
    other.runId == runId &&
    other.toolCallId == toolCallId &&
    other.tool == tool &&
    other.summary == summary &&
    _deepEquality.equals(other.arguments, arguments) &&
    other.status == status &&
    other.decidedBy == decidedBy &&
    other.decidedAt == decidedAt &&
    other.expiresAt == expiresAt &&
    other.createdAt == createdAt &&
    _deepEquality.equals(other.changes, changes) &&
    _deepEquality.equals(other.impact, impact);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (runId.hashCode) +
    (toolCallId.hashCode) +
    (tool.hashCode) +
    (summary.hashCode) +
    (arguments.hashCode) +
    (status.hashCode) +
    (decidedBy == null ? 0 : decidedBy!.hashCode) +
    (decidedAt == null ? 0 : decidedAt!.hashCode) +
    (expiresAt.hashCode) +
    (createdAt.hashCode) +
    (changes.hashCode) +
    (impact.hashCode);

  @override
  String toString() => 'AppAssistantConfirmation[id=$id, runId=$runId, toolCallId=$toolCallId, tool=$tool, summary=$summary, arguments=$arguments, status=$status, decidedBy=$decidedBy, decidedAt=$decidedAt, expiresAt=$expiresAt, createdAt=$createdAt, changes=$changes, impact=$impact]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'run_id'] = this.runId;
      json[r'tool_call_id'] = this.toolCallId;
      json[r'tool'] = this.tool;
      json[r'summary'] = this.summary;
      json[r'arguments'] = this.arguments;
      json[r'status'] = this.status;
    if (this.decidedBy != null) {
      json[r'decided_by'] = this.decidedBy;
    } else {
      json[r'decided_by'] = null;
    }
    if (this.decidedAt != null) {
      json[r'decided_at'] = this.decidedAt!.toUtc().toIso8601String();
    } else {
      json[r'decided_at'] = null;
    }
      json[r'expires_at'] = this.expiresAt.toUtc().toIso8601String();
      json[r'created_at'] = this.createdAt.toUtc().toIso8601String();
      json[r'changes'] = this.changes;
      json[r'impact'] = this.impact;
    return json;
  }

  /// Returns a new [AppAssistantConfirmation] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantConfirmation? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppAssistantConfirmation[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppAssistantConfirmation[id]" has a null value in JSON.');
        assert(json.containsKey(r'run_id'), 'Required key "AppAssistantConfirmation[run_id]" is missing from JSON.');
        assert(json[r'run_id'] != null, 'Required key "AppAssistantConfirmation[run_id]" has a null value in JSON.');
        assert(json.containsKey(r'tool_call_id'), 'Required key "AppAssistantConfirmation[tool_call_id]" is missing from JSON.');
        assert(json[r'tool_call_id'] != null, 'Required key "AppAssistantConfirmation[tool_call_id]" has a null value in JSON.');
        assert(json.containsKey(r'tool'), 'Required key "AppAssistantConfirmation[tool]" is missing from JSON.');
        assert(json[r'tool'] != null, 'Required key "AppAssistantConfirmation[tool]" has a null value in JSON.');
        assert(json.containsKey(r'summary'), 'Required key "AppAssistantConfirmation[summary]" is missing from JSON.');
        assert(json[r'summary'] != null, 'Required key "AppAssistantConfirmation[summary]" has a null value in JSON.');
        assert(json.containsKey(r'arguments'), 'Required key "AppAssistantConfirmation[arguments]" is missing from JSON.');
        assert(json[r'arguments'] != null, 'Required key "AppAssistantConfirmation[arguments]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppAssistantConfirmation[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppAssistantConfirmation[status]" has a null value in JSON.');
        assert(json.containsKey(r'expires_at'), 'Required key "AppAssistantConfirmation[expires_at]" is missing from JSON.');
        assert(json[r'expires_at'] != null, 'Required key "AppAssistantConfirmation[expires_at]" has a null value in JSON.');
        assert(json.containsKey(r'created_at'), 'Required key "AppAssistantConfirmation[created_at]" is missing from JSON.');
        assert(json[r'created_at'] != null, 'Required key "AppAssistantConfirmation[created_at]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantConfirmation(
        id: mapValueOfType<int>(json, r'id')!,
        runId: mapValueOfType<String>(json, r'run_id')!,
        toolCallId: mapValueOfType<String>(json, r'tool_call_id')!,
        tool: mapValueOfType<String>(json, r'tool')!,
        summary: mapValueOfType<String>(json, r'summary')!,
        arguments: mapCastOfType<String, Object>(json, r'arguments')!,
        status: AppAssistantConfirmationStatusEnum.fromJson(json[r'status'])!,
        decidedBy: mapValueOfType<String>(json, r'decided_by'),
        decidedAt: mapDateTime(json, r'decided_at', r''),
        expiresAt: mapDateTime(json, r'expires_at', r'')!,
        createdAt: mapDateTime(json, r'created_at', r'')!,
        changes: AppAssistantChange.listFromJson(json[r'changes']),
        impact: json[r'impact'] is Iterable
            ? (json[r'impact'] as Iterable).cast<String>().toList(growable: false)
            : const [],
      );
    }
    return null;
  }

  static List<AppAssistantConfirmation> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantConfirmation>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantConfirmation.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantConfirmation> mapFromJson(dynamic json) {
    final map = <String, AppAssistantConfirmation>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantConfirmation.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantConfirmation-objects as value to a dart map
  static Map<String, List<AppAssistantConfirmation>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantConfirmation>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantConfirmation.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'run_id',
    'tool_call_id',
    'tool',
    'summary',
    'arguments',
    'status',
    'expires_at',
    'created_at',
  };
}


enum AppAssistantConfirmationStatusEnum {
  pending._(r'pending'),
  approved._(r'approved'),
  denied._(r'denied'),
  expired._(r'expired'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppAssistantConfirmationStatusEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppAssistantConfirmationStatusEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppAssistantConfirmationStatusEnum? fromJson(dynamic value) => AppAssistantConfirmationStatusEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppAssistantConfirmationStatusEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppAssistantConfirmationStatusEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantConfirmationStatusEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantConfirmationStatusEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppAssistantConfirmationStatusEnum] to String,
/// and [decode] dynamic data back to [AppAssistantConfirmationStatusEnum].
class AppAssistantConfirmationStatusEnumTypeTransformer {
  factory AppAssistantConfirmationStatusEnumTypeTransformer() => _instance ??= const AppAssistantConfirmationStatusEnumTypeTransformer._();

  const AppAssistantConfirmationStatusEnumTypeTransformer._();

  String encode(AppAssistantConfirmationStatusEnum data) => data._value;

  /// Returns the instance of [AppAssistantConfirmationStatusEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppAssistantConfirmationStatusEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppAssistantConfirmationStatusEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'pending': return AppAssistantConfirmationStatusEnum.pending;
        case r'approved': return AppAssistantConfirmationStatusEnum.approved;
        case r'denied': return AppAssistantConfirmationStatusEnum.denied;
        case r'expired': return AppAssistantConfirmationStatusEnum.expired;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppAssistantConfirmationStatusEnumTypeTransformer? _instance;
}


