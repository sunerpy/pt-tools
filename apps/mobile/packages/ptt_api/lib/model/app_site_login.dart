//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSiteLogin {
  /// Returns a new [AppSiteLogin] instance.
  AppSiteLogin({
    required this.tier,
    required this.daysRemaining,
    this.lastActiveAt,
    this.lastProbeAt,
    this.lastProbeStatus,
  });

  /// none、30d、14d、7d、3d、banned-imminent 或 unknown
  String tier;

  int daysRemaining;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? lastActiveAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? lastProbeAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? lastProbeStatus;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSiteLogin &&
    other.tier == tier &&
    other.daysRemaining == daysRemaining &&
    other.lastActiveAt == lastActiveAt &&
    other.lastProbeAt == lastProbeAt &&
    other.lastProbeStatus == lastProbeStatus;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (tier.hashCode) +
    (daysRemaining.hashCode) +
    (lastActiveAt == null ? 0 : lastActiveAt!.hashCode) +
    (lastProbeAt == null ? 0 : lastProbeAt!.hashCode) +
    (lastProbeStatus == null ? 0 : lastProbeStatus!.hashCode);

  @override
  String toString() => 'AppSiteLogin[tier=$tier, daysRemaining=$daysRemaining, lastActiveAt=$lastActiveAt, lastProbeAt=$lastProbeAt, lastProbeStatus=$lastProbeStatus]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'tier'] = this.tier;
      json[r'days_remaining'] = this.daysRemaining;
    if (this.lastActiveAt != null) {
      json[r'last_active_at'] = this.lastActiveAt;
    } else {
      json[r'last_active_at'] = null;
    }
    if (this.lastProbeAt != null) {
      json[r'last_probe_at'] = this.lastProbeAt;
    } else {
      json[r'last_probe_at'] = null;
    }
    if (this.lastProbeStatus != null) {
      json[r'last_probe_status'] = this.lastProbeStatus;
    } else {
      json[r'last_probe_status'] = null;
    }
    return json;
  }

  /// Returns a new [AppSiteLogin] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSiteLogin? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'tier'), 'Required key "AppSiteLogin[tier]" is missing from JSON.');
        assert(json[r'tier'] != null, 'Required key "AppSiteLogin[tier]" has a null value in JSON.');
        assert(json.containsKey(r'days_remaining'), 'Required key "AppSiteLogin[days_remaining]" is missing from JSON.');
        assert(json[r'days_remaining'] != null, 'Required key "AppSiteLogin[days_remaining]" has a null value in JSON.');
        return true;
      }());

      return AppSiteLogin(
        tier: mapValueOfType<String>(json, r'tier')!,
        daysRemaining: mapValueOfType<int>(json, r'days_remaining')!,
        lastActiveAt: mapValueOfType<int>(json, r'last_active_at'),
        lastProbeAt: mapValueOfType<int>(json, r'last_probe_at'),
        lastProbeStatus: mapValueOfType<String>(json, r'last_probe_status'),
      );
    }
    return null;
  }

  static List<AppSiteLogin> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSiteLogin>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSiteLogin.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSiteLogin> mapFromJson(dynamic json) {
    final map = <String, AppSiteLogin>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSiteLogin.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSiteLogin-objects as value to a dart map
  static Map<String, List<AppSiteLogin>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSiteLogin>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSiteLogin.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'tier',
    'days_remaining',
  };
}

