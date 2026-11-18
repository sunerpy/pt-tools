//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewEpisode {
  /// Returns a new [AppSearchPreviewEpisode] instance.
  AppSearchPreviewEpisode({
    required this.number,
    required this.state,
    this.site,
    this.airDate,
    this.name,
  });

  int number;

  /// library 已入库、downloading 在下载、waiting_version 等锁住的版本、missing 缺、upcoming 还没播
  String state;

  /// 在下载时的来源站点
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? site;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? airDate;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? name;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewEpisode &&
    other.number == number &&
    other.state == state &&
    other.site == site &&
    other.airDate == airDate &&
    other.name == name;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (number.hashCode) +
    (state.hashCode) +
    (site == null ? 0 : site!.hashCode) +
    (airDate == null ? 0 : airDate!.hashCode) +
    (name == null ? 0 : name!.hashCode);

  @override
  String toString() => 'AppSearchPreviewEpisode[number=$number, state=$state, site=$site, airDate=$airDate, name=$name]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'number'] = this.number;
      json[r'state'] = this.state;
    if (this.site != null) {
      json[r'site'] = this.site;
    } else {
      json[r'site'] = null;
    }
    if (this.airDate != null) {
      json[r'air_date'] = this.airDate;
    } else {
      json[r'air_date'] = null;
    }
    if (this.name != null) {
      json[r'name'] = this.name;
    } else {
      json[r'name'] = null;
    }
    return json;
  }

  /// Returns a new [AppSearchPreviewEpisode] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewEpisode? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'number'), 'Required key "AppSearchPreviewEpisode[number]" is missing from JSON.');
        assert(json[r'number'] != null, 'Required key "AppSearchPreviewEpisode[number]" has a null value in JSON.');
        assert(json.containsKey(r'state'), 'Required key "AppSearchPreviewEpisode[state]" is missing from JSON.');
        assert(json[r'state'] != null, 'Required key "AppSearchPreviewEpisode[state]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewEpisode(
        number: mapValueOfType<int>(json, r'number')!,
        state: mapValueOfType<String>(json, r'state')!,
        site: mapValueOfType<String>(json, r'site'),
        airDate: mapValueOfType<String>(json, r'air_date'),
        name: mapValueOfType<String>(json, r'name'),
      );
    }
    return null;
  }

  static List<AppSearchPreviewEpisode> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewEpisode>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewEpisode.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewEpisode> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewEpisode>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewEpisode.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewEpisode-objects as value to a dart map
  static Map<String, List<AppSearchPreviewEpisode>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewEpisode>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewEpisode.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'number',
    'state',
  };
}

