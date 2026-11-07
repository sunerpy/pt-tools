import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

/// Only these App API operations are reachable from the bundled editor.
/// This is an additional client boundary; the host still checks device scopes.
bool allowedMobileRequest(String verb, String path) {
  final uri = Uri.tryParse(path);
  if (uri == null ||
      uri.hasScheme ||
      uri.hasAuthority ||
      uri.hasFragment ||
      path.contains('%') ||
      path.contains('\\') ||
      path.contains('//') ||
      uri.path.contains('..') ||
      !path.startsWith('/api/app/v1/')) {
    return false;
  }
  final p = uri.path.substring('/api/app/v1'.length);
  if (verb == 'GET') {
    return const {
          '/mobile/options',
          '/meta',
          '/stats/aggregated',
          '/stats/summary',
          '/subscriptions',
          '/subscriptions/draft',
          '/media/libraries',
        }.contains(p) ||
        RegExp(r'^/(subscriptions|brush/tasks)/[1-9][0-9]*$').hasMatch(p) ||
        RegExp(r'^/favicon/[A-Za-z0-9_-]+$').hasMatch(p);
  }
  if (verb == 'POST') {
    return const {
      '/subscriptions',
      '/brush/tasks',
      '/brush/tasks/preview',
      '/subscriptions/schedule/preview',
      '/subscriptions/search/preview',
    }.contains(p);
  }
  return verb == 'PUT' &&
      RegExp(r'^/(subscriptions|brush/tasks)/[1-9][0-9]*$').hasMatch(p);
}

/// The most characters the subscription editor may put on the clipboard at once.
const maxCopyChars = 4096;

/// Checks a `copy` message: only the subscription editor may copy, and only
/// non-empty plain text of at most [maxCopyChars] characters (counted as
/// characters, not bytes). Other pages get 403, a too long text 413.
String mobileCopyText(String page, Object? text) {
  if (page != 'subscription') {
    throw const MobileBridgeError(403, 'Operation is not allowed');
  }
  if (text is! String || text.trim().isEmpty) {
    throw const MobileBridgeError(400, 'Nothing to copy');
  }
  if (text.runes.length > maxCopyChars) {
    throw const MobileBridgeError(413, 'Text is too long');
  }
  return text;
}

class MobileBridgeError implements Exception {
  const MobileBridgeError(this.status, this.message);
  final int status;
  final String message;
}

/// A page owns one bridge. Disposing it cancels all requests, including streams.
class MobileBridge {
  MobileBridge(this.client, {required this.canWrite});
  final http.Client client;
  final bool Function() canWrite;
  final _pending = <String, Completer<void>>{};
  final _seen = <String>{};
  bool _disposed = false;
  static const maxBody = 1 << 20;
  static const maxResponse = 8 << 20;

  Future<Object?> request(
    String id,
    String verb,
    String path,
    Object? body,
  ) async {
    if (_disposed) throw const MobileBridgeError(410, 'Page closed');
    if (!allowedMobileRequest(verb, path)) {
      throw const MobileBridgeError(403, 'Operation is not allowed');
    }
    if (verb != 'GET' && !canWrite()) {
      throw const MobileBridgeError(403, 'Read-only device');
    }
    if (id.isEmpty || id.length > 96 || _seen.contains(id)) {
      throw const MobileBridgeError(409, 'Duplicate request');
    }
    if (_pending.length >= 8 || _seen.length >= 4096) {
      throw const MobileBridgeError(429, 'Too many requests');
    }
    final encoded = body == null ? null : utf8.encode(jsonEncode(body));
    if ((encoded?.length ?? 0) > maxBody) {
      throw const MobileBridgeError(413, 'Request is too large');
    }
    _seen.add(id);
    final abort = Completer<void>();
    _pending[id] = abort;
    try {
      final req = http.AbortableRequest(
        verb,
        Uri.parse('https://pt-tools$path'),
        abortTrigger: abort.future,
      );
      req.headers['Accept'] = 'application/json';
      if (encoded != null) {
        req.headers['Content-Type'] = 'application/json';
        req.bodyBytes = encoded;
      }
      final resp = await client.send(req);
      final bytes = <int>[];
      await for (final chunk in resp.stream) {
        if (bytes.length + chunk.length > maxResponse) {
          cancel(id);
          throw const MobileBridgeError(413, 'Response is too large');
        }
        bytes.addAll(chunk);
      }
      if (_disposed || abort.isCompleted) {
        throw const MobileBridgeError(499, 'Request cancelled');
      }
      final type = resp.headers['content-type'] ?? '';
      if (resp.statusCode >= 400) {
        String message = 'HTTP ${resp.statusCode}';
        try {
          final value = jsonDecode(utf8.decode(bytes));
          if (value is Map && value['message'] is String) {
            message = value['message'] as String;
          }
        } on FormatException {
          /* Never expose an upstream HTML body. */
        }
        throw MobileBridgeError(resp.statusCode, message);
      }
      if (type.startsWith('image/')) {
        return {'base64': base64Encode(bytes), 'contentType': type};
      }
      return bytes.isEmpty ? null : jsonDecode(utf8.decode(bytes));
    } finally {
      _pending.remove(id);
    }
  }

  void cancel(String id) {
    final abort = _pending[id];
    if (abort != null && !abort.isCompleted) abort.complete();
  }

  void dispose() {
    _disposed = true;
    for (final id in _pending.keys.toList()) {
      cancel(id);
    }
  }
}

/// Android returns false for a cancelled document picker. Sharing only reports
/// that the chooser opened; it cannot report delivery to another application.
Map<String, bool> mobileImageResult(String action, bool? completed) =>
    action == 'save'
    ? {'saved': completed == true, 'canceled': completed != true}
    : {'launched': true};
