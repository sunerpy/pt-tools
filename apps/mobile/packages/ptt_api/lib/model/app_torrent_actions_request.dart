//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTorrentActionsRequest {
  /// Returns a new [AppTorrentActionsRequest] instance.
  AppTorrentActionsRequest({
    required this.action,
    this.targets = const [],
  });

  AppTorrentActionsRequestActionEnum action;

  List<AppTorrentTarget> targets;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTorrentActionsRequest &&
    other.action == action &&
    _deepEquality.equals(other.targets, targets);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (action.hashCode) +
    (targets.hashCode);

  @override
  String toString() => 'AppTorrentActionsRequest[action=$action, targets=$targets]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'action'] = this.action;
      json[r'targets'] = this.targets;
    return json;
  }

  /// Returns a new [AppTorrentActionsRequest] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTorrentActionsRequest? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'action'), 'Required key "AppTorrentActionsRequest[action]" is missing from JSON.');
        assert(json[r'action'] != null, 'Required key "AppTorrentActionsRequest[action]" has a null value in JSON.');
        assert(json.containsKey(r'targets'), 'Required key "AppTorrentActionsRequest[targets]" is missing from JSON.');
        assert(json[r'targets'] != null, 'Required key "AppTorrentActionsRequest[targets]" has a null value in JSON.');
        return true;
      }());

      return AppTorrentActionsRequest(
        action: AppTorrentActionsRequestActionEnum.fromJson(json[r'action'])!,
        targets: AppTorrentTarget.listFromJson(json[r'targets']),
      );
    }
    return null;
  }

  static List<AppTorrentActionsRequest> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTorrentActionsRequest>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTorrentActionsRequest.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTorrentActionsRequest> mapFromJson(dynamic json) {
    final map = <String, AppTorrentActionsRequest>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTorrentActionsRequest.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTorrentActionsRequest-objects as value to a dart map
  static Map<String, List<AppTorrentActionsRequest>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTorrentActionsRequest>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTorrentActionsRequest.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'action',
    'targets',
  };
}


enum AppTorrentActionsRequestActionEnum {
  pause._(r'pause'),
  resume._(r'resume'),
  delete._(r'delete'),
  deleteWithFiles._(r'delete_with_files'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppTorrentActionsRequestActionEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppTorrentActionsRequestActionEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppTorrentActionsRequestActionEnum? fromJson(dynamic value) => AppTorrentActionsRequestActionEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppTorrentActionsRequestActionEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppTorrentActionsRequestActionEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTorrentActionsRequestActionEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTorrentActionsRequestActionEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppTorrentActionsRequestActionEnum] to String,
/// and [decode] dynamic data back to [AppTorrentActionsRequestActionEnum].
class AppTorrentActionsRequestActionEnumTypeTransformer {
  factory AppTorrentActionsRequestActionEnumTypeTransformer() => _instance ??= const AppTorrentActionsRequestActionEnumTypeTransformer._();

  const AppTorrentActionsRequestActionEnumTypeTransformer._();

  String encode(AppTorrentActionsRequestActionEnum data) => data._value;

  /// Returns the instance of [AppTorrentActionsRequestActionEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppTorrentActionsRequestActionEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppTorrentActionsRequestActionEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'pause': return AppTorrentActionsRequestActionEnum.pause;
        case r'resume': return AppTorrentActionsRequestActionEnum.resume;
        case r'delete': return AppTorrentActionsRequestActionEnum.delete;
        case r'delete_with_files': return AppTorrentActionsRequestActionEnum.deleteWithFiles;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppTorrentActionsRequestActionEnumTypeTransformer? _instance;
}


