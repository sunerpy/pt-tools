//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTorrentPage {
  /// Returns a new [AppTorrentPage] instance.
  AppTorrentPage({
    this.items = const [],
    required this.total,
    required this.page,
    required this.pageSize,
    this.failures = const [],
  });

  List<AppTorrent> items;

  int total;

  int page;

  int pageSize;

  List<AppDownloaderFailure> failures;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTorrentPage &&
    _deepEquality.equals(other.items, items) &&
    other.total == total &&
    other.page == page &&
    other.pageSize == pageSize &&
    _deepEquality.equals(other.failures, failures);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (items.hashCode) +
    (total.hashCode) +
    (page.hashCode) +
    (pageSize.hashCode) +
    (failures.hashCode);

  @override
  String toString() => 'AppTorrentPage[items=$items, total=$total, page=$page, pageSize=$pageSize, failures=$failures]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'items'] = this.items;
      json[r'total'] = this.total;
      json[r'page'] = this.page;
      json[r'page_size'] = this.pageSize;
      json[r'failures'] = this.failures;
    return json;
  }

  /// Returns a new [AppTorrentPage] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTorrentPage? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'items'), 'Required key "AppTorrentPage[items]" is missing from JSON.');
        assert(json[r'items'] != null, 'Required key "AppTorrentPage[items]" has a null value in JSON.');
        assert(json.containsKey(r'total'), 'Required key "AppTorrentPage[total]" is missing from JSON.');
        assert(json[r'total'] != null, 'Required key "AppTorrentPage[total]" has a null value in JSON.');
        assert(json.containsKey(r'page'), 'Required key "AppTorrentPage[page]" is missing from JSON.');
        assert(json[r'page'] != null, 'Required key "AppTorrentPage[page]" has a null value in JSON.');
        assert(json.containsKey(r'page_size'), 'Required key "AppTorrentPage[page_size]" is missing from JSON.');
        assert(json[r'page_size'] != null, 'Required key "AppTorrentPage[page_size]" has a null value in JSON.');
        assert(json.containsKey(r'failures'), 'Required key "AppTorrentPage[failures]" is missing from JSON.');
        assert(json[r'failures'] != null, 'Required key "AppTorrentPage[failures]" has a null value in JSON.');
        return true;
      }());

      return AppTorrentPage(
        items: AppTorrent.listFromJson(json[r'items']),
        total: mapValueOfType<int>(json, r'total')!,
        page: mapValueOfType<int>(json, r'page')!,
        pageSize: mapValueOfType<int>(json, r'page_size')!,
        failures: AppDownloaderFailure.listFromJson(json[r'failures']),
      );
    }
    return null;
  }

  static List<AppTorrentPage> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTorrentPage>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTorrentPage.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTorrentPage> mapFromJson(dynamic json) {
    final map = <String, AppTorrentPage>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTorrentPage.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTorrentPage-objects as value to a dart map
  static Map<String, List<AppTorrentPage>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTorrentPage>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTorrentPage.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'items',
    'total',
    'page',
    'page_size',
    'failures',
  };
}

