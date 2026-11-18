//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantCreate {
  /// Returns a new [AppAssistantCreate] instance.
  AppAssistantCreate({
    this.title,
    this.message,
    this.model,
    this.effort,
  });

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? title;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? message;

  /// 这一轮用的模型（不能有空格）；不给时用设置里的模型
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? model;

  /// 推理等级（reasoning_effort）；不给时不指定。模型服务不认时去掉它重试一次，会话里多一条提示
  AppAssistantCreateEffortEnum? effort;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantCreate &&
    other.title == title &&
    other.message == message &&
    other.model == model &&
    other.effort == effort;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (title == null ? 0 : title!.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (model == null ? 0 : model!.hashCode) +
    (effort == null ? 0 : effort!.hashCode);

  @override
  String toString() => 'AppAssistantCreate[title=$title, message=$message, model=$model, effort=$effort]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
    if (this.title != null) {
      json[r'title'] = this.title;
    } else {
      json[r'title'] = null;
    }
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
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
    return json;
  }

  /// Returns a new [AppAssistantCreate] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantCreate? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        return true;
      }());

      return AppAssistantCreate(
        title: mapValueOfType<String>(json, r'title'),
        message: mapValueOfType<String>(json, r'message'),
        model: mapValueOfType<String>(json, r'model'),
        effort: AppAssistantCreateEffortEnum.fromJson(json[r'effort']),
      );
    }
    return null;
  }

  static List<AppAssistantCreate> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantCreate>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantCreate.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantCreate> mapFromJson(dynamic json) {
    final map = <String, AppAssistantCreate>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantCreate.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantCreate-objects as value to a dart map
  static Map<String, List<AppAssistantCreate>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantCreate>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantCreate.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
  };
}

/// 推理等级（reasoning_effort）；不给时不指定。模型服务不认时去掉它重试一次，会话里多一条提示
enum AppAssistantCreateEffortEnum {
  none._(r'none'),
  minimal._(r'minimal'),
  low._(r'low'),
  medium._(r'medium'),
  high._(r'high'),
  xhigh._(r'xhigh'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppAssistantCreateEffortEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppAssistantCreateEffortEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppAssistantCreateEffortEnum? fromJson(dynamic value) => AppAssistantCreateEffortEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppAssistantCreateEffortEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppAssistantCreateEffortEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantCreateEffortEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantCreateEffortEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppAssistantCreateEffortEnum] to String,
/// and [decode] dynamic data back to [AppAssistantCreateEffortEnum].
class AppAssistantCreateEffortEnumTypeTransformer {
  factory AppAssistantCreateEffortEnumTypeTransformer() => _instance ??= const AppAssistantCreateEffortEnumTypeTransformer._();

  const AppAssistantCreateEffortEnumTypeTransformer._();

  String encode(AppAssistantCreateEffortEnum data) => data._value;

  /// Returns the instance of [AppAssistantCreateEffortEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppAssistantCreateEffortEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppAssistantCreateEffortEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'none': return AppAssistantCreateEffortEnum.none;
        case r'minimal': return AppAssistantCreateEffortEnum.minimal;
        case r'low': return AppAssistantCreateEffortEnum.low;
        case r'medium': return AppAssistantCreateEffortEnum.medium;
        case r'high': return AppAssistantCreateEffortEnum.high;
        case r'xhigh': return AppAssistantCreateEffortEnum.xhigh;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppAssistantCreateEffortEnumTypeTransformer? _instance;
}


