//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppDownloaderSeeding {
  /// Returns a new [AppDownloaderSeeding] instance.
  AppDownloaderSeeding({
    required this.id,
    required this.name,
    required this.type,
    required this.reachable,
    this.error,
    required this.seedingCount,
    required this.seedingSize,
    required this.idleCount,
    required this.idleSize,
  });

  int id;

  String name;

  /// qbittorrent 或 transmission
  String type;

  /// 读到了种子列表；为假时下面的数都是 0
  bool reachable;

  /// 读不到时的原因（去掉了地址）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? error;

  /// 状态是做种的种子数
  int seedingCount;

  /// 状态是做种的种子体积（字节）
  int seedingSize;

  /// 下载完了、没在做种的种子数（暂停、停止、排队、校验中、出错）
  int idleCount;

  /// 下载完了、没在做种的种子体积（字节）
  int idleSize;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppDownloaderSeeding &&
    other.id == id &&
    other.name == name &&
    other.type == type &&
    other.reachable == reachable &&
    other.error == error &&
    other.seedingCount == seedingCount &&
    other.seedingSize == seedingSize &&
    other.idleCount == idleCount &&
    other.idleSize == idleSize;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (name.hashCode) +
    (type.hashCode) +
    (reachable.hashCode) +
    (error == null ? 0 : error!.hashCode) +
    (seedingCount.hashCode) +
    (seedingSize.hashCode) +
    (idleCount.hashCode) +
    (idleSize.hashCode);

  @override
  String toString() => 'AppDownloaderSeeding[id=$id, name=$name, type=$type, reachable=$reachable, error=$error, seedingCount=$seedingCount, seedingSize=$seedingSize, idleCount=$idleCount, idleSize=$idleSize]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'name'] = this.name;
      json[r'type'] = this.type;
      json[r'reachable'] = this.reachable;
    if (this.error != null) {
      json[r'error'] = this.error;
    } else {
      json[r'error'] = null;
    }
      json[r'seeding_count'] = this.seedingCount;
      json[r'seeding_size'] = this.seedingSize;
      json[r'idle_count'] = this.idleCount;
      json[r'idle_size'] = this.idleSize;
    return json;
  }

  /// Returns a new [AppDownloaderSeeding] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppDownloaderSeeding? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppDownloaderSeeding[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppDownloaderSeeding[id]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppDownloaderSeeding[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppDownloaderSeeding[name]" has a null value in JSON.');
        assert(json.containsKey(r'type'), 'Required key "AppDownloaderSeeding[type]" is missing from JSON.');
        assert(json[r'type'] != null, 'Required key "AppDownloaderSeeding[type]" has a null value in JSON.');
        assert(json.containsKey(r'reachable'), 'Required key "AppDownloaderSeeding[reachable]" is missing from JSON.');
        assert(json[r'reachable'] != null, 'Required key "AppDownloaderSeeding[reachable]" has a null value in JSON.');
        assert(json.containsKey(r'seeding_count'), 'Required key "AppDownloaderSeeding[seeding_count]" is missing from JSON.');
        assert(json[r'seeding_count'] != null, 'Required key "AppDownloaderSeeding[seeding_count]" has a null value in JSON.');
        assert(json.containsKey(r'seeding_size'), 'Required key "AppDownloaderSeeding[seeding_size]" is missing from JSON.');
        assert(json[r'seeding_size'] != null, 'Required key "AppDownloaderSeeding[seeding_size]" has a null value in JSON.');
        assert(json.containsKey(r'idle_count'), 'Required key "AppDownloaderSeeding[idle_count]" is missing from JSON.');
        assert(json[r'idle_count'] != null, 'Required key "AppDownloaderSeeding[idle_count]" has a null value in JSON.');
        assert(json.containsKey(r'idle_size'), 'Required key "AppDownloaderSeeding[idle_size]" is missing from JSON.');
        assert(json[r'idle_size'] != null, 'Required key "AppDownloaderSeeding[idle_size]" has a null value in JSON.');
        return true;
      }());

      return AppDownloaderSeeding(
        id: mapValueOfType<int>(json, r'id')!,
        name: mapValueOfType<String>(json, r'name')!,
        type: mapValueOfType<String>(json, r'type')!,
        reachable: mapValueOfType<bool>(json, r'reachable')!,
        error: mapValueOfType<String>(json, r'error'),
        seedingCount: mapValueOfType<int>(json, r'seeding_count')!,
        seedingSize: mapValueOfType<int>(json, r'seeding_size')!,
        idleCount: mapValueOfType<int>(json, r'idle_count')!,
        idleSize: mapValueOfType<int>(json, r'idle_size')!,
      );
    }
    return null;
  }

  static List<AppDownloaderSeeding> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppDownloaderSeeding>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppDownloaderSeeding.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppDownloaderSeeding> mapFromJson(dynamic json) {
    final map = <String, AppDownloaderSeeding>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppDownloaderSeeding.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppDownloaderSeeding-objects as value to a dart map
  static Map<String, List<AppDownloaderSeeding>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppDownloaderSeeding>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppDownloaderSeeding.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'name',
    'type',
    'reachable',
    'seeding_count',
    'seeding_size',
    'idle_count',
    'idle_size',
  };
}

