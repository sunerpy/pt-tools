//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTotals {
  /// Returns a new [AppTotals] instance.
  AppTotals({
    required this.uploaded,
    required this.downloaded,
    required this.ratio,
    required this.seeding,
    required this.leeching,
    required this.bonus,
    required this.bonusPerHour,
    required this.seedingSize,
    required this.siteCount,
    required this.unreadMessages,
  });

  int uploaded;

  int downloaded;

  double ratio;

  int seeding;

  int leeching;

  double bonus;

  double bonusPerHour;

  int seedingSize;

  int siteCount;

  int unreadMessages;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTotals &&
    other.uploaded == uploaded &&
    other.downloaded == downloaded &&
    other.ratio == ratio &&
    other.seeding == seeding &&
    other.leeching == leeching &&
    other.bonus == bonus &&
    other.bonusPerHour == bonusPerHour &&
    other.seedingSize == seedingSize &&
    other.siteCount == siteCount &&
    other.unreadMessages == unreadMessages;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (uploaded.hashCode) +
    (downloaded.hashCode) +
    (ratio.hashCode) +
    (seeding.hashCode) +
    (leeching.hashCode) +
    (bonus.hashCode) +
    (bonusPerHour.hashCode) +
    (seedingSize.hashCode) +
    (siteCount.hashCode) +
    (unreadMessages.hashCode);

  @override
  String toString() => 'AppTotals[uploaded=$uploaded, downloaded=$downloaded, ratio=$ratio, seeding=$seeding, leeching=$leeching, bonus=$bonus, bonusPerHour=$bonusPerHour, seedingSize=$seedingSize, siteCount=$siteCount, unreadMessages=$unreadMessages]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'uploaded'] = this.uploaded;
      json[r'downloaded'] = this.downloaded;
      json[r'ratio'] = this.ratio;
      json[r'seeding'] = this.seeding;
      json[r'leeching'] = this.leeching;
      json[r'bonus'] = this.bonus;
      json[r'bonus_per_hour'] = this.bonusPerHour;
      json[r'seeding_size'] = this.seedingSize;
      json[r'site_count'] = this.siteCount;
      json[r'unread_messages'] = this.unreadMessages;
    return json;
  }

  /// Returns a new [AppTotals] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTotals? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'uploaded'), 'Required key "AppTotals[uploaded]" is missing from JSON.');
        assert(json[r'uploaded'] != null, 'Required key "AppTotals[uploaded]" has a null value in JSON.');
        assert(json.containsKey(r'downloaded'), 'Required key "AppTotals[downloaded]" is missing from JSON.');
        assert(json[r'downloaded'] != null, 'Required key "AppTotals[downloaded]" has a null value in JSON.');
        assert(json.containsKey(r'ratio'), 'Required key "AppTotals[ratio]" is missing from JSON.');
        assert(json[r'ratio'] != null, 'Required key "AppTotals[ratio]" has a null value in JSON.');
        assert(json.containsKey(r'seeding'), 'Required key "AppTotals[seeding]" is missing from JSON.');
        assert(json[r'seeding'] != null, 'Required key "AppTotals[seeding]" has a null value in JSON.');
        assert(json.containsKey(r'leeching'), 'Required key "AppTotals[leeching]" is missing from JSON.');
        assert(json[r'leeching'] != null, 'Required key "AppTotals[leeching]" has a null value in JSON.');
        assert(json.containsKey(r'bonus'), 'Required key "AppTotals[bonus]" is missing from JSON.');
        assert(json[r'bonus'] != null, 'Required key "AppTotals[bonus]" has a null value in JSON.');
        assert(json.containsKey(r'bonus_per_hour'), 'Required key "AppTotals[bonus_per_hour]" is missing from JSON.');
        assert(json[r'bonus_per_hour'] != null, 'Required key "AppTotals[bonus_per_hour]" has a null value in JSON.');
        assert(json.containsKey(r'seeding_size'), 'Required key "AppTotals[seeding_size]" is missing from JSON.');
        assert(json[r'seeding_size'] != null, 'Required key "AppTotals[seeding_size]" has a null value in JSON.');
        assert(json.containsKey(r'site_count'), 'Required key "AppTotals[site_count]" is missing from JSON.');
        assert(json[r'site_count'] != null, 'Required key "AppTotals[site_count]" has a null value in JSON.');
        assert(json.containsKey(r'unread_messages'), 'Required key "AppTotals[unread_messages]" is missing from JSON.');
        assert(json[r'unread_messages'] != null, 'Required key "AppTotals[unread_messages]" has a null value in JSON.');
        return true;
      }());

      return AppTotals(
        uploaded: mapValueOfType<int>(json, r'uploaded')!,
        downloaded: mapValueOfType<int>(json, r'downloaded')!,
        ratio: mapValueOfType<double>(json, r'ratio')!,
        seeding: mapValueOfType<int>(json, r'seeding')!,
        leeching: mapValueOfType<int>(json, r'leeching')!,
        bonus: mapValueOfType<double>(json, r'bonus')!,
        bonusPerHour: mapValueOfType<double>(json, r'bonus_per_hour')!,
        seedingSize: mapValueOfType<int>(json, r'seeding_size')!,
        siteCount: mapValueOfType<int>(json, r'site_count')!,
        unreadMessages: mapValueOfType<int>(json, r'unread_messages')!,
      );
    }
    return null;
  }

  static List<AppTotals> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTotals>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTotals.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTotals> mapFromJson(dynamic json) {
    final map = <String, AppTotals>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTotals.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTotals-objects as value to a dart map
  static Map<String, List<AppTotals>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTotals>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTotals.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'uploaded',
    'downloaded',
    'ratio',
    'seeding',
    'leeching',
    'bonus',
    'bonus_per_hour',
    'seeding_size',
    'site_count',
    'unread_messages',
  };
}

