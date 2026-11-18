//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppEpisodeRuleInput {
  /// Returns a new [AppEpisodeRuleInput] instance.
  AppEpisodeRuleInput({
    this.group,
    required this.mode,
    this.offset,
    this.note,
  });

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? group;

  AppEpisodeRuleInputModeEnum mode;

  /// mode 是 offset 时用，不能是 0
  ///
  /// Minimum value: -2000
  /// Maximum value: 2000
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? offset;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? note;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppEpisodeRuleInput &&
    other.group == group &&
    other.mode == mode &&
    other.offset == offset &&
    other.note == note;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (group == null ? 0 : group!.hashCode) +
    (mode.hashCode) +
    (offset == null ? 0 : offset!.hashCode) +
    (note == null ? 0 : note!.hashCode);

  @override
  String toString() => 'AppEpisodeRuleInput[group=$group, mode=$mode, offset=$offset, note=$note]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
    if (this.group != null) {
      json[r'group'] = this.group;
    } else {
      json[r'group'] = null;
    }
      json[r'mode'] = this.mode;
    if (this.offset != null) {
      json[r'offset'] = this.offset;
    } else {
      json[r'offset'] = null;
    }
    if (this.note != null) {
      json[r'note'] = this.note;
    } else {
      json[r'note'] = null;
    }
    return json;
  }

  /// Returns a new [AppEpisodeRuleInput] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppEpisodeRuleInput? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'mode'), 'Required key "AppEpisodeRuleInput[mode]" is missing from JSON.');
        assert(json[r'mode'] != null, 'Required key "AppEpisodeRuleInput[mode]" has a null value in JSON.');
        return true;
      }());

      return AppEpisodeRuleInput(
        group: mapValueOfType<String>(json, r'group'),
        mode: AppEpisodeRuleInputModeEnum.fromJson(json[r'mode'])!,
        offset: mapValueOfType<int>(json, r'offset'),
        note: mapValueOfType<String>(json, r'note'),
      );
    }
    return null;
  }

  static List<AppEpisodeRuleInput> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppEpisodeRuleInput>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppEpisodeRuleInput.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppEpisodeRuleInput> mapFromJson(dynamic json) {
    final map = <String, AppEpisodeRuleInput>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppEpisodeRuleInput.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppEpisodeRuleInput-objects as value to a dart map
  static Map<String, List<AppEpisodeRuleInput>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppEpisodeRuleInput>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppEpisodeRuleInput.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'mode',
  };
}


enum AppEpisodeRuleInputModeEnum {
  tmdb._(r'tmdb'),
  periodPart._(r'period_part'),
  date._(r'date'),
  offset._(r'offset'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppEpisodeRuleInputModeEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppEpisodeRuleInputModeEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppEpisodeRuleInputModeEnum? fromJson(dynamic value) => AppEpisodeRuleInputModeEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppEpisodeRuleInputModeEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppEpisodeRuleInputModeEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppEpisodeRuleInputModeEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppEpisodeRuleInputModeEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppEpisodeRuleInputModeEnum] to String,
/// and [decode] dynamic data back to [AppEpisodeRuleInputModeEnum].
class AppEpisodeRuleInputModeEnumTypeTransformer {
  factory AppEpisodeRuleInputModeEnumTypeTransformer() => _instance ??= const AppEpisodeRuleInputModeEnumTypeTransformer._();

  const AppEpisodeRuleInputModeEnumTypeTransformer._();

  String encode(AppEpisodeRuleInputModeEnum data) => data._value;

  /// Returns the instance of [AppEpisodeRuleInputModeEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppEpisodeRuleInputModeEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppEpisodeRuleInputModeEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'tmdb': return AppEpisodeRuleInputModeEnum.tmdb;
        case r'period_part': return AppEpisodeRuleInputModeEnum.periodPart;
        case r'date': return AppEpisodeRuleInputModeEnum.date;
        case r'offset': return AppEpisodeRuleInputModeEnum.offset;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppEpisodeRuleInputModeEnumTypeTransformer? _instance;
}


