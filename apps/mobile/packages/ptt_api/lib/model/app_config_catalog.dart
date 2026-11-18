//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppConfigCatalog {
  /// Returns a new [AppConfigCatalog] instance.
  AppConfigCatalog({
    this.resources = const [],
    this.excluded = const [],
    required this.secretPlaceholder,
  });

  List<AppConfigResource> resources;

  /// 不能经接口读改、只能在网页上改的配置
  List<String> excluded;

  /// 有值的密钥读出来的样子；改的时候交回它表示不改
  String secretPlaceholder;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppConfigCatalog &&
    _deepEquality.equals(other.resources, resources) &&
    _deepEquality.equals(other.excluded, excluded) &&
    other.secretPlaceholder == secretPlaceholder;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (resources.hashCode) +
    (excluded.hashCode) +
    (secretPlaceholder.hashCode);

  @override
  String toString() => 'AppConfigCatalog[resources=$resources, excluded=$excluded, secretPlaceholder=$secretPlaceholder]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'resources'] = this.resources;
      json[r'excluded'] = this.excluded;
      json[r'secret_placeholder'] = this.secretPlaceholder;
    return json;
  }

  /// Returns a new [AppConfigCatalog] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppConfigCatalog? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'resources'), 'Required key "AppConfigCatalog[resources]" is missing from JSON.');
        assert(json[r'resources'] != null, 'Required key "AppConfigCatalog[resources]" has a null value in JSON.');
        assert(json.containsKey(r'excluded'), 'Required key "AppConfigCatalog[excluded]" is missing from JSON.');
        assert(json[r'excluded'] != null, 'Required key "AppConfigCatalog[excluded]" has a null value in JSON.');
        assert(json.containsKey(r'secret_placeholder'), 'Required key "AppConfigCatalog[secret_placeholder]" is missing from JSON.');
        assert(json[r'secret_placeholder'] != null, 'Required key "AppConfigCatalog[secret_placeholder]" has a null value in JSON.');
        return true;
      }());

      return AppConfigCatalog(
        resources: AppConfigResource.listFromJson(json[r'resources']),
        excluded: json[r'excluded'] is Iterable
            ? (json[r'excluded'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        secretPlaceholder: mapValueOfType<String>(json, r'secret_placeholder')!,
      );
    }
    return null;
  }

  static List<AppConfigCatalog> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppConfigCatalog>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppConfigCatalog.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppConfigCatalog> mapFromJson(dynamic json) {
    final map = <String, AppConfigCatalog>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppConfigCatalog.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppConfigCatalog-objects as value to a dart map
  static Map<String, List<AppConfigCatalog>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppConfigCatalog>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppConfigCatalog.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'resources',
    'excluded',
    'secret_placeholder',
  };
}

