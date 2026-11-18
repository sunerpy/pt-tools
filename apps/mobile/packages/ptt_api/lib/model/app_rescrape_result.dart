//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppRescrapeResult {
  /// Returns a new [AppRescrapeResult] instance.
  AppRescrapeResult({
    required this.records,
    required this.written,
    this.messages = const [],
    required this.queued,
  });

  /// 刮削了的整理记录数（同一部在这个媒体库里的）
  int records;

  /// 写了的文件数（内容一样的不重写）
  int written;

  List<String> messages;

  /// 还在后台刮削（等不到结果就先回了），稍后看整理历史
  bool queued;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppRescrapeResult &&
    other.records == records &&
    other.written == written &&
    _deepEquality.equals(other.messages, messages) &&
    other.queued == queued;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (records.hashCode) +
    (written.hashCode) +
    (messages.hashCode) +
    (queued.hashCode);

  @override
  String toString() => 'AppRescrapeResult[records=$records, written=$written, messages=$messages, queued=$queued]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'records'] = this.records;
      json[r'written'] = this.written;
      json[r'messages'] = this.messages;
      json[r'queued'] = this.queued;
    return json;
  }

  /// Returns a new [AppRescrapeResult] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppRescrapeResult? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'records'), 'Required key "AppRescrapeResult[records]" is missing from JSON.');
        assert(json[r'records'] != null, 'Required key "AppRescrapeResult[records]" has a null value in JSON.');
        assert(json.containsKey(r'written'), 'Required key "AppRescrapeResult[written]" is missing from JSON.');
        assert(json[r'written'] != null, 'Required key "AppRescrapeResult[written]" has a null value in JSON.');
        assert(json.containsKey(r'messages'), 'Required key "AppRescrapeResult[messages]" is missing from JSON.');
        assert(json[r'messages'] != null, 'Required key "AppRescrapeResult[messages]" has a null value in JSON.');
        assert(json.containsKey(r'queued'), 'Required key "AppRescrapeResult[queued]" is missing from JSON.');
        assert(json[r'queued'] != null, 'Required key "AppRescrapeResult[queued]" has a null value in JSON.');
        return true;
      }());

      return AppRescrapeResult(
        records: mapValueOfType<int>(json, r'records')!,
        written: mapValueOfType<int>(json, r'written')!,
        messages: json[r'messages'] is Iterable
            ? (json[r'messages'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        queued: mapValueOfType<bool>(json, r'queued')!,
      );
    }
    return null;
  }

  static List<AppRescrapeResult> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppRescrapeResult>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppRescrapeResult.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppRescrapeResult> mapFromJson(dynamic json) {
    final map = <String, AppRescrapeResult>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppRescrapeResult.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppRescrapeResult-objects as value to a dart map
  static Map<String, List<AppRescrapeResult>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppRescrapeResult>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppRescrapeResult.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'records',
    'written',
    'messages',
    'queued',
  };
}

