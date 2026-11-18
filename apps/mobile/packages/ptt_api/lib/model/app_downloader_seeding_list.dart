//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppDownloaderSeedingList {
  /// Returns a new [AppDownloaderSeedingList] instance.
  AppDownloaderSeedingList({
    required this.totalSeedingCount,
    required this.totalSeedingSize,
    this.items = const [],
  });

  /// 读到了的几台正在做种的种子数合计
  int totalSeedingCount;

  /// 读到了的几台正在做种的体积合计（字节）
  int totalSeedingSize;

  List<AppDownloaderSeeding> items;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppDownloaderSeedingList &&
    other.totalSeedingCount == totalSeedingCount &&
    other.totalSeedingSize == totalSeedingSize &&
    _deepEquality.equals(other.items, items);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (totalSeedingCount.hashCode) +
    (totalSeedingSize.hashCode) +
    (items.hashCode);

  @override
  String toString() => 'AppDownloaderSeedingList[totalSeedingCount=$totalSeedingCount, totalSeedingSize=$totalSeedingSize, items=$items]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'total_seeding_count'] = this.totalSeedingCount;
      json[r'total_seeding_size'] = this.totalSeedingSize;
      json[r'items'] = this.items;
    return json;
  }

  /// Returns a new [AppDownloaderSeedingList] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppDownloaderSeedingList? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'total_seeding_count'), 'Required key "AppDownloaderSeedingList[total_seeding_count]" is missing from JSON.');
        assert(json[r'total_seeding_count'] != null, 'Required key "AppDownloaderSeedingList[total_seeding_count]" has a null value in JSON.');
        assert(json.containsKey(r'total_seeding_size'), 'Required key "AppDownloaderSeedingList[total_seeding_size]" is missing from JSON.');
        assert(json[r'total_seeding_size'] != null, 'Required key "AppDownloaderSeedingList[total_seeding_size]" has a null value in JSON.');
        assert(json.containsKey(r'items'), 'Required key "AppDownloaderSeedingList[items]" is missing from JSON.');
        assert(json[r'items'] != null, 'Required key "AppDownloaderSeedingList[items]" has a null value in JSON.');
        return true;
      }());

      return AppDownloaderSeedingList(
        totalSeedingCount: mapValueOfType<int>(json, r'total_seeding_count')!,
        totalSeedingSize: mapValueOfType<int>(json, r'total_seeding_size')!,
        items: AppDownloaderSeeding.listFromJson(json[r'items']),
      );
    }
    return null;
  }

  static List<AppDownloaderSeedingList> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppDownloaderSeedingList>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppDownloaderSeedingList.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppDownloaderSeedingList> mapFromJson(dynamic json) {
    final map = <String, AppDownloaderSeedingList>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppDownloaderSeedingList.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppDownloaderSeedingList-objects as value to a dart map
  static Map<String, List<AppDownloaderSeedingList>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppDownloaderSeedingList>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppDownloaderSeedingList.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'total_seeding_count',
    'total_seeding_size',
    'items',
  };
}

