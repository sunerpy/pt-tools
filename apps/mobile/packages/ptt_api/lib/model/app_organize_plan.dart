//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppOrganizePlan {
  /// Returns a new [AppOrganizePlan] instance.
  AppOrganizePlan({
    required this.downloaderId,
    required this.downloader,
    required this.hash,
    required this.name,
    this.match,
    this.library_,
    this.category,
    this.mode,
    this.items = const [],
    required this.skipped,
    this.problem,
  });

  int downloaderId;

  String downloader;

  String hash;

  /// 种子名
  String name;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppMediaMatch? match;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppLibraryRef? library_;

  /// 放进的二级分类
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? category;

  /// 整理方式
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? mode;

  List<AppOrganizeItem> items;

  /// 不整理的文件（样片、太小的、不是视频的）有几个
  int skipped;

  /// 不为空时整个种子整理不了
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? problem;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppOrganizePlan &&
    other.downloaderId == downloaderId &&
    other.downloader == downloader &&
    other.hash == hash &&
    other.name == name &&
    other.match == match &&
    other.library_ == library_ &&
    other.category == category &&
    other.mode == mode &&
    _deepEquality.equals(other.items, items) &&
    other.skipped == skipped &&
    other.problem == problem;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (downloaderId.hashCode) +
    (downloader.hashCode) +
    (hash.hashCode) +
    (name.hashCode) +
    (match == null ? 0 : match!.hashCode) +
    (library_ == null ? 0 : library_!.hashCode) +
    (category == null ? 0 : category!.hashCode) +
    (mode == null ? 0 : mode!.hashCode) +
    (items.hashCode) +
    (skipped.hashCode) +
    (problem == null ? 0 : problem!.hashCode);

  @override
  String toString() => 'AppOrganizePlan[downloaderId=$downloaderId, downloader=$downloader, hash=$hash, name=$name, match=$match, library_=$library_, category=$category, mode=$mode, items=$items, skipped=$skipped, problem=$problem]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'downloader_id'] = this.downloaderId;
      json[r'downloader'] = this.downloader;
      json[r'hash'] = this.hash;
      json[r'name'] = this.name;
    if (this.match != null) {
      json[r'match'] = this.match;
    } else {
      json[r'match'] = null;
    }
    if (this.library_ != null) {
      json[r'library'] = this.library_;
    } else {
      json[r'library'] = null;
    }
    if (this.category != null) {
      json[r'category'] = this.category;
    } else {
      json[r'category'] = null;
    }
    if (this.mode != null) {
      json[r'mode'] = this.mode;
    } else {
      json[r'mode'] = null;
    }
      json[r'items'] = this.items;
      json[r'skipped'] = this.skipped;
    if (this.problem != null) {
      json[r'problem'] = this.problem;
    } else {
      json[r'problem'] = null;
    }
    return json;
  }

  /// Returns a new [AppOrganizePlan] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppOrganizePlan? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'downloader_id'), 'Required key "AppOrganizePlan[downloader_id]" is missing from JSON.');
        assert(json[r'downloader_id'] != null, 'Required key "AppOrganizePlan[downloader_id]" has a null value in JSON.');
        assert(json.containsKey(r'downloader'), 'Required key "AppOrganizePlan[downloader]" is missing from JSON.');
        assert(json[r'downloader'] != null, 'Required key "AppOrganizePlan[downloader]" has a null value in JSON.');
        assert(json.containsKey(r'hash'), 'Required key "AppOrganizePlan[hash]" is missing from JSON.');
        assert(json[r'hash'] != null, 'Required key "AppOrganizePlan[hash]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppOrganizePlan[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppOrganizePlan[name]" has a null value in JSON.');
        assert(json.containsKey(r'items'), 'Required key "AppOrganizePlan[items]" is missing from JSON.');
        assert(json[r'items'] != null, 'Required key "AppOrganizePlan[items]" has a null value in JSON.');
        assert(json.containsKey(r'skipped'), 'Required key "AppOrganizePlan[skipped]" is missing from JSON.');
        assert(json[r'skipped'] != null, 'Required key "AppOrganizePlan[skipped]" has a null value in JSON.');
        return true;
      }());

      return AppOrganizePlan(
        downloaderId: mapValueOfType<int>(json, r'downloader_id')!,
        downloader: mapValueOfType<String>(json, r'downloader')!,
        hash: mapValueOfType<String>(json, r'hash')!,
        name: mapValueOfType<String>(json, r'name')!,
        match: AppMediaMatch.fromJson(json[r'match']),
        library_: AppLibraryRef.fromJson(json[r'library']),
        category: mapValueOfType<String>(json, r'category'),
        mode: mapValueOfType<String>(json, r'mode'),
        items: AppOrganizeItem.listFromJson(json[r'items']),
        skipped: mapValueOfType<int>(json, r'skipped')!,
        problem: mapValueOfType<String>(json, r'problem'),
      );
    }
    return null;
  }

  static List<AppOrganizePlan> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppOrganizePlan>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppOrganizePlan.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppOrganizePlan> mapFromJson(dynamic json) {
    final map = <String, AppOrganizePlan>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppOrganizePlan.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppOrganizePlan-objects as value to a dart map
  static Map<String, List<AppOrganizePlan>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppOrganizePlan>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppOrganizePlan.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'downloader_id',
    'downloader',
    'hash',
    'name',
    'items',
    'skipped',
  };
}

