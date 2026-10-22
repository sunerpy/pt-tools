//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMediaLibrary {
  /// Returns a new [AppMediaLibrary] instance.
  AppMediaLibrary({
    required this.id,
    required this.name,
    required this.kind,
    required this.anime,
    required this.path,
    required this.mode,
    required this.scrape,
    required this.classifyDir,
    required this.overwriteMode,
    required this.enabled,
  });

  int id;

  String name;

  /// movie 或 tv
  String kind;

  /// 只收 TMDB 类型里有「动画」的条目
  bool anime;

  /// pt-tools 里看到的库目录
  String path;

  /// 整理方式（hardlink、copy、move 等）；空的是整理设置里的默认方式
  String mode;

  /// 写 NFO 与海报
  bool scrape;

  /// 按二级分类放进库目录下的分类目录
  bool classifyDir;

  /// 目标位置已经有 pt-tools 之前放进库的旧版本时：空的不替换，larger 新文件更大时替换
  String overwriteMode;

  bool enabled;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMediaLibrary &&
    other.id == id &&
    other.name == name &&
    other.kind == kind &&
    other.anime == anime &&
    other.path == path &&
    other.mode == mode &&
    other.scrape == scrape &&
    other.classifyDir == classifyDir &&
    other.overwriteMode == overwriteMode &&
    other.enabled == enabled;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (name.hashCode) +
    (kind.hashCode) +
    (anime.hashCode) +
    (path.hashCode) +
    (mode.hashCode) +
    (scrape.hashCode) +
    (classifyDir.hashCode) +
    (overwriteMode.hashCode) +
    (enabled.hashCode);

  @override
  String toString() => 'AppMediaLibrary[id=$id, name=$name, kind=$kind, anime=$anime, path=$path, mode=$mode, scrape=$scrape, classifyDir=$classifyDir, overwriteMode=$overwriteMode, enabled=$enabled]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'name'] = this.name;
      json[r'kind'] = this.kind;
      json[r'anime'] = this.anime;
      json[r'path'] = this.path;
      json[r'mode'] = this.mode;
      json[r'scrape'] = this.scrape;
      json[r'classify_dir'] = this.classifyDir;
      json[r'overwrite_mode'] = this.overwriteMode;
      json[r'enabled'] = this.enabled;
    return json;
  }

  /// Returns a new [AppMediaLibrary] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMediaLibrary? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppMediaLibrary[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppMediaLibrary[id]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppMediaLibrary[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppMediaLibrary[name]" has a null value in JSON.');
        assert(json.containsKey(r'kind'), 'Required key "AppMediaLibrary[kind]" is missing from JSON.');
        assert(json[r'kind'] != null, 'Required key "AppMediaLibrary[kind]" has a null value in JSON.');
        assert(json.containsKey(r'anime'), 'Required key "AppMediaLibrary[anime]" is missing from JSON.');
        assert(json[r'anime'] != null, 'Required key "AppMediaLibrary[anime]" has a null value in JSON.');
        assert(json.containsKey(r'path'), 'Required key "AppMediaLibrary[path]" is missing from JSON.');
        assert(json[r'path'] != null, 'Required key "AppMediaLibrary[path]" has a null value in JSON.');
        assert(json.containsKey(r'mode'), 'Required key "AppMediaLibrary[mode]" is missing from JSON.');
        assert(json[r'mode'] != null, 'Required key "AppMediaLibrary[mode]" has a null value in JSON.');
        assert(json.containsKey(r'scrape'), 'Required key "AppMediaLibrary[scrape]" is missing from JSON.');
        assert(json[r'scrape'] != null, 'Required key "AppMediaLibrary[scrape]" has a null value in JSON.');
        assert(json.containsKey(r'classify_dir'), 'Required key "AppMediaLibrary[classify_dir]" is missing from JSON.');
        assert(json[r'classify_dir'] != null, 'Required key "AppMediaLibrary[classify_dir]" has a null value in JSON.');
        assert(json.containsKey(r'overwrite_mode'), 'Required key "AppMediaLibrary[overwrite_mode]" is missing from JSON.');
        assert(json[r'overwrite_mode'] != null, 'Required key "AppMediaLibrary[overwrite_mode]" has a null value in JSON.');
        assert(json.containsKey(r'enabled'), 'Required key "AppMediaLibrary[enabled]" is missing from JSON.');
        assert(json[r'enabled'] != null, 'Required key "AppMediaLibrary[enabled]" has a null value in JSON.');
        return true;
      }());

      return AppMediaLibrary(
        id: mapValueOfType<int>(json, r'id')!,
        name: mapValueOfType<String>(json, r'name')!,
        kind: mapValueOfType<String>(json, r'kind')!,
        anime: mapValueOfType<bool>(json, r'anime')!,
        path: mapValueOfType<String>(json, r'path')!,
        mode: mapValueOfType<String>(json, r'mode')!,
        scrape: mapValueOfType<bool>(json, r'scrape')!,
        classifyDir: mapValueOfType<bool>(json, r'classify_dir')!,
        overwriteMode: mapValueOfType<String>(json, r'overwrite_mode')!,
        enabled: mapValueOfType<bool>(json, r'enabled')!,
      );
    }
    return null;
  }

  static List<AppMediaLibrary> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMediaLibrary>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMediaLibrary.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMediaLibrary> mapFromJson(dynamic json) {
    final map = <String, AppMediaLibrary>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMediaLibrary.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMediaLibrary-objects as value to a dart map
  static Map<String, List<AppMediaLibrary>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMediaLibrary>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMediaLibrary.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'name',
    'kind',
    'anime',
    'path',
    'mode',
    'scrape',
    'classify_dir',
    'overwrite_mode',
    'enabled',
  };
}

