//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewVerdict {
  /// Returns a new [AppSearchPreviewVerdict] instance.
  AppSearchPreviewVerdict({
    required this.score,
    required this.reject,
  });

  /// 0–100 的分
  int score;

  /// 为空时符合；不为空时是全部原因，例如「标题含排除词 hdtv」
  String reject;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewVerdict &&
    other.score == score &&
    other.reject == reject;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (score.hashCode) +
    (reject.hashCode);

  @override
  String toString() => 'AppSearchPreviewVerdict[score=$score, reject=$reject]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'score'] = this.score;
      json[r'reject'] = this.reject;
    return json;
  }

  /// Returns a new [AppSearchPreviewVerdict] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewVerdict? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'score'), 'Required key "AppSearchPreviewVerdict[score]" is missing from JSON.');
        assert(json[r'score'] != null, 'Required key "AppSearchPreviewVerdict[score]" has a null value in JSON.');
        assert(json.containsKey(r'reject'), 'Required key "AppSearchPreviewVerdict[reject]" is missing from JSON.');
        assert(json[r'reject'] != null, 'Required key "AppSearchPreviewVerdict[reject]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewVerdict(
        score: mapValueOfType<int>(json, r'score')!,
        reject: mapValueOfType<String>(json, r'reject')!,
      );
    }
    return null;
  }

  static List<AppSearchPreviewVerdict> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewVerdict>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewVerdict.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewVerdict> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewVerdict>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewVerdict.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewVerdict-objects as value to a dart map
  static Map<String, List<AppSearchPreviewVerdict>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewVerdict>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewVerdict.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'score',
    'reject',
  };
}

