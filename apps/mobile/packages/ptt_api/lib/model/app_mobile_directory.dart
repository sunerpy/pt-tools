//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMobileDirectory {
  /// Returns a new [AppMobileDirectory] instance.
  AppMobileDirectory({
    required this.downloaderId,
    required this.path,
    this.alias,
    this.isDefault,
  });

  int downloaderId;

  /// 下载器里看到的目录
  String path;

  /// 显示用的名字
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? alias;

  /// 这个下载器的默认目录
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? isDefault;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMobileDirectory &&
    other.downloaderId == downloaderId &&
    other.path == path &&
    other.alias == alias &&
    other.isDefault == isDefault;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (downloaderId.hashCode) +
    (path.hashCode) +
    (alias == null ? 0 : alias!.hashCode) +
    (isDefault == null ? 0 : isDefault!.hashCode);

  @override
  String toString() => 'AppMobileDirectory[downloaderId=$downloaderId, path=$path, alias=$alias, isDefault=$isDefault]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'downloader_id'] = this.downloaderId;
      json[r'path'] = this.path;
    if (this.alias != null) {
      json[r'alias'] = this.alias;
    } else {
      json[r'alias'] = null;
    }
    if (this.isDefault != null) {
      json[r'is_default'] = this.isDefault;
    } else {
      json[r'is_default'] = null;
    }
    return json;
  }

  /// Returns a new [AppMobileDirectory] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMobileDirectory? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'downloader_id'), 'Required key "AppMobileDirectory[downloader_id]" is missing from JSON.');
        assert(json[r'downloader_id'] != null, 'Required key "AppMobileDirectory[downloader_id]" has a null value in JSON.');
        assert(json.containsKey(r'path'), 'Required key "AppMobileDirectory[path]" is missing from JSON.');
        assert(json[r'path'] != null, 'Required key "AppMobileDirectory[path]" has a null value in JSON.');
        return true;
      }());

      return AppMobileDirectory(
        downloaderId: mapValueOfType<int>(json, r'downloader_id')!,
        path: mapValueOfType<String>(json, r'path')!,
        alias: mapValueOfType<String>(json, r'alias'),
        isDefault: mapValueOfType<bool>(json, r'is_default'),
      );
    }
    return null;
  }

  static List<AppMobileDirectory> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMobileDirectory>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMobileDirectory.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMobileDirectory> mapFromJson(dynamic json) {
    final map = <String, AppMobileDirectory>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMobileDirectory.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMobileDirectory-objects as value to a dart map
  static Map<String, List<AppMobileDirectory>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMobileDirectory>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMobileDirectory.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'downloader_id',
    'path',
  };
}

