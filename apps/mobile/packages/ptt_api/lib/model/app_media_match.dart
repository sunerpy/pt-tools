//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMediaMatch {
  /// Returns a new [AppMediaMatch] instance.
  AppMediaMatch({
    required this.id,
    required this.mediaType,
    required this.title,
    this.originalTitle,
    this.year,
    this.overview,
    this.posterPath,
    this.voteAverage,
    this.imdbId,
    this.score,
  });

  /// TMDB 编号
  int id;

  /// movie 或 tv
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

  /// 海报经 GET /images/tmdb/{size}/{file} 取
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
  num? voteAverage;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? imdbId;

  /// 候选的分数
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  num? score;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMediaMatch &&
    other.id == id &&
    other.mediaType == mediaType &&
    other.title == title &&
    other.originalTitle == originalTitle &&
    other.year == year &&
    other.overview == overview &&
    other.posterPath == posterPath &&
    other.voteAverage == voteAverage &&
    other.imdbId == imdbId &&
    other.score == score;

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
    (imdbId == null ? 0 : imdbId!.hashCode) +
    (score == null ? 0 : score!.hashCode);

  @override
  String toString() => 'AppMediaMatch[id=$id, mediaType=$mediaType, title=$title, originalTitle=$originalTitle, year=$year, overview=$overview, posterPath=$posterPath, voteAverage=$voteAverage, imdbId=$imdbId, score=$score]';

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
    if (this.imdbId != null) {
      json[r'imdb_id'] = this.imdbId;
    } else {
      json[r'imdb_id'] = null;
    }
    if (this.score != null) {
      json[r'score'] = this.score;
    } else {
      json[r'score'] = null;
    }
    return json;
  }

  /// Returns a new [AppMediaMatch] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMediaMatch? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppMediaMatch[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppMediaMatch[id]" has a null value in JSON.');
        assert(json.containsKey(r'media_type'), 'Required key "AppMediaMatch[media_type]" is missing from JSON.');
        assert(json[r'media_type'] != null, 'Required key "AppMediaMatch[media_type]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppMediaMatch[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppMediaMatch[title]" has a null value in JSON.');
        return true;
      }());

      return AppMediaMatch(
        id: mapValueOfType<int>(json, r'id')!,
        mediaType: mapValueOfType<String>(json, r'media_type')!,
        title: mapValueOfType<String>(json, r'title')!,
        originalTitle: mapValueOfType<String>(json, r'original_title'),
        year: mapValueOfType<int>(json, r'year'),
        overview: mapValueOfType<String>(json, r'overview'),
        posterPath: mapValueOfType<String>(json, r'poster_path'),
        voteAverage: json[r'vote_average'] == null
            ? null
            : num.parse('${json[r'vote_average']}'),
        imdbId: mapValueOfType<String>(json, r'imdb_id'),
        score: json[r'score'] == null
            ? null
            : num.parse('${json[r'score']}'),
      );
    }
    return null;
  }

  static List<AppMediaMatch> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMediaMatch>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMediaMatch.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMediaMatch> mapFromJson(dynamic json) {
    final map = <String, AppMediaMatch>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMediaMatch.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMediaMatch-objects as value to a dart map
  static Map<String, List<AppMediaMatch>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMediaMatch>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMediaMatch.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'media_type',
    'title',
  };
}

