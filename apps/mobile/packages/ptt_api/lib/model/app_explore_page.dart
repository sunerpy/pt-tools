//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppExplorePage {
  /// Returns a new [AppExplorePage] instance.
  AppExplorePage({
    this.items = const [],
    required this.page,
    required this.totalPages,
  });

  List<AppExploreItem> items;

  int page;

  int totalPages;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppExplorePage &&
    _deepEquality.equals(other.items, items) &&
    other.page == page &&
    other.totalPages == totalPages;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (items.hashCode) +
    (page.hashCode) +
    (totalPages.hashCode);

  @override
  String toString() => 'AppExplorePage[items=$items, page=$page, totalPages=$totalPages]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'items'] = this.items;
      json[r'page'] = this.page;
      json[r'total_pages'] = this.totalPages;
    return json;
  }

  /// Returns a new [AppExplorePage] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppExplorePage? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'items'), 'Required key "AppExplorePage[items]" is missing from JSON.');
        assert(json[r'items'] != null, 'Required key "AppExplorePage[items]" has a null value in JSON.');
        assert(json.containsKey(r'page'), 'Required key "AppExplorePage[page]" is missing from JSON.');
        assert(json[r'page'] != null, 'Required key "AppExplorePage[page]" has a null value in JSON.');
        assert(json.containsKey(r'total_pages'), 'Required key "AppExplorePage[total_pages]" is missing from JSON.');
        assert(json[r'total_pages'] != null, 'Required key "AppExplorePage[total_pages]" has a null value in JSON.');
        return true;
      }());

      return AppExplorePage(
        items: AppExploreItem.listFromJson(json[r'items']),
        page: mapValueOfType<int>(json, r'page')!,
        totalPages: mapValueOfType<int>(json, r'total_pages')!,
      );
    }
    return null;
  }

  static List<AppExplorePage> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppExplorePage>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppExplorePage.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppExplorePage> mapFromJson(dynamic json) {
    final map = <String, AppExplorePage>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppExplorePage.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppExplorePage-objects as value to a dart map
  static Map<String, List<AppExplorePage>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppExplorePage>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppExplorePage.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'items',
    'page',
    'total_pages',
  };
}

