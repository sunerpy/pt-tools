// App 的外壳：主题（见 theme.dart）、路由（没有配对时去配对页）、前后台切换时连上与断开。
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../l10n/app_localizations.dart';
import 'appearance.dart';
import 'pages/appearance_page.dart';
import 'pages/assistant_page.dart';
import 'pages/brush_page.dart';
import 'pages/downloaders_page.dart';
import 'pages/home_shell.dart';
import 'pages/media_page.dart';
import 'pages/more_page.dart';
import 'pages/overview_page.dart';
import 'pages/pair_page.dart';
import 'pages/search_page.dart';
import 'pages/settings_page.dart';
import 'pages/sites_page.dart';
import 'pages/subscription_page.dart';
import 'pages/tasks_page.dart';
import 'pages/torrents_page.dart';
import 'providers.dart';
import 'theme.dart';

final routerProvider = Provider<GoRouter>((ref) {
  final refresh = ValueNotifier<int>(0);
  ref.listen(hostProvider, (_, _) => refresh.value++);
  ref.onDispose(refresh.dispose);
  return GoRouter(
    initialLocation: '/',
    refreshListenable: refresh,
    redirect: (context, state) {
      final host = ref.read(hostProvider);
      if (host.isLoading && !host.hasValue) return null;
      final paired = host.value != null;
      final atPair = state.matchedLocation == '/pair';
      if (!paired && !atPair) return '/pair';
      if (paired && atPair) return '/';
      return null;
    },
    routes: [
      GoRoute(
        path: '/pair',
        builder: (_, state) =>
            PairPage(initialLink: state.uri.queryParameters['link']),
      ),
      StatefulShellRoute.indexedStack(
        builder: (_, _, shell) => HomeShell(shell: shell),
        branches: [
          StatefulShellBranch(
            routes: [
              GoRoute(path: '/', builder: (_, _) => const OverviewPage()),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/torrents',
                builder: (_, _) => const TorrentsPage(),
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(path: '/search', builder: (_, _) => const SearchPage()),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/media',
                builder: (_, _) => const MediaPage(),
                routes: [
                  GoRoute(
                    path: 'subscriptions/:id',
                    builder: (_, state) => SubscriptionPage(
                      id: int.parse(state.pathParameters['id']!),
                    ),
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/more',
                builder: (_, _) => const MorePage(),
                routes: [
                  GoRoute(path: 'sites', builder: (_, _) => const SitesPage()),
                  GoRoute(
                    path: 'assistant',
                    builder: (_, _) => const AssistantPage(),
                  ),
                  GoRoute(path: 'tasks', builder: (_, _) => const TasksPage()),
                  GoRoute(path: 'brush', builder: (_, _) => const BrushPage()),
                  GoRoute(
                    path: 'downloaders',
                    builder: (_, _) => const DownloadersPage(),
                  ),
                  GoRoute(
                    path: 'settings',
                    builder: (_, _) => const SettingsPage(),
                  ),
                  GoRoute(
                    path: 'appearance',
                    builder: (_, _) => const AppearancePage(),
                  ),
                ],
              ),
            ],
          ),
        ],
      ),
    ],
  );
});

class PtToolsApp extends ConsumerStatefulWidget {
  const PtToolsApp({super.key});

  @override
  ConsumerState<PtToolsApp> createState() => _PtToolsAppState();
}

class _PtToolsAppState extends ConsumerState<PtToolsApp> {
  late final AppLifecycleListener _life;

  @override
  void initState() {
    super.initState();
    // v1 只在前台保持连接：到了后台断开，回到前台重连（见 connectionProvider）
    _life = AppLifecycleListener(
      onResume: () => ref.read(foregroundProvider.notifier).set(true),
      onHide: () => ref.read(foregroundProvider.notifier).set(false),
    );
  }

  @override
  void dispose() {
    _life.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final choice = ref.watch(themeChoiceProvider);
    return MaterialApp.router(
      onGenerateTitle: (context) => S.of(context).appTitle,
      theme: buildTheme(choice.lightTheme),
      darkTheme: buildTheme(choice.darkTheme),
      themeMode: choice.mode,
      routerConfig: ref.watch(routerProvider),
      localizationsDelegates: const [
        S.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: S.supportedLocales,
    );
  }
}
