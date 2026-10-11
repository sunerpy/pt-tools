//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantEventPage {
  /// Returns a new [AppAssistantEventPage] instance.
  AppAssistantEventPage({
    this.events = const [],
    required this.reset,
    required this.last,
    required this.running,
  });

  List<AppAssistantEvent> events;

  /// 中间的事件丢了（或者 pt-tools 重启过）：重新读会话，从它的 last_seq 接着读
  bool reset;

  /// 下次读的 after
  int last;

  bool running;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantEventPage &&
    _deepEquality.equals(other.events, events) &&
    other.reset == reset &&
    other.last == last &&
    other.running == running;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (events.hashCode) +
    (reset.hashCode) +
    (last.hashCode) +
    (running.hashCode);

  @override
  String toString() => 'AppAssistantEventPage[events=$events, reset=$reset, last=$last, running=$running]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'events'] = this.events;
      json[r'reset'] = this.reset;
      json[r'last'] = this.last;
      json[r'running'] = this.running;
    return json;
  }

  /// Returns a new [AppAssistantEventPage] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantEventPage? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'events'), 'Required key "AppAssistantEventPage[events]" is missing from JSON.');
        assert(json[r'events'] != null, 'Required key "AppAssistantEventPage[events]" has a null value in JSON.');
        assert(json.containsKey(r'reset'), 'Required key "AppAssistantEventPage[reset]" is missing from JSON.');
        assert(json[r'reset'] != null, 'Required key "AppAssistantEventPage[reset]" has a null value in JSON.');
        assert(json.containsKey(r'last'), 'Required key "AppAssistantEventPage[last]" is missing from JSON.');
        assert(json[r'last'] != null, 'Required key "AppAssistantEventPage[last]" has a null value in JSON.');
        assert(json.containsKey(r'running'), 'Required key "AppAssistantEventPage[running]" is missing from JSON.');
        assert(json[r'running'] != null, 'Required key "AppAssistantEventPage[running]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantEventPage(
        events: AppAssistantEvent.listFromJson(json[r'events']),
        reset: mapValueOfType<bool>(json, r'reset')!,
        last: mapValueOfType<int>(json, r'last')!,
        running: mapValueOfType<bool>(json, r'running')!,
      );
    }
    return null;
  }

  static List<AppAssistantEventPage> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantEventPage>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantEventPage.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantEventPage> mapFromJson(dynamic json) {
    final map = <String, AppAssistantEventPage>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantEventPage.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantEventPage-objects as value to a dart map
  static Map<String, List<AppAssistantEventPage>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantEventPage>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantEventPage.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'events',
    'reset',
    'last',
    'running',
  };
}

