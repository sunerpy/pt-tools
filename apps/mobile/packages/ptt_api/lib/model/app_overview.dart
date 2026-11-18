//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppOverview {
  /// Returns a new [AppOverview] instance.
  AppOverview({
    required this.totals,
    required this.today,
    required this.updatedAt,
  });

  AppTotals totals;

  AppDelta today;

  int updatedAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppOverview &&
    other.totals == totals &&
    other.today == today &&
    other.updatedAt == updatedAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (totals.hashCode) +
    (today.hashCode) +
    (updatedAt.hashCode);

  @override
  String toString() => 'AppOverview[totals=$totals, today=$today, updatedAt=$updatedAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'totals'] = this.totals;
      json[r'today'] = this.today;
      json[r'updated_at'] = this.updatedAt;
    return json;
  }

  /// Returns a new [AppOverview] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppOverview? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'totals'), 'Required key "AppOverview[totals]" is missing from JSON.');
        assert(json[r'totals'] != null, 'Required key "AppOverview[totals]" has a null value in JSON.');
        assert(json.containsKey(r'today'), 'Required key "AppOverview[today]" is missing from JSON.');
        assert(json[r'today'] != null, 'Required key "AppOverview[today]" has a null value in JSON.');
        assert(json.containsKey(r'updated_at'), 'Required key "AppOverview[updated_at]" is missing from JSON.');
        assert(json[r'updated_at'] != null, 'Required key "AppOverview[updated_at]" has a null value in JSON.');
        return true;
      }());

      return AppOverview(
        totals: AppTotals.fromJson(json[r'totals'])!,
        today: AppDelta.fromJson(json[r'today'])!,
        updatedAt: mapValueOfType<int>(json, r'updated_at')!,
      );
    }
    return null;
  }

  static List<AppOverview> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppOverview>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppOverview.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppOverview> mapFromJson(dynamic json) {
    final map = <String, AppOverview>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppOverview.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppOverview-objects as value to a dart map
  static Map<String, List<AppOverview>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppOverview>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppOverview.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'totals',
    'today',
    'updated_at',
  };
}

