//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppStatsSiteDelta {
  /// Returns a new [AppStatsSiteDelta] instance.
  AppStatsSiteDelta({
    required this.site,
    required this.from,
    required this.to,
    required this.uploaded,
    required this.downloaded,
    required this.bonus,
    required this.hasBaseline,
    required this.negative,
  });

  String site;

  String from;

  String to;

  int uploaded;

  int downloaded;

  num bonus;

  bool hasBaseline;

  bool negative;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppStatsSiteDelta &&
    other.site == site &&
    other.from == from &&
    other.to == to &&
    other.uploaded == uploaded &&
    other.downloaded == downloaded &&
    other.bonus == bonus &&
    other.hasBaseline == hasBaseline &&
    other.negative == negative;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (from.hashCode) +
    (to.hashCode) +
    (uploaded.hashCode) +
    (downloaded.hashCode) +
    (bonus.hashCode) +
    (hasBaseline.hashCode) +
    (negative.hashCode);

  @override
  String toString() => 'AppStatsSiteDelta[site=$site, from=$from, to=$to, uploaded=$uploaded, downloaded=$downloaded, bonus=$bonus, hasBaseline=$hasBaseline, negative=$negative]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'from'] = this.from;
      json[r'to'] = this.to;
      json[r'uploaded'] = this.uploaded;
      json[r'downloaded'] = this.downloaded;
      json[r'bonus'] = this.bonus;
      json[r'hasBaseline'] = this.hasBaseline;
      json[r'negative'] = this.negative;
    return json;
  }

  /// Returns a new [AppStatsSiteDelta] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppStatsSiteDelta? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppStatsSiteDelta[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppStatsSiteDelta[site]" has a null value in JSON.');
        assert(json.containsKey(r'from'), 'Required key "AppStatsSiteDelta[from]" is missing from JSON.');
        assert(json[r'from'] != null, 'Required key "AppStatsSiteDelta[from]" has a null value in JSON.');
        assert(json.containsKey(r'to'), 'Required key "AppStatsSiteDelta[to]" is missing from JSON.');
        assert(json[r'to'] != null, 'Required key "AppStatsSiteDelta[to]" has a null value in JSON.');
        assert(json.containsKey(r'uploaded'), 'Required key "AppStatsSiteDelta[uploaded]" is missing from JSON.');
        assert(json[r'uploaded'] != null, 'Required key "AppStatsSiteDelta[uploaded]" has a null value in JSON.');
        assert(json.containsKey(r'downloaded'), 'Required key "AppStatsSiteDelta[downloaded]" is missing from JSON.');
        assert(json[r'downloaded'] != null, 'Required key "AppStatsSiteDelta[downloaded]" has a null value in JSON.');
        assert(json.containsKey(r'bonus'), 'Required key "AppStatsSiteDelta[bonus]" is missing from JSON.');
        assert(json[r'bonus'] != null, 'Required key "AppStatsSiteDelta[bonus]" has a null value in JSON.');
        assert(json.containsKey(r'hasBaseline'), 'Required key "AppStatsSiteDelta[hasBaseline]" is missing from JSON.');
        assert(json[r'hasBaseline'] != null, 'Required key "AppStatsSiteDelta[hasBaseline]" has a null value in JSON.');
        assert(json.containsKey(r'negative'), 'Required key "AppStatsSiteDelta[negative]" is missing from JSON.');
        assert(json[r'negative'] != null, 'Required key "AppStatsSiteDelta[negative]" has a null value in JSON.');
        return true;
      }());

      return AppStatsSiteDelta(
        site: mapValueOfType<String>(json, r'site')!,
        from: mapValueOfType<String>(json, r'from')!,
        to: mapValueOfType<String>(json, r'to')!,
        uploaded: mapValueOfType<int>(json, r'uploaded')!,
        downloaded: mapValueOfType<int>(json, r'downloaded')!,
        bonus: num.parse('${json[r'bonus']}'),
        hasBaseline: mapValueOfType<bool>(json, r'hasBaseline')!,
        negative: mapValueOfType<bool>(json, r'negative')!,
      );
    }
    return null;
  }

  static List<AppStatsSiteDelta> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppStatsSiteDelta>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppStatsSiteDelta.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppStatsSiteDelta> mapFromJson(dynamic json) {
    final map = <String, AppStatsSiteDelta>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppStatsSiteDelta.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppStatsSiteDelta-objects as value to a dart map
  static Map<String, List<AppStatsSiteDelta>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppStatsSiteDelta>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppStatsSiteDelta.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'from',
    'to',
    'uploaded',
    'downloaded',
    'bonus',
    'hasBaseline',
    'negative',
  };
}

