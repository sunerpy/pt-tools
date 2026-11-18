//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppDownloader {
  /// Returns a new [AppDownloader] instance.
  AppDownloader({
    required this.id,
    required this.name,
    required this.type,
    required this.default_,
    required this.reachable,
    this.error,
    this.version,
    required this.uploadSpeed,
    required this.downloadSpeed,
    required this.uploaded,
    required this.downloaded,
    required this.freeSpace,
  });

  int id;

  String name;

  /// qbittorrent 或 transmission
  String type;

  bool default_;

  bool reachable;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? error;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? version;

  int uploadSpeed;

  int downloadSpeed;

  int uploaded;

  int downloaded;

  int freeSpace;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppDownloader &&
    other.id == id &&
    other.name == name &&
    other.type == type &&
    other.default_ == default_ &&
    other.reachable == reachable &&
    other.error == error &&
    other.version == version &&
    other.uploadSpeed == uploadSpeed &&
    other.downloadSpeed == downloadSpeed &&
    other.uploaded == uploaded &&
    other.downloaded == downloaded &&
    other.freeSpace == freeSpace;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (name.hashCode) +
    (type.hashCode) +
    (default_.hashCode) +
    (reachable.hashCode) +
    (error == null ? 0 : error!.hashCode) +
    (version == null ? 0 : version!.hashCode) +
    (uploadSpeed.hashCode) +
    (downloadSpeed.hashCode) +
    (uploaded.hashCode) +
    (downloaded.hashCode) +
    (freeSpace.hashCode);

  @override
  String toString() => 'AppDownloader[id=$id, name=$name, type=$type, default_=$default_, reachable=$reachable, error=$error, version=$version, uploadSpeed=$uploadSpeed, downloadSpeed=$downloadSpeed, uploaded=$uploaded, downloaded=$downloaded, freeSpace=$freeSpace]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'name'] = this.name;
      json[r'type'] = this.type;
      json[r'default'] = this.default_;
      json[r'reachable'] = this.reachable;
    if (this.error != null) {
      json[r'error'] = this.error;
    } else {
      json[r'error'] = null;
    }
    if (this.version != null) {
      json[r'version'] = this.version;
    } else {
      json[r'version'] = null;
    }
      json[r'upload_speed'] = this.uploadSpeed;
      json[r'download_speed'] = this.downloadSpeed;
      json[r'uploaded'] = this.uploaded;
      json[r'downloaded'] = this.downloaded;
      json[r'free_space'] = this.freeSpace;
    return json;
  }

  /// Returns a new [AppDownloader] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppDownloader? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppDownloader[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppDownloader[id]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppDownloader[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppDownloader[name]" has a null value in JSON.');
        assert(json.containsKey(r'type'), 'Required key "AppDownloader[type]" is missing from JSON.');
        assert(json[r'type'] != null, 'Required key "AppDownloader[type]" has a null value in JSON.');
        assert(json.containsKey(r'default'), 'Required key "AppDownloader[default]" is missing from JSON.');
        assert(json[r'default'] != null, 'Required key "AppDownloader[default]" has a null value in JSON.');
        assert(json.containsKey(r'reachable'), 'Required key "AppDownloader[reachable]" is missing from JSON.');
        assert(json[r'reachable'] != null, 'Required key "AppDownloader[reachable]" has a null value in JSON.');
        assert(json.containsKey(r'upload_speed'), 'Required key "AppDownloader[upload_speed]" is missing from JSON.');
        assert(json[r'upload_speed'] != null, 'Required key "AppDownloader[upload_speed]" has a null value in JSON.');
        assert(json.containsKey(r'download_speed'), 'Required key "AppDownloader[download_speed]" is missing from JSON.');
        assert(json[r'download_speed'] != null, 'Required key "AppDownloader[download_speed]" has a null value in JSON.');
        assert(json.containsKey(r'uploaded'), 'Required key "AppDownloader[uploaded]" is missing from JSON.');
        assert(json[r'uploaded'] != null, 'Required key "AppDownloader[uploaded]" has a null value in JSON.');
        assert(json.containsKey(r'downloaded'), 'Required key "AppDownloader[downloaded]" is missing from JSON.');
        assert(json[r'downloaded'] != null, 'Required key "AppDownloader[downloaded]" has a null value in JSON.');
        assert(json.containsKey(r'free_space'), 'Required key "AppDownloader[free_space]" is missing from JSON.');
        assert(json[r'free_space'] != null, 'Required key "AppDownloader[free_space]" has a null value in JSON.');
        return true;
      }());

      return AppDownloader(
        id: mapValueOfType<int>(json, r'id')!,
        name: mapValueOfType<String>(json, r'name')!,
        type: mapValueOfType<String>(json, r'type')!,
        default_: mapValueOfType<bool>(json, r'default')!,
        reachable: mapValueOfType<bool>(json, r'reachable')!,
        error: mapValueOfType<String>(json, r'error'),
        version: mapValueOfType<String>(json, r'version'),
        uploadSpeed: mapValueOfType<int>(json, r'upload_speed')!,
        downloadSpeed: mapValueOfType<int>(json, r'download_speed')!,
        uploaded: mapValueOfType<int>(json, r'uploaded')!,
        downloaded: mapValueOfType<int>(json, r'downloaded')!,
        freeSpace: mapValueOfType<int>(json, r'free_space')!,
      );
    }
    return null;
  }

  static List<AppDownloader> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppDownloader>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppDownloader.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppDownloader> mapFromJson(dynamic json) {
    final map = <String, AppDownloader>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppDownloader.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppDownloader-objects as value to a dart map
  static Map<String, List<AppDownloader>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppDownloader>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppDownloader.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'name',
    'type',
    'default',
    'reachable',
    'upload_speed',
    'download_speed',
    'uploaded',
    'downloaded',
    'free_space',
  };
}

