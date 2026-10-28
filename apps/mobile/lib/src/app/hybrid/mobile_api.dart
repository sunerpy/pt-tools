import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;
import 'package:ptt_api/api.dart';

import '../providers.dart';

Future<Object?> mobileApi(
  WidgetRef ref,
  String path, {
  String verb = 'GET',
  Object? body,
}) async {
  final connection = ref.read(connectionProvider);
  if (connection == null) throw ApiException(503, 'Not connected');
  final request = http.Request(
    verb,
    Uri.parse('https://pt-tools/api/app/v1$path'),
  );
  if (body != null) {
    request.headers['Content-Type'] = 'application/json';
    request.body = jsonEncode(body);
  }
  final response = await http.Response.fromStream(
    await connection.client.send(request),
  );
  if (response.statusCode >= 400) {
    throw ApiException(response.statusCode, response.body);
  }
  return response.body.isEmpty ? null : jsonDecode(response.body);
}
