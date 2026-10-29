//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;

class ApiClient {
  ApiClient({this.basePath = '/api/app/v1', this.authentication,});

  final String basePath;
  final Authentication? authentication;

  var _client = Client();
  final _defaultHeaderMap = <String, String>{};

  /// Returns the current HTTP [Client] instance to use in this class.
  ///
  /// The return value is guaranteed to never be null.
  Client get client => _client;

  /// Requests to use a new HTTP [Client] in this class.
  set client(Client newClient) {
    _client = newClient;
  }

  Map<String, String> get defaultHeaderMap => _defaultHeaderMap;

  void addDefaultHeader(String key, String value) {
     _defaultHeaderMap[key] = value;
  }

  // We don't use a Map<String, String> for queryParams.
  // If collectionFormat is 'multi', a key might appear multiple times.
  Future<Response> invokeAPI(
    String path,
    String method,
    List<QueryParam> queryParams,
    Object? body,
    Map<String, String> headerParams,
    Map<String, String> formParams,
    String? contentType, {
    Future<void>? abortTrigger,
  }) async {
    await authentication?.applyToParams(queryParams, headerParams);

    headerParams.addAll(_defaultHeaderMap);
    if (contentType != null) {
      headerParams['Content-Type'] = contentType;
    }

    final urlEncodedQueryParams = queryParams.map((param) => '$param');
    final queryString = urlEncodedQueryParams.isNotEmpty ? '?${urlEncodedQueryParams.join('&')}' : '';
    final uri = Uri.parse('$basePath$path$queryString');

    try {
      // Special case for uploading a single file which isn't a 'multipart/form-data'.
      if (
        body is MultipartFile && (contentType == null ||
        !contentType.toLowerCase().startsWith('multipart/form-data'))
      ) {
        final request = AbortableStreamedRequest(method, uri, abortTrigger: abortTrigger);
        request.headers.addAll(headerParams);
        request.contentLength = body.length;
        body.finalize().listen(
          request.sink.add,
          onDone: request.sink.close,
          // ignore: avoid_types_on_closure_parameters
          onError: (Object error, StackTrace trace) => request.sink.close(),
          cancelOnError: true,
        );
        final response = await _client.send(request);
        return Response.fromStream(response);
      }

      if (body is MultipartRequest) {
        final request = AbortableMultipartRequest(method, uri, abortTrigger: abortTrigger);
        request.fields.addAll(body.fields);
        request.files.addAll(body.files);
        request.headers.addAll(body.headers);
        request.headers.addAll(headerParams);
        final response = await _client.send(request);
        return Response.fromStream(response);
      }

      final msgBody = contentType == 'application/x-www-form-urlencoded'
        ? formParams
        : await serializeAsync(body);
      final nullableHeaderParams = headerParams.isEmpty ? null : headerParams;

      final request = AbortableRequest(method, uri, abortTrigger: abortTrigger);
      if (nullableHeaderParams != null) {
        request.headers.addAll(nullableHeaderParams);
      }
      if (msgBody is String && msgBody.isNotEmpty) {
        request.body = msgBody;
      } else if (msgBody is List<int> && msgBody.isNotEmpty) {
        request.bodyBytes = msgBody;
      } else if (msgBody is Map<String, String>) {
        request.bodyFields = msgBody;
      }
      final response = await _client.send(request);
      return Response.fromStream(response);
    } on SocketException catch (error, trace) {
      throw ApiException.withInner(
        HttpStatus.badRequest,
        'Socket operation failed: $method $path',
        error,
        trace,
      );
    } on TlsException catch (error, trace) {
      throw ApiException.withInner(
        HttpStatus.badRequest,
        'TLS/SSL communication failed: $method $path',
        error,
        trace,
      );
    } on IOException catch (error, trace) {
      throw ApiException.withInner(
        HttpStatus.badRequest,
        'I/O operation failed: $method $path',
        error,
        trace,
      );
    } on ClientException catch (error, trace) {
      throw ApiException.withInner(
        HttpStatus.badRequest,
        'HTTP connection failed: $method $path',
        error,
        trace,
      );
    } on Exception catch (error, trace) {
      throw ApiException.withInner(
        HttpStatus.badRequest,
        'Exception occurred: $method $path',
        error,
        trace,
      );
    }
  }

  Future<dynamic> deserializeAsync(String value, String targetType, {bool growable = false,}) async =>
    // ignore: deprecated_member_use_from_same_package
    deserialize(value, targetType, growable: growable);

  @Deprecated('Scheduled for removal in OpenAPI Generator 6.x. Use deserializeAsync() instead.')
  dynamic deserialize(String value, String targetType, {bool growable = false,}) {
    // Remove all spaces. Necessary for regular expressions as well.
    targetType = targetType.replaceAll(' ', ''); // ignore: parameter_assignments

    // If the expected target type is String, nothing to do...
    return targetType == 'String'
      ? value
      : fromJson(json.decode(value), targetType, growable: growable);
  }

  // ignore: deprecated_member_use_from_same_package
  Future<String> serializeAsync(Object? value) async => serialize(value);

  @Deprecated('Scheduled for removal in OpenAPI Generator 6.x. Use serializeAsync() instead.')
  String serialize(Object? value) => value == null ? '' : json.encode(value);

  /// Returns a native instance of an OpenAPI class matching the [specified type][targetType].
  static dynamic fromJson(dynamic value, String targetType, {bool growable = false,}) {
    try {
      switch (targetType) {
        case 'String':
          return value is String ? value : value.toString();
        case 'int':
          return value is int ? value : int.parse('$value');
        case 'double':
          return value is double ? value : double.parse('$value');
        case 'bool':
          if (value is bool) {
            return value;
          }
          final valueString = '$value'.toLowerCase();
          return valueString == 'true' || valueString == '1';
        case 'DateTime':
          return value is DateTime ? value : DateTime.tryParse(value);
        case 'AppAssistantAttachment':
          return AppAssistantAttachment.fromJson(value);
        case 'AppAssistantChange':
          return AppAssistantChange.fromJson(value);
        case 'AppAssistantConfirmation':
          return AppAssistantConfirmation.fromJson(value);
        case 'AppAssistantConversation':
          return AppAssistantConversation.fromJson(value);
        case 'AppAssistantConversationDetail':
          return AppAssistantConversationDetail.fromJson(value);
        case 'AppAssistantCreate':
          return AppAssistantCreate.fromJson(value);
        case 'AppAssistantCreateResult':
          return AppAssistantCreateResult.fromJson(value);
        case 'AppAssistantDecide':
          return AppAssistantDecide.fromJson(value);
        case 'AppAssistantEvent':
          return AppAssistantEvent.fromJson(value);
        case 'AppAssistantEventPage':
          return AppAssistantEventPage.fromJson(value);
        case 'AppAssistantMessage':
          return AppAssistantMessage.fromJson(value);
        case 'AppAssistantSend':
          return AppAssistantSend.fromJson(value);
        case 'AppAssistantSendResult':
          return AppAssistantSendResult.fromJson(value);
        case 'AppAssistantStatus':
          return AppAssistantStatus.fromJson(value);
        case 'AppAssistantToolCall':
          return AppAssistantToolCall.fromJson(value);
        case 'AppAssistantUsage':
          return AppAssistantUsage.fromJson(value);
        case 'AppBrushConfig':
          return AppBrushConfig.fromJson(value);
        case 'AppBrushInput':
          return AppBrushInput.fromJson(value);
        case 'AppBrushPeriod':
          return AppBrushPeriod.fromJson(value);
        case 'AppBrushRun':
          return AppBrushRun.fromJson(value);
        case 'AppBrushRunPage':
          return AppBrushRunPage.fromJson(value);
        case 'AppBrushRunStep':
          return AppBrushRunStep.fromJson(value);
        case 'AppBrushStat':
          return AppBrushStat.fromJson(value);
        case 'AppBrushStats':
          return AppBrushStats.fromJson(value);
        case 'AppBrushStatus':
          return AppBrushStatus.fromJson(value);
        case 'AppBrushTask':
          return AppBrushTask.fromJson(value);
        case 'AppBrushTaskPeriods':
          return AppBrushTaskPeriods.fromJson(value);
        case 'AppConfigCatalog':
          return AppConfigCatalog.fromJson(value);
        case 'AppConfigChange':
          return AppConfigChange.fromJson(value);
        case 'AppConfigDiff':
          return AppConfigDiff.fromJson(value);
        case 'AppConfigResource':
          return AppConfigResource.fromJson(value);
        case 'AppConfigResult':
          return AppConfigResult.fromJson(value);
        case 'AppConfigState':
          return AppConfigState.fromJson(value);
        case 'AppDelta':
          return AppDelta.fromJson(value);
        case 'AppDownloader':
          return AppDownloader.fromJson(value);
        case 'AppDownloaderFailure':
          return AppDownloaderFailure.fromJson(value);
        case 'AppDownloaderList':
          return AppDownloaderList.fromJson(value);
        case 'AppEpisode':
          return AppEpisode.fromJson(value);
        case 'AppEpisodeRule':
          return AppEpisodeRule.fromJson(value);
        case 'AppEpisodeRuleInput':
          return AppEpisodeRuleInput.fromJson(value);
        case 'AppEpisodeSkip':
          return AppEpisodeSkip.fromJson(value);
        case 'AppEpisodeUnlock':
          return AppEpisodeUnlock.fromJson(value);
        case 'AppError':
          return AppError.fromJson(value);
        case 'AppExploreItem':
          return AppExploreItem.fromJson(value);
        case 'AppExplorePage':
          return AppExplorePage.fromJson(value);
        case 'AppFromTorrent':
          return AppFromTorrent.fromJson(value);
        case 'AppLibraryRef':
          return AppLibraryRef.fromJson(value);
        case 'AppMediaHistory':
          return AppMediaHistory.fromJson(value);
        case 'AppMediaHistoryPage':
          return AppMediaHistoryPage.fromJson(value);
        case 'AppMediaLibrary':
          return AppMediaLibrary.fromJson(value);
        case 'AppMediaMatch':
          return AppMediaMatch.fromJson(value);
        case 'AppMediaMeta':
          return AppMediaMeta.fromJson(value);
        case 'AppMessage':
          return AppMessage.fromJson(value);
        case 'AppMeta':
          return AppMeta.fromJson(value);
        case 'AppMobileOption':
          return AppMobileOption.fromJson(value);
        case 'AppMobileOptions':
          return AppMobileOptions.fromJson(value);
        case 'AppMobileSiteOption':
          return AppMobileSiteOption.fromJson(value);
        case 'AppOK':
          return AppOK.fromJson(value);
        case 'AppOrganizeInput':
          return AppOrganizeInput.fromJson(value);
        case 'AppOrganizeItem':
          return AppOrganizeItem.fromJson(value);
        case 'AppOrganizePlan':
          return AppOrganizePlan.fromJson(value);
        case 'AppOrganizeResult':
          return AppOrganizeResult.fromJson(value);
        case 'AppOverview':
          return AppOverview.fromJson(value);
        case 'AppPrincipal':
          return AppPrincipal.fromJson(value);
        case 'AppProgress':
          return AppProgress.fromJson(value);
        case 'AppPushRequest':
          return AppPushRequest.fromJson(value);
        case 'AppPushResult':
          return AppPushResult.fromJson(value);
        case 'AppRecognition':
          return AppRecognition.fromJson(value);
        case 'AppRecognizeInput':
          return AppRecognizeInput.fromJson(value);
        case 'AppRelease':
          return AppRelease.fromJson(value);
        case 'AppReleaseAsset':
          return AppReleaseAsset.fromJson(value);
        case 'AppRescrapeResult':
          return AppRescrapeResult.fromJson(value);
        case 'AppRunStep':
          return AppRunStep.fromJson(value);
        case 'AppSchedule':
          return AppSchedule.fromJson(value);
        case 'AppSchedulePreview':
          return AppSchedulePreview.fromJson(value);
        case 'AppSchedulePreviewInput':
          return AppSchedulePreviewInput.fromJson(value);
        case 'AppSearchError':
          return AppSearchError.fromJson(value);
        case 'AppSearchItem':
          return AppSearchItem.fromJson(value);
        case 'AppSearchRequest':
          return AppSearchRequest.fromJson(value);
        case 'AppSearchResult':
          return AppSearchResult.fromJson(value);
        case 'AppSite':
          return AppSite.fromJson(value);
        case 'AppSiteAttendance':
          return AppSiteAttendance.fromJson(value);
        case 'AppSiteDelta':
          return AppSiteDelta.fromJson(value);
        case 'AppSiteList':
          return AppSiteList.fromJson(value);
        case 'AppSiteLogin':
          return AppSiteLogin.fromJson(value);
        case 'AppSiteUser':
          return AppSiteUser.fromJson(value);
        case 'AppStatsAggregated':
          return AppStatsAggregated.fromJson(value);
        case 'AppStatsSite':
          return AppStatsSite.fromJson(value);
        case 'AppStatsSiteDelta':
          return AppStatsSiteDelta.fromJson(value);
        case 'AppStatsSummary':
          return AppStatsSummary.fromJson(value);
        case 'AppSubscription':
          return AppSubscription.fromJson(value);
        case 'AppSubscriptionCreate':
          return AppSubscriptionCreate.fromJson(value);
        case 'AppSubscriptionDetail':
          return AppSubscriptionDetail.fromJson(value);
        case 'AppSubscriptionRun':
          return AppSubscriptionRun.fromJson(value);
        case 'AppSubscriptionRunPage':
          return AppSubscriptionRunPage.fromJson(value);
        case 'AppSubscriptionStatus':
          return AppSubscriptionStatus.fromJson(value);
        case 'AppSubscriptionTorrent':
          return AppSubscriptionTorrent.fromJson(value);
        case 'AppSubscriptionUpdate':
          return AppSubscriptionUpdate.fromJson(value);
        case 'AppTask':
          return AppTask.fromJson(value);
        case 'AppTaskPage':
          return AppTaskPage.fromJson(value);
        case 'AppTemplate':
          return AppTemplate.fromJson(value);
        case 'AppTemplateCatalog':
          return AppTemplateCatalog.fromJson(value);
        case 'AppTemplateGroup':
          return AppTemplateGroup.fromJson(value);
        case 'AppTemplateInput':
          return AppTemplateInput.fromJson(value);
        case 'AppTemplatePreview':
          return AppTemplatePreview.fromJson(value);
        case 'AppTemplatePreviewInput':
          return AppTemplatePreviewInput.fromJson(value);
        case 'AppTemplateVar':
          return AppTemplateVar.fromJson(value);
        case 'AppTorrent':
          return AppTorrent.fromJson(value);
        case 'AppTorrentActionResult':
          return AppTorrentActionResult.fromJson(value);
        case 'AppTorrentActionsRequest':
          return AppTorrentActionsRequest.fromJson(value);
        case 'AppTorrentActionsResult':
          return AppTorrentActionsResult.fromJson(value);
        case 'AppTorrentFile':
          return AppTorrentFile.fromJson(value);
        case 'AppTorrentPage':
          return AppTorrentPage.fromJson(value);
        case 'AppTorrentTarget':
          return AppTorrentTarget.fromJson(value);
        case 'AppTotals':
          return AppTotals.fromJson(value);
        case 'AppUpdates':
          return AppUpdates.fromJson(value);
        default:
          dynamic match;
          if (value is List && (match = _regList.firstMatch(targetType)?.group(1)) != null) {
            return value
              .map<dynamic>((dynamic v) => fromJson(v, match, growable: growable,))
              .toList(growable: growable);
          }
          if (value is Set && (match = _regSet.firstMatch(targetType)?.group(1)) != null) {
            return value
              .map<dynamic>((dynamic v) => fromJson(v, match, growable: growable,))
              .toSet();
          }
          if (value is Map && (match = _regMap.firstMatch(targetType)?.group(1)) != null) {
            return Map<String, dynamic>.fromIterables(
              value.keys.cast<String>(),
              value.values.map<dynamic>((dynamic v) => fromJson(v, match, growable: growable,)),
            );
          }
      }
    } on Exception catch (error, trace) {
      throw ApiException.withInner(HttpStatus.internalServerError, 'Exception during deserialization.', error, trace,);
    }
    throw ApiException(HttpStatus.internalServerError, 'Could not find a suitable class for deserialization',);
  }
}

/// Primarily intended for use in an isolate.
class DeserializationMessage {
  const DeserializationMessage({
    required this.json,
    required this.targetType,
    this.growable = false,
  });

  /// The JSON value to deserialize.
  final String json;

  /// Target type to deserialize to.
  final String targetType;

  /// Whether to make deserialized lists or maps growable.
  final bool growable;
}

/// Primarily intended for use in an isolate.
Future<dynamic> decodeAsync(DeserializationMessage message) async {
  // Remove all spaces. Necessary for regular expressions as well.
  final targetType = message.targetType.replaceAll(' ', '');

  // If the expected target type is String, nothing to do...
  return targetType == 'String'
    ? message.json
    : json.decode(message.json);
}

/// Primarily intended for use in an isolate.
Future<dynamic> deserializeAsync(DeserializationMessage message) async {
  // Remove all spaces. Necessary for regular expressions as well.
  final targetType = message.targetType.replaceAll(' ', '');

  // If the expected target type is String, nothing to do...
  return targetType == 'String'
    ? message.json
    : ApiClient.fromJson(
        json.decode(message.json),
        targetType,
        growable: message.growable,
      );
}

/// Primarily intended for use in an isolate.
Future<String> serializeAsync(Object? value) async => value == null ? '' : json.encode(value);
