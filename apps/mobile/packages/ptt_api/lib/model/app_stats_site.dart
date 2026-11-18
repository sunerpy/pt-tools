//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppStatsSite {
  /// Returns a new [AppStatsSite] instance.
  AppStatsSite({
    required this.site,
    required this.username,
    this.userId,
    required this.uploaded,
    required this.downloaded,
    required this.ratio,
    required this.bonus,
    required this.seeding,
    required this.leeching,
    this.rank,
    this.joinDate,
    this.lastAccess,
    required this.lastUpdate,
    this.levelName,
    this.levelId,
    this.bonusPerHour,
    this.seedingBonus,
    this.seedingBonusPerHour,
    this.unreadMessageCount,
    this.totalMessageCount,
    this.seederCount,
    this.seederSize,
    this.leecherCount,
    this.leecherSize,
    this.hnrUnsatisfied,
    this.hnrPreWarning,
    this.trueUploaded,
    this.trueDownloaded,
    this.uploads,
  });

  String site;

  String username;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? userId;

  int uploaded;

  int downloaded;

  num ratio;

  num bonus;

  int seeding;

  int leeching;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? rank;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? joinDate;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? lastAccess;

  int lastUpdate;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? levelName;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? levelId;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  num? bonusPerHour;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  num? seedingBonus;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  num? seedingBonusPerHour;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? unreadMessageCount;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? totalMessageCount;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? seederCount;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? seederSize;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? leecherCount;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? leecherSize;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? hnrUnsatisfied;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? hnrPreWarning;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? trueUploaded;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? trueDownloaded;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? uploads;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppStatsSite &&
    other.site == site &&
    other.username == username &&
    other.userId == userId &&
    other.uploaded == uploaded &&
    other.downloaded == downloaded &&
    other.ratio == ratio &&
    other.bonus == bonus &&
    other.seeding == seeding &&
    other.leeching == leeching &&
    other.rank == rank &&
    other.joinDate == joinDate &&
    other.lastAccess == lastAccess &&
    other.lastUpdate == lastUpdate &&
    other.levelName == levelName &&
    other.levelId == levelId &&
    other.bonusPerHour == bonusPerHour &&
    other.seedingBonus == seedingBonus &&
    other.seedingBonusPerHour == seedingBonusPerHour &&
    other.unreadMessageCount == unreadMessageCount &&
    other.totalMessageCount == totalMessageCount &&
    other.seederCount == seederCount &&
    other.seederSize == seederSize &&
    other.leecherCount == leecherCount &&
    other.leecherSize == leecherSize &&
    other.hnrUnsatisfied == hnrUnsatisfied &&
    other.hnrPreWarning == hnrPreWarning &&
    other.trueUploaded == trueUploaded &&
    other.trueDownloaded == trueDownloaded &&
    other.uploads == uploads;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (username.hashCode) +
    (userId == null ? 0 : userId!.hashCode) +
    (uploaded.hashCode) +
    (downloaded.hashCode) +
    (ratio.hashCode) +
    (bonus.hashCode) +
    (seeding.hashCode) +
    (leeching.hashCode) +
    (rank == null ? 0 : rank!.hashCode) +
    (joinDate == null ? 0 : joinDate!.hashCode) +
    (lastAccess == null ? 0 : lastAccess!.hashCode) +
    (lastUpdate.hashCode) +
    (levelName == null ? 0 : levelName!.hashCode) +
    (levelId == null ? 0 : levelId!.hashCode) +
    (bonusPerHour == null ? 0 : bonusPerHour!.hashCode) +
    (seedingBonus == null ? 0 : seedingBonus!.hashCode) +
    (seedingBonusPerHour == null ? 0 : seedingBonusPerHour!.hashCode) +
    (unreadMessageCount == null ? 0 : unreadMessageCount!.hashCode) +
    (totalMessageCount == null ? 0 : totalMessageCount!.hashCode) +
    (seederCount == null ? 0 : seederCount!.hashCode) +
    (seederSize == null ? 0 : seederSize!.hashCode) +
    (leecherCount == null ? 0 : leecherCount!.hashCode) +
    (leecherSize == null ? 0 : leecherSize!.hashCode) +
    (hnrUnsatisfied == null ? 0 : hnrUnsatisfied!.hashCode) +
    (hnrPreWarning == null ? 0 : hnrPreWarning!.hashCode) +
    (trueUploaded == null ? 0 : trueUploaded!.hashCode) +
    (trueDownloaded == null ? 0 : trueDownloaded!.hashCode) +
    (uploads == null ? 0 : uploads!.hashCode);

  @override
  String toString() => 'AppStatsSite[site=$site, username=$username, userId=$userId, uploaded=$uploaded, downloaded=$downloaded, ratio=$ratio, bonus=$bonus, seeding=$seeding, leeching=$leeching, rank=$rank, joinDate=$joinDate, lastAccess=$lastAccess, lastUpdate=$lastUpdate, levelName=$levelName, levelId=$levelId, bonusPerHour=$bonusPerHour, seedingBonus=$seedingBonus, seedingBonusPerHour=$seedingBonusPerHour, unreadMessageCount=$unreadMessageCount, totalMessageCount=$totalMessageCount, seederCount=$seederCount, seederSize=$seederSize, leecherCount=$leecherCount, leecherSize=$leecherSize, hnrUnsatisfied=$hnrUnsatisfied, hnrPreWarning=$hnrPreWarning, trueUploaded=$trueUploaded, trueDownloaded=$trueDownloaded, uploads=$uploads]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'username'] = this.username;
    if (this.userId != null) {
      json[r'userId'] = this.userId;
    } else {
      json[r'userId'] = null;
    }
      json[r'uploaded'] = this.uploaded;
      json[r'downloaded'] = this.downloaded;
      json[r'ratio'] = this.ratio;
      json[r'bonus'] = this.bonus;
      json[r'seeding'] = this.seeding;
      json[r'leeching'] = this.leeching;
    if (this.rank != null) {
      json[r'rank'] = this.rank;
    } else {
      json[r'rank'] = null;
    }
    if (this.joinDate != null) {
      json[r'joinDate'] = this.joinDate;
    } else {
      json[r'joinDate'] = null;
    }
    if (this.lastAccess != null) {
      json[r'lastAccess'] = this.lastAccess;
    } else {
      json[r'lastAccess'] = null;
    }
      json[r'lastUpdate'] = this.lastUpdate;
    if (this.levelName != null) {
      json[r'levelName'] = this.levelName;
    } else {
      json[r'levelName'] = null;
    }
    if (this.levelId != null) {
      json[r'levelId'] = this.levelId;
    } else {
      json[r'levelId'] = null;
    }
    if (this.bonusPerHour != null) {
      json[r'bonusPerHour'] = this.bonusPerHour;
    } else {
      json[r'bonusPerHour'] = null;
    }
    if (this.seedingBonus != null) {
      json[r'seedingBonus'] = this.seedingBonus;
    } else {
      json[r'seedingBonus'] = null;
    }
    if (this.seedingBonusPerHour != null) {
      json[r'seedingBonusPerHour'] = this.seedingBonusPerHour;
    } else {
      json[r'seedingBonusPerHour'] = null;
    }
    if (this.unreadMessageCount != null) {
      json[r'unreadMessageCount'] = this.unreadMessageCount;
    } else {
      json[r'unreadMessageCount'] = null;
    }
    if (this.totalMessageCount != null) {
      json[r'totalMessageCount'] = this.totalMessageCount;
    } else {
      json[r'totalMessageCount'] = null;
    }
    if (this.seederCount != null) {
      json[r'seederCount'] = this.seederCount;
    } else {
      json[r'seederCount'] = null;
    }
    if (this.seederSize != null) {
      json[r'seederSize'] = this.seederSize;
    } else {
      json[r'seederSize'] = null;
    }
    if (this.leecherCount != null) {
      json[r'leecherCount'] = this.leecherCount;
    } else {
      json[r'leecherCount'] = null;
    }
    if (this.leecherSize != null) {
      json[r'leecherSize'] = this.leecherSize;
    } else {
      json[r'leecherSize'] = null;
    }
    if (this.hnrUnsatisfied != null) {
      json[r'hnrUnsatisfied'] = this.hnrUnsatisfied;
    } else {
      json[r'hnrUnsatisfied'] = null;
    }
    if (this.hnrPreWarning != null) {
      json[r'hnrPreWarning'] = this.hnrPreWarning;
    } else {
      json[r'hnrPreWarning'] = null;
    }
    if (this.trueUploaded != null) {
      json[r'trueUploaded'] = this.trueUploaded;
    } else {
      json[r'trueUploaded'] = null;
    }
    if (this.trueDownloaded != null) {
      json[r'trueDownloaded'] = this.trueDownloaded;
    } else {
      json[r'trueDownloaded'] = null;
    }
    if (this.uploads != null) {
      json[r'uploads'] = this.uploads;
    } else {
      json[r'uploads'] = null;
    }
    return json;
  }

  /// Returns a new [AppStatsSite] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppStatsSite? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppStatsSite[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppStatsSite[site]" has a null value in JSON.');
        assert(json.containsKey(r'username'), 'Required key "AppStatsSite[username]" is missing from JSON.');
        assert(json[r'username'] != null, 'Required key "AppStatsSite[username]" has a null value in JSON.');
        assert(json.containsKey(r'uploaded'), 'Required key "AppStatsSite[uploaded]" is missing from JSON.');
        assert(json[r'uploaded'] != null, 'Required key "AppStatsSite[uploaded]" has a null value in JSON.');
        assert(json.containsKey(r'downloaded'), 'Required key "AppStatsSite[downloaded]" is missing from JSON.');
        assert(json[r'downloaded'] != null, 'Required key "AppStatsSite[downloaded]" has a null value in JSON.');
        assert(json.containsKey(r'ratio'), 'Required key "AppStatsSite[ratio]" is missing from JSON.');
        assert(json[r'ratio'] != null, 'Required key "AppStatsSite[ratio]" has a null value in JSON.');
        assert(json.containsKey(r'bonus'), 'Required key "AppStatsSite[bonus]" is missing from JSON.');
        assert(json[r'bonus'] != null, 'Required key "AppStatsSite[bonus]" has a null value in JSON.');
        assert(json.containsKey(r'seeding'), 'Required key "AppStatsSite[seeding]" is missing from JSON.');
        assert(json[r'seeding'] != null, 'Required key "AppStatsSite[seeding]" has a null value in JSON.');
        assert(json.containsKey(r'leeching'), 'Required key "AppStatsSite[leeching]" is missing from JSON.');
        assert(json[r'leeching'] != null, 'Required key "AppStatsSite[leeching]" has a null value in JSON.');
        assert(json.containsKey(r'lastUpdate'), 'Required key "AppStatsSite[lastUpdate]" is missing from JSON.');
        assert(json[r'lastUpdate'] != null, 'Required key "AppStatsSite[lastUpdate]" has a null value in JSON.');
        return true;
      }());

      return AppStatsSite(
        site: mapValueOfType<String>(json, r'site')!,
        username: mapValueOfType<String>(json, r'username')!,
        userId: mapValueOfType<String>(json, r'userId'),
        uploaded: mapValueOfType<int>(json, r'uploaded')!,
        downloaded: mapValueOfType<int>(json, r'downloaded')!,
        ratio: num.parse('${json[r'ratio']}'),
        bonus: num.parse('${json[r'bonus']}'),
        seeding: mapValueOfType<int>(json, r'seeding')!,
        leeching: mapValueOfType<int>(json, r'leeching')!,
        rank: mapValueOfType<String>(json, r'rank'),
        joinDate: mapValueOfType<int>(json, r'joinDate'),
        lastAccess: mapValueOfType<int>(json, r'lastAccess'),
        lastUpdate: mapValueOfType<int>(json, r'lastUpdate')!,
        levelName: mapValueOfType<String>(json, r'levelName'),
        levelId: mapValueOfType<int>(json, r'levelId'),
        bonusPerHour: json[r'bonusPerHour'] == null
            ? null
            : num.parse('${json[r'bonusPerHour']}'),
        seedingBonus: json[r'seedingBonus'] == null
            ? null
            : num.parse('${json[r'seedingBonus']}'),
        seedingBonusPerHour: json[r'seedingBonusPerHour'] == null
            ? null
            : num.parse('${json[r'seedingBonusPerHour']}'),
        unreadMessageCount: mapValueOfType<int>(json, r'unreadMessageCount'),
        totalMessageCount: mapValueOfType<int>(json, r'totalMessageCount'),
        seederCount: mapValueOfType<int>(json, r'seederCount'),
        seederSize: mapValueOfType<int>(json, r'seederSize'),
        leecherCount: mapValueOfType<int>(json, r'leecherCount'),
        leecherSize: mapValueOfType<int>(json, r'leecherSize'),
        hnrUnsatisfied: mapValueOfType<int>(json, r'hnrUnsatisfied'),
        hnrPreWarning: mapValueOfType<int>(json, r'hnrPreWarning'),
        trueUploaded: mapValueOfType<int>(json, r'trueUploaded'),
        trueDownloaded: mapValueOfType<int>(json, r'trueDownloaded'),
        uploads: mapValueOfType<int>(json, r'uploads'),
      );
    }
    return null;
  }

  static List<AppStatsSite> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppStatsSite>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppStatsSite.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppStatsSite> mapFromJson(dynamic json) {
    final map = <String, AppStatsSite>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppStatsSite.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppStatsSite-objects as value to a dart map
  static Map<String, List<AppStatsSite>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppStatsSite>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppStatsSite.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'username',
    'uploaded',
    'downloaded',
    'ratio',
    'bonus',
    'seeding',
    'leeching',
    'lastUpdate',
  };
}

