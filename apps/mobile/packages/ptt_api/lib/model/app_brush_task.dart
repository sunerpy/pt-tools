//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppBrushTask {
  /// Returns a new [AppBrushTask] instance.
  AppBrushTask({
    required this.id,
    required this.name,
    required this.enabled,
    required this.site,
    required this.siteEnabled,
    required this.downloader,
    required this.active,
    required this.downloading,
    required this.activeSize,
    required this.today,
    required this.total,
  });

  int id;

  String name;

  bool enabled;

  String site;

  bool siteEnabled;

  String downloader;

  int active;

  int downloading;

  int activeSize;

  AppBrushStat today;

  AppBrushStat total;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppBrushTask &&
    other.id == id &&
    other.name == name &&
    other.enabled == enabled &&
    other.site == site &&
    other.siteEnabled == siteEnabled &&
    other.downloader == downloader &&
    other.active == active &&
    other.downloading == downloading &&
    other.activeSize == activeSize &&
    other.today == today &&
    other.total == total;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (name.hashCode) +
    (enabled.hashCode) +
    (site.hashCode) +
    (siteEnabled.hashCode) +
    (downloader.hashCode) +
    (active.hashCode) +
    (downloading.hashCode) +
    (activeSize.hashCode) +
    (today.hashCode) +
    (total.hashCode);

  @override
  String toString() => 'AppBrushTask[id=$id, name=$name, enabled=$enabled, site=$site, siteEnabled=$siteEnabled, downloader=$downloader, active=$active, downloading=$downloading, activeSize=$activeSize, today=$today, total=$total]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'name'] = this.name;
      json[r'enabled'] = this.enabled;
      json[r'site'] = this.site;
      json[r'site_enabled'] = this.siteEnabled;
      json[r'downloader'] = this.downloader;
      json[r'active'] = this.active;
      json[r'downloading'] = this.downloading;
      json[r'active_size'] = this.activeSize;
      json[r'today'] = this.today;
      json[r'total'] = this.total;
    return json;
  }

  /// Returns a new [AppBrushTask] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppBrushTask? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppBrushTask[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppBrushTask[id]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppBrushTask[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppBrushTask[name]" has a null value in JSON.');
        assert(json.containsKey(r'enabled'), 'Required key "AppBrushTask[enabled]" is missing from JSON.');
        assert(json[r'enabled'] != null, 'Required key "AppBrushTask[enabled]" has a null value in JSON.');
        assert(json.containsKey(r'site'), 'Required key "AppBrushTask[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppBrushTask[site]" has a null value in JSON.');
        assert(json.containsKey(r'site_enabled'), 'Required key "AppBrushTask[site_enabled]" is missing from JSON.');
        assert(json[r'site_enabled'] != null, 'Required key "AppBrushTask[site_enabled]" has a null value in JSON.');
        assert(json.containsKey(r'downloader'), 'Required key "AppBrushTask[downloader]" is missing from JSON.');
        assert(json[r'downloader'] != null, 'Required key "AppBrushTask[downloader]" has a null value in JSON.');
        assert(json.containsKey(r'active'), 'Required key "AppBrushTask[active]" is missing from JSON.');
        assert(json[r'active'] != null, 'Required key "AppBrushTask[active]" has a null value in JSON.');
        assert(json.containsKey(r'downloading'), 'Required key "AppBrushTask[downloading]" is missing from JSON.');
        assert(json[r'downloading'] != null, 'Required key "AppBrushTask[downloading]" has a null value in JSON.');
        assert(json.containsKey(r'active_size'), 'Required key "AppBrushTask[active_size]" is missing from JSON.');
        assert(json[r'active_size'] != null, 'Required key "AppBrushTask[active_size]" has a null value in JSON.');
        assert(json.containsKey(r'today'), 'Required key "AppBrushTask[today]" is missing from JSON.');
        assert(json[r'today'] != null, 'Required key "AppBrushTask[today]" has a null value in JSON.');
        assert(json.containsKey(r'total'), 'Required key "AppBrushTask[total]" is missing from JSON.');
        assert(json[r'total'] != null, 'Required key "AppBrushTask[total]" has a null value in JSON.');
        return true;
      }());

      return AppBrushTask(
        id: mapValueOfType<int>(json, r'id')!,
        name: mapValueOfType<String>(json, r'name')!,
        enabled: mapValueOfType<bool>(json, r'enabled')!,
        site: mapValueOfType<String>(json, r'site')!,
        siteEnabled: mapValueOfType<bool>(json, r'site_enabled')!,
        downloader: mapValueOfType<String>(json, r'downloader')!,
        active: mapValueOfType<int>(json, r'active')!,
        downloading: mapValueOfType<int>(json, r'downloading')!,
        activeSize: mapValueOfType<int>(json, r'active_size')!,
        today: AppBrushStat.fromJson(json[r'today'])!,
        total: AppBrushStat.fromJson(json[r'total'])!,
      );
    }
    return null;
  }

  static List<AppBrushTask> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppBrushTask>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppBrushTask.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppBrushTask> mapFromJson(dynamic json) {
    final map = <String, AppBrushTask>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppBrushTask.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppBrushTask-objects as value to a dart map
  static Map<String, List<AppBrushTask>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppBrushTask>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppBrushTask.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'name',
    'enabled',
    'site',
    'site_enabled',
    'downloader',
    'active',
    'downloading',
    'active_size',
    'today',
    'total',
  };
}

