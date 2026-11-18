//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSubscriptionStatus {
  /// Returns a new [AppSubscriptionStatus] instance.
  AppSubscriptionStatus({
    required this.status,
  });

  AppSubscriptionStatusStatusEnum status;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSubscriptionStatus &&
    other.status == status;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (status.hashCode);

  @override
  String toString() => 'AppSubscriptionStatus[status=$status]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'status'] = this.status;
    return json;
  }

  /// Returns a new [AppSubscriptionStatus] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSubscriptionStatus? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'status'), 'Required key "AppSubscriptionStatus[status]" is missing from JSON.');
        assert(json[r'status'] != null, 'Required key "AppSubscriptionStatus[status]" has a null value in JSON.');
        return true;
      }());

      return AppSubscriptionStatus(
        status: AppSubscriptionStatusStatusEnum.fromJson(json[r'status'])!,
      );
    }
    return null;
  }

  static List<AppSubscriptionStatus> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionStatus>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionStatus.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSubscriptionStatus> mapFromJson(dynamic json) {
    final map = <String, AppSubscriptionStatus>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSubscriptionStatus.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSubscriptionStatus-objects as value to a dart map
  static Map<String, List<AppSubscriptionStatus>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSubscriptionStatus>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSubscriptionStatus.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'status',
  };
}


enum AppSubscriptionStatusStatusEnum {
  active._(r'active'),
  paused._(r'paused'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppSubscriptionStatusStatusEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppSubscriptionStatusStatusEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppSubscriptionStatusStatusEnum? fromJson(dynamic value) => AppSubscriptionStatusStatusEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppSubscriptionStatusStatusEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppSubscriptionStatusStatusEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSubscriptionStatusStatusEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSubscriptionStatusStatusEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppSubscriptionStatusStatusEnum] to String,
/// and [decode] dynamic data back to [AppSubscriptionStatusStatusEnum].
class AppSubscriptionStatusStatusEnumTypeTransformer {
  factory AppSubscriptionStatusStatusEnumTypeTransformer() => _instance ??= const AppSubscriptionStatusStatusEnumTypeTransformer._();

  const AppSubscriptionStatusStatusEnumTypeTransformer._();

  String encode(AppSubscriptionStatusStatusEnum data) => data._value;

  /// Returns the instance of [AppSubscriptionStatusStatusEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppSubscriptionStatusStatusEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppSubscriptionStatusStatusEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'active': return AppSubscriptionStatusStatusEnum.active;
        case r'paused': return AppSubscriptionStatusStatusEnum.paused;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppSubscriptionStatusStatusEnumTypeTransformer? _instance;
}


