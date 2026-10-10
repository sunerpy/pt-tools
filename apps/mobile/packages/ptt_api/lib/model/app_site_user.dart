//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSiteUser {
  /// Returns a new [AppSiteUser] instance.
  AppSiteUser({
    required this.username,
    this.level,
    required this.uploaded,
    required this.downloaded,
    required this.ratio,
    required this.bonus,
    this.bonusPerHour,
    required this.seeding,
    required this.leeching,
    this.seedingSize,
    required this.unreadMessages,
    this.hnrUnsatisfied,
    required this.updatedAt,
  });

  String username;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? level;

  int uploaded;

  int downloaded;

  double ratio;

  double bonus;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  double? bonusPerHour;

  int seeding;

  int leeching;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? seedingSize;

  int unreadMessages;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? hnrUnsatisfied;

  int updatedAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSiteUser &&
    other.username == username &&
    other.level == level &&
    other.uploaded == uploaded &&
    other.downloaded == downloaded &&
    other.ratio == ratio &&
    other.bonus == bonus &&
    other.bonusPerHour == bonusPerHour &&
    other.seeding == seeding &&
    other.leeching == leeching &&
    other.seedingSize == seedingSize &&
    other.unreadMessages == unreadMessages &&
    other.hnrUnsatisfied == hnrUnsatisfied &&
    other.updatedAt == updatedAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (username.hashCode) +
    (level == null ? 0 : level!.hashCode) +
    (uploaded.hashCode) +
    (downloaded.hashCode) +
    (ratio.hashCode) +
    (bonus.hashCode) +
    (bonusPerHour == null ? 0 : bonusPerHour!.hashCode) +
    (seeding.hashCode) +
    (leeching.hashCode) +
    (seedingSize == null ? 0 : seedingSize!.hashCode) +
    (unreadMessages.hashCode) +
    (hnrUnsatisfied == null ? 0 : hnrUnsatisfied!.hashCode) +
    (updatedAt.hashCode);

  @override
  String toString() => 'AppSiteUser[username=$username, level=$level, uploaded=$uploaded, downloaded=$downloaded, ratio=$ratio, bonus=$bonus, bonusPerHour=$bonusPerHour, seeding=$seeding, leeching=$leeching, seedingSize=$seedingSize, unreadMessages=$unreadMessages, hnrUnsatisfied=$hnrUnsatisfied, updatedAt=$updatedAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'username'] = this.username;
    if (this.level != null) {
      json[r'level'] = this.level;
    } else {
      json[r'level'] = null;
    }
      json[r'uploaded'] = this.uploaded;
      json[r'downloaded'] = this.downloaded;
      json[r'ratio'] = this.ratio;
      json[r'bonus'] = this.bonus;
    if (this.bonusPerHour != null) {
      json[r'bonus_per_hour'] = this.bonusPerHour;
    } else {
      json[r'bonus_per_hour'] = null;
    }
      json[r'seeding'] = this.seeding;
      json[r'leeching'] = this.leeching;
    if (this.seedingSize != null) {
      json[r'seeding_size'] = this.seedingSize;
    } else {
      json[r'seeding_size'] = null;
    }
      json[r'unread_messages'] = this.unreadMessages;
    if (this.hnrUnsatisfied != null) {
      json[r'hnr_unsatisfied'] = this.hnrUnsatisfied;
    } else {
      json[r'hnr_unsatisfied'] = null;
    }
      json[r'updated_at'] = this.updatedAt;
    return json;
  }

  /// Returns a new [AppSiteUser] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSiteUser? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'username'), 'Required key "AppSiteUser[username]" is missing from JSON.');
        assert(json[r'username'] != null, 'Required key "AppSiteUser[username]" has a null value in JSON.');
        assert(json.containsKey(r'uploaded'), 'Required key "AppSiteUser[uploaded]" is missing from JSON.');
        assert(json[r'uploaded'] != null, 'Required key "AppSiteUser[uploaded]" has a null value in JSON.');
        assert(json.containsKey(r'downloaded'), 'Required key "AppSiteUser[downloaded]" is missing from JSON.');
        assert(json[r'downloaded'] != null, 'Required key "AppSiteUser[downloaded]" has a null value in JSON.');
        assert(json.containsKey(r'ratio'), 'Required key "AppSiteUser[ratio]" is missing from JSON.');
        assert(json[r'ratio'] != null, 'Required key "AppSiteUser[ratio]" has a null value in JSON.');
        assert(json.containsKey(r'bonus'), 'Required key "AppSiteUser[bonus]" is missing from JSON.');
        assert(json[r'bonus'] != null, 'Required key "AppSiteUser[bonus]" has a null value in JSON.');
        assert(json.containsKey(r'seeding'), 'Required key "AppSiteUser[seeding]" is missing from JSON.');
        assert(json[r'seeding'] != null, 'Required key "AppSiteUser[seeding]" has a null value in JSON.');
        assert(json.containsKey(r'leeching'), 'Required key "AppSiteUser[leeching]" is missing from JSON.');
        assert(json[r'leeching'] != null, 'Required key "AppSiteUser[leeching]" has a null value in JSON.');
        assert(json.containsKey(r'unread_messages'), 'Required key "AppSiteUser[unread_messages]" is missing from JSON.');
        assert(json[r'unread_messages'] != null, 'Required key "AppSiteUser[unread_messages]" has a null value in JSON.');
        assert(json.containsKey(r'updated_at'), 'Required key "AppSiteUser[updated_at]" is missing from JSON.');
        assert(json[r'updated_at'] != null, 'Required key "AppSiteUser[updated_at]" has a null value in JSON.');
        return true;
      }());

      return AppSiteUser(
        username: mapValueOfType<String>(json, r'username')!,
        level: mapValueOfType<String>(json, r'level'),
        uploaded: mapValueOfType<int>(json, r'uploaded')!,
        downloaded: mapValueOfType<int>(json, r'downloaded')!,
        ratio: mapValueOfType<double>(json, r'ratio')!,
        bonus: mapValueOfType<double>(json, r'bonus')!,
        bonusPerHour: mapValueOfType<double>(json, r'bonus_per_hour'),
        seeding: mapValueOfType<int>(json, r'seeding')!,
        leeching: mapValueOfType<int>(json, r'leeching')!,
        seedingSize: mapValueOfType<int>(json, r'seeding_size'),
        unreadMessages: mapValueOfType<int>(json, r'unread_messages')!,
        hnrUnsatisfied: mapValueOfType<int>(json, r'hnr_unsatisfied'),
        updatedAt: mapValueOfType<int>(json, r'updated_at')!,
      );
    }
    return null;
  }

  static List<AppSiteUser> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSiteUser>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSiteUser.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSiteUser> mapFromJson(dynamic json) {
    final map = <String, AppSiteUser>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSiteUser.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSiteUser-objects as value to a dart map
  static Map<String, List<AppSiteUser>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSiteUser>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSiteUser.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'username',
    'uploaded',
    'downloaded',
    'ratio',
    'bonus',
    'seeding',
    'leeching',
    'unread_messages',
    'updated_at',
  };
}

