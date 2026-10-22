//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class AppAssistantConversationDetail {
  /// Returns a new [AppAssistantConversationDetail] instance.
  AppAssistantConversationDetail({
    required this.id,
    required this.title,
    required this.entry,
    required this.running,
    required this.createdAt,
    required this.updatedAt,
    this.messages = const [],
    this.confirmations = const [],
    this.runId,
    this.partial,
    this.partialReasoning,
    required this.lastSeq,
    this.usage,
  });

  int id;

  String title;

  String entry;

  bool running;

  DateTime createdAt;

  DateTime updatedAt;

  List<AppAssistantMessage> messages;

  List<AppAssistantConfirmation> confirmations;

  /// 进行中的运行
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? runId;

  /// 进行中的运行到现在的回答文字
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? partial;

  /// 进行中的运行这一步到现在的推理内容（思考过程）
  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  String? partialReasoning;

  /// 读事件的 after：从它接着读不会漏
  int lastSeq;

  ///
  /// Please note: This property should have been non-nullable! Since the specification file
  /// does not include a default value (using the "default:" property), however, the generated
  /// source code must fall back to having a nullable type.
  /// Consider adding a "default:" property in the specification file to hide this note.
  ///
  AppAssistantUsage? usage;

  @override
  bool operator ==(Object other) => identical(this, other) || other is AppAssistantConversationDetail &&
    other.id == id &&
    other.title == title &&
    other.entry == entry &&
    other.running == running &&
    other.createdAt == createdAt &&
    other.updatedAt == updatedAt &&
    _deepEquality.equals(other.messages, messages) &&
    _deepEquality.equals(other.confirmations, confirmations) &&
    other.runId == runId &&
    other.partial == partial &&
    other.partialReasoning == partialReasoning &&
    other.lastSeq == lastSeq &&
    other.usage == usage;

  @override
  int get hashCode =>
    // ignore: unnecessary_parenthesis
    (id.hashCode) +
    (title.hashCode) +
    (entry.hashCode) +
    (running.hashCode) +
    (createdAt.hashCode) +
    (updatedAt.hashCode) +
    (messages.hashCode) +
    (confirmations.hashCode) +
    (runId == null ? 0 : runId!.hashCode) +
    (partial == null ? 0 : partial!.hashCode) +
    (partialReasoning == null ? 0 : partialReasoning!.hashCode) +
    (lastSeq.hashCode) +
    (usage == null ? 0 : usage!.hashCode);

  @override
  String toString() => 'AppAssistantConversationDetail[id=$id, title=$title, entry=$entry, running=$running, createdAt=$createdAt, updatedAt=$updatedAt, messages=$messages, confirmations=$confirmations, runId=$runId, partial=$partial, partialReasoning=$partialReasoning, lastSeq=$lastSeq, usage=$usage]';

  Map<String, dynamic> toJson() {
    final json = <String, dynamic>{};
      json[r'id'] = this.id;
      json[r'title'] = this.title;
      json[r'entry'] = this.entry;
      json[r'running'] = this.running;
      json[r'created_at'] = this.createdAt.toUtc().toIso8601String();
      json[r'updated_at'] = this.updatedAt.toUtc().toIso8601String();
      json[r'messages'] = this.messages;
      json[r'confirmations'] = this.confirmations;
    if (this.runId != null) {
      json[r'run_id'] = this.runId;
    } else {
      json[r'run_id'] = null;
    }
    if (this.partial != null) {
      json[r'partial'] = this.partial;
    } else {
      json[r'partial'] = null;
    }
    if (this.partialReasoning != null) {
      json[r'partial_reasoning'] = this.partialReasoning;
    } else {
      json[r'partial_reasoning'] = null;
    }
      json[r'last_seq'] = this.lastSeq;
    if (this.usage != null) {
      json[r'usage'] = this.usage;
    } else {
      json[r'usage'] = null;
    }
    return json;
  }

  /// Returns a new [AppAssistantConversationDetail] instance and imports its values from
  /// [value] if it's a [Map], null otherwise.
  // ignore: prefer_constructors_over_static_methods
  static AppAssistantConversationDetail? fromJson(dynamic value) {
    if (value is Map) {
      final json = value.cast<String, dynamic>();

      // Ensure that the map contains the required keys.
      // Note 1: the values aren't checked for validity beyond being non-null.
      // Note 2: this code is stripped in release mode!
      assert(() {
        assert(json.containsKey(r'id'), 'Required key "AppAssistantConversationDetail[id]" is missing from JSON.');
        assert(json[r'id'] != null, 'Required key "AppAssistantConversationDetail[id]" has a null value in JSON.');
        assert(json.containsKey(r'title'), 'Required key "AppAssistantConversationDetail[title]" is missing from JSON.');
        assert(json[r'title'] != null, 'Required key "AppAssistantConversationDetail[title]" has a null value in JSON.');
        assert(json.containsKey(r'entry'), 'Required key "AppAssistantConversationDetail[entry]" is missing from JSON.');
        assert(json[r'entry'] != null, 'Required key "AppAssistantConversationDetail[entry]" has a null value in JSON.');
        assert(json.containsKey(r'running'), 'Required key "AppAssistantConversationDetail[running]" is missing from JSON.');
        assert(json[r'running'] != null, 'Required key "AppAssistantConversationDetail[running]" has a null value in JSON.');
        assert(json.containsKey(r'created_at'), 'Required key "AppAssistantConversationDetail[created_at]" is missing from JSON.');
        assert(json[r'created_at'] != null, 'Required key "AppAssistantConversationDetail[created_at]" has a null value in JSON.');
        assert(json.containsKey(r'updated_at'), 'Required key "AppAssistantConversationDetail[updated_at]" is missing from JSON.');
        assert(json[r'updated_at'] != null, 'Required key "AppAssistantConversationDetail[updated_at]" has a null value in JSON.');
        assert(json.containsKey(r'messages'), 'Required key "AppAssistantConversationDetail[messages]" is missing from JSON.');
        assert(json[r'messages'] != null, 'Required key "AppAssistantConversationDetail[messages]" has a null value in JSON.');
        assert(json.containsKey(r'confirmations'), 'Required key "AppAssistantConversationDetail[confirmations]" is missing from JSON.');
        assert(json[r'confirmations'] != null, 'Required key "AppAssistantConversationDetail[confirmations]" has a null value in JSON.');
        assert(json.containsKey(r'last_seq'), 'Required key "AppAssistantConversationDetail[last_seq]" is missing from JSON.');
        assert(json[r'last_seq'] != null, 'Required key "AppAssistantConversationDetail[last_seq]" has a null value in JSON.');
        return true;
      }());

      return AppAssistantConversationDetail(
        id: mapValueOfType<int>(json, r'id')!,
        title: mapValueOfType<String>(json, r'title')!,
        entry: mapValueOfType<String>(json, r'entry')!,
        running: mapValueOfType<bool>(json, r'running')!,
        createdAt: mapDateTime(json, r'created_at', r'')!,
        updatedAt: mapDateTime(json, r'updated_at', r'')!,
        messages: AppAssistantMessage.listFromJson(json[r'messages']),
        confirmations: AppAssistantConfirmation.listFromJson(json[r'confirmations']),
        runId: mapValueOfType<String>(json, r'run_id'),
        partial: mapValueOfType<String>(json, r'partial'),
        partialReasoning: mapValueOfType<String>(json, r'partial_reasoning'),
        lastSeq: mapValueOfType<int>(json, r'last_seq')!,
        usage: AppAssistantUsage.fromJson(json[r'usage']),
      );
    }
    return null;
  }

  static List<AppAssistantConversationDetail> listFromJson(dynamic json, {bool growable = false,}) {
    final result = <AppAssistantConversationDetail>[];
    if (json is List && json.isNotEmpty) {
      for (final row in json) {
        final value = AppAssistantConversationDetail.fromJson(row);
        if (value != null) {
          result.add(value);
        }
      }
    }
    return result.toList(growable: growable);
  }

  static Map<String, AppAssistantConversationDetail> mapFromJson(dynamic json) {
    final map = <String, AppAssistantConversationDetail>{};
    if (json is Map && json.isNotEmpty) {
      json = json.cast<String, dynamic>(); // ignore: parameter_assignments
      for (final entry in json.entries) {
        final value = AppAssistantConversationDetail.fromJson(entry.value);
        if (value != null) {
          map[entry.key] = value;
        }
      }
    }
    return map;
  }

  // maps a json object with a list of AppAssistantConversationDetail-objects as value to a dart map
  static Map<String, List<AppAssistantConversationDetail>> mapListFromJson(dynamic json, {bool growable = false,}) {
    final map = <String, List<AppAssistantConversationDetail>>{};
    if (json is Map && json.isNotEmpty) {
      // ignore: parameter_assignments
      json = json.cast<String, dynamic>();
      for (final entry in json.entries) {
        map[entry.key] = AppAssistantConversationDetail.listFromJson(entry.value, growable: growable,);
      }
    }
    return map;
  }

  /// The list of required keys that must be present in a JSON.
  static const requiredKeys = <String>{
    'id',
    'title',
    'entry',
    'running',
    'created_at',
    'updated_at',
    'messages',
    'confirmations',
    'last_seq',
  };
}

