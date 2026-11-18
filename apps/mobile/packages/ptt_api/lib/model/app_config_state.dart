//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppConfigState {
  /// Returns a new [AppConfigState] instance.
  AppConfigState({
    required this.resource,
    required this.kind,
    this.idField,
    this.item = const {},
    this.items = const [],
  });

  String resource;

  String kind;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? idField;

  /// 对象类配置的现状（键与网页接口的一样）
  Map<String, Object> item;

  /// 列表类配置的各项（每项是一个对象，键与网页接口的一样）
  List<Object> items;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppConfigState &&
    other.resource == resource &&
    other.kind == kind &&
    other.idField == idField &&
    _deepEquality.equals(other.item, item) &&
    _deepEquality.equals(other.items, items);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (resource.hashCode) +
    (kind.hashCode) +
    (idField == null ? 0 : idField!.hashCode) +
    (item.hashCode) +
    (items.hashCode);

  @override
  String toString() => 'AppConfigState[resource=$resource, kind=$kind, idField=$idField, item=$item, items=$items]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'resource'] = this.resource;
      json[r'kind'] = this.kind;
    if (this.idField != null) {
      json[r'id_field'] = this.idField;
    } else {
      json[r'id_field'] = null;
    }
      json[r'item'] = this.item;
      json[r'items'] = this.items;
    return json;
  }

  /// Returns a new [AppConfigState] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppConfigState? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'resource'), 'Required key "AppConfigState[resource]" is missing from JSON.');
        assert(json[r'resource'] != null, 'Required key "AppConfigState[resource]" has a null value in JSON.');
        assert(json.containsKey(r'kind'), 'Required key "AppConfigState[kind]" is missing from JSON.');
        assert(json[r'kind'] != null, 'Required key "AppConfigState[kind]" has a null value in JSON.');
        return true;
      }());

      return AppConfigState(
        resource: mapValueOfType<String>(json, r'resource')!,
        kind: mapValueOfType<String>(json, r'kind')!,
        idField: mapValueOfType<String>(json, r'id_field'),
        item: mapCastOfType<String, Object>(json, r'item') ?? const {},
        items: json[r'items'] is Iterable
            ? (json[r'items'] as Iterable).cast<Object>().toList(growable: false)
            : const [],
      );
    }
    return null;
  }

  static List<AppConfigState> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppConfigState>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppConfigState.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppConfigState> mapFromJson(dynamic json) {
    final map = <String, AppConfigState>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppConfigState.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppConfigState-objects as value to a dart map
  static Map<String, List<AppConfigState>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppConfigState>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppConfigState.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'resource',
    'kind',
  };
}

