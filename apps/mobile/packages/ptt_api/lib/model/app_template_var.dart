//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTemplateVar {
  /// Returns a new [AppTemplateVar] instance.
  AppTemplateVar({
    required this.key,
    required this.type,
    required this.desc,
    this.sample,
    this.item = const [],
  });

  String key;

  AppTemplateVarTypeEnum type;

  String desc;

  /// 示例值写成的文字；列表没有
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? sample;

  /// 列表每一项的字段
  List<AppTemplateVar> item;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTemplateVar &&
    other.key == key &&
    other.type == type &&
    other.desc == desc &&
    other.sample == sample &&
    _deepEquality.equals(other.item, item);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (key.hashCode) +
    (type.hashCode) +
    (desc.hashCode) +
    (sample == null ? 0 : sample!.hashCode) +
    (item.hashCode);

  @override
  String toString() => 'AppTemplateVar[key=$key, type=$type, desc=$desc, sample=$sample, item=$item]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'key'] = this.key;
      json[r'type'] = this.type;
      json[r'desc'] = this.desc;
    if (this.sample != null) {
      json[r'sample'] = this.sample;
    } else {
      json[r'sample'] = null;
    }
      json[r'item'] = this.item;
    return json;
  }

  /// Returns a new [AppTemplateVar] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTemplateVar? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'key'), 'Required key "AppTemplateVar[key]" is missing from JSON.');
        assert(json[r'key'] != null, 'Required key "AppTemplateVar[key]" has a null value in JSON.');
        assert(json.containsKey(r'type'), 'Required key "AppTemplateVar[type]" is missing from JSON.');
        assert(json[r'type'] != null, 'Required key "AppTemplateVar[type]" has a null value in JSON.');
        assert(json.containsKey(r'desc'), 'Required key "AppTemplateVar[desc]" is missing from JSON.');
        assert(json[r'desc'] != null, 'Required key "AppTemplateVar[desc]" has a null value in JSON.');
        return true;
      }());

      return AppTemplateVar(
        key: mapValueOfType<String>(json, r'key')!,
        type: AppTemplateVarTypeEnum.fromJson(json[r'type'])!,
        desc: mapValueOfType<String>(json, r'desc')!,
        sample: mapValueOfType<String>(json, r'sample'),
        item: AppTemplateVar.listFromJson(json[r'item']),
      );
    }
    return null;
  }

  static List<AppTemplateVar> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTemplateVar>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTemplateVar.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTemplateVar> mapFromJson(dynamic json) {
    final map = <String, AppTemplateVar>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTemplateVar.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTemplateVar-objects as value to a dart map
  static Map<String, List<AppTemplateVar>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTemplateVar>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTemplateVar.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'key',
    'type',
    'desc',
  };
}


enum AppTemplateVarTypeEnum {
  string._(r'string'),
  int_._(r'int'),
  float._(r'float'),
  bool_._(r'bool'),
  list._(r'list'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppTemplateVarTypeEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppTemplateVarTypeEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppTemplateVarTypeEnum? fromJson(dynamic value) => AppTemplateVarTypeEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppTemplateVarTypeEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppTemplateVarTypeEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTemplateVarTypeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTemplateVarTypeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppTemplateVarTypeEnum] to String,
/// and [decode] dynamic data back to [AppTemplateVarTypeEnum].
class AppTemplateVarTypeEnumTypeTransformer {
  factory AppTemplateVarTypeEnumTypeTransformer() => _instance ??= const AppTemplateVarTypeEnumTypeTransformer._();

  const AppTemplateVarTypeEnumTypeTransformer._();

  String encode(AppTemplateVarTypeEnum data) => data._value;

  /// Returns the instance of [AppTemplateVarTypeEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppTemplateVarTypeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppTemplateVarTypeEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'string': return AppTemplateVarTypeEnum.string;
        case r'int': return AppTemplateVarTypeEnum.int_;
        case r'float': return AppTemplateVarTypeEnum.float;
        case r'bool': return AppTemplateVarTypeEnum.bool_;
        case r'list': return AppTemplateVarTypeEnum.list;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppTemplateVarTypeEnumTypeTransformer? _instance;
}


