//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewUnmatched {
  /// Returns a new [AppSearchPreviewUnmatched] instance.
  AppSearchPreviewUnmatched({
    required this.site,
    required this.torrentId,
    required this.title,
    required this.subtitle,
    required this.reason,
  });

  String site;

  String torrentId;

  String title;

  String subtitle;

  /// 对不上的原因
  String reason;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewUnmatched &&
    other.site == site &&
    other.torrentId == torrentId &&
    other.title == title &&
    other.subtitle == subtitle &&
    other.reason == reason;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (site.hashCode) +
    (torrentId.hashCode) +
    (title.hashCode) +
    (subtitle.hashCode) +
    (reason.hashCode);

  @override
  String toString() => 'AppSearchPreviewUnmatched[site=$site, torrentId=$torrentId, title=$title, subtitle=$subtitle, reason=$reason]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'site'] = this.site;
      json[r'torrent_id'] = this.torrentId;
      json[r'title'] = this.title;
      json[r'subtitle'] = this.subtitle;
      json[r'reason'] = this.reason;
    return json;
  }

  /// Returns a new [AppSearchPreviewUnmatched] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewUnmatched? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'site'), 'Required key "AppSearchPreviewUnmatched[site]" is missing from JSON.');
        assert(json[r'site'] != null, 'Required key "AppSearchPreviewUnmatched[site]" has a null value in JSON.');
        assert(json.containsKey(r'torrent_id'), 'Required key "AppSearchPreviewUnmatched[torrent_id]" is missing from JSON.');
        assert(json[r'torrent_id'] != null, 'Required key "AppSearchPreviewUnmatched[torrent_id]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppSearchPreviewUnmatched[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppSearchPreviewUnmatched[title]" has a null value in JSON.');
        assert(json.containsKey(r'subtitle'), 'Required key "AppSearchPreviewUnmatched[subtitle]" is missing from JSON.');
        assert(json[r'subtitle'] != null, 'Required key "AppSearchPreviewUnmatched[subtitle]" has a null value in JSON.');
        assert(json.containsKey(r'reason'), 'Required key "AppSearchPreviewUnmatched[reason]" is missing from JSON.');
        assert(json[r'reason'] != null, 'Required key "AppSearchPreviewUnmatched[reason]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewUnmatched(
        site: mapValueOfType<String>(json, r'site')!,
        torrentId: mapValueOfType<String>(json, r'torrent_id')!,
        title: mapValueOfType<String>(json, r'title')!,
        subtitle: mapValueOfType<String>(json, r'subtitle')!,
        reason: mapValueOfType<String>(json, r'reason')!,
      );
    }
    return null;
  }

  static List<AppSearchPreviewUnmatched> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewUnmatched>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewUnmatched.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewUnmatched> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewUnmatched>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewUnmatched.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewUnmatched-objects as value to a dart map
  static Map<String, List<AppSearchPreviewUnmatched>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewUnmatched>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewUnmatched.listFromJson(entry.value, growable: growable,);
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
    'reason',
  };
}

