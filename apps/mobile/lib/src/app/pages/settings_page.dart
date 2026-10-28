import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../../../l10n/app_localizations.dart';
import '../../remote/connection.dart';
import '../icons.dart';
import '../providers.dart';
import '../theme.dart';
import '../widgets/common.dart';

/// 连接与设备：现在怎么连的（直连或 relay、地址）、主机版本、这台设备的名字与权限；重新连接、忘掉这台主机。
class SettingsPage extends ConsumerWidget {
  const SettingsPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final host = ref.watch(hostProvider).value;
    final st = ref.watch(connectionStatusProvider).value;
    final meta = ref.watch(metaProvider).value;
    final canWrite = ref.watch(canWriteProvider);
    if (host == null) return const SizedBox.shrink();
    final stateText = switch (st?.state) {
      LinkState.online => s.connOnline,
      LinkState.connecting => s.connConnecting,
      LinkState.revoked => s.connRevoked,
      LinkState.disabled => s.connDisabled,
      LinkState.failed => s.connFailed(st?.message ?? ''),
      _ => s.connOffline,
    };
    return Scaffold(
      appBar: AppBar(title: Text(s.moreSettings)),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
        children: [
          RowGroup(
            title: s.settingsConnection,
            children: [
              ListTile(
                leading: PtIcon(
                  PtGlyph.broadcast,
                  size: 20,
                  color: st?.state == LinkState.online ? context.pt.ok : null,
                ),
                title: Text(stateText),
                subtitle: Text('${s.settingsVia}：${viaLabel(s, st?.via)}'),
              ),
              if (st?.endpoint != null)
                ListTile(
                  leading: const PtIcon(PtGlyph.link, size: 20),
                  title: Text(s.settingsEndpoint),
                  subtitle: Text(st!.endpoint!),
                ),
              ListTile(
                leading: const PtIcon(PtGlyph.monitor, size: 20),
                title: Text(s.settingsHost),
                subtitle: Text(
                  [
                    host.hostId,
                    if (meta != null) 'pt-tools ${meta.version}',
                  ].join('\n'),
                ),
                isThreeLine: meta != null,
              ),
              NavRow(
                icon: PtGlyph.refresh,
                title: s.settingsReconnect,
                trailing: const SizedBox.shrink(),
                onTap: () {
                  final c = ref.read(connectionProvider);
                  c?.stop();
                  c?.start();
                },
              ),
            ],
          ),
          const SizedBox(height: 20),
          RowGroup(
            title: s.settingsDevice,
            children: [
              ListTile(
                leading: const PtIcon(PtGlyph.phone, size: 20),
                title: Text(host.deviceName),
                subtitle: Text(
                  s.settingsPairedAt(
                    DateFormat('yyyy-MM-dd HH:mm')
                        .format(host.pairedAt.toLocal()),
                  ),
                ),
              ),
              ListTile(
                leading: const PtIcon(PtGlyph.shieldCheck, size: 20),
                title: Text(s.settingsScopes),
                subtitle: Text(canWrite ? s.scopeFull : s.scopeRead),
              ),
              NavRow(
                icon: PtGlyph.linkOff,
                title: s.settingsForget,
                danger: true,
                onTap: () async {
                  final ok = await showDialog<bool>(
                    context: context,
                    builder: (c) => AlertDialog(
                      title: Text(s.settingsForget),
                      content: Text(s.forgetConfirm),
                      actions: [
                        TextButton(
                          onPressed: () => Navigator.pop(c, false),
                          child: Text(s.cancel),
                        ),
                        FilledButton(
                          style: FilledButton.styleFrom(
                            backgroundColor: Theme.of(c).colorScheme.error,
                            foregroundColor: Theme.of(c).colorScheme.onError,
                          ),
                          onPressed: () => Navigator.pop(c, true),
                          child: Text(s.settingsForget),
                        ),
                      ],
                    ),
                  );
                  if (ok == true) {
                    await ref.read(hostProvider.notifier).forget();
                  }
                },
              ),
            ],
          ),
        ],
      ),
    );
  }
}
