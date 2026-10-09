// App 的外壳：主题、路由（没有配对时去配对页）、前后台切换时连上与断开。
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../l10n/app_localizations.dart';
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

/// 品牌色（与网页的 logo 一致）。
const brandTeal = Color(0xFF14B8A6);
const brandOrange = Color(0xFFF97316);

ThemeData buildTheme(Brightness b) {
  final scheme = ColorScheme.fromSeed(
    seedColor: brandTeal,
    brightness: b,
    secondary: brandOrange,
  );
  return ThemeData(
    colorScheme: scheme,
    useMaterial3: true,
    visualDensity: VisualDensity.standard,
    cardTheme: const CardThemeData(elevation: 0, margin: EdgeInsets.zero),
    listTileTheme: const ListTileThemeData(
      contentPadding: EdgeInsets.symmetric(horizontal: 16),
    ),
  );
}

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
      // 用这个 App 打开的配对链接（pttools://pair?v=1&h=…）：路由拿到的是它的查询串
      final q = state.uri.queryParameters;
      if (!paired &&
          state.matchedLocation != '/pair' &&
          q.containsKey('h') &&
          q.containsKey('s')) {
        return Uri(
          path: '/pair',
          queryParameters: {'link': 'pttools://pair?${state.uri.query}'},
        ).toString();
      }
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
    return MaterialApp.router(
      onGenerateTitle: (context) => S.of(context).appTitle,
      theme: buildTheme(Brightness.light),
      darkTheme: buildTheme(Brightness.dark),
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
