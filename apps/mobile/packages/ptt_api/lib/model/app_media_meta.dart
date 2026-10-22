//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppMediaMeta {
  /// Returns a new [AppMediaMeta] instance.
  AppMediaMeta({
    this.nameCn,
    this.nameEn,
    this.year,
    this.type,
    this.season,
    this.seasonEnd,
    this.episode,
    this.episodeEnd,
    this.complete,
    this.resolution,
    this.source_,
    this.videoCodec,
    this.hdr = const [],
    this.audio = const [],
    this.group,
    this.edition = const [],
  });

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? nameCn;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? nameEn;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? year;

  /// movie 或 tv
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? type;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? season;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? seasonEnd;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? episode;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? episodeEnd;

  /// 整季（或多季）的合集
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  bool? complete;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? resolution;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? source_;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? videoCodec;

  List<String> hdr;

  List<String> audio;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? group;

  List<String> edition;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppMediaMeta &&
    other.nameCn == nameCn &&
    other.nameEn == nameEn &&
    other.year == year &&
    other.type == type &&
    other.season == season &&
    other.seasonEnd == seasonEnd &&
    other.episode == episode &&
    other.episodeEnd == episodeEnd &&
    other.complete == complete &&
    other.resolution == resolution &&
    other.source_ == source_ &&
    other.videoCodec == videoCodec &&
    _deepEquality.equals(other.hdr, hdr) &&
    _deepEquality.equals(other.audio, audio) &&
    other.group == group &&
    _deepEquality.equals(other.edition, edition);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (nameCn == null ? 0 : nameCn!.hashCode) +
    (nameEn == null ? 0 : nameEn!.hashCode) +
    (year == null ? 0 : year!.hashCode) +
    (type == null ? 0 : type!.hashCode) +
    (season == null ? 0 : season!.hashCode) +
    (seasonEnd == null ? 0 : seasonEnd!.hashCode) +
    (episode == null ? 0 : episode!.hashCode) +
    (episodeEnd == null ? 0 : episodeEnd!.hashCode) +
    (complete == null ? 0 : complete!.hashCode) +
    (resolution == null ? 0 : resolution!.hashCode) +
    (source_ == null ? 0 : source_!.hashCode) +
    (videoCodec == null ? 0 : videoCodec!.hashCode) +
    (hdr.hashCode) +
    (audio.hashCode) +
    (group == null ? 0 : group!.hashCode) +
    (edition.hashCode);

  @override
  String toString() => 'AppMediaMeta[nameCn=$nameCn, nameEn=$nameEn, year=$year, type=$type, season=$season, seasonEnd=$seasonEnd, episode=$episode, episodeEnd=$episodeEnd, complete=$complete, resolution=$resolution, source_=$source_, videoCodec=$videoCodec, hdr=$hdr, audio=$audio, group=$group, edition=$edition]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
    if (this.nameCn != null) {
      json[r'name_cn'] = this.nameCn;
    } else {
      json[r'name_cn'] = null;
    }
    if (this.nameEn != null) {
      json[r'name_en'] = this.nameEn;
    } else {
      json[r'name_en'] = null;
    }
    if (this.year != null) {
      json[r'year'] = this.year;
    } else {
      json[r'year'] = null;
    }
    if (this.type != null) {
      json[r'type'] = this.type;
    } else {
      json[r'type'] = null;
    }
    if (this.season != null) {
      json[r'season'] = this.season;
    } else {
      json[r'season'] = null;
    }
    if (this.seasonEnd != null) {
      json[r'season_end'] = this.seasonEnd;
    } else {
      json[r'season_end'] = null;
    }
    if (this.episode != null) {
      json[r'episode'] = this.episode;
    } else {
      json[r'episode'] = null;
    }
    if (this.episodeEnd != null) {
      json[r'episode_end'] = this.episodeEnd;
    } else {
      json[r'episode_end'] = null;
    }
    if (this.complete != null) {
      json[r'complete'] = this.complete;
    } else {
      json[r'complete'] = null;
    }
    if (this.resolution != null) {
      json[r'resolution'] = this.resolution;
    } else {
      json[r'resolution'] = null;
    }
    if (this.source_ != null) {
      json[r'source'] = this.source_;
    } else {
      json[r'source'] = null;
    }
    if (this.videoCodec != null) {
      json[r'video_codec'] = this.videoCodec;
    } else {
      json[r'video_codec'] = null;
    }
      json[r'hdr'] = this.hdr;
      json[r'audio'] = this.audio;
    if (this.group != null) {
      json[r'group'] = this.group;
    } else {
      json[r'group'] = null;
    }
      json[r'edition'] = this.edition;
    return json;
  }

  /// Returns a new [AppMediaMeta] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppMediaMeta? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        return true;
      }());

      return AppMediaMeta(
        nameCn: mapValueOfType<String>(json, r'name_cn'),
        nameEn: mapValueOfType<String>(json, r'name_en'),
        year: mapValueOfType<int>(json, r'year'),
        type: mapValueOfType<String>(json, r'type'),
        season: mapValueOfType<int>(json, r'season'),
        seasonEnd: mapValueOfType<int>(json, r'season_end'),
        episode: mapValueOfType<int>(json, r'episode'),
        episodeEnd: mapValueOfType<int>(json, r'episode_end'),
        complete: mapValueOfType<bool>(json, r'complete'),
        resolution: mapValueOfType<String>(json, r'resolution'),
        source_: mapValueOfType<String>(json, r'source'),
        videoCodec: mapValueOfType<String>(json, r'video_codec'),
        hdr: json[r'hdr'] is Iterable
            ? (json[r'hdr'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        audio: json[r'audio'] is Iterable
            ? (json[r'audio'] as Iterable).cast<String>().toList(growable: false)
            : const [],
        group: mapValueOfType<String>(json, r'group'),
        edition: json[r'edition'] is Iterable
            ? (json[r'edition'] as Iterable).cast<String>().toList(growable: false)
            : const [],
      );
    }
    return null;
  }

  static List<AppMediaMeta> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppMediaMeta>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppMediaMeta.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppMediaMeta> mapFromJson(dynamic json) {
    final map = <String, AppMediaMeta>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppMediaMeta.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppMediaMeta-objects as value to a dart map
  static Map<String, List<AppMediaMeta>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppMediaMeta>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppMediaMeta.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
  };
}

