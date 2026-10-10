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

part 'model/app_brush_stat.dart';
part 'model/app_brush_task.dart';
part 'model/app_delta.dart';
part 'model/app_downloader.dart';
part 'model/app_downloader_failure.dart';
part 'model/app_downloader_list.dart';
part 'model/app_episode.dart';
part 'model/app_error.dart';
part 'model/app_explore_item.dart';
part 'model/app_explore_page.dart';
part 'model/app_media_history.dart';
part 'model/app_media_history_page.dart';
part 'model/app_message.dart';
part 'model/app_meta.dart';
part 'model/app_ok.dart';
part 'model/app_overview.dart';
part 'model/app_principal.dart';
part 'model/app_progress.dart';
part 'model/app_push_request.dart';
part 'model/app_push_result.dart';
part 'model/app_release.dart';
part 'model/app_release_asset.dart';
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
part 'model/app_subscription_status.dart';
part 'model/app_subscription_torrent.dart';
part 'model/app_task.dart';
part 'model/app_task_page.dart';
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
