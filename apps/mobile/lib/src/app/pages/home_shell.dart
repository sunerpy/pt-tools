import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../../l10n/app_localizations.dart';
import '../widgets/common.dart';

/// 底部导航：概览、种子、搜索、媒体、更多；顶部是连接条。
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
      bottomNavigationBar: NavigationBar(
        selectedIndex: shell.currentIndex,
        onDestinationSelected: (i) =>
            shell.goBranch(i, initialLocation: i == shell.currentIndex),
        destinations: [
          NavigationDestination(
            icon: const Icon(Icons.dashboard_outlined),
            selectedIcon: const Icon(Icons.dashboard),
            label: s.navOverview,
          ),
          NavigationDestination(
            icon: const Icon(Icons.swap_vert_circle_outlined),
            selectedIcon: const Icon(Icons.swap_vert_circle),
            label: s.navTorrents,
          ),
          NavigationDestination(
            icon: const Icon(Icons.search),
            label: s.navSearch,
          ),
          NavigationDestination(
            icon: const Icon(Icons.movie_outlined),
            selectedIcon: const Icon(Icons.movie),
            label: s.navMedia,
          ),
          NavigationDestination(
            icon: const Icon(Icons.more_horiz),
            label: s.navMore,
          ),
        ],
      ),
    );
  }
}
