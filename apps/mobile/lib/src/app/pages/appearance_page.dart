import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../l10n/app_localizations.dart';
import '../appearance.dart';
import '../icons.dart';
import '../theme.dart';
import '../widgets/common.dart';

/// 外观：四套主题的缩略图（照 Voltip 的外观页，两列），下面是「跟随系统」。
/// 点一套主题就固定用它（跟随系统随之关掉）；跟随系统时选中的是现在显示的那套。
class AppearancePage extends ConsumerWidget {
  const AppearancePage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final choice = ref.watch(themeChoiceProvider);
    final shown = choice.shown(MediaQuery.platformBrightnessOf(context));
    final notifier = ref.read(themeChoiceProvider.notifier);
    return Scaffold(
      appBar: AppBar(title: Text(s.moreAppearance)),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
        children: [
          RowGroup(
            title: s.appearanceTheme,
            children: [
              Padding(
                padding: const EdgeInsets.all(14),
                child: LayoutBuilder(
                  builder: (context, c) {
                    const gap = 12.0;
                    final w = (c.maxWidth - gap) / 2;
                    return Wrap(
                      spacing: gap,
                      runSpacing: 16,
                      children: [
                        for (final t in PtTheme.values)
                          SizedBox(
                            width: w,
                            child: ThemeTile(
                              theme: t,
                              selected: shown == t,
                              onTap: () => notifier.set(
                                ThemeChoice.values.byName(t.name),
                              ),
                            ),
                          ),
                      ],
                    );
                  },
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          RowGroup(
            children: [
              SwitchListTile(
                key: const Key('appearance-follow-system'),
                title: Text(s.appearanceFollowSystem),
                subtitle: Text(s.appearanceFollowSystemHelp),
                value: choice == ThemeChoice.system,
                onChanged: (on) => notifier.set(
                  on
                      ? ThemeChoice.system
                      : ThemeChoice.values.byName(shown.name),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

/// 主题的名字与副名（明亮 · 白瓷……）。
(String, String) themeNames(S s, PtTheme t) => switch (t) {
  PtTheme.light => (s.themeLight, s.themeLightSub),
  PtTheme.dark => (s.themeDark, s.themeDarkSub),
  PtTheme.warm => (s.themeWarm, s.themeWarmSub),
  PtTheme.graphite => (s.themeGraphite, s.themeGraphiteSub),
};

/// 现在的选择怎么写（更多页那一行的说明）。
String themeChoiceLabel(S s, ThemeChoice c) => switch (c) {
  ThemeChoice.system => s.appearanceFollowSystem,
  _ => themeNames(s, PtTheme.values.byName(c.name)).$1,
};

/// 一套主题的缩略图：用这套主题自己的颜色画一个小手机界面（顶栏、一张卡片、底栏），下面是名字。
class ThemeTile extends StatelessWidget {
  const ThemeTile({
    super.key,
    required this.theme,
    required this.selected,
    required this.onTap,
  });

  final PtTheme theme;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final cur = Theme.of(context);
    final p = PtPalette.of(theme);
    final (name, sub) = themeNames(s, theme);
    final edge = selected
        ? BorderSide(color: cur.colorScheme.primary, width: 2)
        : BorderSide(color: context.pt.hairlineStrong);
    Widget bar(double w, double h, Color c) => Container(
      width: w,
      height: h,
      decoration: BoxDecoration(
        color: c,
        borderRadius: BorderRadius.circular(h / 2),
      ),
    );
    final preview = Container(
      height: 112,
      decoration: BoxDecoration(
        color: p.canvas,
        borderRadius: BorderRadius.circular(12),
        border: Border.fromBorderSide(edge),
      ),
      clipBehavior: Clip.antiAlias,
      child: Stack(
        children: [
          Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Container(
                height: 18,
                padding: const EdgeInsets.symmetric(horizontal: 8),
                alignment: Alignment.centerLeft,
                decoration: BoxDecoration(
                  color: p.surface,
                  border: Border(bottom: BorderSide(color: p.border)),
                ),
                child: bar(26, 4, p.fg),
              ),
              Expanded(
                child: Padding(
                  padding: const EdgeInsets.all(8),
                  child: Container(
                    padding: const EdgeInsets.all(7),
                    decoration: BoxDecoration(
                      color: p.surface,
                      border: Border.all(color: p.border),
                      borderRadius: BorderRadius.circular(7),
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisAlignment: MainAxisAlignment.spaceBetween,
                      children: [
                        Row(
                          children: [
                            Container(
                              width: 6,
                              height: 6,
                              decoration: BoxDecoration(
                                color: p.ok,
                                shape: BoxShape.circle,
                              ),
                            ),
                            const SizedBox(width: 5),
                            bar(34, 4, p.fg),
                          ],
                        ),
                        Row(
                          children: [
                            for (var i = 0; i < 5; i++) ...[
                              if (i > 0) const SizedBox(width: 2),
                              Container(
                                width: 7,
                                height: 7,
                                decoration: BoxDecoration(
                                  color: i < 3 ? p.accent : p.border,
                                  borderRadius: BorderRadius.circular(1.5),
                                ),
                              ),
                            ],
                            const Spacer(),
                            Container(
                              width: 22,
                              height: 9,
                              decoration: BoxDecoration(
                                color: p.fg,
                                borderRadius: BorderRadius.circular(3),
                              ),
                            ),
                          ],
                        ),
                        bar(20, 3, p.subtle),
                      ],
                    ),
                  ),
                ),
              ),
              Container(
                height: 18,
                decoration: BoxDecoration(
                  color: p.surface,
                  border: Border(top: BorderSide(color: p.border)),
                ),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceEvenly,
                  children: [
                    Container(
                      width: 18,
                      height: 8,
                      decoration: BoxDecoration(
                        color: p.navActive,
                        borderRadius: BorderRadius.circular(4),
                      ),
                    ),
                    for (var i = 0; i < 3; i++) bar(8, 3, p.muted),
                  ],
                ),
              ),
            ],
          ),
          if (selected)
            Positioned(
              top: 24,
              right: 6,
              child: Container(
                width: 18,
                height: 18,
                decoration: BoxDecoration(
                  color: cur.colorScheme.primary,
                  shape: BoxShape.circle,
                ),
                alignment: Alignment.center,
                child: PtIcon(
                  PtGlyph.check,
                  size: 12,
                  weight: 3,
                  color: cur.colorScheme.onPrimary,
                ),
              ),
            ),
        ],
      ),
    );
    return Semantics(
      key: Key('theme-${theme.name}'),
      button: true,
      inMutuallyExclusiveGroup: true,
      checked: selected,
      label: '$name $sub',
      excludeSemantics: true,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            preview,
            const SizedBox(height: 8),
            Row(
              crossAxisAlignment: CrossAxisAlignment.baseline,
              textBaseline: TextBaseline.alphabetic,
              children: [
                Text(
                  name,
                  style: cur.textTheme.bodyMedium?.copyWith(
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const SizedBox(width: 6),
                Flexible(
                  child: Text(
                    sub,
                    overflow: TextOverflow.ellipsis,
                    style: cur.textTheme.bodySmall?.copyWith(
                      color: cur.colorScheme.onSurfaceVariant,
                    ),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
