//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

library openapi.api;

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:collection/collection.dart';
import 'package:http/http.dart';
import 'package:intl/intl.dart';
import 'package:meta/meta.dart';

part 'api_client.dart';
part 'api_helper.dart';
part 'api_exception.dart';
part 'auth/authentication.dart';
part 'auth/api_key_auth.dart';
part 'auth/oauth.dart';
part 'auth/http_basic_auth.dart';
part 'auth/http_bearer_auth.dart';

part 'api/app_api.dart';

part 'model/app_assistant_attachment.dart';
part 'model/app_assistant_change.dart';
part 'model/app_assistant_confirmation.dart';
part 'model/app_assistant_conversation.dart';
part 'model/app_assistant_conversation_detail.dart';
part 'model/app_assistant_create.dart';
part 'model/app_assistant_create_result.dart';
part 'model/app_assistant_decide.dart';
part 'model/app_assistant_event.dart';
part 'model/app_assistant_event_page.dart';
part 'model/app_assistant_message.dart';
part 'model/app_assistant_send.dart';
part 'model/app_assistant_send_result.dart';
part 'model/app_assistant_status.dart';
part 'model/app_assistant_tool_call.dart';
part 'model/app_assistant_usage.dart';
part 'model/app_brush_stat.dart';
part 'model/app_brush_task.dart';
part 'model/app_config_catalog.dart';
part 'model/app_config_change.dart';
part 'model/app_config_diff.dart';
part 'model/app_config_resource.dart';
part 'model/app_config_result.dart';
part 'model/app_config_state.dart';
part 'model/app_delta.dart';
part 'model/app_downloader.dart';
part 'model/app_downloader_failure.dart';
part 'model/app_downloader_list.dart';
part 'model/app_episode.dart';
part 'model/app_episode_rule.dart';
part 'model/app_episode_rule_input.dart';
part 'model/app_episode_skip.dart';
part 'model/app_episode_unlock.dart';
part 'model/app_error.dart';
part 'model/app_explore_item.dart';
part 'model/app_explore_page.dart';
part 'model/app_from_torrent.dart';
part 'model/app_library_ref.dart';
part 'model/app_media_history.dart';
part 'model/app_media_history_page.dart';
part 'model/app_media_library.dart';
part 'model/app_media_match.dart';
part 'model/app_media_meta.dart';
part 'model/app_message.dart';
part 'model/app_meta.dart';
part 'model/app_ok.dart';
part 'model/app_organize_input.dart';
part 'model/app_organize_item.dart';
part 'model/app_organize_plan.dart';
part 'model/app_organize_result.dart';
part 'model/app_overview.dart';
part 'model/app_principal.dart';
part 'model/app_progress.dart';
part 'model/app_push_request.dart';
part 'model/app_push_result.dart';
part 'model/app_recognition.dart';
part 'model/app_recognize_input.dart';
part 'model/app_release.dart';
part 'model/app_release_asset.dart';
part 'model/app_run_step.dart';
part 'model/app_schedule.dart';
part 'model/app_search_error.dart';
part 'model/app_search_item.dart';
part 'model/app_search_request.dart';
part 'model/app_search_result.dart';
part 'model/app_site.dart';
part 'model/app_site_attendance.dart';
part 'model/app_site_delta.dart';
part 'model/app_site_list.dart';
part 'model/app_site_login.dart';
part 'model/app_site_user.dart';
part 'model/app_subscription.dart';
part 'model/app_subscription_create.dart';
part 'model/app_subscription_detail.dart';
part 'model/app_subscription_run.dart';
part 'model/app_subscription_run_page.dart';
part 'model/app_subscription_status.dart';
part 'model/app_subscription_torrent.dart';
part 'model/app_subscription_update.dart';
part 'model/app_task.dart';
part 'model/app_task_page.dart';
part 'model/app_template.dart';
part 'model/app_template_catalog.dart';
part 'model/app_template_group.dart';
part 'model/app_template_input.dart';
part 'model/app_template_preview.dart';
part 'model/app_template_preview_input.dart';
part 'model/app_template_var.dart';
part 'model/app_torrent.dart';
part 'model/app_torrent_action_result.dart';
part 'model/app_torrent_actions_request.dart';
part 'model/app_torrent_actions_result.dart';
part 'model/app_torrent_page.dart';
part 'model/app_torrent_target.dart';
part 'model/app_totals.dart';
part 'model/app_updates.dart';


/// An [ApiClient] instance that uses the default values obtained from
/// the OpenAPI specification file.
var defaultApiClient = ApiClient();

const _delimiters = {'csv': ',', 'ssv': ' ', 'tsv': '\t', 'pipes': '|'};
const _dateEpochMarker = 'epoch';
const _deepEquality = DeepCollectionEquality();
final _dateFormatter = DateFormat('yyyy-MM-dd');
final _regList = RegExp(r'^List<(.*)>$');
final _regSet = RegExp(r'^Set<(.*)>$');
final _regMap = RegExp(r'^Map<String,(.*)>$');

bool _isEpochMarker(String? pattern) => pattern == _dateEpochMarker || pattern == '/$_dateEpochMarker/';
