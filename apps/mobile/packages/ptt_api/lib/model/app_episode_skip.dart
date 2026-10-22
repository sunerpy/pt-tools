//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppEpisodeSkip {
  /// Returns a new [AppEpisodeSkip] instance.
  AppEpisodeSkip({
    required this.skip,
  });

  /// true 跳过这一集，false 重新要
  bool skip;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppEpisodeSkip &&
    other.skip == skip;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (skip.hashCode);

  @override
  String toString() => 'AppEpisodeSkip[skip=$skip]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'skip'] = this.skip;
    return json;
  }

  /// Returns a new [AppEpisodeSkip] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppEpisodeSkip? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'skip'), 'Required key "AppEpisodeSkip[skip]" is missing from JSON.');
        assert(json[r'skip'] != null, 'Required key "AppEpisodeSkip[skip]" has a null value in JSON.');
        return true;
      }());

      return AppEpisodeSkip(
        skip: mapValueOfType<bool>(json, r'skip')!,
      );
    }
    return null;
  }

  static List<AppEpisodeSkip> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppEpisodeSkip>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppEpisodeSkip.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppEpisodeSkip> mapFromJson(dynamic json) {
    final map = <String, AppEpisodeSkip>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppEpisodeSkip.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppEpisodeSkip-objects as value to a dart map
  static Map<String, List<AppEpisodeSkip>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppEpisodeSkip>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppEpisodeSkip.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'skip',
  };
}

