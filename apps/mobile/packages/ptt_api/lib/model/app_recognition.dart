//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppRecognition {
  /// Returns a new [AppRecognition] instance.
  AppRecognition({
    required this.summary,
    required this.meta,
    this.match,
    required this.source_,
    this.score,
    this.candidates = const [],
    this.message,
    this.error,
  });

  /// 给人看的一句话：名字、年份、季集
  String summary;

  AppMediaMeta meta;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppMediaMatch? match;

  /// 怎么认出来的：override（手动纠正过）、imdb（按站点给的 IMDb 编号）、search（按名字搜）、none（没认出来）
  String source_;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  num? score;

  /// 最多 5 个候选（没有可靠的匹配时从这里挑）
  List<AppMediaMatch> candidates;

  /// 提示，例如没有填 TMDB API Key、没有可靠的匹配
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? message;

  /// TMDB 出错的原因（标题照样解析了）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? error;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppRecognition &&
    other.summary == summary &&
    other.meta == meta &&
    other.match == match &&
    other.source_ == source_ &&
    other.score == score &&
    _deepEquality.equals(other.candidates, candidates) &&
    other.message == message &&
    other.error == error;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (summary.hashCode) +
    (meta.hashCode) +
    (match == null ? 0 : match!.hashCode) +
    (source_.hashCode) +
    (score == null ? 0 : score!.hashCode) +
    (candidates.hashCode) +
    (message == null ? 0 : message!.hashCode) +
    (error == null ? 0 : error!.hashCode);

  @override
  String toString() => 'AppRecognition[summary=$summary, meta=$meta, match=$match, source_=$source_, score=$score, candidates=$candidates, message=$message, error=$error]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'summary'] = this.summary;
      json[r'meta'] = this.meta;
    if (this.match != null) {
      json[r'match'] = this.match;
    } else {
      json[r'match'] = null;
    }
      json[r'source'] = this.source_;
    if (this.score != null) {
      json[r'score'] = this.score;
    } else {
      json[r'score'] = null;
    }
      json[r'candidates'] = this.candidates;
    if (this.message != null) {
      json[r'message'] = this.message;
    } else {
      json[r'message'] = null;
    }
    if (this.error != null) {
      json[r'error'] = this.error;
    } else {
      json[r'error'] = null;
    }
    return json;
  }

  /// Returns a new [AppRecognition] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppRecognition? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'summary'), 'Required key "AppRecognition[summary]" is missing from JSON.');
        assert(json[r'summary'] != null, 'Required key "AppRecognition[summary]" has a null value in JSON.');
        assert(json.containsKey(r'meta'), 'Required key "AppRecognition[meta]" is missing from JSON.');
        assert(json[r'meta'] != null, 'Required key "AppRecognition[meta]" has a null value in JSON.');
        assert(json.containsKey(r'source'), 'Required key "AppRecognition[source]" is missing from JSON.');
        assert(json[r'source'] != null, 'Required key "AppRecognition[source]" has a null value in JSON.');
        assert(json.containsKey(r'candidates'), 'Required key "AppRecognition[candidates]" is missing from JSON.');
        assert(json[r'candidates'] != null, 'Required key "AppRecognition[candidates]" has a null value in JSON.');
        return true;
      }());

      return AppRecognition(
        summary: mapValueOfType<String>(json, r'summary')!,
        meta: AppMediaMeta.fromJson(json[r'meta'])!,
        match: AppMediaMatch.fromJson(json[r'match']),
        source_: mapValueOfType<String>(json, r'source')!,
        score: json[r'score'] == null
            ? null
            : num.parse('${json[r'score']}'),
        candidates: AppMediaMatch.listFromJson(json[r'candidates']),
        message: mapValueOfType<String>(json, r'message'),
        error: mapValueOfType<String>(json, r'error'),
      );
    }
    return null;
  }

  static List<AppRecognition> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppRecognition>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppRecognition.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppRecognition> mapFromJson(dynamic json) {
    final map = <String, AppRecognition>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppRecognition.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppRecognition-objects as value to a dart map
  static Map<String, List<AppRecognition>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppRecognition>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppRecognition.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'summary',
    'meta',
    'source',
    'candidates',
  };
}

