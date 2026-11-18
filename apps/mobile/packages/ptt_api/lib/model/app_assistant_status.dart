//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantStatus {
  /// Returns a new [AppAssistantStatus] instance.
  AppAssistantStatus({
    required this.enabled,
    required this.model,
    required this.canConfirm,
    required this.chatops,
  });

  /// 启用了而且接口地址与模型都配好了
  bool enabled;

  /// 没启用时为空
  String model;

  /// 调用者能不能确认改动类操作（有 app:write）
  bool canConfirm;

  /// 通知渠道里的非命令消息交不交给助手
  bool chatops;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantStatus &&
    other.enabled == enabled &&
    other.model == model &&
    other.canConfirm == canConfirm &&
    other.chatops == chatops;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (enabled.hashCode) +
    (model.hashCode) +
    (canConfirm.hashCode) +
    (chatops.hashCode);

  @override
  String toString() => 'AppAssistantStatus[enabled=$enabled, model=$model, canConfirm=$canConfirm, chatops=$chatops]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'enabled'] = this.enabled;
      json[r'model'] = this.model;
      json[r'can_confirm'] = this.canConfirm;
      json[r'chatops'] = this.chatops;
    return json;
  }

  /// Returns a new [AppAssistantStatus] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantStatus? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'enabled'), 'Required key "AppAssistantStatus[enabled]" is missing from JSON.');
        assert(json[r'enabled'] != null, 'Required key "AppAssistantStatus[enabled]" has a null value in JSON.');
        assert(json.containsKey(r'model'), 'Required key "AppAssistantStatus[model]" is missing from JSON.');
        assert(json[r'model'] != null, 'Required key "AppAssistantStatus[model]" has a null value in JSON.');
        assert(json.containsKey(r'can_confirm'), 'Required key "AppAssistantStatus[can_confirm]" is missing from JSON.');
        assert(json[r'can_confirm'] != null, 'Required key "AppAssistantStatus[can_confirm]" has a null value in JSON.');
        assert(json.containsKey(r'chatops'), 'Required key "AppAssistantStatus[chatops]" is missing from JSON.');
        assert(json[r'chatops'] != null, 'Required key "AppAssistantStatus[chatops]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantStatus(
        enabled: mapValueOfType<bool>(json, r'enabled')!,
        model: mapValueOfType<String>(json, r'model')!,
        canConfirm: mapValueOfType<bool>(json, r'can_confirm')!,
        chatops: mapValueOfType<bool>(json, r'chatops')!,
      );
    }
    return null;
  }

  static List<AppAssistantStatus> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantStatus>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantStatus.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantStatus> mapFromJson(dynamic json) {
    final map = <String, AppAssistantStatus>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantStatus.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantStatus-objects as value to a dart map
  static Map<String, List<AppAssistantStatus>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantStatus>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantStatus.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'enabled',
    'model',
    'can_confirm',
    'chatops',
  };
}

