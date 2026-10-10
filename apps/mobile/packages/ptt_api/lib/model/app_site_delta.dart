//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSiteDelta {
  /// Returns a new [AppSiteDelta] instance.
  AppSiteDelta({
    required this.site,
    required this.uploaded,
    required this.downloaded,
    required this.bonus,
  });

  String site;

  int uploaded;

  int downloaded;

  double bonus;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSiteDelta &&
    other.site == site &&
    other.uploaded == uploaded &&
    other.downloaded == downloaded &&
    other.bonus == bonus;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (uploaded.hashCode) +
    (downloaded.hashCode) +
    (bonus.hashCode);

  @override
  String toString() => 'AppSiteDelta[site=$site, uploaded=$uploaded, downloaded=$downloaded, bonus=$bonus]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'uploaded'] = this.uploaded;
      json[r'downloaded'] = this.downloaded;
      json[r'bonus'] = this.bonus;
    return json;
  }

  /// Returns a new [AppSiteDelta] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSiteDelta? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppSiteDelta[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppSiteDelta[site]" has a null value in JSON.');
        assert(json.containsKey(r'uploaded'), 'Required key "AppSiteDelta[uploaded]" is missing from JSON.');
        assert(json[r'uploaded'] != null, 'Required key "AppSiteDelta[uploaded]" has a null value in JSON.');
        assert(json.containsKey(r'downloaded'), 'Required key "AppSiteDelta[downloaded]" is missing from JSON.');
        assert(json[r'downloaded'] != null, 'Required key "AppSiteDelta[downloaded]" has a null value in JSON.');
        assert(json.containsKey(r'bonus'), 'Required key "AppSiteDelta[bonus]" is missing from JSON.');
        assert(json[r'bonus'] != null, 'Required key "AppSiteDelta[bonus]" has a null value in JSON.');
        return true;
      }());

      return AppSiteDelta(
        site: mapValueOfType<String>(json, r'site')!,
        uploaded: mapValueOfType<int>(json, r'uploaded')!,
        downloaded: mapValueOfType<int>(json, r'downloaded')!,
        bonus: mapValueOfType<double>(json, r'bonus')!,
      );
    }
    return null;
  }

  static List<AppSiteDelta> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSiteDelta>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSiteDelta.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSiteDelta> mapFromJson(dynamic json) {
    final map = <String, AppSiteDelta>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSiteDelta.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSiteDelta-objects as value to a dart map
  static Map<String, List<AppSiteDelta>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSiteDelta>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSiteDelta.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'uploaded',
    'downloaded',
    'bonus',
  };
}

