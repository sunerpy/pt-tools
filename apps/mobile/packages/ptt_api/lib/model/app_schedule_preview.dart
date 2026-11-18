//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSchedulePreview {
  /// Returns a new [AppSchedulePreview] instance.
  AppSchedulePreview({
    this.next = const [],
    this.note,
  });

  /// 接下来几次搜索的时间（Unix 秒，不算随机偏移；两次之间至少隔 30 分钟）
  List<int> next;

  /// 按播出时间的不给时间，这里说明怎么算
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? note;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSchedulePreview &&
    _deepEquality.equals(other.next, next) &&
    other.note == note;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (next.hashCode) +
    (note == null ? 0 : note!.hashCode);

  @override
  String toString() => 'AppSchedulePreview[next=$next, note=$note]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'next'] = this.next;
    if (this.note != null) {
      json[r'note'] = this.note;
    } else {
      json[r'note'] = null;
    }
    return json;
  }

  /// Returns a new [AppSchedulePreview] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSchedulePreview? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'next'), 'Required key "AppSchedulePreview[next]" is missing from JSON.');
        assert(json[r'next'] != null, 'Required key "AppSchedulePreview[next]" has a null value in JSON.');
        return true;
      }());

      return AppSchedulePreview(
        next: json[r'next'] is Iterable
            ? (json[r'next'] as Iterable).cast<int>().toList(growable: false)
            : const [],
        note: mapValueOfType<String>(json, r'note'),
      );
    }
    return null;
  }

  static List<AppSchedulePreview> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSchedulePreview>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSchedulePreview.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSchedulePreview> mapFromJson(dynamic json) {
    final map = <String, AppSchedulePreview>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSchedulePreview.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSchedulePreview-objects as value to a dart map
  static Map<String, List<AppSchedulePreview>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSchedulePreview>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSchedulePreview.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'next',
  };
}

