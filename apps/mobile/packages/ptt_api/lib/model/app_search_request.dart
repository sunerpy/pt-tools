//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchRequest {
  /// Returns a new [AppSearchRequest] instance.
  AppSearchRequest({
    required this.keyword,
    this.sites = const [],
    this.category,
    this.freeOnly,
    this.minSeeders,
  });

  String keyword;

  List<String> sites;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? category;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? freeOnly;

  /// Minimum value: 0
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? minSeeders;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchRequest &&
    other.keyword == keyword &&
    _deepEquality.equals(other.sites, sites) &&
    other.category == category &&
    other.freeOnly == freeOnly &&
    other.minSeeders == minSeeders;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (keyword.hashCode) +
    (sites.hashCode) +
    (category == null ? 0 : category!.hashCode) +
    (freeOnly == null ? 0 : freeOnly!.hashCode) +
    (minSeeders == null ? 0 : minSeeders!.hashCode);

  @override
  String toString() => 'AppSearchRequest[keyword=$keyword, sites=$sites, category=$category, freeOnly=$freeOnly, minSeeders=$minSeeders]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'keyword'] = this.keyword;
      json[r'sites'] = this.sites;
    if (this.category != null) {
      json[r'category'] = this.category;
    } else {
      json[r'category'] = null;
    }
    if (this.freeOnly != null) {
      json[r'free_only'] = this.freeOnly;
    } else {
      json[r'free_only'] = null;
    }
    if (this.minSeeders != null) {
      json[r'min_seeders'] = this.minSeeders;
    } else {
      json[r'min_seeders'] = null;
    }
    return json;
  }

  /// Returns a new [AppSearchRequest] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchRequest? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'keyword'), 'Required key "AppSearchRequest[keyword]" is missing from JSON.');
        assert(json[r'keyword'] != null, 'Required key "AppSearchRequest[keyword]" has a null value in JSON.');
        return true;
      }());

      return AppSearchRequest(
        keyword: mapValueOfType<String>(json, r'keyword')!,
        sites: json[r'sites'] is Iterable
            ? (json[r'sites'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        category: mapValueOfType<String>(json, r'category'),
        freeOnly: mapValueOfType<bool>(json, r'free_only'),
        minSeeders: mapValueOfType<int>(json, r'min_seeders'),
      );
    }
    return null;
  }

  static List<AppSearchRequest> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchRequest>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchRequest.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchRequest> mapFromJson(dynamic json) {
    final map = <String, AppSearchRequest>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchRequest.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchRequest-objects as value to a dart map
  static Map<String, List<AppSearchRequest>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchRequest>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchRequest.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'keyword',
  };
}

