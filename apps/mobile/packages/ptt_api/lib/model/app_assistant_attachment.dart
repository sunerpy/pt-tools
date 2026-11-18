//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantAttachment {
  /// Returns a new [AppAssistantAttachment] instance.
  AppAssistantAttachment({
    required this.id,
    required this.name,
    required this.kind,
    required this.mime,
    required this.size,
  });

  int id;

  /// 文件名
  String name;

  AppAssistantAttachmentKindEnum kind;

  /// 图片是 image/png 这类；文本是 text/plain; charset=utf-8
  String mime;

  /// 上传的文件大小（字节）
  int size;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantAttachment &&
    other.id == id &&
    other.name == name &&
    other.kind == kind &&
    other.mime == mime &&
    other.size == size;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (name.hashCode) +
    (kind.hashCode) +
    (mime.hashCode) +
    (size.hashCode);

  @override
  String toString() => 'AppAssistantAttachment[id=$id, name=$name, kind=$kind, mime=$mime, size=$size]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'name'] = this.name;
      json[r'kind'] = this.kind;
      json[r'mime'] = this.mime;
      json[r'size'] = this.size;
    return json;
  }

  /// Returns a new [AppAssistantAttachment] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantAttachment? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppAssistantAttachment[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppAssistantAttachment[id]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppAssistantAttachment[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppAssistantAttachment[name]" has a null value in JSON.');
        assert(json.containsKey(r'kind'), 'Required key "AppAssistantAttachment[kind]" is missing from JSON.');
        assert(json[r'kind'] != null, 'Required key "AppAssistantAttachment[kind]" has a null value in JSON.');
        assert(json.containsKey(r'mime'), 'Required key "AppAssistantAttachment[mime]" is missing from JSON.');
        assert(json[r'mime'] != null, 'Required key "AppAssistantAttachment[mime]" has a null value in JSON.');
        assert(json.containsKey(r'size'), 'Required key "AppAssistantAttachment[size]" is missing from JSON.');
        assert(json[r'size'] != null, 'Required key "AppAssistantAttachment[size]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantAttachment(
        id: mapValueOfType<int>(json, r'id')!,
        name: mapValueOfType<String>(json, r'name')!,
        kind: AppAssistantAttachmentKindEnum.fromJson(json[r'kind'])!,
        mime: mapValueOfType<String>(json, r'mime')!,
        size: mapValueOfType<int>(json, r'size')!,
      );
    }
    return null;
  }

  static List<AppAssistantAttachment> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantAttachment>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantAttachment.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantAttachment> mapFromJson(dynamic json) {
    final map = <String, AppAssistantAttachment>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantAttachment.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantAttachment-objects as value to a dart map
  static Map<String, List<AppAssistantAttachment>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantAttachment>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantAttachment.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'name',
    'kind',
    'mime',
    'size',
  };
}


enum AppAssistantAttachmentKindEnum {
  image._(r'image'),
  text._(r'text'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppAssistantAttachmentKindEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppAssistantAttachmentKindEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppAssistantAttachmentKindEnum? fromJson(dynamic value) => AppAssistantAttachmentKindEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppAssistantAttachmentKindEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppAssistantAttachmentKindEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantAttachmentKindEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantAttachmentKindEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppAssistantAttachmentKindEnum] to String,
/// and [decode] dynamic data back to [AppAssistantAttachmentKindEnum].
class AppAssistantAttachmentKindEnumTypeTransformer {
  factory AppAssistantAttachmentKindEnumTypeTransformer() => _instance ??= const AppAssistantAttachmentKindEnumTypeTransformer._();

  const AppAssistantAttachmentKindEnumTypeTransformer._();

  String encode(AppAssistantAttachmentKindEnum data) => data._value;

  /// Returns the instance of [AppAssistantAttachmentKindEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppAssistantAttachmentKindEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppAssistantAttachmentKindEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'image': return AppAssistantAttachmentKindEnum.image;
        case r'text': return AppAssistantAttachmentKindEnum.text;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppAssistantAttachmentKindEnumTypeTransformer? _instance;
}


