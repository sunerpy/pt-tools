//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantMessage {
  /// Returns a new [AppAssistantMessage] instance.
  AppAssistantMessage({
    required this.id,
    this.runId,
    required this.role,
    required this.content,
    this.reasoning,
    this.model,
    this.effort,
    this.toolCalls = const [],
    this.attachments = const [],
    this.toolName,
    this.toolCallId,
    this.isError,
    required this.createdAt,
  });

  int id;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? runId;

  AppAssistantMessageRoleEnum role;

  /// 脱敏以后的文字；tool 是工具返回的 JSON 文本
  String content;

  /// 模型的推理内容（思考过程，脱敏以后的）：只有助手消息有，模型没给时没有
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? reasoning;

  /// 这一轮的模型：用户消息是发的时候选的，助手消息是实际请求的；没有时是设置里的模型
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? model;

  /// 这一轮的推理等级（none、minimal、low、medium、high、xhigh）；没有时是不指定
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? effort;

  List<AppAssistantToolCall> toolCalls;

  /// 用户消息带的附件（在网页上传的；内容不经 App API 给）
  List<AppAssistantAttachment> attachments;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? toolName;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? toolCallId;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? isError;

  DateTime createdAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantMessage &&
    other.id == id &&
    other.runId == runId &&
    other.role == role &&
    other.content == content &&
    other.reasoning == reasoning &&
    other.model == model &&
    other.effort == effort &&
    _deepEquality.equals(other.toolCalls, toolCalls) &&
    _deepEquality.equals(other.attachments, attachments) &&
    other.toolName == toolName &&
    other.toolCallId == toolCallId &&
    other.isError == isError &&
    other.createdAt == createdAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (runId == null ? 0 : runId!.hashCode) +
    (role.hashCode) +
    (content.hashCode) +
    (reasoning == null ? 0 : reasoning!.hashCode) +
    (model == null ? 0 : model!.hashCode) +
    (effort == null ? 0 : effort!.hashCode) +
    (toolCalls.hashCode) +
    (attachments.hashCode) +
    (toolName == null ? 0 : toolName!.hashCode) +
    (toolCallId == null ? 0 : toolCallId!.hashCode) +
    (isError == null ? 0 : isError!.hashCode) +
    (createdAt.hashCode);

  @override
  String toString() => 'AppAssistantMessage[id=$id, runId=$runId, role=$role, content=$content, reasoning=$reasoning, model=$model, effort=$effort, toolCalls=$toolCalls, attachments=$attachments, toolName=$toolName, toolCallId=$toolCallId, isError=$isError, createdAt=$createdAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
    if (this.runId != null) {
      json[r'run_id'] = this.runId;
    } else {
      json[r'run_id'] = null;
    }
      json[r'role'] = this.role;
      json[r'content'] = this.content;
    if (this.reasoning != null) {
      json[r'reasoning'] = this.reasoning;
    } else {
      json[r'reasoning'] = null;
    }
    if (this.model != null) {
      json[r'model'] = this.model;
    } else {
      json[r'model'] = null;
    }
    if (this.effort != null) {
      json[r'effort'] = this.effort;
    } else {
      json[r'effort'] = null;
    }
      json[r'tool_calls'] = this.toolCalls;
      json[r'attachments'] = this.attachments;
    if (this.toolName != null) {
      json[r'tool_name'] = this.toolName;
    } else {
      json[r'tool_name'] = null;
    }
    if (this.toolCallId != null) {
      json[r'tool_call_id'] = this.toolCallId;
    } else {
      json[r'tool_call_id'] = null;
    }
    if (this.isError != null) {
      json[r'is_error'] = this.isError;
    } else {
      json[r'is_error'] = null;
    }
      json[r'created_at'] = this.createdAt.toUtc().toIso8601String();
    return json;
  }

  /// Returns a new [AppAssistantMessage] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantMessage? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppAssistantMessage[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppAssistantMessage[id]" has a null value in JSON.');
        assert(json.containsKey(r'role'), 'Required key "AppAssistantMessage[role]" is missing from JSON.');
        assert(json[r'role'] != null, 'Required key "AppAssistantMessage[role]" has a null value in JSON.');
        assert(json.containsKey(r'content'), 'Required key "AppAssistantMessage[content]" is missing from JSON.');
        assert(json[r'content'] != null, 'Required key "AppAssistantMessage[content]" has a null value in JSON.');
        assert(json.containsKey(r'created_at'), 'Required key "AppAssistantMessage[created_at]" is missing from JSON.');
        assert(json[r'created_at'] != null, 'Required key "AppAssistantMessage[created_at]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantMessage(
        id: mapValueOfType<int>(json, r'id')!,
        runId: mapValueOfType<String>(json, r'run_id'),
        role: AppAssistantMessageRoleEnum.fromJson(json[r'role'])!,
        content: mapValueOfType<String>(json, r'content')!,
        reasoning: mapValueOfType<String>(json, r'reasoning'),
        model: mapValueOfType<String>(json, r'model'),
        effort: mapValueOfType<String>(json, r'effort'),
        toolCalls: AppAssistantToolCall.listFromJson(json[r'tool_calls']),
        attachments: AppAssistantAttachment.listFromJson(json[r'attachments']),
        toolName: mapValueOfType<String>(json, r'tool_name'),
        toolCallId: mapValueOfType<String>(json, r'tool_call_id'),
        isError: mapValueOfType<bool>(json, r'is_error'),
        createdAt: mapDateTime(json, r'created_at', r'')!,
      );
    }
    return null;
  }

  static List<AppAssistantMessage> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantMessage>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantMessage.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantMessage> mapFromJson(dynamic json) {
    final map = <String, AppAssistantMessage>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantMessage.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantMessage-objects as value to a dart map
  static Map<String, List<AppAssistantMessage>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantMessage>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantMessage.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'role',
    'content',
    'created_at',
  };
}


enum AppAssistantMessageRoleEnum {
  user._(r'user'),
  assistant._(r'assistant'),
  tool._(r'tool'),
  notice._(r'notice'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppAssistantMessageRoleEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppAssistantMessageRoleEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppAssistantMessageRoleEnum? fromJson(dynamic value) => AppAssistantMessageRoleEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppAssistantMessageRoleEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppAssistantMessageRoleEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantMessageRoleEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantMessageRoleEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppAssistantMessageRoleEnum] to String,
/// and [decode] dynamic data back to [AppAssistantMessageRoleEnum].
class AppAssistantMessageRoleEnumTypeTransformer {
  factory AppAssistantMessageRoleEnumTypeTransformer() => _instance ??= const AppAssistantMessageRoleEnumTypeTransformer._();

  const AppAssistantMessageRoleEnumTypeTransformer._();

  String encode(AppAssistantMessageRoleEnum data) => data._value;

  /// Returns the instance of [AppAssistantMessageRoleEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppAssistantMessageRoleEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppAssistantMessageRoleEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'user': return AppAssistantMessageRoleEnum.user;
        case r'assistant': return AppAssistantMessageRoleEnum.assistant;
        case r'tool': return AppAssistantMessageRoleEnum.tool;
        case r'notice': return AppAssistantMessageRoleEnum.notice;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppAssistantMessageRoleEnumTypeTransformer? _instance;
}


