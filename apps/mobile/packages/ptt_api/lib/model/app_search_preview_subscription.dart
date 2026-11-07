//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewSubscription {
  /// Returns a new [AppSearchPreviewSubscription] instance.
  AppSearchPreviewSubscription({
    required this.id,
    required this.status,
    this.lock,
  });

  int id;

  String status;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppSearchPreviewSubscriptionLock? lock;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewSubscription &&
    other.id == id &&
    other.status == status &&
    other.lock == lock;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (status.hashCode) +
    (lock == null ? 0 : lock!.hashCode);

  @override
  String toString() => 'AppSearchPreviewSubscription[id=$id, status=$status, lock=$lock]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'status'] = this.status;
    if (this.lock != null) {
      json[r'lock'] = this.lock;
    } else {
      json[r'lock'] = null;
    }
    return json;
  }

  /// Returns a new [AppSearchPreviewSubscription] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewSubscription? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppSearchPreviewSubscription[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppSearchPreviewSubscription[id]" has a null value in JSON.');
        assert(json.containsKey(r'status'), 'Required key "AppSearchPreviewSubscription[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppSearchPreviewSubscription[status]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewSubscription(
        id: mapValueOfType<int>(json, r'id')!,
        status: mapValueOfType<String>(json, r'status')!,
        lock: AppSearchPreviewSubscriptionLock.fromJson(json[r'lock']),
      );
    }
    return null;
  }

  static List<AppSearchPreviewSubscription> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewSubscription>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewSubscription.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewSubscription> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewSubscription>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewSubscription.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewSubscription-objects as value to a dart map
  static Map<String, List<AppSearchPreviewSubscription>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewSubscription>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewSubscription.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'status',
  };
}

