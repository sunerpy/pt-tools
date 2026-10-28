//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppStatsAggregated {
  /// Returns a new [AppStatsAggregated] instance.
  AppStatsAggregated({
    required this.totalUploaded,
    required this.totalDownloaded,
    this.averageRatio,
    this.totalSeeding,
    this.totalLeeching,
    this.totalBonus,
    this.siteCount,
    this.lastUpdate,
    this.perSiteStats = const [],
  });

  int totalUploaded;

  int totalDownloaded;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  num? averageRatio;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? totalSeeding;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? totalLeeching;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  num? totalBonus;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? siteCount;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? lastUpdate;

  List<AppStatsSite> perSiteStats;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppStatsAggregated &&
    other.totalUploaded == totalUploaded &&
    other.totalDownloaded == totalDownloaded &&
    other.averageRatio == averageRatio &&
    other.totalSeeding == totalSeeding &&
    other.totalLeeching == totalLeeching &&
    other.totalBonus == totalBonus &&
    other.siteCount == siteCount &&
    other.lastUpdate == lastUpdate &&
    _deepEquality.equals(other.perSiteStats, perSiteStats);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (totalUploaded.hashCode) +
    (totalDownloaded.hashCode) +
    (averageRatio == null ? 0 : averageRatio!.hashCode) +
    (totalSeeding == null ? 0 : totalSeeding!.hashCode) +
    (totalLeeching == null ? 0 : totalLeeching!.hashCode) +
    (totalBonus == null ? 0 : totalBonus!.hashCode) +
    (siteCount == null ? 0 : siteCount!.hashCode) +
    (lastUpdate == null ? 0 : lastUpdate!.hashCode) +
    (perSiteStats.hashCode);

  @override
  String toString() => 'AppStatsAggregated[totalUploaded=$totalUploaded, totalDownloaded=$totalDownloaded, averageRatio=$averageRatio, totalSeeding=$totalSeeding, totalLeeching=$totalLeeching, totalBonus=$totalBonus, siteCount=$siteCount, lastUpdate=$lastUpdate, perSiteStats=$perSiteStats]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'totalUploaded'] = this.totalUploaded;
      json[r'totalDownloaded'] = this.totalDownloaded;
    if (this.averageRatio != null) {
      json[r'averageRatio'] = this.averageRatio;
    } else {
      json[r'averageRatio'] = null;
    }
    if (this.totalSeeding != null) {
      json[r'totalSeeding'] = this.totalSeeding;
    } else {
      json[r'totalSeeding'] = null;
    }
    if (this.totalLeeching != null) {
      json[r'totalLeeching'] = this.totalLeeching;
    } else {
      json[r'totalLeeching'] = null;
    }
    if (this.totalBonus != null) {
      json[r'totalBonus'] = this.totalBonus;
    } else {
      json[r'totalBonus'] = null;
    }
    if (this.siteCount != null) {
      json[r'siteCount'] = this.siteCount;
    } else {
      json[r'siteCount'] = null;
    }
    if (this.lastUpdate != null) {
      json[r'lastUpdate'] = this.lastUpdate;
    } else {
      json[r'lastUpdate'] = null;
    }
      json[r'perSiteStats'] = this.perSiteStats;
    return json;
  }

  /// Returns a new [AppStatsAggregated] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppStatsAggregated? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'totalUploaded'), 'Required key "AppStatsAggregated[totalUploaded]" is missing from JSON.');
        assert(json[r'totalUploaded'] != null, 'Required key "AppStatsAggregated[totalUploaded]" has a null value in JSON.');
        assert(json.containsKey(r'totalDownloaded'), 'Required key "AppStatsAggregated[totalDownloaded]" is missing from JSON.');
        assert(json[r'totalDownloaded'] != null, 'Required key "AppStatsAggregated[totalDownloaded]" has a null value in JSON.');
        assert(json.containsKey(r'perSiteStats'), 'Required key "AppStatsAggregated[perSiteStats]" is missing from JSON.');
        assert(json[r'perSiteStats'] != null, 'Required key "AppStatsAggregated[perSiteStats]" has a null value in JSON.');
        return true;
      }());

      return AppStatsAggregated(
        totalUploaded: mapValueOfType<int>(json, r'totalUploaded')!,
        totalDownloaded: mapValueOfType<int>(json, r'totalDownloaded')!,
        averageRatio: json[r'averageRatio'] == null
            ? null
            : num.parse('${json[r'averageRatio']}'),
        totalSeeding: mapValueOfType<int>(json, r'totalSeeding'),
        totalLeeching: mapValueOfType<int>(json, r'totalLeeching'),
        totalBonus: json[r'totalBonus'] == null
            ? null
            : num.parse('${json[r'totalBonus']}'),
        siteCount: mapValueOfType<int>(json, r'siteCount'),
        lastUpdate: mapValueOfType<int>(json, r'lastUpdate'),
        perSiteStats: AppStatsSite.listFromJson(json[r'perSiteStats']),
      );
    }
    return null;
  }

  static List<AppStatsAggregated> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppStatsAggregated>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppStatsAggregated.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppStatsAggregated> mapFromJson(dynamic json) {
    final map = <String, AppStatsAggregated>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppStatsAggregated.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppStatsAggregated-objects as value to a dart map
  static Map<String, List<AppStatsAggregated>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppStatsAggregated>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppStatsAggregated.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'totalUploaded',
    'totalDownloaded',
    'perSiteStats',
  };
}

