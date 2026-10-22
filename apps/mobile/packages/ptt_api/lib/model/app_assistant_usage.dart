//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantUsage {
  /// Returns a new [AppAssistantUsage] instance.
  AppAssistantUsage({
    required this.context,
    this.estimated,
    required this.window,
    required this.windowSource,
    required this.autoCompact,
    required this.compactPercent,
    required this.input,
    required this.output,
    this.compacted,
  });

  /// 下一次请求大约要带的 token
  int context;

  /// 按字数估算的（服务商没报，或者刚压缩过）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? estimated;

  /// 上下文窗口（token）
  int window;

  /// setting、provider、table 或 default
  String windowSource;

  bool autoCompact;

  /// 用到窗口的这个比例时自动压缩
  int compactPercent;

  /// 本会话服务商报的输入 token 合计
  int input;

  /// 本会话服务商报的输出 token 合计
  int output;

  /// 这个会话压缩过
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? compacted;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantUsage &&
    other.context == context &&
    other.estimated == estimated &&
    other.window == window &&
    other.windowSource == windowSource &&
    other.autoCompact == autoCompact &&
    other.compactPercent == compactPercent &&
    other.input == input &&
    other.output == output &&
    other.compacted == compacted;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (context.hashCode) +
    (estimated == null ? 0 : estimated!.hashCode) +
    (window.hashCode) +
    (windowSource.hashCode) +
    (autoCompact.hashCode) +
    (compactPercent.hashCode) +
    (input.hashCode) +
    (output.hashCode) +
    (compacted == null ? 0 : compacted!.hashCode);

  @override
  String toString() => 'AppAssistantUsage[context=$context, estimated=$estimated, window=$window, windowSource=$windowSource, autoCompact=$autoCompact, compactPercent=$compactPercent, input=$input, output=$output, compacted=$compacted]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'context'] = this.context;
    if (this.estimated != null) {
      json[r'estimated'] = this.estimated;
    } else {
      json[r'estimated'] = null;
    }
      json[r'window'] = this.window;
      json[r'window_source'] = this.windowSource;
      json[r'auto_compact'] = this.autoCompact;
      json[r'compact_percent'] = this.compactPercent;
      json[r'input'] = this.input;
      json[r'output'] = this.output;
    if (this.compacted != null) {
      json[r'compacted'] = this.compacted;
    } else {
      json[r'compacted'] = null;
    }
    return json;
  }

  /// Returns a new [AppAssistantUsage] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantUsage? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'context'), 'Required key "AppAssistantUsage[context]" is missing from JSON.');
        assert(json[r'context'] != null, 'Required key "AppAssistantUsage[context]" has a null value in JSON.');
        assert(json.containsKey(r'window'), 'Required key "AppAssistantUsage[window]" is missing from JSON.');
        assert(json[r'window'] != null, 'Required key "AppAssistantUsage[window]" has a null value in JSON.');
        assert(json.containsKey(r'window_source'), 'Required key "AppAssistantUsage[window_source]" is missing from JSON.');
        assert(json[r'window_source'] != null, 'Required key "AppAssistantUsage[window_source]" has a null value in JSON.');
        assert(json.containsKey(r'auto_compact'), 'Required key "AppAssistantUsage[auto_compact]" is missing from JSON.');
        assert(json[r'auto_compact'] != null, 'Required key "AppAssistantUsage[auto_compact]" has a null value in JSON.');
        assert(json.containsKey(r'compact_percent'), 'Required key "AppAssistantUsage[compact_percent]" is missing from JSON.');
        assert(json[r'compact_percent'] != null, 'Required key "AppAssistantUsage[compact_percent]" has a null value in JSON.');
        assert(json.containsKey(r'input'), 'Required key "AppAssistantUsage[input]" is missing from JSON.');
        assert(json[r'input'] != null, 'Required key "AppAssistantUsage[input]" has a null value in JSON.');
        assert(json.containsKey(r'output'), 'Required key "AppAssistantUsage[output]" is missing from JSON.');
        assert(json[r'output'] != null, 'Required key "AppAssistantUsage[output]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantUsage(
        context: mapValueOfType<int>(json, r'context')!,
        estimated: mapValueOfType<bool>(json, r'estimated'),
        window: mapValueOfType<int>(json, r'window')!,
        windowSource: mapValueOfType<String>(json, r'window_source')!,
        autoCompact: mapValueOfType<bool>(json, r'auto_compact')!,
        compactPercent: mapValueOfType<int>(json, r'compact_percent')!,
        input: mapValueOfType<int>(json, r'input')!,
        output: mapValueOfType<int>(json, r'output')!,
        compacted: mapValueOfType<bool>(json, r'compacted'),
      );
    }
    return null;
  }

  static List<AppAssistantUsage> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantUsage>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantUsage.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantUsage> mapFromJson(dynamic json) {
    final map = <String, AppAssistantUsage>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantUsage.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantUsage-objects as value to a dart map
  static Map<String, List<AppAssistantUsage>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantUsage>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantUsage.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'context',
    'window',
    'window_source',
    'auto_compact',
    'compact_percent',
    'input',
    'output',
  };
}

