//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantEvent {
  /// Returns a new [AppAssistantEvent] instance.
  AppAssistantEvent({
    required this.seq,
    required this.type,
    this.runId,
    this.text,
    this.replace,
    this.message,
    this.tool,
    this.confirmation,
    this.status,
    this.error,
    required this.at,
  });

  int seq;

  AppAssistantEventTypeEnum type;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? runId;

  /// delta 的文字
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? text;

  /// 为真时 text 是到现在的全部回答文字，整段替换
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? replace;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppAssistantMessage? message;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppAssistantToolCall? tool;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppAssistantConfirmation? confirmation;

  /// run_finished 的结束状态
  AppAssistantEventStatusEnum? status;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? error;

  DateTime at;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantEvent &&
    other.seq == seq &&
    other.type == type &&
    other.runId == runId &&
    other.text == text &&
    other.replace == replace &&
    other.message == message &&
    other.tool == tool &&
    other.confirmation == confirmation &&
    other.status == status &&
    other.error == error &&
    other.at == at;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (seq.hashCode) +
    (type.hashCode) +
    (runId == null ? 0 : runId!.hashCode) +
    (text == null ? 0 : text!.hashCode) +
    (replace == null ? 0 : replace!.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (tool == null ? 0 : tool!.hashCode) +
    (confirmation == null ? 0 : confirmation!.hashCode) +
    (status == null ? 0 : status!.hashCode) +
    (error == null ? 0 : error!.hashCode) +
    (at.hashCode);

  @override
  String toString() => 'AppAssistantEvent[seq=$seq, type=$type, runId=$runId, text=$text, replace=$replace, message=$message, tool=$tool, confirmation=$confirmation, status=$status, error=$error, at=$at]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'seq'] = this.seq;
      json[r'type'] = this.type;
    if (this.runId != null) {
      json[r'run_id'] = this.runId;
    } else {
      json[r'run_id'] = null;
    }
    if (this.text != null) {
      json[r'text'] = this.text;
    } else {
      json[r'text'] = null;
    }
    if (this.replace != null) {
      json[r'replace'] = this.replace;
    } else {
      json[r'replace'] = null;
    }
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
    }
    if (this.tool != null) {
      json[r'tool'] = this.tool;
    } else {
      json[r'tool'] = null;
    }
    if (this.confirmation != null) {
      json[r'confirmation'] = this.confirmation;
    } else {
      json[r'confirmation'] = null;
    }
    if (this.status != null) {
      json[r'status'] = this.status;
    } else {
      json[r'status'] = null;
    }
    if (this.error != null) {
      json[r'error'] = this.error;
    } else {
      json[r'error'] = null;
    }
      json[r'at'] = this.at.toUtc().toIso8601String();
    return json;
  }

  /// Returns a new [AppAssistantEvent] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantEvent? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'seq'), 'Required key "AppAssistantEvent[seq]" is missing from JSON.');
        assert(json[r'seq'] != null, 'Required key "AppAssistantEvent[seq]" has a null value in JSON.');
        assert(json.containsKey(r'type'), 'Required key "AppAssistantEvent[type]" is missing from JSON.');
        assert(json[r'type'] != null, 'Required key "AppAssistantEvent[type]" has a null value in JSON.');
        assert(json.containsKey(r'at'), 'Required key "AppAssistantEvent[at]" is missing from JSON.');
        assert(json[r'at'] != null, 'Required key "AppAssistantEvent[at]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantEvent(
        seq: mapValueOfType<int>(json, r'seq')!,
        type: AppAssistantEventTypeEnum.fromJson(json[r'type'])!,
        runId: mapValueOfType<String>(json, r'run_id'),
        text: mapValueOfType<String>(json, r'text'),
        replace: mapValueOfType<bool>(json, r'replace'),
        message: AppAssistantMessage.fromJson(json[r'message']),
        tool: AppAssistantToolCall.fromJson(json[r'tool']),
        confirmation: AppAssistantConfirmation.fromJson(json[r'confirmation']),
        status: AppAssistantEventStatusEnum.fromJson(json[r'status']),
        error: mapValueOfType<String>(json, r'error'),
        at: mapDateTime(json, r'at', r'')!,
      );
    }
    return null;
  }

  static List<AppAssistantEvent> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantEvent>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantEvent.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantEvent> mapFromJson(dynamic json) {
    final map = <String, AppAssistantEvent>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantEvent.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantEvent-objects as value to a dart map
  static Map<String, List<AppAssistantEvent>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantEvent>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantEvent.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'seq',
    'type',
    'at',
  };
}


enum AppAssistantEventTypeEnum {
  runStarted._(r'run_started'),
  delta._(r'delta'),
  message._(r'message'),
  toolStarted._(r'tool_started'),
  confirmation._(r'confirmation'),
  runFinished._(r'run_finished'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppAssistantEventTypeEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppAssistantEventTypeEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppAssistantEventTypeEnum? fromJson(dynamic value) => AppAssistantEventTypeEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppAssistantEventTypeEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppAssistantEventTypeEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantEventTypeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantEventTypeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppAssistantEventTypeEnum] to String,
/// and [decode] dynamic data back to [AppAssistantEventTypeEnum].
class AppAssistantEventTypeEnumTypeTransformer {
  factory AppAssistantEventTypeEnumTypeTransformer() => _instance ??= const AppAssistantEventTypeEnumTypeTransformer._();

  const AppAssistantEventTypeEnumTypeTransformer._();

  String encode(AppAssistantEventTypeEnum data) => data._value;

  /// Returns the instance of [AppAssistantEventTypeEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppAssistantEventTypeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppAssistantEventTypeEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'run_started': return AppAssistantEventTypeEnum.runStarted;
        case r'delta': return AppAssistantEventTypeEnum.delta;
        case r'message': return AppAssistantEventTypeEnum.message;
        case r'tool_started': return AppAssistantEventTypeEnum.toolStarted;
        case r'confirmation': return AppAssistantEventTypeEnum.confirmation;
        case r'run_finished': return AppAssistantEventTypeEnum.runFinished;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppAssistantEventTypeEnumTypeTransformer? _instance;
}


/// run_finished 的结束状态
enum AppAssistantEventStatusEnum {
  done._(r'done'),
  canceled._(r'canceled'),
  failed._(r'failed'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppAssistantEventStatusEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppAssistantEventStatusEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppAssistantEventStatusEnum? fromJson(dynamic value) => AppAssistantEventStatusEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppAssistantEventStatusEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppAssistantEventStatusEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantEventStatusEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantEventStatusEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppAssistantEventStatusEnum] to String,
/// and [decode] dynamic data back to [AppAssistantEventStatusEnum].
class AppAssistantEventStatusEnumTypeTransformer {
  factory AppAssistantEventStatusEnumTypeTransformer() => _instance ??= const AppAssistantEventStatusEnumTypeTransformer._();

  const AppAssistantEventStatusEnumTypeTransformer._();

  String encode(AppAssistantEventStatusEnum data) => data._value;

  /// Returns the instance of [AppAssistantEventStatusEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppAssistantEventStatusEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppAssistantEventStatusEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'done': return AppAssistantEventStatusEnum.done;
        case r'canceled': return AppAssistantEventStatusEnum.canceled;
        case r'failed': return AppAssistantEventStatusEnum.failed;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppAssistantEventStatusEnumTypeTransformer? _instance;
}


