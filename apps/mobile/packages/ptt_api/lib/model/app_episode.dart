//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppEpisode {
  /// Returns a new [AppEpisode] instance.
  AppEpisode({
    required this.number,
    this.name,
    this.airDate,
    required this.state,
  });

  int number;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? name;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? airDate;

  /// library、downloading、missing 或 upcoming
  String state;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppEpisode &&
    other.number == number &&
    other.name == name &&
    other.airDate == airDate &&
    other.state == state;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (number.hashCode) +
    (name == null ? 0 : name!.hashCode) +
    (airDate == null ? 0 : airDate!.hashCode) +
    (state.hashCode);

  @override
  String toString() => 'AppEpisode[number=$number, name=$name, airDate=$airDate, state=$state]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'number'] = this.number;
    if (this.name != null) {
      json[r'name'] = this.name;
    } else {
      json[r'name'] = null;
    }
    if (this.airDate != null) {
      json[r'air_date'] = this.airDate;
    } else {
      json[r'air_date'] = null;
    }
      json[r'state'] = this.state;
    return json;
  }

  /// Returns a new [AppEpisode] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppEpisode? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'number'), 'Required key "AppEpisode[number]" is missing from JSON.');
        assert(json[r'number'] != null, 'Required key "AppEpisode[number]" has a null value in JSON.');
        assert(json.containsKey(r'state'), 'Required key "AppEpisode[state]" is missing from JSON.');
        assert(json[r'state'] != null, 'Required key "AppEpisode[state]" has a null value in JSON.');
        return true;
      }());

      return AppEpisode(
        number: mapValueOfType<int>(json, r'number')!,
        name: mapValueOfType<String>(json, r'name'),
        airDate: mapValueOfType<String>(json, r'air_date'),
        state: mapValueOfType<String>(json, r'state')!,
      );
    }
    return null;
  }

  static List<AppEpisode> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppEpisode>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppEpisode.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppEpisode> mapFromJson(dynamic json) {
    final map = <String, AppEpisode>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppEpisode.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppEpisode-objects as value to a dart map
  static Map<String, List<AppEpisode>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppEpisode>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppEpisode.listFromJson(entry.value, growable: growable,);
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

