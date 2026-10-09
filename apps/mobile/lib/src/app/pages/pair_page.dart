import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../../../l10n/app_localizations.dart';
import '../../remote/link.dart';
import '../../remote/pairing.dart';
import '../providers.dart';

/// 配对：扫网页上的二维码，或者粘贴链接；填设备名；配对成功以后存下主机，进入概览。
class PairPage extends ConsumerStatefulWidget {
  const PairPage({super.key, this.initialLink});

  /// 用 App 打开配对链接时带进来的链接（填进输入框，等用户点配对）
  final String? initialLink;

  @override
  ConsumerState<PairPage> createState() => _PairPageState();
}

class _PairPageState extends ConsumerState<PairPage> {
  final _link = TextEditingController();
  final _name = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    if (widget.initialLink != null) _link.text = widget.initialLink!;
  }

  @override
  void dispose() {
    _link.dispose();
    _name.dispose();
    super.dispose();
  }

  Future<void> _scan() async {
    final raw = await Navigator.of(context)
        .push<String>(MaterialPageRoute(builder: (_) => const _ScanPage()));
    // 扫到的链接填进输入框，等用户看过设备名再点配对（和打开链接时一样）
    if (raw != null && mounted) {
      setState(() {
        _link.text = raw;
        _error = null;
      });
    }
  }

  Future<void> _paste() async {
    final data = await Clipboard.getData(Clipboard.kTextPlain);
    if (data?.text != null) setState(() => _link.text = data!.text!.trim());
  }

  Future<void> _pair() async {
    final s = S.of(context);
    final PairingLink link;
    try {
      link = PairingLink.parse(_link.text);
    } on LinkException catch (e) {
      setState(
        () => _error = e.unsupportedVersion
            ? s.pairUpgrade
            : s.pairInvalidLink(e.message),
      );
      return;
    }
    final name = _name.text.trim().isEmpty
        ? s.pairNameDefault
        : _name.text.trim();
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final host = await pairWithLink(
        link,
        name,
        channel: ref.read(channelFactoryProvider),
        client: appClientName,
      );
      await ref.read(hostProvider.notifier).paired(host);
    } on PairingFailure catch (e) {
      if (mounted) setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final t = Theme.of(context);
    final canScan =
        !kIsWeb &&
        (defaultTargetPlatform == TargetPlatform.android ||
            defaultTargetPlatform == TargetPlatform.iOS);
    return Scaffold(
      appBar: AppBar(title: Text(s.pairTitle)),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            Row(
              children: [
                Icon(Icons.qr_code_2, size: 40, color: t.colorScheme.primary),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(s.pairIntro, style: t.textTheme.bodyMedium),
                ),
              ],
            ),
            const SizedBox(height: 20),
            if (canScan) ...[
              FilledButton.icon(
                onPressed: _busy ? null : _scan,
                icon: const Icon(Icons.qr_code_scanner),
                label: Text(s.pairScan),
                style: FilledButton.styleFrom(
                  minimumSize: const Size.fromHeight(48),
                ),
              ),
              const SizedBox(height: 20),
            ],
            TextField(
              key: const Key('pair-link'),
              controller: _link,
              enabled: !_busy,
              minLines: 2,
              maxLines: 4,
              decoration: InputDecoration(
                labelText: s.pairLinkLabel,
                hintText: 'pttools://pair?v=1&…',
                border: const OutlineInputBorder(),
                suffixIcon: IconButton(
                  tooltip: s.paste,
                  icon: const Icon(Icons.content_paste),
                  onPressed: _busy ? null : _paste,
                ),
              ),
            ),
            const SizedBox(height: 16),
            TextField(
              key: const Key('pair-name'),
              controller: _name,
              enabled: !_busy,
              maxLength: 64,
              decoration: InputDecoration(
                labelText: s.pairNameLabel,
                hintText: s.pairNameDefault,
                border: const OutlineInputBorder(),
              ),
            ),
            if (_error != null) ...[
              const SizedBox(height: 8),
              Text(
                _error!,
                key: const Key('pair-error'),
                style: TextStyle(color: t.colorScheme.error),
              ),
            ],
            const SizedBox(height: 16),
            FilledButton(
              key: const Key('pair-submit'),
              onPressed: _busy ? null : _pair,
              style: FilledButton.styleFrom(
                minimumSize: const Size.fromHeight(48),
              ),
              child: _busy
                  ? Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        ),
                        const SizedBox(width: 10),
                        Text(s.pairWorking),
                      ],
                    )
                  : Text(s.pairButton),
            ),
            const SizedBox(height: 24),
            Text(s.pairPrivacy, style: t.textTheme.bodySmall),
          ],
        ),
      ),
    );
  }
}

/// 扫码页：读到 pttools:// 开头的二维码就返回。
class _ScanPage extends StatefulWidget {
  const _ScanPage();

  @override
  State<_ScanPage> createState() => _ScanPageState();
}

class _ScanPageState extends State<_ScanPage> {
  final _ctl = MobileScannerController(formats: const [BarcodeFormat.qrCode]);
  bool _done = false;

  @override
  void dispose() {
    unawaited(_ctl.dispose());
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(s.scanTitle)),
      body: MobileScanner(
        controller: _ctl,
        errorBuilder: (context, error) => Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text(s.scanUnavailable),
          ),
        ),
        onDetect: (capture) {
          if (_done) return;
          for (final b in capture.barcodes) {
            final v = b.rawValue;
            if (v != null && v.startsWith('pttools://')) {
              _done = true;
              Navigator.of(context).pop(v);
              return;
            }
          }
        },
      ),
    );
  }
}
