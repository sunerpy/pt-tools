//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppSearchPreviewGroup {
  /// Returns a new [AppSearchPreviewGroup] instance.
  AppSearchPreviewGroup({
    required this.groupId,
    required this.meta,
    this.verdict,
    this.span,
    this.covers = const [],
    this.downloading = const [],
    required this.inLibrary,
    required this.blocked,
    this.members = const [],
  });

  String groupId;

  AppMediaMeta meta;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppSearchPreviewVerdict? verdict;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppSearchPreviewGroupSpan? span;

  /// 能补上的集（缺的与等版本的）
  List<int> covers;

  /// 这一组里正在下载的集
  List<int> downloading;

  /// 这一组的集都已入库（电影是库里有）
  bool inLibrary;

  /// 成员都拉黑了（排在最后）
  bool blocked;

  /// 各站点上的这个资源，第一个是首选（meta 与 verdict 是它的）
  List<AppSearchPreviewMember> members;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppSearchPreviewGroup &&
    other.groupId == groupId &&
    other.meta == meta &&
    other.verdict == verdict &&
    other.span == span &&
    _deepEquality.equals(other.covers, covers) &&
    _deepEquality.equals(other.downloading, downloading) &&
    other.inLibrary == inLibrary &&
    other.blocked == blocked &&
    _deepEquality.equals(other.members, members);

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (groupId.hashCode) +
    (meta.hashCode) +
    (verdict == null ? 0 : verdict!.hashCode) +
    (span == null ? 0 : span!.hashCode) +
    (covers.hashCode) +
    (downloading.hashCode) +
    (inLibrary.hashCode) +
    (blocked.hashCode) +
    (members.hashCode);

  @override
  String toString() => 'AppSearchPreviewGroup[groupId=$groupId, meta=$meta, verdict=$verdict, span=$span, covers=$covers, downloading=$downloading, inLibrary=$inLibrary, blocked=$blocked, members=$members]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'group_id'] = this.groupId;
      json[r'meta'] = this.meta;
    if (this.verdict != null) {
      json[r'verdict'] = this.verdict;
    } else {
      json[r'verdict'] = null;
    }
    if (this.span != null) {
      json[r'span'] = this.span;
    } else {
      json[r'span'] = null;
    }
      json[r'covers'] = this.covers;
      json[r'downloading'] = this.downloading;
      json[r'in_library'] = this.inLibrary;
      json[r'blocked'] = this.blocked;
      json[r'members'] = this.members;
    return json;
  }

  /// Returns a new [AppSearchPreviewGroup] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppSearchPreviewGroup? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'group_id'), 'Required key "AppSearchPreviewGroup[group_id]" is missing from JSON.');
        assert(json[r'group_id'] != null, 'Required key "AppSearchPreviewGroup[group_id]" has a null value in JSON.');
        assert(json.containsKey(r'meta'), 'Required key "AppSearchPreviewGroup[meta]" is missing from JSON.');
        assert(json[r'meta'] != null, 'Required key "AppSearchPreviewGroup[meta]" has a null value in JSON.');
        assert(json.containsKey(r'covers'), 'Required key "AppSearchPreviewGroup[covers]" is missing from JSON.');
        assert(json[r'covers'] != null, 'Required key "AppSearchPreviewGroup[covers]" has a null value in JSON.');
        assert(json.containsKey(r'downloading'), 'Required key "AppSearchPreviewGroup[downloading]" is missing from JSON.');
        assert(json[r'downloading'] != null, 'Required key "AppSearchPreviewGroup[downloading]" has a null value in JSON.');
        assert(json.containsKey(r'in_library'), 'Required key "AppSearchPreviewGroup[in_library]" is missing from JSON.');
        assert(json[r'in_library'] != null, 'Required key "AppSearchPreviewGroup[in_library]" has a null value in JSON.');
        assert(json.containsKey(r'blocked'), 'Required key "AppSearchPreviewGroup[blocked]" is missing from JSON.');
        assert(json[r'blocked'] != null, 'Required key "AppSearchPreviewGroup[blocked]" has a null value in JSON.');
        assert(json.containsKey(r'members'), 'Required key "AppSearchPreviewGroup[members]" is missing from JSON.');
        assert(json[r'members'] != null, 'Required key "AppSearchPreviewGroup[members]" has a null value in JSON.');
        return true;
      }());

      return AppSearchPreviewGroup(
        groupId: mapValueOfType<String>(json, r'group_id')!,
        meta: AppMediaMeta.fromJson(json[r'meta'])!,
        verdict: AppSearchPreviewVerdict.fromJson(json[r'verdict']),
        span: AppSearchPreviewGroupSpan.fromJson(json[r'span']),
        covers: json[r'covers'] is Iterable
            ? (json[r'covers'] as Iterable).cast<int>().toList(growable: false)
            : const [],
        downloading: json[r'downloading'] is Iterable
            ? (json[r'downloading'] as Iterable).cast<int>().toList(growable: false)
            : const [],
        inLibrary: mapValueOfType<bool>(json, r'in_library')!,
        blocked: mapValueOfType<bool>(json, r'blocked')!,
        members: AppSearchPreviewMember.listFromJson(json[r'members']),
      );
    }
    return null;
  }

  static List<AppSearchPreviewGroup> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppSearchPreviewGroup>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppSearchPreviewGroup.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppSearchPreviewGroup> mapFromJson(dynamic json) {
    final map = <String, AppSearchPreviewGroup>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppSearchPreviewGroup.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppSearchPreviewGroup-objects as value to a dart map
  static Map<String, List<AppSearchPreviewGroup>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppSearchPreviewGroup>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppSearchPreviewGroup.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'group_id',
    'meta',
    'covers',
    'downloading',
    'in_library',
    'blocked',
    'members',
  };
}

