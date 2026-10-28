import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:webview_flutter/webview_flutter.dart';

import '../../../l10n/app_localizations.dart';
import '../hybrid/mobile_bridge.dart';
import '../hybrid/mobile_assets.dart';
import '../providers.dart';
import '../widgets/common.dart';

Future<bool> openMobileEditor(
  BuildContext context,
  WidgetRef ref,
  String page, {
  int? id,
  Map<String, String> values = const {},
}) async {
  final feature = page == 'poster'
      ? 'stats_export'
      : page == 'brush'
      ? 'brush_manage'
      : 'mobile_editors';
  try {
    final meta = await ref.read(metaProvider.future);
    if (!context.mounted) return false;
    if (!meta.features.contains(feature)) {
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(S.of(context).mobileUpgrade)));
      return false;
    }
    return await Navigator.of(context, rootNavigator: true).push<bool>(
          MaterialPageRoute(
            builder: (_) =>
                MobileEditorPage(page: page, id: id, values: values),
          ),
        ) ??
        false;
  } on Object catch (e) {
    if (context.mounted) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(describeError(e, S.of(context)))));
    }
    return false;
  }
}

class MobileEditorPage extends ConsumerStatefulWidget {
  const MobileEditorPage({
    super.key,
    required this.page,
    this.id,
    this.values = const {},
  });
  final String page;
  final int? id;
  final Map<String, String> values;
  @override
  ConsumerState<MobileEditorPage> createState() => _MobileEditorPageState();
}

class _MobileEditorPageState extends ConsumerState<MobileEditorPage> {
  static const _assetPath = '/mobile.html';
  MobileAssets? _assets;
  static const _images = MethodChannel('app.firlab.pttools/images');
  WebViewController? _controller;
  MobileBridge? _bridge;
  bool _dirty = false;
  bool _trusted = false;
  bool _leaving = false;
  bool _imageBusy = false;
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    if (!kIsWeb && defaultTargetPlatform == TargetPlatform.android) {
      final conn = ref.read(connectionProvider);
      if (conn != null) {
        _bridge = MobileBridge(
          conn.client,
          canWrite: () => ref.read(canWriteProvider),
        );
      }
      _controller = WebViewController()
        ..setJavaScriptMode(JavaScriptMode.unrestricted)
        ..addJavaScriptChannel(
          'PtMobile',
          onMessageReceived: (msg) => unawaited(_message(msg.message)),
        )
        ..setNavigationDelegate(
          NavigationDelegate(
            onNavigationRequest: (r) => _local(r.url)
                ? NavigationDecision.navigate
                : NavigationDecision.prevent,
            onPageStarted: (_) {
              _trusted = false;
            },
            onPageFinished: (url) async {
              if (!_local(url) || !mounted) return;
              _trusted = true;
              final config = {
                'canWrite': ref.read(canWriteProvider),
                'lang': Localizations.localeOf(context).languageCode,
                'dark': Theme.of(context).brightness == Brightness.dark,
              };
              await _controller?.runJavaScript(
                'window.ptMobileConfig=${jsonEncode(config)};window.dispatchEvent(new Event("ptMobileReady"));',
              );
              if (mounted) setState(() => _loading = false);
            },
            onWebResourceError: (e) {
              if (e.isForMainFrame == true && mounted) {
                setState(() {
                  _error = e.description;
                  _loading = false;
                });
              }
            },
          ),
        );
    }
  }

  bool _started = false;
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_started && _controller != null) {
      _started = true;
      unawaited(_loadAssets());
    }
  }

  Future<void> _loadAssets() async {
    try {
      final assets = await MobileAssets.start();
      if (!mounted) {
        await assets.close();
        return;
      }
      _assets = assets;
      final uri = assets.origin.replace(
        path: _assetPath,
        queryParameters: {
          'page': widget.page,
          if (widget.id != null) 'id': '${widget.id}',
          'lang': Localizations.localeOf(context).languageCode,
          'dark': Theme.of(context).brightness == Brightness.dark ? '1' : '0',
          ...widget.values,
        },
      );
      await _controller!.loadRequest(uri);
    } on Object catch (e) {
      if (mounted) {
        setState(() {
          _loading = false;
          _error = S.of(context).errorPrefix(e.toString());
        });
      }
    }
  }

  bool _local(String url) {
    final u = Uri.tryParse(url);
    return u != null &&
        u.scheme == 'http' &&
        _assets != null &&
        u.origin == _assets!.origin.origin &&
        u.path == _assetPath;
  }

  Future<void> _respond(
    String id, {
    Object? data,
    int? status,
    String? error,
  }) async {
    if (!mounted || !_trusted) return;
    final payload = status == null
        ? {'id': id, 'ok': true, 'data': data}
        : {
            'id': id,
            'ok': false,
            'error': {'status': status, 'message': error},
          };
    await _controller?.runJavaScript(
      'window.ptMobileReceive?.(${jsonEncode(payload)});',
    );
  }

  Future<void> _message(String raw) async {
    if (!_trusted ||
        !mounted ||
        !_local(await _controller!.currentUrl() ?? '')) {
      return;
    }
    String id = '';
    try {
      if (raw.length > 24 << 20) {
        throw const MobileBridgeError(413, 'Message is too large');
      }
      final value = jsonDecode(raw);
      if (value is! Map<String, dynamic>) return;
      id = value['id'] is String ? value['id'] as String : '';
      switch (value['method']) {
        case 'ready':
          return;
        case 'dirty':
          setState(() => _dirty = value['value'] == true);
          return;
        case 'cancel':
          _bridge?.cancel(value['requestId'] as String? ?? '');
          return;
        case 'close':
          if (value['changed'] == true) {
            _dirty = false;
            await _leave(true);
          } else {
            await _back();
          }
          return;
        case 'request':
          if (_bridge == null) {
            throw const MobileBridgeError(503, 'Not connected');
          }
          final result = await _bridge!
              .request(
                id,
                value['verb'] as String? ?? '',
                value['path'] as String? ?? '',
                value['body'],
              )
              .timeout(
                const Duration(seconds: 35),
                onTimeout: () {
                  _bridge?.cancel(id);
                  throw const MobileBridgeError(504, 'Request timed out');
                },
              );
          await _respond(id, data: result);
        case 'image':
          if (_imageBusy) {
            throw const MobileBridgeError(409, 'Image operation in progress');
          }
          final action = value['action'];
          if (widget.page != 'poster' ||
              (action != 'save' && action != 'share')) {
            throw const MobileBridgeError(403, 'Operation is not allowed');
          }
          final encoded = value['base64'];
          if (encoded is! String || encoded.length > (16 << 20) * 4 ~/ 3 + 4) {
            throw const MobileBridgeError(413, 'Image is too large');
          }
          final bytes = base64Decode(encoded);
          if (bytes.length > 16 << 20 ||
              bytes.length < 24 ||
              !listEquals(bytes.sublist(0, 8), [
                137,
                80,
                78,
                71,
                13,
                10,
                26,
                10,
              ])) {
            throw const MobileBridgeError(400, 'Invalid PNG');
          }
          _imageBusy = true;
          try {
            final result = await _images.invokeMethod<bool>(action as String, {
              'bytes': Uint8List.fromList(bytes),
            });
            await _respond(id, data: mobileImageResult(action, result));
          } finally {
            _imageBusy = false;
          }
        default:
          throw const MobileBridgeError(403, 'Operation is not allowed');
      }
    } on MobileBridgeError catch (e) {
      await _respond(id, status: e.status, error: e.message);
    } on Object {
      await _respond(
        id,
        status: 500,
        error: mounted ? S.of(context).connFailed('') : 'Request failed',
      );
    }
  }

  Future<void> _leave(bool changed) async {
    _bridge?.dispose();
    _trusted = false;
    if (!mounted) return;
    setState(() => _leaving = true);
    await WidgetsBinding.instance.endOfFrame;
    if (mounted) Navigator.of(context).pop(changed);
  }

  Future<void> _back() async {
    if (_dirty) {
      final s = S.of(context);
      final leave = await showDialog<bool>(
        context: context,
        builder: (c) => AlertDialog(
          title: Text(s.mobileDiscard),
          content: Text(s.mobileDiscardBody),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(c, false),
              child: Text(s.mobileKeep),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(c, true),
              child: Text(s.mobileDiscard),
            ),
          ],
        ),
      );
      if (leave != true) return;
    }
    await _leave(false);
  }

  @override
  void dispose() {
    _trusted = false;
    _bridge?.dispose();
    unawaited(_assets?.close());
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return PopScope(
      canPop: _leaving,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) unawaited(_back());
      },
      child: Scaffold(
        appBar: AppBar(
          title: Text(widget.page == 'poster' ? s.mobileShare : s.mobileEditor),
          leading: BackButton(onPressed: _back),
        ),
        body: _controller == null
            ? Center(child: Text(s.mobileUnsupported))
            : _error != null
            ? Center(child: Text(_error!))
            : Stack(
                children: [
                  WebViewWidget(controller: _controller!),
                  if (_loading)
                    const Center(child: CircularProgressIndicator()),
                ],
              ),
      ),
    );
  }
}
