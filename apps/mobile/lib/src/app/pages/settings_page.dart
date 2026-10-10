import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../../../l10n/app_localizations.dart';
import '../../remote/connection.dart';
import '../providers.dart';
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
        children: [
          ListTile(
            title: Text(
              s.settingsConnection,
              style: Theme.of(context).textTheme.titleSmall,
            ),
          ),
          ListTile(
            leading: const Icon(Icons.wifi_tethering),
            title: Text(stateText),
            subtitle: Text('${s.settingsVia}：${viaLabel(s, st?.via)}'),
          ),
          if (st?.endpoint != null)
            ListTile(
              leading: const Icon(Icons.link),
              title: Text(s.settingsEndpoint),
              subtitle: Text(st!.endpoint!),
            ),
          ListTile(
            leading: const Icon(Icons.computer),
            title: Text(s.settingsHost),
            subtitle: Text(
              [
                host.hostId,
                if (meta != null) 'pt-tools ${meta.version}',
              ].join('\n'),
            ),
            isThreeLine: meta != null,
          ),
          ListTile(
            leading: const Icon(Icons.refresh),
            title: Text(s.settingsReconnect),
            onTap: () {
              final c = ref.read(connectionProvider);
              c?.stop();
              c?.start();
            },
          ),
          const Divider(),
          ListTile(
            title: Text(
              s.settingsDevice,
              style: Theme.of(context).textTheme.titleSmall,
            ),
          ),
          ListTile(
            leading: const Icon(Icons.smartphone),
            title: Text(host.deviceName),
            subtitle: Text(
              s.settingsPairedAt(
                DateFormat('yyyy-MM-dd HH:mm').format(host.pairedAt.toLocal()),
              ),
            ),
          ),
          ListTile(
            leading: const Icon(Icons.verified_user_outlined),
            title: Text(s.settingsScopes),
            subtitle: Text(canWrite ? s.scopeFull : s.scopeRead),
          ),
          ListTile(
            leading: Icon(
              Icons.link_off,
              color: Theme.of(context).colorScheme.error,
            ),
            title: Text(
              s.settingsForget,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
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
                      onPressed: () => Navigator.pop(c, true),
                      child: Text(s.settingsForget),
                    ),
                  ],
                ),
              );
              if (ok == true) await ref.read(hostProvider.notifier).forget();
            },
          ),
        ],
      ),
    );
  }
}
