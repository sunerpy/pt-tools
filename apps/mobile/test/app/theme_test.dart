// 主题与图标：每套主题的字在它实际所在的底上都过 WCAG AA，选择的读写与映射，图标的路径都读得懂。
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:pt_tools_app/src/app/appearance.dart';
import 'package:pt_tools_app/src/app/icons.dart';
import 'package:pt_tools_app/src/app/theme.dart';

double _luminance(Color c) {
  double ch(double v) =>
      v <= 0.03928 ? v / 12.92 : math.pow((v + 0.055) / 1.055, 2.4).toDouble();
  return 0.2126 * ch(c.r) + 0.7152 * ch(c.g) + 0.0722 * ch(c.b);
}

double _contrast(Color a, Color b) {
  final la = _luminance(a), lb = _luminance(b);
  final hi = math.max(la, lb), lo = math.min(la, lb);
  return (hi + 0.05) / (lo + 0.05);
}

void main() {
  for (final theme in PtTheme.values) {
    group('主题 ${theme.name}', () {
      final p = PtPalette.of(theme);
      final data = buildTheme(theme);
      final c = data.colorScheme;
      // 正文与次要文字会出现在页面底、卡片面与按下 / 选中的浅灰底上
      final grounds = [p.canvas, p.surface, p.inset2, p.navActive];

      test('明暗与主题一致', () {
        expect(c.brightness == Brightness.dark, theme.isDark);
        expect(data.scaffoldBackgroundColor, p.canvas);
      });

      test('正文、次要文字与链接都不低于 4.5:1', () {
        for (final g in grounds) {
          expect(_contrast(c.onSurface, g), greaterThanOrEqualTo(4.5));
          expect(_contrast(c.onSurfaceVariant, g), greaterThanOrEqualTo(4.5));
          expect(_contrast(p.accentText, g), greaterThanOrEqualTo(4.5));
        }
      });

      test('墨色按钮上的字不低于 4.5:1', () {
        expect(_contrast(c.onPrimary, c.primary), greaterThanOrEqualTo(4.5));
        expect(_contrast(c.onError, c.error), greaterThanOrEqualTo(4.5));
      });

      test('状态色的字压在卡片面、页面底与自己 12% 的淡染底上都不低于 4.5:1', () {
        for (final s in [p.ok, p.warn, p.danger, p.info, p.violet, p.pink]) {
          for (final g in [p.surface, p.canvas, tint(s, p.surface, 0.12)]) {
            expect(
              _contrast(s, g),
              greaterThanOrEqualTo(4.5),
              reason: '$s on $g',
            );
          }
        }
      });

      test('开关的轨道（强调色）与卡片面不低于 3:1', () {
        expect(_contrast(p.accent, p.surface), greaterThanOrEqualTo(3));
      });
    });
  }

  group('主题的选择', () {
    test('读不懂的值当作跟随系统', () {
      expect(ThemeChoice.parse(null), ThemeChoice.system);
      expect(ThemeChoice.parse('glass'), ThemeChoice.system);
      expect(ThemeChoice.parse('warm'), ThemeChoice.warm);
    });

    test('跟随系统是明亮与暗黑；固定的主题只用自己', () {
      expect(ThemeChoice.system.mode, ThemeMode.system);
      expect(ThemeChoice.system.shown(Brightness.light), PtTheme.light);
      expect(ThemeChoice.system.shown(Brightness.dark), PtTheme.dark);
      expect(ThemeChoice.warm.mode, ThemeMode.light);
      expect(ThemeChoice.warm.lightTheme, PtTheme.warm);
      expect(ThemeChoice.warm.shown(Brightness.dark), PtTheme.warm);
      expect(ThemeChoice.graphite.mode, ThemeMode.dark);
      expect(ThemeChoice.graphite.darkTheme, PtTheme.graphite);
      expect(ThemeChoice.dark.shown(Brightness.light), PtTheme.dark);
    });

    test('启动时读不出来就跟随系统', () async {
      expect(await loadInitialTheme(_BrokenStore()), ThemeChoice.system);
      expect(
        await loadInitialTheme(MemoryAppearanceStore(ThemeChoice.graphite)),
        ThemeChoice.graphite,
      );
    });
  });

  group('图标', () {
    test('每个图标的路径都读得懂，并且画在 24 × 24 的画布里', () {
      // 沿着线取点（getBounds 是控制点的包围盒，圆弧会被高估）；圆弧方向画反了也会跑出画布
      for (final g in PtGlyph.values) {
        final metrics = parseSvgPath(g.data).computeMetrics().toList();
        expect(metrics, isNotEmpty, reason: g.name);
        for (final m in metrics) {
          for (var d = 0.0; d <= m.length; d += 0.25) {
            final o = m.getTangentForOffset(d)!.position;
            expect(o.dx, inInclusiveRange(-0.01, 24.01), reason: g.name);
            expect(o.dy, inInclusiveRange(-0.01, 24.01), reason: g.name);
          }
        }
      }
    });

    test('相对坐标、省略的命令、紧挨着的数字与圆弧', () {
      // 「M3 10.5 12 3」里第二对坐标是 L；「0-7」「.3」这样的数字也要拆得开
      final house = parseSvgPath(
        'M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z',
      ).getBounds();
      expect(house.left, closeTo(3, 0.01));
      expect(house.top, closeTo(3, 0.01));
      expect(house.right, closeTo(21, 0.01));
      expect(house.bottom, closeTo(21, 0.01));
      final circle = parseSvgPath(
        'M12 8.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7z',
      ).getBounds();
      expect(circle.width, closeTo(7, 0.05));
      expect(circle.height, closeTo(7, 0.05));
      expect(circle.center.dx, closeTo(12, 0.05));
      expect(() => parseSvgPath('X1 2'), throwsFormatException);
      expect(() => parseSvgPath('M1'), throwsFormatException);
    });

    testWidgets('颜色与大小跟着 IconTheme', (tester) async {
      await tester.pumpWidget(
        const IconTheme(
          data: IconThemeData(size: 30, color: Color(0xFF123456)),
          child: Center(child: PtIcon(PtGlyph.search)),
        ),
      );
      expect(tester.getSize(find.byType(PtIcon)), const Size(30, 30));
    });
  });
}

class _BrokenStore implements AppearanceStore {
  @override
  Future<ThemeChoice> loadTheme() => Future.error(StateError('no storage'));

  @override
  Future<void> saveTheme(ThemeChoice choice) async {}
}
