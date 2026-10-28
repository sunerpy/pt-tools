//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppStatsSummary {
  /// Returns a new [AppStatsSummary] instance.
  AppStatsSummary({
    required this.range,
    required this.from,
    required this.to,
    required this.totalUploaded,
    required this.totalDownloaded,
    required this.totalBonus,
    this.sites = const [],
  });

  String range;

  String from;

  String to;

  int totalUploaded;

  int totalDownloaded;

  num totalBonus;

  List<AppStatsSiteDelta> sites;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppStatsSummary &&
    other.range == range &&
    other.from == from &&
    other.to == to &&
    other.totalUploaded == totalUploaded &&
    other.totalDownloaded == totalDownloaded &&
    other.totalBonus == totalBonus &&
    _deepEquality.equals(other.sites, sites);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (range.hashCode) +
    (from.hashCode) +
    (to.hashCode) +
    (totalUploaded.hashCode) +
    (totalDownloaded.hashCode) +
    (totalBonus.hashCode) +
    (sites.hashCode);

  @override
  String toString() => 'AppStatsSummary[range=$range, from=$from, to=$to, totalUploaded=$totalUploaded, totalDownloaded=$totalDownloaded, totalBonus=$totalBonus, sites=$sites]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'range'] = this.range;
      json[r'from'] = this.from;
      json[r'to'] = this.to;
      json[r'totalUploaded'] = this.totalUploaded;
      json[r'totalDownloaded'] = this.totalDownloaded;
      json[r'totalBonus'] = this.totalBonus;
      json[r'sites'] = this.sites;
    return json;
  }

  /// Returns a new [AppStatsSummary] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppStatsSummary? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'range'), 'Required key "AppStatsSummary[range]" is missing from JSON.');
        assert(json[r'range'] != null, 'Required key "AppStatsSummary[range]" has a null value in JSON.');
        assert(json.containsKey(r'from'), 'Required key "AppStatsSummary[from]" is missing from JSON.');
        assert(json[r'from'] != null, 'Required key "AppStatsSummary[from]" has a null value in JSON.');
        assert(json.containsKey(r'to'), 'Required key "AppStatsSummary[to]" is missing from JSON.');
        assert(json[r'to'] != null, 'Required key "AppStatsSummary[to]" has a null value in JSON.');
        assert(json.containsKey(r'totalUploaded'), 'Required key "AppStatsSummary[totalUploaded]" is missing from JSON.');
        assert(json[r'totalUploaded'] != null, 'Required key "AppStatsSummary[totalUploaded]" has a null value in JSON.');
        assert(json.containsKey(r'totalDownloaded'), 'Required key "AppStatsSummary[totalDownloaded]" is missing from JSON.');
        assert(json[r'totalDownloaded'] != null, 'Required key "AppStatsSummary[totalDownloaded]" has a null value in JSON.');
        assert(json.containsKey(r'totalBonus'), 'Required key "AppStatsSummary[totalBonus]" is missing from JSON.');
        assert(json[r'totalBonus'] != null, 'Required key "AppStatsSummary[totalBonus]" has a null value in JSON.');
        assert(json.containsKey(r'sites'), 'Required key "AppStatsSummary[sites]" is missing from JSON.');
        assert(json[r'sites'] != null, 'Required key "AppStatsSummary[sites]" has a null value in JSON.');
        return true;
      }());

      return AppStatsSummary(
        range: mapValueOfType<String>(json, r'range')!,
        from: mapValueOfType<String>(json, r'from')!,
        to: mapValueOfType<String>(json, r'to')!,
        totalUploaded: mapValueOfType<int>(json, r'totalUploaded')!,
        totalDownloaded: mapValueOfType<int>(json, r'totalDownloaded')!,
        totalBonus: num.parse('${json[r'totalBonus']}'),
        sites: AppStatsSiteDelta.listFromJson(json[r'sites']),
      );
    }
    return null;
  }

  static List<AppStatsSummary> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppStatsSummary>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppStatsSummary.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppStatsSummary> mapFromJson(dynamic json) {
    final map = <String, AppStatsSummary>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppStatsSummary.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppStatsSummary-objects as value to a dart map
  static Map<String, List<AppStatsSummary>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppStatsSummary>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppStatsSummary.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'range',
    'from',
    'to',
    'totalUploaded',
    'totalDownloaded',
    'totalBonus',
    'sites',
  };
}

