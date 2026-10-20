//
// AUTO-GENERATED FILE, DO NOT MODIFY!
//
// @dart=2.18

// ignore_for_file: unused_element, unused_import
// ignore_for_file: always_put_required_named_parameters_first
// ignore_for_file: constant_identifier_names
// ignore_for_file: lines_longer_than_80_chars

part of openapi.api;


class AppApi {
  AppApi([ApiClient? apiClient]) : apiClient = apiClient ?? defaultApiClient;

  final ApiClient apiClient;

  /// 保存一次改动（要 app:config 与 app:write）：按现状重新算一遍再保存，回改完的样子
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] resource (required):
  ///   配置的名字，例如 global、downloaders、sites、rss（GET /config 列出全部）
  ///
  /// * [AppConfigChange] appConfigChange (required):
  Future<Response> applyConfigChangeWithHttpInfo(String resource, AppConfigChange appConfigChange, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/config/{resource}/changes'
      .replaceAll('{resource}', resource);

    // ignore: prefer_final_locals
    Object? postBody = appConfigChange;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 保存一次改动（要 app:config 与 app:write）：按现状重新算一遍再保存，回改完的样子
  ///
  /// Parameters:
  ///
  /// * [String] resource (required):
  ///   配置的名字，例如 global、downloaders、sites、rss（GET /config 列出全部）
  ///
  /// * [AppConfigChange] appConfigChange (required):
  Future<AppConfigResult?> applyConfigChange(String resource, AppConfigChange appConfigChange, { Future<void>? abortTrigger, }) async {
    final response = await applyConfigChangeWithHttpInfo(resource, appConfigChange, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppConfigResult',) as AppConfigResult;
    
    }
    return null;
  }

  /// 立即签到
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] site (required):
  Future<Response> attendSiteWithHttpInfo(String site, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/sites/{site}/attend'
      .replaceAll('{site}', site);

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 立即签到
  ///
  /// Parameters:
  ///
  /// * [String] site (required):
  Future<AppSiteAttendance?> attendSite(String site, { Future<void>? abortTrigger, }) async {
    final response = await attendSiteWithHttpInfo(site, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppSiteAttendance',) as AppSiteAttendance;
    
    }
    return null;
  }

  /// 停止会话进行中的回答（没有时什么也不做）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<Response> cancelAssistantRunWithHttpInfo(int id, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/assistant/conversations/{id}/cancel'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 停止会话进行中的回答（没有时什么也不做）
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<AppOK?> cancelAssistantRun(int id, { Future<void>? abortTrigger, }) async {
    final response = await cancelAssistantRunWithHttpInfo(id, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppOK',) as AppOK;
    
    }
    return null;
  }

  /// 新建会话；带了 message 时接着发这一条（回 202 与这次运行）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [AppAssistantCreate] appAssistantCreate (required):
  Future<Response> createAssistantConversationWithHttpInfo(AppAssistantCreate appAssistantCreate, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/assistant/conversations';

    // ignore: prefer_final_locals
    Object? postBody = appAssistantCreate;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 新建会话；带了 message 时接着发这一条（回 202 与这次运行）
  ///
  /// Parameters:
  ///
  /// * [AppAssistantCreate] appAssistantCreate (required):
  Future<AppAssistantCreateResult?> createAssistantConversation(AppAssistantCreate appAssistantCreate, { Future<void>? abortTrigger, }) async {
    final response = await createAssistantConversationWithHttpInfo(appAssistantCreate, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppAssistantCreateResult',) as AppAssistantCreateResult;
    
    }
    return null;
  }

  /// 新建订阅
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [AppSubscriptionCreate] appSubscriptionCreate (required):
  Future<Response> createSubscriptionWithHttpInfo(AppSubscriptionCreate appSubscriptionCreate, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/subscriptions';

    // ignore: prefer_final_locals
    Object? postBody = appSubscriptionCreate;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 新建订阅
  ///
  /// Parameters:
  ///
  /// * [AppSubscriptionCreate] appSubscriptionCreate (required):
  Future<AppSubscription?> createSubscription(AppSubscriptionCreate appSubscriptionCreate, { Future<void>? abortTrigger, }) async {
    final response = await createSubscriptionWithHttpInfo(appSubscriptionCreate, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppSubscription',) as AppSubscription;
    
    }
    return null;
  }

  /// 确认或拒绝一个待确认的改动类操作（要 app:write；已经有结论的回 409）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///
  /// * [AppAssistantDecide] appAssistantDecide (required):
  Future<Response> decideAssistantConfirmationWithHttpInfo(int id, AppAssistantDecide appAssistantDecide, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/assistant/confirmations/{id}'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody = appAssistantDecide;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 确认或拒绝一个待确认的改动类操作（要 app:write；已经有结论的回 409）
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///
  /// * [AppAssistantDecide] appAssistantDecide (required):
  Future<AppAssistantConfirmation?> decideAssistantConfirmation(int id, AppAssistantDecide appAssistantDecide, { Future<void>? abortTrigger, }) async {
    final response = await decideAssistantConfirmationWithHttpInfo(id, appAssistantDecide, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppAssistantConfirmation',) as AppAssistantConfirmation;
    
    }
    return null;
  }

  /// 删除会话连同消息与待确认记录（正在回答的回 409）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<Response> deleteAssistantConversationWithHttpInfo(int id, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/assistant/conversations/{id}'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'DELETE',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 删除会话连同消息与待确认记录（正在回答的回 409）
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<AppOK?> deleteAssistantConversation(int id, { Future<void>? abortTrigger, }) async {
    final response = await deleteAssistantConversationWithHttpInfo(id, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppOK',) as AppOK;
    
    }
    return null;
  }

  /// 删除订阅（下载器里的种子与媒体库里的文件不动）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<Response> deleteSubscriptionWithHttpInfo(int id, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/subscriptions/{id}'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'DELETE',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 删除订阅（下载器里的种子与媒体库里的文件不动）
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<AppOK?> deleteSubscription(int id, { Future<void>? abortTrigger, }) async {
    final response = await deleteSubscriptionWithHttpInfo(id, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppOK',) as AppOK;
    
    }
    return null;
  }

  /// 探索：TMDB 的热门、流行与搜索
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] kind (required):
  ///
  /// * [String] list:
  ///
  /// * [String] q:
  ///
  /// * [int] page:
  Future<Response> exploreWithHttpInfo(String kind, { String? list, String? q, int? page, Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/explore';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

      queryParams.addAll(_queryParams('', 'kind', kind));
    if (list != null) {
      queryParams.addAll(_queryParams('', 'list', list));
    }
    if (q != null) {
      queryParams.addAll(_queryParams('', 'q', q));
    }
    if (page != null) {
      queryParams.addAll(_queryParams('', 'page', page));
    }

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 探索：TMDB 的热门、流行与搜索
  ///
  /// Parameters:
  ///
  /// * [String] kind (required):
  ///
  /// * [String] list:
  ///
  /// * [String] q:
  ///
  /// * [int] page:
  Future<AppExplorePage?> explore(String kind, { String? list, String? q, int? page, Future<void>? abortTrigger, }) async {
    final response = await exploreWithHttpInfo(kind, list: list, q: q, page: page, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppExplorePage',) as AppExplorePage;
    
    }
    return null;
  }

  /// 一个会话：最近的消息、待确认操作与接着读事件的位置
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<Response> getAssistantConversationWithHttpInfo(int id, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/assistant/conversations/{id}'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 一个会话：最近的消息、待确认操作与接着读事件的位置
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<AppAssistantConversationDetail?> getAssistantConversation(int id, { Future<void>? abortTrigger, }) async {
    final response = await getAssistantConversationWithHttpInfo(id, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppAssistantConversationDetail',) as AppAssistantConversationDetail;
    
    }
    return null;
  }

  /// 长轮询会话的事件：after 以后有事件马上回，没有时最多等 wait 秒
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///
  /// * [int] after:
  ///   上次读到的序号（会话详情的 last_seq，或者上一页的 last）
  ///
  /// * [int] wait:
  ///   没有事件时最多等几秒，默认与上限都是 25
  Future<Response> getAssistantEventsWithHttpInfo(int id, { int? after, int? wait, Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/assistant/conversations/{id}/events'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (after != null) {
      queryParams.addAll(_queryParams('', 'after', after));
    }
    if (wait != null) {
      queryParams.addAll(_queryParams('', 'wait', wait));
    }

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 长轮询会话的事件：after 以后有事件马上回，没有时最多等 wait 秒
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///
  /// * [int] after:
  ///   上次读到的序号（会话详情的 last_seq，或者上一页的 last）
  ///
  /// * [int] wait:
  ///   没有事件时最多等几秒，默认与上限都是 25
  Future<AppAssistantEventPage?> getAssistantEvents(int id, { int? after, int? wait, Future<void>? abortTrigger, }) async {
    final response = await getAssistantEventsWithHttpInfo(id, after: after, wait: wait, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppAssistantEventPage',) as AppAssistantEventPage;
    
    }
    return null;
  }

  /// AI 助手启用没有、用的模型、调用者能不能确认改动类操作（不含 Key 与接口地址）
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> getAssistantStatusWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/assistant/status';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// AI 助手启用没有、用的模型、调用者能不能确认改动类操作（不含 Key 与接口地址）
  Future<AppAssistantStatus?> getAssistantStatus({ Future<void>? abortTrigger, }) async {
    final response = await getAssistantStatusWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppAssistantStatus',) as AppAssistantStatus;
    
    }
    return null;
  }

  /// 一类配置的现状（要 app:config 与 app:read）；密钥的值是 <已设置> 或空串
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] resource (required):
  ///   配置的名字，例如 global、downloaders、sites、rss（GET /config 列出全部）
  ///
  /// * [String] id:
  ///   列表类配置只看这一项（id_field 的值）
  Future<Response> getConfigWithHttpInfo(String resource, { String? id, Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/config/{resource}'
      .replaceAll('{resource}', resource);

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (id != null) {
      queryParams.addAll(_queryParams('', 'id', id));
    }

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 一类配置的现状（要 app:config 与 app:read）；密钥的值是 <已设置> 或空串
  ///
  /// Parameters:
  ///
  /// * [String] resource (required):
  ///   配置的名字，例如 global、downloaders、sites、rss（GET /config 列出全部）
  ///
  /// * [String] id:
  ///   列表类配置只看这一项（id_field 的值）
  Future<AppConfigState?> getConfig(String resource, { String? id, Future<void>? abortTrigger, }) async {
    final response = await getConfigWithHttpInfo(resource, id: id, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppConfigState',) as AppConfigState;
    
    }
    return null;
  }

  /// 配置目录（要 app:config 与 app:read）：能经接口读改的各类配置、能做的操作、能改的字段与密钥字段
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> getConfigCatalogWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/config';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 配置目录（要 app:config 与 app:read）：能经接口读改的各类配置、能做的操作、能改的字段与密钥字段
  Future<AppConfigCatalog?> getConfigCatalog({ Future<void>? abortTrigger, }) async {
    final response = await getConfigCatalogWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppConfigCatalog',) as AppConfigCatalog;
    
    }
    return null;
  }

  /// 站点图标（图片）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] site (required):
  Future<Response> getFaviconWithHttpInfo(String site, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/favicon/{site}'
      .replaceAll('{site}', site);

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 站点图标（图片）
  ///
  /// Parameters:
  ///
  /// * [String] site (required):
  Future<MultipartFile?> getFavicon(String site, { Future<void>? abortTrigger, }) async {
    final response = await getFaviconWithHttpInfo(site, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'MultipartFile',) as MultipartFile;
    
    }
    return null;
  }

  /// 版本、兼容级别、功能组与调用者自己的权限
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> getMetaWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/meta';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 版本、兼容级别、功能组与调用者自己的权限
  Future<AppMeta?> getMeta({ Future<void>? abortTrigger, }) async {
    final response = await getMetaWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppMeta',) as AppMeta;
    
    }
    return null;
  }

  /// 已启用站点的合计与今天的增量
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> getOverviewWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/overview';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 已启用站点的合计与今天的增量
  Future<AppOverview?> getOverview({ Future<void>? abortTrigger, }) async {
    final response = await getOverviewWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppOverview',) as AppOverview;
    
    }
    return null;
  }

  /// 订阅详情：每一集与下载过的种子
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<Response> getSubscriptionWithHttpInfo(int id, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/subscriptions/{id}'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 订阅详情：每一集与下载过的种子
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<AppSubscriptionDetail?> getSubscription(int id, { Future<void>? abortTrigger, }) async {
    final response = await getSubscriptionWithHttpInfo(id, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppSubscriptionDetail',) as AppSubscriptionDetail;
    
    }
    return null;
  }

  /// TMDB 图片（海报等），由 pt-tools 按自己的 TMDB 图片地址与代理去取
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] size (required):
  ///   海报尺寸（不提供原图）
  ///
  /// * [String] file (required):
  ///   poster_path 去掉开头的 /
  Future<Response> getTmdbImageWithHttpInfo(String size, String file, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/images/tmdb/{size}/{file}'
      .replaceAll('{size}', size)
      .replaceAll('{file}', file);

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// TMDB 图片（海报等），由 pt-tools 按自己的 TMDB 图片地址与代理去取
  ///
  /// Parameters:
  ///
  /// * [String] size (required):
  ///   海报尺寸（不提供原图）
  ///
  /// * [String] file (required):
  ///   poster_path 去掉开头的 /
  Future<MultipartFile?> getTmdbImage(String size, String file, { Future<void>? abortTrigger, }) async {
    final response = await getTmdbImageWithHttpInfo(size, file, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'MultipartFile',) as MultipartFile;
    
    }
    return null;
  }

  /// 有没有新版本
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] includePrerelease:
  Future<Response> getUpdatesWithHttpInfo({ String? includePrerelease, Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/updates';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (includePrerelease != null) {
      queryParams.addAll(_queryParams('', 'include_prerelease', includePrerelease));
    }

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 有没有新版本
  ///
  /// Parameters:
  ///
  /// * [String] includePrerelease:
  Future<AppUpdates?> getUpdates({ String? includePrerelease, Future<void>? abortTrigger, }) async {
    final response = await getUpdatesWithHttpInfo(includePrerelease: includePrerelease, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppUpdates',) as AppUpdates;
    
    }
    return null;
  }

  /// 调用者自己的 AI 助手会话（最近的在前，最多 100 个）
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> listAssistantConversationsWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/assistant/conversations';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 调用者自己的 AI 助手会话（最近的在前，最多 100 个）
  Future<List<AppAssistantConversation>?> listAssistantConversations({ Future<void>? abortTrigger, }) async {
    final response = await listAssistantConversationsWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      final responseBody = await _decodeBodyBytes(response);
      return (await apiClient.deserializeAsync(responseBody, 'List<AppAssistantConversation>') as List)
        .cast<AppAssistantConversation>()
        .toList(growable: false);

    }
    return null;
  }

  /// 刷流任务
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> listBrushTasksWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/brush/tasks';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 刷流任务
  Future<List<AppBrushTask>?> listBrushTasks({ Future<void>? abortTrigger, }) async {
    final response = await listBrushTasksWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      final responseBody = await _decodeBodyBytes(response);
      return (await apiClient.deserializeAsync(responseBody, 'List<AppBrushTask>') as List)
        .cast<AppBrushTask>()
        .toList(growable: false);

    }
    return null;
  }

  /// 启用的下载器与它们现在的速度、剩余空间
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> listDownloadersWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/downloaders';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 启用的下载器与它们现在的速度、剩余空间
  Future<AppDownloaderList?> listDownloaders({ Future<void>? abortTrigger, }) async {
    final response = await listDownloadersWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppDownloaderList',) as AppDownloaderList;
    
    }
    return null;
  }

  /// 整理历史
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] status:
  ///
  /// * [String] q:
  ///
  /// * [int] page:
  ///
  /// * [int] pageSize:
  Future<Response> listMediaHistoryWithHttpInfo({ String? status, String? q, int? page, int? pageSize, Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/media/history';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (status != null) {
      queryParams.addAll(_queryParams('', 'status', status));
    }
    if (q != null) {
      queryParams.addAll(_queryParams('', 'q', q));
    }
    if (page != null) {
      queryParams.addAll(_queryParams('', 'page', page));
    }
    if (pageSize != null) {
      queryParams.addAll(_queryParams('', 'page_size', pageSize));
    }

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 整理历史
  ///
  /// Parameters:
  ///
  /// * [String] status:
  ///
  /// * [String] q:
  ///
  /// * [int] page:
  ///
  /// * [int] pageSize:
  Future<AppMediaHistoryPage?> listMediaHistory({ String? status, String? q, int? page, int? pageSize, Future<void>? abortTrigger, }) async {
    final response = await listMediaHistoryWithHttpInfo(status: status, q: q, page: page, pageSize: pageSize, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppMediaHistoryPage',) as AppMediaHistoryPage;
    
    }
    return null;
  }

  /// 媒体库列表
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> listMediaLibrariesWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/media/libraries';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 媒体库列表
  Future<List<AppMediaLibrary>?> listMediaLibraries({ Future<void>? abortTrigger, }) async {
    final response = await listMediaLibrariesWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      final responseBody = await _decodeBodyBytes(response);
      return (await apiClient.deserializeAsync(responseBody, 'List<AppMediaLibrary>') as List)
        .cast<AppMediaLibrary>()
        .toList(growable: false);

    }
    return null;
  }

  /// 站点、登录状态、今天的签到与站点上的用户数据
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> listSitesWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/sites';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 站点、登录状态、今天的签到与站点上的用户数据
  Future<AppSiteList?> listSites({ Future<void>? abortTrigger, }) async {
    final response = await listSitesWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppSiteList',) as AppSiteList;
    
    }
    return null;
  }

  /// 订阅列表
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] status:
  ///
  /// * [String] q:
  Future<Response> listSubscriptionsWithHttpInfo({ String? status, String? q, Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/subscriptions';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (status != null) {
      queryParams.addAll(_queryParams('', 'status', status));
    }
    if (q != null) {
      queryParams.addAll(_queryParams('', 'q', q));
    }

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 订阅列表
  ///
  /// Parameters:
  ///
  /// * [String] status:
  ///
  /// * [String] q:
  Future<List<AppSubscription>?> listSubscriptions({ String? status, String? q, Future<void>? abortTrigger, }) async {
    final response = await listSubscriptionsWithHttpInfo(status: status, q: q, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      final responseBody = await _decodeBodyBytes(response);
      return (await apiClient.deserializeAsync(responseBody, 'List<AppSubscription>') as List)
        .cast<AppSubscription>()
        .toList(growable: false);

    }
    return null;
  }

  /// 任务列表（推送记录）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] site:
  ///
  /// * [String] q:
  ///
  /// * [String] pushed:
  ///
  /// * [int] page:
  ///
  /// * [int] pageSize:
  Future<Response> listTasksWithHttpInfo({ String? site, String? q, String? pushed, int? page, int? pageSize, Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/tasks';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (site != null) {
      queryParams.addAll(_queryParams('', 'site', site));
    }
    if (q != null) {
      queryParams.addAll(_queryParams('', 'q', q));
    }
    if (pushed != null) {
      queryParams.addAll(_queryParams('', 'pushed', pushed));
    }
    if (page != null) {
      queryParams.addAll(_queryParams('', 'page', page));
    }
    if (pageSize != null) {
      queryParams.addAll(_queryParams('', 'page_size', pageSize));
    }

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 任务列表（推送记录）
  ///
  /// Parameters:
  ///
  /// * [String] site:
  ///
  /// * [String] q:
  ///
  /// * [String] pushed:
  ///
  /// * [int] page:
  ///
  /// * [int] pageSize:
  Future<AppTaskPage?> listTasks({ String? site, String? q, String? pushed, int? page, int? pageSize, Future<void>? abortTrigger, }) async {
    final response = await listTasksWithHttpInfo(site: site, q: q, pushed: pushed, page: page, pageSize: pageSize, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppTaskPage',) as AppTaskPage;
    
    }
    return null;
  }

  /// 消息模板：事件的组、公共变量与各事件的变量、内置文字的样子与现在的模板
  ///
  /// Note: This method returns the HTTP [Response].
  Future<Response> listTemplatesWithHttpInfo({ Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/templates';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 消息模板：事件的组、公共变量与各事件的变量、内置文字的样子与现在的模板
  Future<AppTemplateCatalog?> listTemplates({ Future<void>? abortTrigger, }) async {
    final response = await listTemplatesWithHttpInfo(abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppTemplateCatalog',) as AppTemplateCatalog;
    
    }
    return null;
  }

  /// 下载器里的种子
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] downloaderId:
  ///
  /// * [String] state:
  ///
  /// * [String] q:
  ///
  /// * [String] sort:
  ///
  /// * [String] order:
  ///
  /// * [int] page:
  ///
  /// * [int] pageSize:
  Future<Response> listTorrentsWithHttpInfo({ int? downloaderId, String? state, String? q, String? sort, String? order, int? page, int? pageSize, Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/torrents';

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    if (downloaderId != null) {
      queryParams.addAll(_queryParams('', 'downloader_id', downloaderId));
    }
    if (state != null) {
      queryParams.addAll(_queryParams('', 'state', state));
    }
    if (q != null) {
      queryParams.addAll(_queryParams('', 'q', q));
    }
    if (sort != null) {
      queryParams.addAll(_queryParams('', 'sort', sort));
    }
    if (order != null) {
      queryParams.addAll(_queryParams('', 'order', order));
    }
    if (page != null) {
      queryParams.addAll(_queryParams('', 'page', page));
    }
    if (pageSize != null) {
      queryParams.addAll(_queryParams('', 'page_size', pageSize));
    }

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'GET',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 下载器里的种子
  ///
  /// Parameters:
  ///
  /// * [int] downloaderId:
  ///
  /// * [String] state:
  ///
  /// * [String] q:
  ///
  /// * [String] sort:
  ///
  /// * [String] order:
  ///
  /// * [int] page:
  ///
  /// * [int] pageSize:
  Future<AppTorrentPage?> listTorrents({ int? downloaderId, String? state, String? q, String? sort, String? order, int? page, int? pageSize, Future<void>? abortTrigger, }) async {
    final response = await listTorrentsWithHttpInfo(downloaderId: downloaderId, state: state, q: q, sort: sort, order: order, page: page, pageSize: pageSize, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppTorrentPage',) as AppTorrentPage;
    
    }
    return null;
  }

  /// 把下载器里的一个种子整理进媒体库（按媒体库的方式硬链接、复制或移动，刮削，通知媒体服务器扫描）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [AppOrganizeInput] appOrganizeInput (required):
  Future<Response> organizeMediaWithHttpInfo(AppOrganizeInput appOrganizeInput, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/media/organize';

    // ignore: prefer_final_locals
    Object? postBody = appOrganizeInput;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 把下载器里的一个种子整理进媒体库（按媒体库的方式硬链接、复制或移动，刮削，通知媒体服务器扫描）
  ///
  /// Parameters:
  ///
  /// * [AppOrganizeInput] appOrganizeInput (required):
  Future<AppOrganizeResult?> organizeMedia(AppOrganizeInput appOrganizeInput, { Future<void>? abortTrigger, }) async {
    final response = await organizeMediaWithHttpInfo(appOrganizeInput, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppOrganizeResult',) as AppOrganizeResult;
    
    }
    return null;
  }

  /// 预览一次改动（要 app:config 与 app:read）：每个字段改以前、改以后与可能的影响，不保存
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] resource (required):
  ///   配置的名字，例如 global、downloaders、sites、rss（GET /config 列出全部）
  ///
  /// * [AppConfigChange] appConfigChange (required):
  Future<Response> previewConfigChangeWithHttpInfo(String resource, AppConfigChange appConfigChange, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/config/{resource}/preview'
      .replaceAll('{resource}', resource);

    // ignore: prefer_final_locals
    Object? postBody = appConfigChange;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 预览一次改动（要 app:config 与 app:read）：每个字段改以前、改以后与可能的影响，不保存
  ///
  /// Parameters:
  ///
  /// * [String] resource (required):
  ///   配置的名字，例如 global、downloaders、sites、rss（GET /config 列出全部）
  ///
  /// * [AppConfigChange] appConfigChange (required):
  Future<AppConfigResult?> previewConfigChange(String resource, AppConfigChange appConfigChange, { Future<void>? abortTrigger, }) async {
    final response = await previewConfigChangeWithHttpInfo(resource, appConfigChange, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppConfigResult',) as AppConfigResult;
    
    }
    return null;
  }

  /// 预览整理下载器里的一个种子：认出的条目、媒体库、每个视频的目标位置（不改动任何东西）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [AppOrganizeInput] appOrganizeInput (required):
  Future<Response> previewMediaOrganizeWithHttpInfo(AppOrganizeInput appOrganizeInput, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/media/organize/preview';

    // ignore: prefer_final_locals
    Object? postBody = appOrganizeInput;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 预览整理下载器里的一个种子：认出的条目、媒体库、每个视频的目标位置（不改动任何东西）
  ///
  /// Parameters:
  ///
  /// * [AppOrganizeInput] appOrganizeInput (required):
  Future<AppOrganizePlan?> previewMediaOrganize(AppOrganizeInput appOrganizeInput, { Future<void>? abortTrigger, }) async {
    final response = await previewMediaOrganizeWithHttpInfo(appOrganizeInput, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppOrganizePlan',) as AppOrganizePlan;
    
    }
    return null;
  }

  /// 用示例数据渲染一份模板（不保存；模板不对时回 400 并写明哪里不对）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [AppTemplatePreviewInput] appTemplatePreviewInput (required):
  Future<Response> previewTemplateWithHttpInfo(AppTemplatePreviewInput appTemplatePreviewInput, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/templates/preview';

    // ignore: prefer_final_locals
    Object? postBody = appTemplatePreviewInput;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 用示例数据渲染一份模板（不保存；模板不对时回 400 并写明哪里不对）
  ///
  /// Parameters:
  ///
  /// * [AppTemplatePreviewInput] appTemplatePreviewInput (required):
  Future<AppTemplatePreview?> previewTemplate(AppTemplatePreviewInput appTemplatePreviewInput, { Future<void>? abortTrigger, }) async {
    final response = await previewTemplateWithHttpInfo(appTemplatePreviewInput, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppTemplatePreview',) as AppTemplatePreview;
    
    }
    return null;
  }

  /// 把搜索到的种子推送到下载器
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [AppPushRequest] appPushRequest (required):
  Future<Response> pushWithHttpInfo(AppPushRequest appPushRequest, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/push';

    // ignore: prefer_final_locals
    Object? postBody = appPushRequest;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 把搜索到的种子推送到下载器
  ///
  /// Parameters:
  ///
  /// * [AppPushRequest] appPushRequest (required):
  Future<AppPushResult?> push(AppPushRequest appPushRequest, { Future<void>? abortTrigger, }) async {
    final response = await pushWithHttpInfo(appPushRequest, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppPushResult',) as AppPushResult;
    
    }
    return null;
  }

  /// 识别一个种子标题：解析名字、年份、季集、规格，按手动纠正与识别词到 TMDB 找对应的条目（不改动任何东西）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [AppRecognizeInput] appRecognizeInput (required):
  Future<Response> recognizeMediaWithHttpInfo(AppRecognizeInput appRecognizeInput, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/media/recognize';

    // ignore: prefer_final_locals
    Object? postBody = appRecognizeInput;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 识别一个种子标题：解析名字、年份、季集、规格，按手动纠正与识别词到 TMDB 找对应的条目（不改动任何东西）
  ///
  /// Parameters:
  ///
  /// * [AppRecognizeInput] appRecognizeInput (required):
  Future<AppRecognition?> recognizeMedia(AppRecognizeInput appRecognizeInput, { Future<void>? abortTrigger, }) async {
    final response = await recognizeMediaWithHttpInfo(appRecognizeInput, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppRecognition',) as AppRecognition;
    
    }
    return null;
  }

  /// 恢复默认（要 app:write）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] type (required):
  ///   事件类型，例如 rss.matched、reply.status、assistant.reply
  Future<Response> resetTemplateWithHttpInfo(String type, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/templates/{type}'
      .replaceAll('{type}', type);

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'DELETE',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 恢复默认（要 app:write）
  ///
  /// Parameters:
  ///
  /// * [String] type (required):
  ///   事件类型，例如 rss.matched、reply.status、assistant.reply
  Future<AppOK?> resetTemplate(String type, { Future<void>? abortTrigger, }) async {
    final response = await resetTemplateWithHttpInfo(type, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppOK',) as AppOK;
    
    }
    return null;
  }

  /// 重新整理一条整理记录对应的种子（多半是失败的那条）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///   整理记录的编号（GET /media/history 的 id）
  Future<Response> retryMediaHistoryWithHttpInfo(int id, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/media/history/{id}/retry'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 重新整理一条整理记录对应的种子（多半是失败的那条）
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///   整理记录的编号（GET /media/history 的 id）
  Future<AppOrganizeResult?> retryMediaHistory(int id, { Future<void>? abortTrigger, }) async {
    final response = await retryMediaHistoryWithHttpInfo(id, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppOrganizeResult',) as AppOrganizeResult;
    
    }
    return null;
  }

  /// 保存一种事件的模板（要 app:write；先用示例数据校验，不对时回 400；两项都空等于恢复默认）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [String] type (required):
  ///   事件类型，例如 rss.matched、reply.status、assistant.reply
  ///
  /// * [AppTemplateInput] appTemplateInput (required):
  Future<Response> saveTemplateWithHttpInfo(String type, AppTemplateInput appTemplateInput, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/templates/{type}'
      .replaceAll('{type}', type);

    // ignore: prefer_final_locals
    Object? postBody = appTemplateInput;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'PUT',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 保存一种事件的模板（要 app:write；先用示例数据校验，不对时回 400；两项都空等于恢复默认）
  ///
  /// Parameters:
  ///
  /// * [String] type (required):
  ///   事件类型，例如 rss.matched、reply.status、assistant.reply
  ///
  /// * [AppTemplateInput] appTemplateInput (required):
  Future<AppTemplate?> saveTemplate(String type, AppTemplateInput appTemplateInput, { Future<void>? abortTrigger, }) async {
    final response = await saveTemplateWithHttpInfo(type, appTemplateInput, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppTemplate',) as AppTemplate;
    
    }
    return null;
  }

  /// 多站搜索（只要读取权限）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [AppSearchRequest] appSearchRequest (required):
  Future<Response> searchWithHttpInfo(AppSearchRequest appSearchRequest, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/search';

    // ignore: prefer_final_locals
    Object? postBody = appSearchRequest;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 多站搜索（只要读取权限）
  ///
  /// Parameters:
  ///
  /// * [AppSearchRequest] appSearchRequest (required):
  Future<AppSearchResult?> search(AppSearchRequest appSearchRequest, { Future<void>? abortTrigger, }) async {
    final response = await searchWithHttpInfo(appSearchRequest, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppSearchResult',) as AppSearchResult;
    
    }
    return null;
  }

  /// 立即搜索一次
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<Response> searchSubscriptionWithHttpInfo(int id, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/subscriptions/{id}/search'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>[];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 立即搜索一次
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  Future<AppMessage?> searchSubscription(int id, { Future<void>? abortTrigger, }) async {
    final response = await searchSubscriptionWithHttpInfo(id, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppMessage',) as AppMessage;
    
    }
    return null;
  }

  /// 发一条消息并在后台开始回答（同一个会话同时只有一次运行，另一个回 409）
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///
  /// * [AppAssistantSend] appAssistantSend (required):
  Future<Response> sendAssistantMessageWithHttpInfo(int id, AppAssistantSend appAssistantSend, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/assistant/conversations/{id}/messages'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody = appAssistantSend;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 发一条消息并在后台开始回答（同一个会话同时只有一次运行，另一个回 409）
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///
  /// * [AppAssistantSend] appAssistantSend (required):
  Future<AppAssistantSendResult?> sendAssistantMessage(int id, AppAssistantSend appAssistantSend, { Future<void>? abortTrigger, }) async {
    final response = await sendAssistantMessageWithHttpInfo(id, appAssistantSend, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppAssistantSendResult',) as AppAssistantSendResult;
    
    }
    return null;
  }

  /// 暂停或恢复订阅
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///
  /// * [AppSubscriptionStatus] appSubscriptionStatus (required):
  Future<Response> setSubscriptionStatusWithHttpInfo(int id, AppSubscriptionStatus appSubscriptionStatus, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/subscriptions/{id}/status'
      .replaceAll('{id}', id.toString());

    // ignore: prefer_final_locals
    Object? postBody = appSubscriptionStatus;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 暂停或恢复订阅
  ///
  /// Parameters:
  ///
  /// * [int] id (required):
  ///
  /// * [AppSubscriptionStatus] appSubscriptionStatus (required):
  Future<AppSubscription?> setSubscriptionStatus(int id, AppSubscriptionStatus appSubscriptionStatus, { Future<void>? abortTrigger, }) async {
    final response = await setSubscriptionStatusWithHttpInfo(id, appSubscriptionStatus, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppSubscription',) as AppSubscription;
    
    }
    return null;
  }

  /// 暂停、继续、删除种子
  ///
  /// Note: This method returns the HTTP [Response].
  ///
  /// Parameters:
  ///
  /// * [AppTorrentActionsRequest] appTorrentActionsRequest (required):
  Future<Response> torrentActionsWithHttpInfo(AppTorrentActionsRequest appTorrentActionsRequest, { Future<void>? abortTrigger, }) async {
    // ignore: prefer_const_declarations
    final path = r'/torrents/actions';

    // ignore: prefer_final_locals
    Object? postBody = appTorrentActionsRequest;

    final queryParams = <QueryParam>[];
    final headerParams = <String, String>{};
    final formParams = <String, String>{};

    const contentTypes = <String>['application/json'];


    return apiClient.invokeAPI(
      path,
      'POST',
      queryParams,
      postBody,
      headerParams,
      formParams,
      contentTypes.isEmpty ? null : contentTypes.first,
      abortTrigger: abortTrigger,
    );
  }

  /// 暂停、继续、删除种子
  ///
  /// Parameters:
  ///
  /// * [AppTorrentActionsRequest] appTorrentActionsRequest (required):
  Future<AppTorrentActionsResult?> torrentActions(AppTorrentActionsRequest appTorrentActionsRequest, { Future<void>? abortTrigger, }) async {
    final response = await torrentActionsWithHttpInfo(appTorrentActionsRequest, abortTrigger: abortTrigger,);
    if (response.statusCode >= HttpStatus.badRequest) {
      throw ApiException(response.statusCode, await _decodeBodyBytes(response));
    }
    // When a remote server returns no body with a status of 204, we shall not decode it.
    // At the time of writing this, `dart:convert` will throw an "Unexpected end of input"
    // FormatException when trying to decode an empty string.
    if (response.body.isNotEmpty && response.statusCode != HttpStatus.noContent) {
      return await apiClient.deserializeAsync(await _decodeBodyBytes(response), 'AppTorrentActionsResult',) as AppTorrentActionsResult;
    
    }
    return null;
  }
}
