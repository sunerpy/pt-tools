//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchItem {
  /// Returns a new [AppSearchItem] instance.
  AppSearchItem({
    required this.site,
    required this.torrentId,
    required this.title,
    this.subtitle,
    this.infoHash,
    required this.size,
    required this.seeders,
    required this.leechers,
    this.snatched,
    this.uploadedAt,
    this.category,
    this.tags = const [],
    this.discount,
    this.discountEndAt,
    required this.free,
    required this.hasHr,
    this.imdbId,
    this.doubanId,
  });

  String site;

  String torrentId;

  String title;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? subtitle;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? infoHash;

  int size;

  int seeders;

  int leechers;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? snatched;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? uploadedAt;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? category;

  List<String> tags;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? discount;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? discountEndAt;

  bool free;

  bool hasHr;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? imdbId;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? doubanId;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchItem &&
    other.site == site &&
    other.torrentId == torrentId &&
    other.title == title &&
    other.subtitle == subtitle &&
    other.infoHash == infoHash &&
    other.size == size &&
    other.seeders == seeders &&
    other.leechers == leechers &&
    other.snatched == snatched &&
    other.uploadedAt == uploadedAt &&
    other.category == category &&
    _deepEquality.equals(other.tags, tags) &&
    other.discount == discount &&
    other.discountEndAt == discountEndAt &&
    other.free == free &&
    other.hasHr == hasHr &&
    other.imdbId == imdbId &&
    other.doubanId == doubanId;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (torrentId.hashCode) +
    (title.hashCode) +
    (subtitle == null ? 0 : subtitle!.hashCode) +
    (infoHash == null ? 0 : infoHash!.hashCode) +
    (size.hashCode) +
    (seeders.hashCode) +
    (leechers.hashCode) +
    (snatched == null ? 0 : snatched!.hashCode) +
    (uploadedAt == null ? 0 : uploadedAt!.hashCode) +
    (category == null ? 0 : category!.hashCode) +
    (tags.hashCode) +
    (discount == null ? 0 : discount!.hashCode) +
    (discountEndAt == null ? 0 : discountEndAt!.hashCode) +
    (free.hashCode) +
    (hasHr.hashCode) +
    (imdbId == null ? 0 : imdbId!.hashCode) +
    (doubanId == null ? 0 : doubanId!.hashCode);

  @override
  String toString() => 'AppSearchItem[site=$site, torrentId=$torrentId, title=$title, subtitle=$subtitle, infoHash=$infoHash, size=$size, seeders=$seeders, leechers=$leechers, snatched=$snatched, uploadedAt=$uploadedAt, category=$category, tags=$tags, discount=$discount, discountEndAt=$discountEndAt, free=$free, hasHr=$hasHr, imdbId=$imdbId, doubanId=$doubanId]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'torrent_id'] = this.torrentId;
      json[r'title'] = this.title;
    if (this.subtitle != null) {
      json[r'subtitle'] = this.subtitle;
    } else {
      json[r'subtitle'] = null;
    }
    if (this.infoHash != null) {
      json[r'info_hash'] = this.infoHash;
    } else {
      json[r'info_hash'] = null;
    }
      json[r'size'] = this.size;
      json[r'seeders'] = this.seeders;
      json[r'leechers'] = this.leechers;
    if (this.snatched != null) {
      json[r'snatched'] = this.snatched;
    } else {
      json[r'snatched'] = null;
    }
    if (this.uploadedAt != null) {
      json[r'uploaded_at'] = this.uploadedAt;
    } else {
      json[r'uploaded_at'] = null;
    }
    if (this.category != null) {
      json[r'category'] = this.category;
    } else {
      json[r'category'] = null;
    }
      json[r'tags'] = this.tags;
    if (this.discount != null) {
      json[r'discount'] = this.discount;
    } else {
      json[r'discount'] = null;
    }
    if (this.discountEndAt != null) {
      json[r'discount_end_at'] = this.discountEndAt;
    } else {
      json[r'discount_end_at'] = null;
    }
      json[r'free'] = this.free;
      json[r'has_hr'] = this.hasHr;
    if (this.imdbId != null) {
      json[r'imdb_id'] = this.imdbId;
    } else {
      json[r'imdb_id'] = null;
    }
    if (this.doubanId != null) {
      json[r'douban_id'] = this.doubanId;
    } else {
      json[r'douban_id'] = null;
    }
    return json;
  }

  /// Returns a new [AppSearchItem] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchItem? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppSearchItem[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppSearchItem[site]" has a null value in JSON.');
        assert(json.containsKey(r'torrent_id'), 'Required key "AppSearchItem[torrent_id]" is missing from JSON.');
        assert(json[r'torrent_id'] != null, 'Required key "AppSearchItem[torrent_id]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppSearchItem[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppSearchItem[title]" has a null value in JSON.');
        assert(json.containsKey(r'size'), 'Required key "AppSearchItem[size]" is missing from JSON.');
        assert(json[r'size'] != null, 'Required key "AppSearchItem[size]" has a null value in JSON.');
        assert(json.containsKey(r'seeders'), 'Required key "AppSearchItem[seeders]" is missing from JSON.');
        assert(json[r'seeders'] != null, 'Required key "AppSearchItem[seeders]" has a null value in JSON.');
        assert(json.containsKey(r'leechers'), 'Required key "AppSearchItem[leechers]" is missing from JSON.');
        assert(json[r'leechers'] != null, 'Required key "AppSearchItem[leechers]" has a null value in JSON.');
        assert(json.containsKey(r'free'), 'Required key "AppSearchItem[free]" is missing from JSON.');
        assert(json[r'free'] != null, 'Required key "AppSearchItem[free]" has a null value in JSON.');
        assert(json.containsKey(r'has_hr'), 'Required key "AppSearchItem[has_hr]" is missing from JSON.');
        assert(json[r'has_hr'] != null, 'Required key "AppSearchItem[has_hr]" has a null value in JSON.');
        return true;
      }());

      return AppSearchItem(
        site: mapValueOfType<String>(json, r'site')!,
        torrentId: mapValueOfType<String>(json, r'torrent_id')!,
        title: mapValueOfType<String>(json, r'title')!,
        subtitle: mapValueOfType<String>(json, r'subtitle'),
        infoHash: mapValueOfType<String>(json, r'info_hash'),
        size: mapValueOfType<int>(json, r'size')!,
        seeders: mapValueOfType<int>(json, r'seeders')!,
        leechers: mapValueOfType<int>(json, r'leechers')!,
        snatched: mapValueOfType<int>(json, r'snatched'),
        uploadedAt: mapValueOfType<int>(json, r'uploaded_at'),
        category: mapValueOfType<String>(json, r'category'),
        tags: json[r'tags'] is Iterable
            ? (json[r'tags'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        discount: mapValueOfType<String>(json, r'discount'),
        discountEndAt: mapValueOfType<int>(json, r'discount_end_at'),
        free: mapValueOfType<bool>(json, r'free')!,
        hasHr: mapValueOfType<bool>(json, r'has_hr')!,
        imdbId: mapValueOfType<String>(json, r'imdb_id'),
        doubanId: mapValueOfType<String>(json, r'douban_id'),
      );
    }
    return null;
  }

  static List<AppSearchItem> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchItem>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchItem.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchItem> mapFromJson(dynamic json) {
    final map = <String, AppSearchItem>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchItem.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchItem-objects as value to a dart map
  static Map<String, List<AppSearchItem>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchItem>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchItem.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'torrent_id',
    'title',
    'size',
    'seeders',
    'leechers',
    'free',
    'has_hr',
  };
}

