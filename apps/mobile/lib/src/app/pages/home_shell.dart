import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../../l10n/app_localizations.dart';
import '../icons.dart';
import '../theme.dart';
import '../widgets/common.dart';

/// 底部导航：概览、种子、搜索、媒体、更多（白面加顶部细线，当前页是浅灰的胶囊、墨色的图标与字，
/// 图标的线也加粗一点）；顶部是连接条。
class HomeShell extends StatelessWidget {
  const HomeShell({super.key, required this.shell});
  final StatefulNavigationShell shell;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return Scaffold(
      body: Column(
        children: [
          const ConnectionBanner(),
          Expanded(child: shell),
        ],
      ),
      bottomNavigationBar: DecoratedBox(
        // 画在导航条上面：画在下面会被它自己的底色盖住
        position: DecorationPosition.foreground,
        decoration: BoxDecoration(
          border: Border(top: BorderSide(color: context.pt.hairline)),
        ),
        child: NavigationBar(
          selectedIndex: shell.currentIndex,
          onDestinationSelected: (i) =>
              shell.goBranch(i, initialLocation: i == shell.currentIndex),
          destinations: [
            for (final (glyph, label) in [
              (PtGlyph.dashboard, s.navOverview),
              (PtGlyph.torrents, s.navTorrents),
              (PtGlyph.search, s.navSearch),
              (PtGlyph.film, s.navMedia),
              (PtGlyph.more, s.navMore),
            ])
              NavigationDestination(
                icon: PtIcon(glyph),
                selectedIcon: PtIcon(glyph, weight: 2.2),
                label: label,
              ),
          ],
        ),
      ),
    );
  }
}
