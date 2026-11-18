//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppFromTorrent {
  /// Returns a new [AppFromTorrent] instance.
  AppFromTorrent({
    required this.site,
    required this.torrentId,
    this.title,
  });

  String site;

  String torrentId;

  /// 种子标题：只在搜索缓存里没有这个种子时，用作在这个站点上搜索的关键字
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? title;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppFromTorrent &&
    other.site == site &&
    other.torrentId == torrentId &&
    other.title == title;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (torrentId.hashCode) +
    (title == null ? 0 : title!.hashCode);

  @override
  String toString() => 'AppFromTorrent[site=$site, torrentId=$torrentId, title=$title]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'torrent_id'] = this.torrentId;
    if (this.title != null) {
      json[r'title'] = this.title;
    } else {
      json[r'title'] = null;
    }
    return json;
  }

  /// Returns a new [AppFromTorrent] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppFromTorrent? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppFromTorrent[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppFromTorrent[site]" has a null value in JSON.');
        assert(json.containsKey(r'torrent_id'), 'Required key "AppFromTorrent[torrent_id]" is missing from JSON.');
        assert(json[r'torrent_id'] != null, 'Required key "AppFromTorrent[torrent_id]" has a null value in JSON.');
        return true;
      }());

      return AppFromTorrent(
        site: mapValueOfType<String>(json, r'site')!,
        torrentId: mapValueOfType<String>(json, r'torrent_id')!,
        title: mapValueOfType<String>(json, r'title'),
      );
    }
    return null;
  }

  static List<AppFromTorrent> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppFromTorrent>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppFromTorrent.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppFromTorrent> mapFromJson(dynamic json) {
    final map = <String, AppFromTorrent>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppFromTorrent.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppFromTorrent-objects as value to a dart map
  static Map<String, List<AppFromTorrent>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppFromTorrent>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppFromTorrent.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'site',
    'torrent_id',
  };
}

