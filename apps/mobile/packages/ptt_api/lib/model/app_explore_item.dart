//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppExploreItem {
  /// Returns a new [AppExploreItem] instance.
  AppExploreItem({
    required this.id,
    required this.mediaType,
    required this.title,
    this.originalTitle,
    this.year,
    this.overview,
    this.posterPath,
    this.voteAverage,
    required this.inLibrary,
    required this.subscribed,
    this.subscriptionId,
  });

  int id;

  String mediaType;

  String title;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? originalTitle;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? year;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? overview;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? posterPath;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  double? voteAverage;

  bool inLibrary;

  bool subscribed;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? subscriptionId;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppExploreItem &&
    other.id == id &&
    other.mediaType == mediaType &&
    other.title == title &&
    other.originalTitle == originalTitle &&
    other.year == year &&
    other.overview == overview &&
    other.posterPath == posterPath &&
    other.voteAverage == voteAverage &&
    other.inLibrary == inLibrary &&
    other.subscribed == subscribed &&
    other.subscriptionId == subscriptionId;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (mediaType.hashCode) +
    (title.hashCode) +
    (originalTitle == null ? 0 : originalTitle!.hashCode) +
    (year == null ? 0 : year!.hashCode) +
    (overview == null ? 0 : overview!.hashCode) +
    (posterPath == null ? 0 : posterPath!.hashCode) +
    (voteAverage == null ? 0 : voteAverage!.hashCode) +
    (inLibrary.hashCode) +
    (subscribed.hashCode) +
    (subscriptionId == null ? 0 : subscriptionId!.hashCode);

  @override
  String toString() => 'AppExploreItem[id=$id, mediaType=$mediaType, title=$title, originalTitle=$originalTitle, year=$year, overview=$overview, posterPath=$posterPath, voteAverage=$voteAverage, inLibrary=$inLibrary, subscribed=$subscribed, subscriptionId=$subscriptionId]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'media_type'] = this.mediaType;
      json[r'title'] = this.title;
    if (this.originalTitle != null) {
      json[r'original_title'] = this.originalTitle;
    } else {
      json[r'original_title'] = null;
    }
    if (this.year != null) {
      json[r'year'] = this.year;
    } else {
      json[r'year'] = null;
    }
    if (this.overview != null) {
      json[r'overview'] = this.overview;
    } else {
      json[r'overview'] = null;
    }
    if (this.posterPath != null) {
      json[r'poster_path'] = this.posterPath;
    } else {
      json[r'poster_path'] = null;
    }
    if (this.voteAverage != null) {
      json[r'vote_average'] = this.voteAverage;
    } else {
      json[r'vote_average'] = null;
    }
      json[r'in_library'] = this.inLibrary;
      json[r'subscribed'] = this.subscribed;
    if (this.subscriptionId != null) {
      json[r'subscription_id'] = this.subscriptionId;
    } else {
      json[r'subscription_id'] = null;
    }
    return json;
  }

  /// Returns a new [AppExploreItem] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppExploreItem? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppExploreItem[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppExploreItem[id]" has a null value in JSON.');
        assert(json.containsKey(r'media_type'), 'Required key "AppExploreItem[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppExploreItem[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppExploreItem[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppExploreItem[title]" has a null value in JSON.');
        assert(json.containsKey(r'in_library'), 'Required key "AppExploreItem[in_library]" is missing from JSON.');
        assert(json[r'in_library'] != null, 'Required key "AppExploreItem[in_library]" has a null value in JSON.');
        assert(json.containsKey(r'subscribed'), 'Required key "AppExploreItem[subscribed]" is missing from JSON.');
        assert(json[r'subscribed'] != null, 'Required key "AppExploreItem[subscribed]" has a null value in JSON.');
        return true;
      }());

      return AppExploreItem(
        id: mapValueOfType<int>(json, r'id')!,
        mediaType: mapValueOfType<String>(json, r'media_type')!,
        title: mapValueOfType<String>(json, r'title')!,
        originalTitle: mapValueOfType<String>(json, r'original_title'),
        year: mapValueOfType<int>(json, r'year'),
        overview: mapValueOfType<String>(json, r'overview'),
        posterPath: mapValueOfType<String>(json, r'poster_path'),
        voteAverage: mapValueOfType<double>(json, r'vote_average'),
        inLibrary: mapValueOfType<bool>(json, r'in_library')!,
        subscribed: mapValueOfType<bool>(json, r'subscribed')!,
        subscriptionId: mapValueOfType<int>(json, r'subscription_id'),
      );
    }
    return null;
  }

  static List<AppExploreItem> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppExploreItem>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppExploreItem.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppExploreItem> mapFromJson(dynamic json) {
    final map = <String, AppExploreItem>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppExploreItem.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppExploreItem-objects as value to a dart map
  static Map<String, List<AppExploreItem>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppExploreItem>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppExploreItem.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'media_type',
    'title',
    'in_library',
    'subscribed',
  };
}

