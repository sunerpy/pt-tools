//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSite {
  /// Returns a new [AppSite] instance.
  AppSite({
    required this.name,
    required this.displayName,
    required this.enabled,
    required this.login,
    required this.attendance,
    this.user,
  });

  String name;

  String displayName;

  bool enabled;

  AppSiteLogin login;

  AppSiteAttendance attendance;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppSiteUser? user;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSite &&
    other.name == name &&
    other.displayName == displayName &&
    other.enabled == enabled &&
    other.login == login &&
    other.attendance == attendance &&
    other.user == user;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (name.hashCode) +
    (displayName.hashCode) +
    (enabled.hashCode) +
    (login.hashCode) +
    (attendance.hashCode) +
    (user == null ? 0 : user!.hashCode);

  @override
  String toString() => 'AppSite[name=$name, displayName=$displayName, enabled=$enabled, login=$login, attendance=$attendance, user=$user]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'name'] = this.name;
      json[r'display_name'] = this.displayName;
      json[r'enabled'] = this.enabled;
      json[r'login'] = this.login;
      json[r'attendance'] = this.attendance;
    if (this.user != null) {
      json[r'user'] = this.user;
    } else {
      json[r'user'] = null;
    }
    return json;
  }

  /// Returns a new [AppSite] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSite? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'name'), 'Required key "AppSite[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppSite[name]" has a null value in JSON.');
        assert(json.containsKey(r'display_name'), 'Required key "AppSite[display_name]" is missing from JSON.');
        assert(json[r'display_name'] != null, 'Required key "AppSite[display_name]" has a null value in JSON.');
        assert(json.containsKey(r'enabled'), 'Required key "AppSite[enabled]" is missing from JSON.');
        assert(json[r'enabled'] != null, 'Required key "AppSite[enabled]" has a null value in JSON.');
        assert(json.containsKey(r'login'), 'Required key "AppSite[login]" is missing from JSON.');
        assert(json[r'login'] != null, 'Required key "AppSite[login]" has a null value in JSON.');
        assert(json.containsKey(r'attendance'), 'Required key "AppSite[attendance]" is missing from JSON.');
        assert(json[r'attendance'] != null, 'Required key "AppSite[attendance]" has a null value in JSON.');
        return true;
      }());

      return AppSite(
        name: mapValueOfType<String>(json, r'name')!,
        displayName: mapValueOfType<String>(json, r'display_name')!,
        enabled: mapValueOfType<bool>(json, r'enabled')!,
        login: AppSiteLogin.fromJson(json[r'login'])!,
        attendance: AppSiteAttendance.fromJson(json[r'attendance'])!,
        user: AppSiteUser.fromJson(json[r'user']),
      );
    }
    return null;
  }

  static List<AppSite> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSite>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSite.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSite> mapFromJson(dynamic json) {
    final map = <String, AppSite>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSite.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSite-objects as value to a dart map
  static Map<String, List<AppSite>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSite>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSite.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'name',
    'display_name',
    'enabled',
    'login',
    'attendance',
  };
}

