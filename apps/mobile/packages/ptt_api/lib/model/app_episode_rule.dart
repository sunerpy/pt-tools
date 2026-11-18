//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppEpisodeRule {
  /// Returns a new [AppEpisodeRule] instance.
  AppEpisodeRule({
    required this.group,
    required this.mode,
    required this.offset,
    required this.source_,
    this.note,
    required this.updatedAt,
  });

  /// 制作组；空的对所有组
  String group;

  /// tmdb（按 TMDB 的集号）、period_part（按第几期与上下）、date（按播出日期）或 offset（TMDB 的集号 = 标题里的集号 + offset）
  String mode;

  int offset;

  /// manual、ai 或 auto
  String source_;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? note;

  int updatedAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppEpisodeRule &&
    other.group == group &&
    other.mode == mode &&
    other.offset == offset &&
    other.source_ == source_ &&
    other.note == note &&
    other.updatedAt == updatedAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (group.hashCode) +
    (mode.hashCode) +
    (offset.hashCode) +
    (source_.hashCode) +
    (note == null ? 0 : note!.hashCode) +
    (updatedAt.hashCode);

  @override
  String toString() => 'AppEpisodeRule[group=$group, mode=$mode, offset=$offset, source_=$source_, note=$note, updatedAt=$updatedAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'group'] = this.group;
      json[r'mode'] = this.mode;
      json[r'offset'] = this.offset;
      json[r'source'] = this.source_;
    if (this.note != null) {
      json[r'note'] = this.note;
    } else {
      json[r'note'] = null;
    }
      json[r'updated_at'] = this.updatedAt;
    return json;
  }

  /// Returns a new [AppEpisodeRule] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppEpisodeRule? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'group'), 'Required key "AppEpisodeRule[group]" is missing from JSON.');
        assert(json[r'group'] != null, 'Required key "AppEpisodeRule[group]" has a null value in JSON.');
        assert(json.containsKey(r'mode'), 'Required key "AppEpisodeRule[mode]" is missing from JSON.');
        assert(json[r'mode'] != null, 'Required key "AppEpisodeRule[mode]" has a null value in JSON.');
        assert(json.containsKey(r'offset'), 'Required key "AppEpisodeRule[offset]" is missing from JSON.');
        assert(json[r'offset'] != null, 'Required key "AppEpisodeRule[offset]" has a null value in JSON.');
        assert(json.containsKey(r'source'), 'Required key "AppEpisodeRule[source]" is missing from JSON.');
        assert(json[r'source'] != null, 'Required key "AppEpisodeRule[source]" has a null value in JSON.');
        assert(json.containsKey(r'updated_at'), 'Required key "AppEpisodeRule[updated_at]" is missing from JSON.');
        assert(json[r'updated_at'] != null, 'Required key "AppEpisodeRule[updated_at]" has a null value in JSON.');
        return true;
      }());

      return AppEpisodeRule(
        group: mapValueOfType<String>(json, r'group')!,
        mode: mapValueOfType<String>(json, r'mode')!,
        offset: mapValueOfType<int>(json, r'offset')!,
        source_: mapValueOfType<String>(json, r'source')!,
        note: mapValueOfType<String>(json, r'note'),
        updatedAt: mapValueOfType<int>(json, r'updated_at')!,
      );
    }
    return null;
  }

  static List<AppEpisodeRule> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppEpisodeRule>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppEpisodeRule.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppEpisodeRule> mapFromJson(dynamic json) {
    final map = <String, AppEpisodeRule>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppEpisodeRule.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppEpisodeRule-objects as value to a dart map
  static Map<String, List<AppEpisodeRule>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppEpisodeRule>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppEpisodeRule.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'group',
    'mode',
    'offset',
    'source',
    'updated_at',
  };
}

