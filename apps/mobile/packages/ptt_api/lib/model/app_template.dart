//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppTemplate {
  /// Returns a new [AppTemplate] instance.
  AppTemplate({
    required this.type,
    required this.group,
    required this.name,
    required this.desc,
    required this.noTitle,
    this.vars = const [],
    required this.sampleTitle,
    required this.sampleText,
    required this.title,
    required this.text,
    required this.customized,
    this.updatedAt,
  });

  String type;

  String group;

  String name;

  String desc;

  /// 只有正文（ChatOps 命令回复与 AI 助手回复）
  bool noTitle;

  List<AppTemplateVar> vars;

  /// 内置标题的样子
  String sampleTitle;

  /// 内置正文的样子
  String sampleText;

  /// 标题模板，空着用内置标题；只给有 app:write 的调用者，只读的调用者是空的
  String title;

  /// 正文模板，空着用内置正文；只给有 app:write 的调用者，只读的调用者是空的
  String text;

  bool customized;

  /// 自定义的时间（秒级时间戳）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  int? updatedAt;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppTemplate &&
    other.type == type &&
    other.group == group &&
    other.name == name &&
    other.desc == desc &&
    other.noTitle == noTitle &&
    _deepEquality.equals(other.vars, vars) &&
    other.sampleTitle == sampleTitle &&
    other.sampleText == sampleText &&
    other.title == title &&
    other.text == text &&
    other.customized == customized &&
    other.updatedAt == updatedAt;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (type.hashCode) +
    (group.hashCode) +
    (name.hashCode) +
    (desc.hashCode) +
    (noTitle.hashCode) +
    (vars.hashCode) +
    (sampleTitle.hashCode) +
    (sampleText.hashCode) +
    (title.hashCode) +
    (text.hashCode) +
    (customized.hashCode) +
    (updatedAt == null ? 0 : updatedAt!.hashCode);

  @override
  String toString() => 'AppTemplate[type=$type, group=$group, name=$name, desc=$desc, noTitle=$noTitle, vars=$vars, sampleTitle=$sampleTitle, sampleText=$sampleText, title=$title, text=$text, customized=$customized, updatedAt=$updatedAt]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'type'] = this.type;
      json[r'group'] = this.group;
      json[r'name'] = this.name;
      json[r'desc'] = this.desc;
      json[r'no_title'] = this.noTitle;
      json[r'vars'] = this.vars;
      json[r'sample_title'] = this.sampleTitle;
      json[r'sample_text'] = this.sampleText;
      json[r'title'] = this.title;
      json[r'text'] = this.text;
      json[r'customized'] = this.customized;
    if (this.updatedAt != null) {
      json[r'updated_at'] = this.updatedAt;
    } else {
      json[r'updated_at'] = null;
    }
    return json;
  }

  /// Returns a new [AppTemplate] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppTemplate? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'type'), 'Required key "AppTemplate[type]" is missing from JSON.');
        assert(json[r'type'] != null, 'Required key "AppTemplate[type]" has a null value in JSON.');
        assert(json.containsKey(r'group'), 'Required key "AppTemplate[group]" is missing from JSON.');
        assert(json[r'group'] != null, 'Required key "AppTemplate[group]" has a null value in JSON.');
        assert(json.containsKey(r'name'), 'Required key "AppTemplate[name]" is missing from JSON.');
        assert(json[r'name'] != null, 'Required key "AppTemplate[name]" has a null value in JSON.');
        assert(json.containsKey(r'desc'), 'Required key "AppTemplate[desc]" is missing from JSON.');
        assert(json[r'desc'] != null, 'Required key "AppTemplate[desc]" has a null value in JSON.');
        assert(json.containsKey(r'no_title'), 'Required key "AppTemplate[no_title]" is missing from JSON.');
        assert(json[r'no_title'] != null, 'Required key "AppTemplate[no_title]" has a null value in JSON.');
        assert(json.containsKey(r'vars'), 'Required key "AppTemplate[vars]" is missing from JSON.');
        assert(json[r'vars'] != null, 'Required key "AppTemplate[vars]" has a null value in JSON.');
        assert(json.containsKey(r'sample_title'), 'Required key "AppTemplate[sample_title]" is missing from JSON.');
        assert(json[r'sample_title'] != null, 'Required key "AppTemplate[sample_title]" has a null value in JSON.');
        assert(json.containsKey(r'sample_text'), 'Required key "AppTemplate[sample_text]" is missing from JSON.');
        assert(json[r'sample_text'] != null, 'Required key "AppTemplate[sample_text]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppTemplate[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppTemplate[title]" has a null value in JSON.');
        assert(json.containsKey(r'text'), 'Required key "AppTemplate[text]" is missing from JSON.');
        assert(json[r'text'] != null, 'Required key "AppTemplate[text]" has a null value in JSON.');
        assert(json.containsKey(r'customized'), 'Required key "AppTemplate[customized]" is missing from JSON.');
        assert(json[r'customized'] != null, 'Required key "AppTemplate[customized]" has a null value in JSON.');
        return true;
      }());

      return AppTemplate(
        type: mapValueOfType<String>(json, r'type')!,
        group: mapValueOfType<String>(json, r'group')!,
        name: mapValueOfType<String>(json, r'name')!,
        desc: mapValueOfType<String>(json, r'desc')!,
        noTitle: mapValueOfType<bool>(json, r'no_title')!,
        vars: AppTemplateVar.listFromJson(json[r'vars']),
        sampleTitle: mapValueOfType<String>(json, r'sample_title')!,
        sampleText: mapValueOfType<String>(json, r'sample_text')!,
        title: mapValueOfType<String>(json, r'title')!,
        text: mapValueOfType<String>(json, r'text')!,
        customized: mapValueOfType<bool>(json, r'customized')!,
        updatedAt: mapValueOfType<int>(json, r'updated_at'),
      );
    }
    return null;
  }

  static List<AppTemplate> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppTemplate>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppTemplate.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppTemplate> mapFromJson(dynamic json) {
    final map = <String, AppTemplate>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppTemplate.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppTemplate-objects as value to a dart map
  static Map<String, List<AppTemplate>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppTemplate>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppTemplate.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'type',
    'group',
    'name',
    'desc',
    'no_title',
    'vars',
    'sample_title',
    'sample_text',
    'title',
    'text',
    'customized',
  };
}

