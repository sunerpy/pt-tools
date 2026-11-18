//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppPrincipal {
  /// Returns a new [AppPrincipal] instance.
  AppPrincipal({
    required this.kind,
    this.name,
    this.scopes = const [],
  });

  /// api_token、session 或 remote_device
  String kind;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? name;

  List<String> scopes;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppPrincipal &&
    other.kind == kind &&
    other.name == name &&
    _deepEquality.equals(other.scopes, scopes);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (kind.hashCode) +
    (name == null ? 0 : name!.hashCode) +
    (scopes.hashCode);

  @override
  String toString() => 'AppPrincipal[kind=$kind, name=$name, scopes=$scopes]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'kind'] = this.kind;
    if (this.name != null) {
      json[r'name'] = this.name;
    } else {
      json[r'name'] = null;
    }
      json[r'scopes'] = this.scopes;
    return json;
  }

  /// Returns a new [AppPrincipal] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppPrincipal? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'kind'), 'Required key "AppPrincipal[kind]" is missing from JSON.');
        assert(json[r'kind'] != null, 'Required key "AppPrincipal[kind]" has a null value in JSON.');
        assert(json.containsKey(r'scopes'), 'Required key "AppPrincipal[scopes]" is missing from JSON.');
        assert(json[r'scopes'] != null, 'Required key "AppPrincipal[scopes]" has a null value in JSON.');
        return true;
      }());

      return AppPrincipal(
        kind: mapValueOfType<String>(json, r'kind')!,
        name: mapValueOfType<String>(json, r'name'),
        scopes: json[r'scopes'] is Iterable
            ? (json[r'scopes'] as Iterable).cast<String>().toList(growable: false)
            : const [],
      );
    }
    return null;
  }

  static List<AppPrincipal> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppPrincipal>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppPrincipal.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppPrincipal> mapFromJson(dynamic json) {
    final map = <String, AppPrincipal>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppPrincipal.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppPrincipal-objects as value to a dart map
  static Map<String, List<AppPrincipal>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppPrincipal>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppPrincipal.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'kind',
    'scopes',
  };
}

