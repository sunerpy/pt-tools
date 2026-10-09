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
