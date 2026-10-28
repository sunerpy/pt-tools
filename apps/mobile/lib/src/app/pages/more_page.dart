import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../l10n/app_localizations.dart';
import '../appearance.dart';
import '../icons.dart';
import '../providers.dart';
import '../theme.dart';
import '../widgets/common.dart';
import 'appearance_page.dart';

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
    final theme = ref.watch(themeChoiceProvider);
    return Scaffold(
      appBar: AppBar(title: Text(s.navMore)),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
        children: [
          RowGroup(
            title: s.moreGroupTools,
            children: [
              if (assistant)
                NavRow(
                  key: const Key('more-assistant'),
                  icon: PtGlyph.sparkles,
                  title: s.moreAssistant,
                  onTap: () => context.go('/more/assistant'),
                ),
              NavRow(
                icon: PtGlyph.globe,
                title: s.moreSites,
                trailing: unread > 0
                    ? Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Badge(label: Text('$unread')),
                          const SizedBox(width: 6),
                          PtIcon(
                            PtGlyph.chevronRight,
                            size: 18,
                            color: context.pt.subtle,
                          ),
                        ],
                      )
                    : null,
                onTap: () => context.go('/more/sites'),
              ),
              NavRow(
                icon: PtGlyph.rss,
                title: s.moreTasks,
                onTap: () => context.go('/more/tasks'),
              ),
              NavRow(
                icon: PtGlyph.cycle,
                title: s.moreBrush,
                onTap: () => context.go('/more/brush'),
              ),
              NavRow(
                icon: PtGlyph.server,
                title: s.moreDownloaders,
                onTap: () => context.go('/more/downloaders'),
              ),
            ],
          ),
          const SizedBox(height: 20),
          RowGroup(
            title: s.moreGroupApp,
            children: [
              NavRow(
                key: const Key('more-appearance'),
                icon: PtGlyph.palette,
                title: s.moreAppearance,
                subtitle: themeChoiceLabel(s, theme),
                onTap: () => context.go('/more/appearance'),
              ),
              NavRow(
                icon: PtGlyph.phone,
                title: s.moreSettings,
                subtitle: st == null ? null : viaLabel(s, st.via),
                onTap: () => context.go('/more/settings'),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
