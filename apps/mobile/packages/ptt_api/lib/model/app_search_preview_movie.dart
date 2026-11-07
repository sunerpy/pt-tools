//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewMovie {
  /// Returns a new [AppSearchPreviewMovie] instance.
  AppSearchPreviewMovie({
    required this.state,
    this.site,
  });

  /// library、downloading 或 missing
  String state;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? site;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewMovie &&
    other.state == state &&
    other.site == site;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (state.hashCode) +
    (site == null ? 0 : site!.hashCode);

  @override
  String toString() => 'AppSearchPreviewMovie[state=$state, site=$site]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'state'] = this.state;
    if (this.site != null) {
      json[r'site'] = this.site;
    } else {
      json[r'site'] = null;
    }
    return json;
  }

  /// Returns a new [AppSearchPreviewMovie] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewMovie? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'state'), 'Required key "AppSearchPreviewMovie[state]" is missing from JSON.');
        assert(json[r'state'] != null, 'Required key "AppSearchPreviewMovie[state]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewMovie(
        state: mapValueOfType<String>(json, r'state')!,
        site: mapValueOfType<String>(json, r'site'),
      );
    }
    return null;
  }

  static List<AppSearchPreviewMovie> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewMovie>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewMovie.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewMovie> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewMovie>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewMovie.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewMovie-objects as value to a dart map
  static Map<String, List<AppSearchPreviewMovie>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewMovie>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewMovie.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'state',
  };
}

