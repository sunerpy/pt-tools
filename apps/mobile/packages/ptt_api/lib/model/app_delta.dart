//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppDelta {
  /// Returns a new [AppDelta] instance.
  AppDelta({
    required this.from,
    required this.to,
    required this.uploaded,
    required this.downloaded,
    required this.bonus,
    this.sites = const [],
    this.error,
  });

  String from;

  String to;

  int uploaded;

  int downloaded;

  double bonus;

  List<AppSiteDelta> sites;

  /// 增量算不出来时的原因（这时的 0 不代表今天没有流量）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? error;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppDelta &&
    other.from == from &&
    other.to == to &&
    other.uploaded == uploaded &&
    other.downloaded == downloaded &&
    other.bonus == bonus &&
    _deepEquality.equals(other.sites, sites) &&
    other.error == error;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (from.hashCode) +
    (to.hashCode) +
    (uploaded.hashCode) +
    (downloaded.hashCode) +
    (bonus.hashCode) +
    (sites.hashCode) +
    (error == null ? 0 : error!.hashCode);

  @override
  String toString() => 'AppDelta[from=$from, to=$to, uploaded=$uploaded, downloaded=$downloaded, bonus=$bonus, sites=$sites, error=$error]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'from'] = this.from;
      json[r'to'] = this.to;
      json[r'uploaded'] = this.uploaded;
      json[r'downloaded'] = this.downloaded;
      json[r'bonus'] = this.bonus;
      json[r'sites'] = this.sites;
    if (this.error != null) {
      json[r'error'] = this.error;
    } else {
      json[r'error'] = null;
    }
    return json;
  }

  /// Returns a new [AppDelta] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppDelta? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'from'), 'Required key "AppDelta[from]" is missing from JSON.');
        assert(json[r'from'] != null, 'Required key "AppDelta[from]" has a null value in JSON.');
        assert(json.containsKey(r'to'), 'Required key "AppDelta[to]" is missing from JSON.');
        assert(json[r'to'] != null, 'Required key "AppDelta[to]" has a null value in JSON.');
        assert(json.containsKey(r'uploaded'), 'Required key "AppDelta[uploaded]" is missing from JSON.');
        assert(json[r'uploaded'] != null, 'Required key "AppDelta[uploaded]" has a null value in JSON.');
        assert(json.containsKey(r'downloaded'), 'Required key "AppDelta[downloaded]" is missing from JSON.');
        assert(json[r'downloaded'] != null, 'Required key "AppDelta[downloaded]" has a null value in JSON.');
        assert(json.containsKey(r'bonus'), 'Required key "AppDelta[bonus]" is missing from JSON.');
        assert(json[r'bonus'] != null, 'Required key "AppDelta[bonus]" has a null value in JSON.');
        assert(json.containsKey(r'sites'), 'Required key "AppDelta[sites]" is missing from JSON.');
        assert(json[r'sites'] != null, 'Required key "AppDelta[sites]" has a null value in JSON.');
        return true;
      }());

      return AppDelta(
        from: mapValueOfType<String>(json, r'from')!,
        to: mapValueOfType<String>(json, r'to')!,
        uploaded: mapValueOfType<int>(json, r'uploaded')!,
        downloaded: mapValueOfType<int>(json, r'downloaded')!,
        bonus: mapValueOfType<double>(json, r'bonus')!,
        sites: AppSiteDelta.listFromJson(json[r'sites']),
        error: mapValueOfType<String>(json, r'error'),
      );
    }
    return null;
  }

  static List<AppDelta> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppDelta>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppDelta.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppDelta> mapFromJson(dynamic json) {
    final map = <String, AppDelta>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppDelta.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppDelta-objects as value to a dart map
  static Map<String, List<AppDelta>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppDelta>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppDelta.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'from',
    'to',
    'uploaded',
    'downloaded',
    'bonus',
    'sites',
  };
}

