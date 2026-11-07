//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreview {
  /// Returns a new [AppSearchPreview] instance.
  AppSearchPreview({
    required this.item,
    this.subscription,
    this.profile,
    this.keywords = const [],
    this.episodes = const [],
    this.movie,
    this.groups = const [],
    required this.total,
    required this.blockedCount,
    required this.unmatchedCount,
    this.unmatched = const [],
    this.errors = const [],
    required this.durationMs,
  });

  AppSearchPreviewItem item;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppSearchPreviewSubscription? subscription;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppSearchPreviewProfile? profile;

  /// 这次在站点上搜的词（按 TMDB 上的名字与订阅的别名）
  List<String> keywords;

  /// 这一季每集现在的情况（电影是空的）
  List<AppSearchPreviewEpisode> episodes;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppSearchPreviewMovie? movie;

  /// 对上这个条目的结果，同一资源一组，排好了序
  List<AppSearchPreviewGroup> groups;

  /// 对上这个条目的种子数（各组成员数之和）
  int total;

  /// 对上了、但拉黑了的种子数
  int blockedCount;

  /// 对不上这个条目的种子数
  int unmatchedCount;

  /// 对不上这个条目的种子（只在请求 include_unmatched 时有，最多 50 条）
  List<AppSearchPreviewUnmatched> unmatched;

  List<AppSearchPreviewError> errors;

  /// 用了多久（毫秒，从收到请求算起）
  int durationMs;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreview &&
    other.item == item &&
    other.subscription == subscription &&
    other.profile == profile &&
    _deepEquality.equals(other.keywords, keywords) &&
    _deepEquality.equals(other.episodes, episodes) &&
    other.movie == movie &&
    _deepEquality.equals(other.groups, groups) &&
    other.total == total &&
    other.blockedCount == blockedCount &&
    other.unmatchedCount == unmatchedCount &&
    _deepEquality.equals(other.unmatched, unmatched) &&
    _deepEquality.equals(other.errors, errors) &&
    other.durationMs == durationMs;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (item.hashCode) +
    (subscription == null ? 0 : subscription!.hashCode) +
    (profile == null ? 0 : profile!.hashCode) +
    (keywords.hashCode) +
    (episodes.hashCode) +
    (movie == null ? 0 : movie!.hashCode) +
    (groups.hashCode) +
    (total.hashCode) +
    (blockedCount.hashCode) +
    (unmatchedCount.hashCode) +
    (unmatched.hashCode) +
    (errors.hashCode) +
    (durationMs.hashCode);

  @override
  String toString() => 'AppSearchPreview[item=$item, subscription=$subscription, profile=$profile, keywords=$keywords, episodes=$episodes, movie=$movie, groups=$groups, total=$total, blockedCount=$blockedCount, unmatchedCount=$unmatchedCount, unmatched=$unmatched, errors=$errors, durationMs=$durationMs]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'item'] = this.item;
    if (this.subscription != null) {
      json[r'subscription'] = this.subscription;
    } else {
      json[r'subscription'] = null;
    }
    if (this.profile != null) {
      json[r'profile'] = this.profile;
    } else {
      json[r'profile'] = null;
    }
      json[r'keywords'] = this.keywords;
      json[r'episodes'] = this.episodes;
    if (this.movie != null) {
      json[r'movie'] = this.movie;
    } else {
      json[r'movie'] = null;
    }
      json[r'groups'] = this.groups;
      json[r'total'] = this.total;
      json[r'blocked_count'] = this.blockedCount;
      json[r'unmatched_count'] = this.unmatchedCount;
      json[r'unmatched'] = this.unmatched;
      json[r'errors'] = this.errors;
      json[r'duration_ms'] = this.durationMs;
    return json;
  }

  /// Returns a new [AppSearchPreview] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreview? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'item'), 'Required key "AppSearchPreview[item]" is missing from JSON.');
        assert(json[r'item'] != null, 'Required key "AppSearchPreview[item]" has a null value in JSON.');
        assert(json.containsKey(r'keywords'), 'Required key "AppSearchPreview[keywords]" is missing from JSON.');
        assert(json[r'keywords'] != null, 'Required key "AppSearchPreview[keywords]" has a null value in JSON.');
        assert(json.containsKey(r'episodes'), 'Required key "AppSearchPreview[episodes]" is missing from JSON.');
        assert(json[r'episodes'] != null, 'Required key "AppSearchPreview[episodes]" has a null value in JSON.');
        assert(json.containsKey(r'groups'), 'Required key "AppSearchPreview[groups]" is missing from JSON.');
        assert(json[r'groups'] != null, 'Required key "AppSearchPreview[groups]" has a null value in JSON.');
        assert(json.containsKey(r'total'), 'Required key "AppSearchPreview[total]" is missing from JSON.');
        assert(json[r'total'] != null, 'Required key "AppSearchPreview[total]" has a null value in JSON.');
        assert(json.containsKey(r'blocked_count'), 'Required key "AppSearchPreview[blocked_count]" is missing from JSON.');
        assert(json[r'blocked_count'] != null, 'Required key "AppSearchPreview[blocked_count]" has a null value in JSON.');
        assert(json.containsKey(r'unmatched_count'), 'Required key "AppSearchPreview[unmatched_count]" is missing from JSON.');
        assert(json[r'unmatched_count'] != null, 'Required key "AppSearchPreview[unmatched_count]" has a null value in JSON.');
        assert(json.containsKey(r'unmatched'), 'Required key "AppSearchPreview[unmatched]" is missing from JSON.');
        assert(json[r'unmatched'] != null, 'Required key "AppSearchPreview[unmatched]" has a null value in JSON.');
        assert(json.containsKey(r'errors'), 'Required key "AppSearchPreview[errors]" is missing from JSON.');
        assert(json[r'errors'] != null, 'Required key "AppSearchPreview[errors]" has a null value in JSON.');
        assert(json.containsKey(r'duration_ms'), 'Required key "AppSearchPreview[duration_ms]" is missing from JSON.');
        assert(json[r'duration_ms'] != null, 'Required key "AppSearchPreview[duration_ms]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreview(
        item: AppSearchPreviewItem.fromJson(json[r'item'])!,
        subscription: AppSearchPreviewSubscription.fromJson(json[r'subscription']),
        profile: AppSearchPreviewProfile.fromJson(json[r'profile']),
        keywords: json[r'keywords'] is Iterable
            ? (json[r'keywords'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        episodes: AppSearchPreviewEpisode.listFromJson(json[r'episodes']),
        movie: AppSearchPreviewMovie.fromJson(json[r'movie']),
        groups: AppSearchPreviewGroup.listFromJson(json[r'groups']),
        total: mapValueOfType<int>(json, r'total')!,
        blockedCount: mapValueOfType<int>(json, r'blocked_count')!,
        unmatchedCount: mapValueOfType<int>(json, r'unmatched_count')!,
        unmatched: AppSearchPreviewUnmatched.listFromJson(json[r'unmatched']),
        errors: AppSearchPreviewError.listFromJson(json[r'errors']),
        durationMs: mapValueOfType<int>(json, r'duration_ms')!,
      );
    }
    return null;
  }

  static List<AppSearchPreview> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreview>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreview.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreview> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreview>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreview.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreview-objects as value to a dart map
  static Map<String, List<AppSearchPreview>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreview>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreview.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'item',
    'keywords',
    'episodes',
    'groups',
    'total',
    'blocked_count',
    'unmatched_count',
    'unmatched',
    'errors',
    'duration_ms',
  };
}

