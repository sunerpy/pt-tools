import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../l10n/app_localizations.dart';
import '../providers.dart';
import '../widgets/common.dart';

class MorePage extends ConsumerWidget {
  const MorePage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final st = ref.watch(connectionStatusProvider).value;
    final unread =
        ref.watch(overviewProvider).value?.totals.unreadMessages ?? 0;
    // 主机启用了 AI 助手才显示入口（主机的版本没有这个功能时也不显示）
    final assistant =
        ref.watch(assistantStatusProvider).value?.enabled ?? false;
    return Scaffold(
      appBar: AppBar(title: Text(s.navMore)),
      body: ListView(
        children: [
          if (assistant)
            ListTile(
              key: const Key('more-assistant'),
              leading: const Icon(Icons.smart_toy_outlined),
              title: Text(s.moreAssistant),
              trailing: const Icon(Icons.chevron_right),
              onTap: () => context.go('/more/assistant'),
            ),
          ListTile(
            leading: const Icon(Icons.public),
            title: Text(s.moreSites),
            trailing: unread > 0
                ? Badge(label: Text('$unread'))
                : const Icon(Icons.chevron_right),
            onTap: () => context.go('/more/sites'),
          ),
          ListTile(
            leading: const Icon(Icons.rss_feed),
            title: Text(s.moreTasks),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.go('/more/tasks'),
          ),
          ListTile(
            leading: const Icon(Icons.autorenew),
            title: Text(s.moreBrush),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.go('/more/brush'),
          ),
          ListTile(
            leading: const Icon(Icons.dns_outlined),
            title: Text(s.moreDownloaders),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.go('/more/downloaders'),
          ),
          const Divider(),
          ListTile(
            leading: const Icon(Icons.phonelink_lock_outlined),
            title: Text(s.moreSettings),
            subtitle: st == null ? null : Text(viaLabel(s, st.via)),
            trailing: const Icon(Icons.chevron_right),
            onTap: () => context.go('/more/settings'),
          ),
        ],
      ),
    );
  }
}
