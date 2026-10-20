//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppConfigResource {
  /// Returns a new [AppConfigResource] instance.
  AppConfigResource({
    required this.name,
    required this.title,
    required this.description,
    required this.kind,
    this.idField,
    this.ops = const [],
    this.fields = const [],
    this.required_ = const [],
    this.secrets = const [],
    this.hints = const {},
  });

  String name;

  String title;

  String description;

  /// object（一份设置，op 只能 set）或 list（一组，op 是 add、update、delete）
  String kind;

  /// 列表类的编号字段
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? idField;

  List<String> ops;

  /// 能改的字段（config.<键> 这样的写法表示以它开头的字段都能改）
  List<String> fields;

  /// 新增时必须给的字段
  List<String> required_;

  /// 密钥字段：只写不读
  List<String> secrets;

  /// 字段说明（字段 → 说明）
  Map<String, String> hints;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppConfigResource &&
    other.name == name &&
    other.title == title &&
    other.description == description &&
    other.kind == kind &&
    other.idField == idField &&
    _deepEquality.equals(other.ops, ops) &&
    _deepEquality.equals(other.fields, fields) &&
    _deepEquality.equals(other.required_, required_) &&
    _deepEquality.equals(other.secrets, secrets) &&
    _deepEquality.equals(other.hints, hints);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (name.hashCode) +
    (title.hashCode) +
    (description.hashCode) +
    (kind.hashCode) +
    (idField == null ? 0 : idField!.hashCode) +
    (ops.hashCode) +
    (fields.hashCode) +
    (required_.hashCode) +
    (secrets.hashCode) +
    (hints.hashCode);

  @override
  String toString() => 'AppConfigResource[name=$name, title=$title, description=$description, kind=$kind, idField=$idField, ops=$ops, fields=$fields, required_=$required_, secrets=$secrets, hints=$hints]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'name'] = this.name;
      json[r'title'] = this.title;
      json[r'description'] = this.description;
      json[r'kind'] = this.kind;
    if (this.idField != null) {
      json[r'id_field'] = this.idField;
    } else {
      json[r'id_field'] = null;
    }
      json[r'ops'] = this.ops;
      json[r'fields'] = this.fields;
      json[r'required'] = this.required_;
      json[r'secrets'] = this.secrets;
      json[r'hints'] = this.hints;
    return json;
  }

  /// Returns a new [AppConfigResource] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppConfigResource? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'name'), 'Required key "AppConfigResource[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppConfigResource[name]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppConfigResource[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppConfigResource[title]" has a null value in JSON.');
        assert(json.containsKey(r'description'), 'Required key "AppConfigResource[description]" is missing from JSON.');
        assert(json[r'description'] != null, 'Required key "AppConfigResource[description]" has a null value in JSON.');
        assert(json.containsKey(r'kind'), 'Required key "AppConfigResource[kind]" is missing from JSON.');
        assert(json[r'kind'] != null, 'Required key "AppConfigResource[kind]" has a null value in JSON.');
        assert(json.containsKey(r'ops'), 'Required key "AppConfigResource[ops]" is missing from JSON.');
        assert(json[r'ops'] != null, 'Required key "AppConfigResource[ops]" has a null value in JSON.');
        assert(json.containsKey(r'fields'), 'Required key "AppConfigResource[fields]" is missing from JSON.');
        assert(json[r'fields'] != null, 'Required key "AppConfigResource[fields]" has a null value in JSON.');
        return true;
      }());

      return AppConfigResource(
        name: mapValueOfType<String>(json, r'name')!,
        title: mapValueOfType<String>(json, r'title')!,
        description: mapValueOfType<String>(json, r'description')!,
        kind: mapValueOfType<String>(json, r'kind')!,
        idField: mapValueOfType<String>(json, r'id_field'),
        ops: json[r'ops'] is Iterable
            ? (json[r'ops'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        fields: json[r'fields'] is Iterable
            ? (json[r'fields'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        required_: json[r'required'] is Iterable
            ? (json[r'required'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        secrets: json[r'secrets'] is Iterable
            ? (json[r'secrets'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        hints: mapCastOfType<String, String>(json, r'hints') ?? const {},
      );
    }
    return null;
  }

  static List<AppConfigResource> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppConfigResource>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppConfigResource.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppConfigResource> mapFromJson(dynamic json) {
    final map = <String, AppConfigResource>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppConfigResource.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppConfigResource-objects as value to a dart map
  static Map<String, List<AppConfigResource>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppConfigResource>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppConfigResource.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'name',
    'title',
    'description',
    'kind',
    'ops',
    'fields',
  };
}

