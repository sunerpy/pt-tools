//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewSubscriptionLock {
  /// Returns a new [AppSearchPreviewSubscriptionLock] instance.
  AppSearchPreviewSubscriptionLock({
    required this.group,
    required this.resolution,
  });

  /// 制作组，认不出时为空
  String group;

  /// 分辨率，认不出时为空
  String resolution;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewSubscriptionLock &&
    other.group == group &&
    other.resolution == resolution;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (group.hashCode) +
    (resolution.hashCode);

  @override
  String toString() => 'AppSearchPreviewSubscriptionLock[group=$group, resolution=$resolution]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'group'] = this.group;
      json[r'resolution'] = this.resolution;
    return json;
  }

  /// Returns a new [AppSearchPreviewSubscriptionLock] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewSubscriptionLock? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'group'), 'Required key "AppSearchPreviewSubscriptionLock[group]" is missing from JSON.');
        assert(json[r'group'] != null, 'Required key "AppSearchPreviewSubscriptionLock[group]" has a null value in JSON.');
        assert(json.containsKey(r'resolution'), 'Required key "AppSearchPreviewSubscriptionLock[resolution]" is missing from JSON.');
        assert(json[r'resolution'] != null, 'Required key "AppSearchPreviewSubscriptionLock[resolution]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewSubscriptionLock(
        group: mapValueOfType<String>(json, r'group')!,
        resolution: mapValueOfType<String>(json, r'resolution')!,
      );
    }
    return null;
  }

  static List<AppSearchPreviewSubscriptionLock> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewSubscriptionLock>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewSubscriptionLock.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewSubscriptionLock> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewSubscriptionLock>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewSubscriptionLock.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewSubscriptionLock-objects as value to a dart map
  static Map<String, List<AppSearchPreviewSubscriptionLock>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewSubscriptionLock>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewSubscriptionLock.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'group',
    'resolution',
  };
}

