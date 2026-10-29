//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppBrushStats {
  /// Returns a new [AppBrushStats] instance.
  AppBrushStats({
    required this.granularity,
    required this.periods,
    required this.from,
    required this.to,
    this.totals = const [],
    this.tasks = const [],
  });

  AppBrushStatsGranularityEnum granularity;

  int periods;

  String from;

  String to;

  List<AppBrushPeriod> totals;

  List<AppBrushTaskPeriods> tasks;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppBrushStats &&
    other.granularity == granularity &&
    other.periods == periods &&
    other.from == from &&
    other.to == to &&
    _deepEquality.equals(other.totals, totals) &&
    _deepEquality.equals(other.tasks, tasks);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (granularity.hashCode) +
    (periods.hashCode) +
    (from.hashCode) +
    (to.hashCode) +
    (totals.hashCode) +
    (tasks.hashCode);

  @override
  String toString() => 'AppBrushStats[granularity=$granularity, periods=$periods, from=$from, to=$to, totals=$totals, tasks=$tasks]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'granularity'] = this.granularity;
      json[r'periods'] = this.periods;
      json[r'from'] = this.from;
      json[r'to'] = this.to;
      json[r'totals'] = this.totals;
      json[r'tasks'] = this.tasks;
    return json;
  }

  /// Returns a new [AppBrushStats] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppBrushStats? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'granularity'), 'Required key "AppBrushStats[granularity]" is missing from JSON.');
        assert(json[r'granularity'] != null, 'Required key "AppBrushStats[granularity]" has a null value in JSON.');
        assert(json.containsKey(r'periods'), 'Required key "AppBrushStats[periods]" is missing from JSON.');
        assert(json[r'periods'] != null, 'Required key "AppBrushStats[periods]" has a null value in JSON.');
        assert(json.containsKey(r'from'), 'Required key "AppBrushStats[from]" is missing from JSON.');
        assert(json[r'from'] != null, 'Required key "AppBrushStats[from]" has a null value in JSON.');
        assert(json.containsKey(r'to'), 'Required key "AppBrushStats[to]" is missing from JSON.');
        assert(json[r'to'] != null, 'Required key "AppBrushStats[to]" has a null value in JSON.');
        assert(json.containsKey(r'totals'), 'Required key "AppBrushStats[totals]" is missing from JSON.');
        assert(json[r'totals'] != null, 'Required key "AppBrushStats[totals]" has a null value in JSON.');
        assert(json.containsKey(r'tasks'), 'Required key "AppBrushStats[tasks]" is missing from JSON.');
        assert(json[r'tasks'] != null, 'Required key "AppBrushStats[tasks]" has a null value in JSON.');
        return true;
      }());

      return AppBrushStats(
        granularity: AppBrushStatsGranularityEnum.fromJson(json[r'granularity'])!,
        periods: mapValueOfType<int>(json, r'periods')!,
        from: mapValueOfType<String>(json, r'from')!,
        to: mapValueOfType<String>(json, r'to')!,
        totals: AppBrushPeriod.listFromJson(json[r'totals']),
        tasks: AppBrushTaskPeriods.listFromJson(json[r'tasks']),
      );
    }
    return null;
  }

  static List<AppBrushStats> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppBrushStats>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppBrushStats.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppBrushStats> mapFromJson(dynamic json) {
    final map = <String, AppBrushStats>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppBrushStats.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppBrushStats-objects as value to a dart map
  static Map<String, List<AppBrushStats>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppBrushStats>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppBrushStats.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'granularity',
    'periods',
    'from',
    'to',
    'totals',
    'tasks',
  };
}


enum AppBrushStatsGranularityEnum {
  day._(r'day'),
  week._(r'week'),
  month._(r'month'),
  quarter._(r'quarter'),
  year._(r'year'),
  ;

  /// Instantiate a new enum with the provided value.
  const AppBrushStatsGranularityEnum._(this._value);

  /// The underlying value of this enum member.
  final String _value;

  @override
  String toString() => _value;

  /// Encodes this enum as a value suitable for JSON.
  String toJson() => _value;

  /// Returns the instance of [AppBrushStatsGranularityEnum] that was successfully decoded
  /// from the passed [value] on success, null otherwise.
  static AppBrushStatsGranularityEnum? fromJson(dynamic value) => AppBrushStatsGranularityEnumTypeTransformer().decode(value);

  /// Returns a [List] containing instances of [AppBrushStatsGranularityEnum]
  /// that were successfully decoded from the passed [JSON][json].
  static List<AppBrushStatsGranularityEnum> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppBrushStatsGranularityEnum>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppBrushStatsGranularityEnum.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }
}

/// Transformation class that can [encode] an instance of [AppBrushStatsGranularityEnum] to String,
/// and [decode] dynamic data back to [AppBrushStatsGranularityEnum].
class AppBrushStatsGranularityEnumTypeTransformer {
  factory AppBrushStatsGranularityEnumTypeTransformer() => _instance ??= const AppBrushStatsGranularityEnumTypeTransformer._();

  const AppBrushStatsGranularityEnumTypeTransformer._();

  String encode(AppBrushStatsGranularityEnum data) => data._value;

  /// Returns the instance of [AppBrushStatsGranularityEnum] that was successfully decoded
  /// from the passed [data] value on success, null otherwise.
  ///
  /// If [allowNull] is true and the [dynamic value][data] cannot be decoded successfully,
  /// then null is returned. However, if [allowNull] is false and the [dynamic value][data]
  /// cannot be decoded successfully, then an [UnimplementedError] is thrown.
  ///
  /// The [allowNull] is very handy when an API changes and a new enum value is added or removed,
  /// and users are still using an old app with the old code.
  AppBrushStatsGranularityEnum? decode(dynamic data, {bool allowNull = true}) {
    if (data is AppBrushStatsGranularityEnum) {
      return data;
    }
    if (data != null) {
      switch (data) {
        case r'day': return AppBrushStatsGranularityEnum.day;
        case r'week': return AppBrushStatsGranularityEnum.week;
        case r'month': return AppBrushStatsGranularityEnum.month;
        case r'quarter': return AppBrushStatsGranularityEnum.quarter;
        case r'year': return AppBrushStatsGranularityEnum.year;
        default:
          if (!allowNull) {
            throw ArgumentError('Unknown enum value to decode: $data');
          }
      }
    }
    return null;
  }

  /// The singleton instance of this transformer.
  static AppBrushStatsGranularityEnumTypeTransformer? _instance;
}


