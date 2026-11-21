//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppDownloaderLabels {
  /// Returns a new [AppDownloaderLabels] instance.
  AppDownloaderLabels({
    required this.downloaderId,
    required this.downloader,
    required this.type,
    required this.supportsCategory,
    this.categories = const [],
    this.tags = const [],
  });

  int downloaderId;

  /// 下载器的名字
  String downloader;

  /// qbittorrent 或 transmission
  String type;

  /// 有没有分类：Transmission 没有（推送时给的分类会写成第一个 label）
  bool supportsCategory;

  /// qBittorrent 里建好的分类加上种子上用到的分类，排好序；没有分类的下载器是空的
  List<String> categories;

  /// 种子上用到的标签（Transmission 是 labels），排好序
  List<String> tags;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppDownloaderLabels &&
    other.downloaderId == downloaderId &&
    other.downloader == downloader &&
    other.type == type &&
    other.supportsCategory == supportsCategory &&
    _deepEquality.equals(other.categories, categories) &&
    _deepEquality.equals(other.tags, tags);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (downloaderId.hashCode) +
    (downloader.hashCode) +
    (type.hashCode) +
    (supportsCategory.hashCode) +
    (categories.hashCode) +
    (tags.hashCode);

  @override
  String toString() => 'AppDownloaderLabels[downloaderId=$downloaderId, downloader=$downloader, type=$type, supportsCategory=$supportsCategory, categories=$categories, tags=$tags]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'downloader_id'] = this.downloaderId;
      json[r'downloader'] = this.downloader;
      json[r'type'] = this.type;
      json[r'supports_category'] = this.supportsCategory;
      json[r'categories'] = this.categories;
      json[r'tags'] = this.tags;
    return json;
  }

  /// Returns a new [AppDownloaderLabels] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppDownloaderLabels? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'downloader_id'), 'Required key "AppDownloaderLabels[downloader_id]" is missing from JSON.');
        assert(json[r'downloader_id'] != null, 'Required key "AppDownloaderLabels[downloader_id]" has a null value in JSON.');
        assert(json.containsKey(r'downloader'), 'Required key "AppDownloaderLabels[downloader]" is missing from JSON.');
        assert(json[r'downloader'] != null, 'Required key "AppDownloaderLabels[downloader]" has a null value in JSON.');
        assert(json.containsKey(r'type'), 'Required key "AppDownloaderLabels[type]" is missing from JSON.');
        assert(json[r'type'] != null, 'Required key "AppDownloaderLabels[type]" has a null value in JSON.');
        assert(json.containsKey(r'supports_category'), 'Required key "AppDownloaderLabels[supports_category]" is missing from JSON.');
        assert(json[r'supports_category'] != null, 'Required key "AppDownloaderLabels[supports_category]" has a null value in JSON.');
        assert(json.containsKey(r'categories'), 'Required key "AppDownloaderLabels[categories]" is missing from JSON.');
        assert(json[r'categories'] != null, 'Required key "AppDownloaderLabels[categories]" has a null value in JSON.');
        assert(json.containsKey(r'tags'), 'Required key "AppDownloaderLabels[tags]" is missing from JSON.');
        assert(json[r'tags'] != null, 'Required key "AppDownloaderLabels[tags]" has a null value in JSON.');
        return true;
      }());

      return AppDownloaderLabels(
        downloaderId: mapValueOfType<int>(json, r'downloader_id')!,
        downloader: mapValueOfType<String>(json, r'downloader')!,
        type: mapValueOfType<String>(json, r'type')!,
        supportsCategory: mapValueOfType<bool>(json, r'supports_category')!,
        categories: json[r'categories'] is Iterable
            ? (json[r'categories'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        tags: json[r'tags'] is Iterable
            ? (json[r'tags'] as Iterable).cast<String>().toList(growable: false)
            : const [],
      );
    }
    return null;
  }

  static List<AppDownloaderLabels> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppDownloaderLabels>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppDownloaderLabels.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppDownloaderLabels> mapFromJson(dynamic json) {
    final map = <String, AppDownloaderLabels>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppDownloaderLabels.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppDownloaderLabels-objects as value to a dart map
  static Map<String, List<AppDownloaderLabels>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppDownloaderLabels>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppDownloaderLabels.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'downloader_id',
    'downloader',
    'type',
    'supports_category',
    'categories',
    'tags',
  };
}

