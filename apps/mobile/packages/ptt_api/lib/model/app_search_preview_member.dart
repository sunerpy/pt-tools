//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewMember {
  /// Returns a new [AppSearchPreviewMember] instance.
  AppSearchPreviewMember({
    required this.site,
    required this.torrentId,
    required this.title,
    required this.subtitle,
    required this.sizeBytes,
    required this.seeders,
    required this.leechers,
    required this.snatched,
    required this.uploadedAt,
    required this.discount,
    required this.free,
    required this.freeLeftSeconds,
    required this.hr,
    required this.hrHours,
    required this.category,
    required this.byImdb,
    required this.blocked,
    this.verdict,
  });

  String site;

  String torrentId;

  String title;

  String subtitle;

  int sizeBytes;

  int seeders;

  int leechers;

  int snatched;

  int uploadedAt;

  /// 站点的优惠原样
  String discount;

  bool free;

  /// 免费还剩几秒；不知道结束时间时为 0
  int freeLeftSeconds;

  /// 有 H&R 要求（种子自己有，或者站点整站有）
  bool hr;

  /// 要做种多少小时，不知道时为 0
  int hrHours;

  String category;

  /// 按 IMDb 编号对上的（不是按名字）
  bool byImdb;

  /// 拉黑了
  bool blocked;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppSearchPreviewVerdict? verdict;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewMember &&
    other.site == site &&
    other.torrentId == torrentId &&
    other.title == title &&
    other.subtitle == subtitle &&
    other.sizeBytes == sizeBytes &&
    other.seeders == seeders &&
    other.leechers == leechers &&
    other.snatched == snatched &&
    other.uploadedAt == uploadedAt &&
    other.discount == discount &&
    other.free == free &&
    other.freeLeftSeconds == freeLeftSeconds &&
    other.hr == hr &&
    other.hrHours == hrHours &&
    other.category == category &&
    other.byImdb == byImdb &&
    other.blocked == blocked &&
    other.verdict == verdict;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (torrentId.hashCode) +
    (title.hashCode) +
    (subtitle.hashCode) +
    (sizeBytes.hashCode) +
    (seeders.hashCode) +
    (leechers.hashCode) +
    (snatched.hashCode) +
    (uploadedAt.hashCode) +
    (discount.hashCode) +
    (free.hashCode) +
    (freeLeftSeconds.hashCode) +
    (hr.hashCode) +
    (hrHours.hashCode) +
    (category.hashCode) +
    (byImdb.hashCode) +
    (blocked.hashCode) +
    (verdict == null ? 0 : verdict!.hashCode);

  @override
  String toString() => 'AppSearchPreviewMember[site=$site, torrentId=$torrentId, title=$title, subtitle=$subtitle, sizeBytes=$sizeBytes, seeders=$seeders, leechers=$leechers, snatched=$snatched, uploadedAt=$uploadedAt, discount=$discount, free=$free, freeLeftSeconds=$freeLeftSeconds, hr=$hr, hrHours=$hrHours, category=$category, byImdb=$byImdb, blocked=$blocked, verdict=$verdict]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'torrent_id'] = this.torrentId;
      json[r'title'] = this.title;
      json[r'subtitle'] = this.subtitle;
      json[r'size_bytes'] = this.sizeBytes;
      json[r'seeders'] = this.seeders;
      json[r'leechers'] = this.leechers;
      json[r'snatched'] = this.snatched;
      json[r'uploaded_at'] = this.uploadedAt;
      json[r'discount'] = this.discount;
      json[r'free'] = this.free;
      json[r'free_left_seconds'] = this.freeLeftSeconds;
      json[r'hr'] = this.hr;
      json[r'hr_hours'] = this.hrHours;
      json[r'category'] = this.category;
      json[r'by_imdb'] = this.byImdb;
      json[r'blocked'] = this.blocked;
    if (this.verdict != null) {
      json[r'verdict'] = this.verdict;
    } else {
      json[r'verdict'] = null;
    }
    return json;
  }

  /// Returns a new [AppSearchPreviewMember] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewMember? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppSearchPreviewMember[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppSearchPreviewMember[site]" has a null value in JSON.');
        assert(json.containsKey(r'torrent_id'), 'Required key "AppSearchPreviewMember[torrent_id]" is missing from JSON.');
        assert(json[r'torrent_id'] != null, 'Required key "AppSearchPreviewMember[torrent_id]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppSearchPreviewMember[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppSearchPreviewMember[title]" has a null value in JSON.');
        assert(json.containsKey(r'subtitle'), 'Required key "AppSearchPreviewMember[subtitle]" is missing from JSON.');
        assert(json[r'subtitle'] != null, 'Required key "AppSearchPreviewMember[subtitle]" has a null value in JSON.');
        assert(json.containsKey(r'size_bytes'), 'Required key "AppSearchPreviewMember[size_bytes]" is missing from JSON.');
        assert(json[r'size_bytes'] != null, 'Required key "AppSearchPreviewMember[size_bytes]" has a null value in JSON.');
        assert(json.containsKey(r'seeders'), 'Required key "AppSearchPreviewMember[seeders]" is missing from JSON.');
        assert(json[r'seeders'] != null, 'Required key "AppSearchPreviewMember[seeders]" has a null value in JSON.');
        assert(json.containsKey(r'leechers'), 'Required key "AppSearchPreviewMember[leechers]" is missing from JSON.');
        assert(json[r'leechers'] != null, 'Required key "AppSearchPreviewMember[leechers]" has a null value in JSON.');
        assert(json.containsKey(r'snatched'), 'Required key "AppSearchPreviewMember[snatched]" is missing from JSON.');
        assert(json[r'snatched'] != null, 'Required key "AppSearchPreviewMember[snatched]" has a null value in JSON.');
        assert(json.containsKey(r'uploaded_at'), 'Required key "AppSearchPreviewMember[uploaded_at]" is missing from JSON.');
        assert(json[r'uploaded_at'] != null, 'Required key "AppSearchPreviewMember[uploaded_at]" has a null value in JSON.');
        assert(json.containsKey(r'discount'), 'Required key "AppSearchPreviewMember[discount]" is missing from JSON.');
        assert(json[r'discount'] != null, 'Required key "AppSearchPreviewMember[discount]" has a null value in JSON.');
        assert(json.containsKey(r'free'), 'Required key "AppSearchPreviewMember[free]" is missing from JSON.');
        assert(json[r'free'] != null, 'Required key "AppSearchPreviewMember[free]" has a null value in JSON.');
        assert(json.containsKey(r'free_left_seconds'), 'Required key "AppSearchPreviewMember[free_left_seconds]" is missing from JSON.');
        assert(json[r'free_left_seconds'] != null, 'Required key "AppSearchPreviewMember[free_left_seconds]" has a null value in JSON.');
        assert(json.containsKey(r'hr'), 'Required key "AppSearchPreviewMember[hr]" is missing from JSON.');
        assert(json[r'hr'] != null, 'Required key "AppSearchPreviewMember[hr]" has a null value in JSON.');
        assert(json.containsKey(r'hr_hours'), 'Required key "AppSearchPreviewMember[hr_hours]" is missing from JSON.');
        assert(json[r'hr_hours'] != null, 'Required key "AppSearchPreviewMember[hr_hours]" has a null value in JSON.');
        assert(json.containsKey(r'category'), 'Required key "AppSearchPreviewMember[category]" is missing from JSON.');
        assert(json[r'category'] != null, 'Required key "AppSearchPreviewMember[category]" has a null value in JSON.');
        assert(json.containsKey(r'by_imdb'), 'Required key "AppSearchPreviewMember[by_imdb]" is missing from JSON.');
        assert(json[r'by_imdb'] != null, 'Required key "AppSearchPreviewMember[by_imdb]" has a null value in JSON.');
        assert(json.containsKey(r'blocked'), 'Required key "AppSearchPreviewMember[blocked]" is missing from JSON.');
        assert(json[r'blocked'] != null, 'Required key "AppSearchPreviewMember[blocked]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewMember(
        site: mapValueOfType<String>(json, r'site')!,
        torrentId: mapValueOfType<String>(json, r'torrent_id')!,
        title: mapValueOfType<String>(json, r'title')!,
        subtitle: mapValueOfType<String>(json, r'subtitle')!,
        sizeBytes: mapValueOfType<int>(json, r'size_bytes')!,
        seeders: mapValueOfType<int>(json, r'seeders')!,
        leechers: mapValueOfType<int>(json, r'leechers')!,
        snatched: mapValueOfType<int>(json, r'snatched')!,
        uploadedAt: mapValueOfType<int>(json, r'uploaded_at')!,
        discount: mapValueOfType<String>(json, r'discount')!,
        free: mapValueOfType<bool>(json, r'free')!,
        freeLeftSeconds: mapValueOfType<int>(json, r'free_left_seconds')!,
        hr: mapValueOfType<bool>(json, r'hr')!,
        hrHours: mapValueOfType<int>(json, r'hr_hours')!,
        category: mapValueOfType<String>(json, r'category')!,
        byImdb: mapValueOfType<bool>(json, r'by_imdb')!,
        blocked: mapValueOfType<bool>(json, r'blocked')!,
        verdict: AppSearchPreviewVerdict.fromJson(json[r'verdict']),
      );
    }
    return null;
  }

  static List<AppSearchPreviewMember> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewMember>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewMember.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewMember> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewMember>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewMember.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewMember-objects as value to a dart map
  static Map<String, List<AppSearchPreviewMember>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewMember>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewMember.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'torrent_id',
    'title',
    'subtitle',
    'size_bytes',
    'seeders',
    'leechers',
    'snatched',
    'uploaded_at',
    'discount',
    'free',
    'free_left_seconds',
    'hr',
    'hr_hours',
    'category',
    'by_imdb',
    'blocked',
  };
}

