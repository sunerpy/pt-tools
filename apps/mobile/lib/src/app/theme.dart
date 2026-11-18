// App 的主题：照 Voltip 手机端的四套经典主题（明亮·白瓷、暗黑·夜灯、暖纸·手稿、石墨·仪表），
// 面、线、字的色值取 Voltip packages/ui/src/tokens.css。主色是墨色（浅色主题近黑、深色主题近白）：
// 主要按钮、选中的分段与筛选芯片、当前页的底栏都用它，黑白分明；强调色（蓝，暖纸是赭石）只用在开关、
// 进度条与链接上。顶栏与卡片是白面加 1 dp 细线，页面是浅灰底，底栏是白面加顶部细线。
// 状态色（成功、警告、危险、信息）与数值卡片的色相在各主题下都按 WCAG AA 校过：字压在它自己 12% 的淡染底、
// 卡片面与页面底上都不低于 4.5:1（暖纸、石墨的几个 Voltip 原值不够，沿明度挪到刚好过线）。
import 'package:flutter/material.dart';

import 'icons.dart';

/// 卡片的圆角（Voltip 手机端：卡片 14、按钮与输入框 12）。
const cardRadius = 14.0;
const controlRadius = 12.0;

/// 一套主题。
enum PtTheme {
  light,
  dark,
  warm,
  graphite;

  bool get isDark => this == PtTheme.dark || this == PtTheme.graphite;
}

/// 「外观」里的选择：跟随系统（系统浅色用明亮、深色用暗黑），或者固定一套主题。
enum ThemeChoice {
  system,
  light,
  dark,
  warm,
  graphite;

  /// 读存下来的值；读不懂的当作跟随系统。
  static ThemeChoice parse(String? v) =>
      values.where((c) => c.name == v).firstOrNull ?? ThemeChoice.system;

  /// 系统是浅色时用的主题。
  PtTheme get lightTheme =>
      this == ThemeChoice.warm ? PtTheme.warm : PtTheme.light;

  /// 系统是深色时用的主题。
  PtTheme get darkTheme =>
      this == ThemeChoice.graphite ? PtTheme.graphite : PtTheme.dark;

  ThemeMode get mode => switch (this) {
    ThemeChoice.system => ThemeMode.system,
    ThemeChoice.light || ThemeChoice.warm => ThemeMode.light,
    ThemeChoice.dark || ThemeChoice.graphite => ThemeMode.dark,
  };

  /// 现在显示的是哪套主题。
  PtTheme shown(Brightness system) => switch (this) {
    ThemeChoice.system =>
      system == Brightness.dark ? PtTheme.dark : PtTheme.light,
    ThemeChoice.light => PtTheme.light,
    ThemeChoice.dark => PtTheme.dark,
    ThemeChoice.warm => PtTheme.warm,
    ThemeChoice.graphite => PtTheme.graphite,
  };
}

/// 配色角色之外的颜色：页面底、细线、内嵌面、强调色，以及状态色（数值卡片、种子与集的状态、免费标记）。
@immutable
class PtColors extends ThemeExtension<PtColors> {
  const PtColors({
    required this.page,
    required this.inset,
    required this.hairline,
    required this.hairlineStrong,
    required this.subtle,
    required this.navActive,
    required this.accent,
    required this.accentText,
    required this.ok,
    required this.warn,
    required this.danger,
    required this.info,
    required this.violet,
    required this.pink,
  });

  /// 页面底；卡片与顶栏是 [ColorScheme.surface]。
  final Color page;

  /// 卡片里的内嵌面（占位、按下）。
  final Color inset;

  /// 卡片、分隔线、进度条轨道的细线；[hairlineStrong] 是输入框与按钮的边。
  final Color hairline;
  final Color hairlineStrong;

  /// 列表行尾的箭头这类装饰（不用于正文与有含义的图标）。
  final Color subtle;

  /// 底栏当前页的底、按下的行。
  final Color navActive;

  /// 强调色（开关、进度条）；[accentText] 是链接这类字的颜色。
  final Color accent;
  final Color accentText;

  final Color ok;
  final Color warn;
  final Color danger;
  final Color info;
  final Color violet;
  final Color pink;

  @override
  PtColors copyWith({
    Color? page,
    Color? inset,
    Color? hairline,
    Color? hairlineStrong,
    Color? subtle,
    Color? navActive,
    Color? accent,
    Color? accentText,
    Color? ok,
    Color? warn,
    Color? danger,
    Color? info,
    Color? violet,
    Color? pink,
  }) => PtColors(
    page: page ?? this.page,
    inset: inset ?? this.inset,
    hairline: hairline ?? this.hairline,
    hairlineStrong: hairlineStrong ?? this.hairlineStrong,
    subtle: subtle ?? this.subtle,
    navActive: navActive ?? this.navActive,
    accent: accent ?? this.accent,
    accentText: accentText ?? this.accentText,
    ok: ok ?? this.ok,
    warn: warn ?? this.warn,
    danger: danger ?? this.danger,
    info: info ?? this.info,
    violet: violet ?? this.violet,
    pink: pink ?? this.pink,
  );

  @override
  PtColors lerp(PtColors? other, double t) {
    if (other == null) return this;
    Color l(Color a, Color b) => Color.lerp(a, b, t)!;
    return PtColors(
      page: l(page, other.page),
      inset: l(inset, other.inset),
      hairline: l(hairline, other.hairline),
      hairlineStrong: l(hairlineStrong, other.hairlineStrong),
      subtle: l(subtle, other.subtle),
      navActive: l(navActive, other.navActive),
      accent: l(accent, other.accent),
      accentText: l(accentText, other.accentText),
      ok: l(ok, other.ok),
      warn: l(warn, other.warn),
      danger: l(danger, other.danger),
      info: l(info, other.info),
      violet: l(violet, other.violet),
      pink: l(pink, other.pink),
    );
  }
}

/// 当前主题的 [PtColors]。
extension PtColorsOf on BuildContext {
  PtColors get pt => Theme.of(this).extension<PtColors>()!;
}

/// 一套主题的全部色值（Voltip tokens.css 的同名位置）。
class PtPalette {
  const PtPalette({
    required this.brightness,
    required this.canvas,
    required this.surface,
    required this.inset,
    required this.inset2,
    required this.border,
    required this.borderStrong,
    required this.fg,
    required this.muted,
    required this.subtle,
    required this.onInk,
    required this.accent,
    required this.accentText,
    required this.inverseAccent,
    required this.navActive,
    required this.ok,
    required this.warn,
    required this.danger,
    required this.info,
    required this.violet,
    required this.pink,
  });

  final Brightness brightness;
  final Color canvas, surface, inset, inset2, border, borderStrong;

  /// 正文（也是墨色主色）、次要文字、装饰。
  final Color fg, muted, subtle;

  /// 压在墨色上的字。
  final Color onInk;

  /// 强调色、强调色的字、反色提示条（深底）上的强调色字。
  final Color accent, accentText, inverseAccent;
  final Color navActive;
  final Color ok, warn, danger, info, violet, pink;

  static PtPalette of(PtTheme t) => switch (t) {
    PtTheme.light => light,
    PtTheme.dark => dark,
    PtTheme.warm => warm,
    PtTheme.graphite => graphite,
  };

  /// 明亮 · 白瓷。状态色用网页 cockpit 校过对比度的那组（Voltip 的 warning 在白底上不到 4.5）。
  static const light = PtPalette(
    brightness: Brightness.light,
    canvas: Color(0xFFF2F3F6),
    surface: Color(0xFFFFFFFF),
    inset: Color(0xFFF7F8FA),
    inset2: Color(0xFFEEF0F3),
    border: Color(0xFFE1E4EA),
    borderStrong: Color(0xFFD6DAE1),
    fg: Color(0xFF1B1D22),
    muted: Color(0xFF5F6570),
    subtle: Color(0xFF8A9099),
    onInk: Color(0xFFFFFFFF),
    // Voltip 的 #339CFF 在白底上只有 2.9:1，开关的轨道要 3:1，取同色相更深一档
    accent: Color(0xFF1677FF),
    accentText: Color(0xFF036ABB),
    inverseAccent: Color(0xFF6CB6FF),
    navActive: Color(0xFFE6E9EE),
    ok: Color(0xFF00773A),
    warn: Color(0xFF995700),
    danger: Color(0xFFB32318),
    info: Color(0xFF1260D7),
    violet: Color(0xFF6D4FE0),
    pink: Color(0xFFBE185D),
  );

  /// 暗黑 · 夜灯。
  static const dark = PtPalette(
    brightness: Brightness.dark,
    canvas: Color(0xFF0C0D11),
    surface: Color(0xFF14161B),
    inset: Color(0xFF1C1F26),
    inset2: Color(0xFF22252D),
    border: Color(0xFF2A2E37),
    borderStrong: Color(0xFF363B45),
    fg: Color(0xFFF1F2F4),
    muted: Color(0xFF9AA1AC),
    subtle: Color(0xFF6B7280),
    onInk: Color(0xFF0C0D11),
    accent: Color(0xFF339CFF),
    accentText: Color(0xFF4FA6FD),
    inverseAccent: Color(0xFF036DC0),
    navActive: Color(0xFF22252D),
    ok: Color(0xFF4CCB93),
    warn: Color(0xFFFFB224),
    danger: Color(0xFFF26D63),
    info: Color(0xFF48A3FD),
    violet: Color(0xFFA78BFA),
    pink: Color(0xFFF472B6),
  );

  /// 暖纸 · 手稿：米色的纸、墨色的字、赭石的强调色。
  static const warm = PtPalette(
    brightness: Brightness.light,
    canvas: Color(0xFFF5EFE4),
    surface: Color(0xFFFDFAF4),
    inset: Color(0xFFF5EFE4),
    inset2: Color(0xFFEEE6D8),
    border: Color(0xFFDED4C4),
    borderStrong: Color(0xFFC9BFAF),
    fg: Color(0xFF2A2521),
    muted: Color(0xFF6D6359),
    subtle: Color(0xFF8A8078),
    onInk: Color(0xFFFDFAF4),
    accent: Color(0xFFB85C38),
    accentText: Color(0xFF9E4A2A),
    inverseAccent: Color(0xFFE9A27F),
    navActive: Color(0xFFEAE2D4),
    ok: Color(0xFF3B7355),
    warn: Color(0xFF8D5E16),
    danger: Color(0xFFB23A30),
    info: Color(0xFF0165C4),
    violet: Color(0xFF7A4FB8),
    pink: Color(0xFFB0306A),
  );

  /// 石墨 · 仪表：不那么黑的深灰。
  static const graphite = PtPalette(
    brightness: Brightness.dark,
    canvas: Color(0xFF22252B),
    surface: Color(0xFF2A2E35),
    inset: Color(0xFF333840),
    inset2: Color(0xFF3A4048),
    border: Color(0xFF414750),
    borderStrong: Color(0xFF4C535D),
    fg: Color(0xFFECEEF1),
    muted: Color(0xFFA5ACB6),
    subtle: Color(0xFF7B838E),
    onInk: Color(0xFF22252B),
    accent: Color(0xFF339CFF),
    accentText: Color(0xFF83BEFE),
    inverseAccent: Color(0xFF036DC0),
    navActive: Color(0xFF333840),
    ok: Color(0xFF3EDDB2),
    warn: Color(0xFFF2C14E),
    danger: Color(0xFFF28780),
    info: Color(0xFF7BBAFE),
    violet: Color(0xFFB9A2FB),
    pink: Color(0xFFF58FC4),
  );
}

/// [c] 按 [t] 的比例混进 [base]（CSS 的 color-mix）。
Color tint(Color c, Color base, double t) => Color.lerp(base, c, t)!;

ThemeData buildTheme(PtTheme theme) {
  final p = PtPalette.of(theme);
  final c = ColorScheme(
    brightness: p.brightness,
    primary: p.fg,
    onPrimary: p.onInk,
    primaryContainer: p.navActive,
    onPrimaryContainer: p.fg,
    inversePrimary: p.inverseAccent,
    secondary: p.muted,
    onSecondary: p.surface,
    secondaryContainer: p.inset2,
    onSecondaryContainer: p.fg,
    tertiary: p.accentText,
    onTertiary: p.onInk,
    tertiaryContainer: tint(p.accent, p.surface, 0.12),
    onTertiaryContainer: p.accentText,
    error: p.danger,
    onError: p.onInk,
    errorContainer: tint(p.danger, p.surface, 0.12),
    onErrorContainer: p.danger,
    surface: p.surface,
    onSurface: p.fg,
    onSurfaceVariant: p.muted,
    surfaceDim: p.canvas,
    surfaceBright: p.surface,
    surfaceContainerLowest: p.surface,
    surfaceContainerLow: p.inset,
    surfaceContainer: p.inset,
    surfaceContainerHigh: p.inset2,
    surfaceContainerHighest: p.inset2,
    outline: p.subtle,
    outlineVariant: p.border,
    shadow: const Color(0xFF000000),
    scrim: const Color(0xFF000000),
    inverseSurface: p.fg,
    onInverseSurface: p.surface,
    // 白卡片、对话框、菜单不叠主色的色调
    surfaceTint: const Color(0x00000000),
  );
  final pt = PtColors(
    page: p.canvas,
    inset: p.inset2,
    hairline: p.border,
    hairlineStrong: p.borderStrong,
    subtle: p.subtle,
    navActive: p.navActive,
    accent: p.accent,
    accentText: p.accentText,
    ok: p.ok,
    warn: p.warn,
    danger: p.danger,
    info: p.info,
    violet: p.violet,
    pink: p.pink,
  );
  final base = ThemeData(colorScheme: c, useMaterial3: true);
  final text = base.textTheme;
  // 组件的字号要写明：ThemeData.textTheme 只有颜色与字族，字号是 Theme.of 时才合进来的，
  // 组件主题里直接用 text.titleLarge 会落到 14 的默认字号
  TextStyle style(
    TextStyle? from,
    double size,
    FontWeight weight, [
    Color? color,
  ]) => (from ?? const TextStyle()).copyWith(
    fontSize: size,
    fontWeight: weight,
    color: color,
  );
  const control = RoundedRectangleBorder(
    borderRadius: BorderRadius.all(Radius.circular(controlRadius)),
  );
  final buttonText = style(text.labelLarge, 15, FontWeight.w600);
  final fieldBorder = OutlineInputBorder(
    borderRadius: BorderRadius.circular(controlRadius),
    borderSide: BorderSide(color: p.borderStrong),
  );
  bool sel(Set<WidgetState> s) => s.contains(WidgetState.selected);

  return base.copyWith(
    visualDensity: VisualDensity.standard,
    extensions: [pt],
    scaffoldBackgroundColor: p.canvas,
    canvasColor: p.canvas,
    dividerColor: p.border,
    iconTheme: IconThemeData(color: p.fg, size: 22),
    // 返回与关闭也用描边图标（Voltip 的返回是朝左的尖角）
    actionIconTheme: ActionIconThemeData(
      backButtonIconBuilder: (_) => const PtIcon(PtGlyph.chevronLeft, size: 24),
      closeButtonIconBuilder: (_) => const PtIcon(PtGlyph.x, size: 22),
    ),
    appBarTheme: AppBarThemeData(
      backgroundColor: p.surface,
      foregroundColor: p.fg,
      elevation: 0,
      scrolledUnderElevation: 0,
      surfaceTintColor: Colors.transparent,
      toolbarHeight: 52,
      shape: Border(bottom: BorderSide(color: p.border)),
      iconTheme: IconThemeData(color: p.fg, size: 22),
      actionsIconTheme: IconThemeData(color: p.fg, size: 22),
      titleTextStyle: style(text.titleLarge, 17, FontWeight.w600, p.fg),
      toolbarTextStyle: style(text.bodyMedium, 14, FontWeight.w400, p.fg),
    ),
    cardTheme: CardThemeData(
      color: p.surface,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(cardRadius),
        side: BorderSide(color: p.border),
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: p.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      height: 62,
      indicatorColor: p.navActive,
      indicatorShape: const StadiumBorder(),
      labelBehavior: NavigationDestinationLabelBehavior.alwaysShow,
      iconTheme: WidgetStateProperty.resolveWith(
        (s) => IconThemeData(size: 21, color: sel(s) ? p.fg : p.muted),
      ),
      labelTextStyle: WidgetStateProperty.resolveWith(
        (s) => style(
          text.labelMedium,
          11.5,
          sel(s) ? FontWeight.w600 : FontWeight.w500,
          sel(s) ? p.fg : p.muted,
        ),
      ),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size(64, 46),
        padding: const EdgeInsets.symmetric(horizontal: 20),
        shape: control,
        textStyle: buttonText,
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size(64, 46),
        padding: const EdgeInsets.symmetric(horizontal: 20),
        shape: control,
        foregroundColor: p.fg,
        backgroundColor: p.surface,
        side: BorderSide(color: p.borderStrong),
        textStyle: buttonText,
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        minimumSize: const Size(48, 44),
        padding: const EdgeInsets.symmetric(horizontal: 14),
        shape: control,
        foregroundColor: p.fg,
        textStyle: buttonText,
      ),
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        minimumSize: const Size(64, 46),
        shape: control,
        elevation: 0,
        backgroundColor: p.surface,
        foregroundColor: p.fg,
        textStyle: buttonText,
      ),
    ),
    iconButtonTheme: IconButtonThemeData(
      style: IconButton.styleFrom(minimumSize: const Size(44, 44)),
    ),
    segmentedButtonTheme: SegmentedButtonThemeData(
      style: SegmentedButton.styleFrom(
        backgroundColor: p.surface,
        foregroundColor: p.muted,
        selectedBackgroundColor: p.fg,
        selectedForegroundColor: p.onInk,
        side: BorderSide(color: p.borderStrong),
        shape: control,
        minimumSize: const Size(0, 42),
        textStyle: style(text.labelLarge, 14, FontWeight.w600),
      ),
    ),
    chipTheme: ChipThemeData(
      backgroundColor: p.surface,
      selectedColor: p.fg,
      secondarySelectedColor: p.fg,
      disabledColor: p.inset2,
      checkmarkColor: p.onInk,
      showCheckmark: false,
      side: WidgetStateBorderSide.resolveWith(
        (s) => BorderSide(color: sel(s) ? p.fg : p.borderStrong),
      ),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(10)),
      ),
      labelStyle: style(text.labelLarge, 13.5, FontWeight.w500).copyWith(
        color: WidgetStateColor.resolveWith((s) => sel(s) ? p.onInk : p.fg),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 5),
    ),
    inputDecorationTheme: InputDecorationThemeData(
      filled: true,
      fillColor: p.surface,
      contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      hintStyle: style(text.bodyLarge, 15, FontWeight.w400, p.muted),
      border: fieldBorder,
      enabledBorder: fieldBorder,
      disabledBorder: fieldBorder.copyWith(
        borderSide: BorderSide(color: p.border),
      ),
      // 输入框聚焦是墨色的边（Voltip 的 field：focus 时边框与 1 px 的环都是正文色）
      focusedBorder: fieldBorder.copyWith(
        borderSide: BorderSide(color: p.fg, width: 1.5),
      ),
      errorBorder: fieldBorder.copyWith(
        borderSide: BorderSide(color: p.danger),
      ),
      focusedErrorBorder: fieldBorder.copyWith(
        borderSide: BorderSide(color: p.danger, width: 1.5),
      ),
    ),
    searchBarTheme: SearchBarThemeData(
      backgroundColor: WidgetStatePropertyAll(p.surface),
      surfaceTintColor: const WidgetStatePropertyAll(Colors.transparent),
      elevation: const WidgetStatePropertyAll(0),
      side: WidgetStatePropertyAll(BorderSide(color: p.border)),
      shape: const WidgetStatePropertyAll(control),
      constraints: const BoxConstraints(minHeight: 46),
      textStyle: WidgetStatePropertyAll(
        style(text.bodyLarge, 15, FontWeight.w400, p.fg),
      ),
      hintStyle: WidgetStatePropertyAll(
        style(text.bodyLarge, 15, FontWeight.w400, p.muted),
      ),
      padding: const WidgetStatePropertyAll(
        EdgeInsets.symmetric(horizontal: 14),
      ),
    ),
    listTileTheme: ListTileThemeData(
      contentPadding: const EdgeInsets.symmetric(horizontal: 16),
      iconColor: p.muted,
      textColor: p.fg,
      minLeadingWidth: 22,
      horizontalTitleGap: 14,
      titleTextStyle: style(text.bodyLarge, 15, FontWeight.w500, p.fg),
      subtitleTextStyle: style(text.bodyMedium, 13, FontWeight.w400, p.muted),
    ),
    dividerTheme: DividerThemeData(color: p.border, thickness: 1, space: 1),
    expansionTileTheme: ExpansionTileThemeData(
      shape: const Border(),
      collapsedShape: const Border(),
      iconColor: p.muted,
      collapsedIconColor: p.muted,
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: p.surface,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      titleTextStyle: style(text.titleLarge, 18, FontWeight.w600, p.fg),
      contentTextStyle: style(text.bodyMedium, 15, FontWeight.w400, p.muted),
    ),
    bottomSheetTheme: BottomSheetThemeData(
      backgroundColor: p.surface,
      modalBackgroundColor: p.surface,
      surfaceTintColor: Colors.transparent,
      showDragHandle: true,
      dragHandleColor: p.borderStrong,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
    ),
    popupMenuTheme: PopupMenuThemeData(
      color: p.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 3,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(controlRadius),
        side: BorderSide(color: p.border),
      ),
      textStyle: style(text.bodyLarge, 15, FontWeight.w400, p.fg),
    ),
    snackBarTheme: SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: p.fg,
      contentTextStyle: style(
        text.bodyMedium,
        14.5,
        FontWeight.w400,
        p.surface,
      ),
      actionTextColor: p.inverseAccent,
      shape: control,
    ),
    tooltipTheme: TooltipThemeData(
      decoration: BoxDecoration(
        color: p.fg,
        borderRadius: BorderRadius.circular(8),
      ),
      textStyle: style(text.bodySmall, 13, FontWeight.w400, p.surface),
    ),
    badgeTheme: BadgeThemeData(backgroundColor: p.danger, textColor: p.onInk),
    progressIndicatorTheme: ProgressIndicatorThemeData(
      color: p.accent,
      linearTrackColor: p.border,
      circularTrackColor: Colors.transparent,
    ),
    // 开关跟 Voltip 一样用强调色（蓝；暖纸是赭石），拇指是白的
    switchTheme: SwitchThemeData(
      thumbColor: WidgetStateProperty.resolveWith(
        (s) => sel(s) ? const Color(0xFFFFFFFF) : p.subtle,
      ),
      trackColor: WidgetStateProperty.resolveWith(
        (s) => sel(s) ? p.accent : p.inset2,
      ),
      trackOutlineColor: WidgetStateProperty.resolveWith(
        (s) => sel(s) ? Colors.transparent : p.borderStrong,
      ),
    ),
    checkboxTheme: CheckboxThemeData(
      fillColor: WidgetStateProperty.resolveWith(
        (s) => sel(s) ? p.fg : Colors.transparent,
      ),
      checkColor: WidgetStatePropertyAll(p.onInk),
      side: BorderSide(color: p.muted, width: 1.5),
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.all(Radius.circular(4)),
      ),
    ),
    radioTheme: RadioThemeData(
      fillColor: WidgetStateProperty.resolveWith(
        (s) => sel(s) ? p.fg : p.muted,
      ),
    ),
    floatingActionButtonTheme: FloatingActionButtonThemeData(
      backgroundColor: p.fg,
      foregroundColor: p.onInk,
      elevation: 2,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
    ),
    tabBarTheme: TabBarThemeData(
      labelColor: p.fg,
      unselectedLabelColor: p.muted,
      indicatorColor: p.fg,
      dividerColor: p.border,
      labelStyle: style(text.titleSmall, 14.5, FontWeight.w600),
      unselectedLabelStyle: style(text.titleSmall, 14.5, FontWeight.w500),
    ),
    textSelectionTheme: TextSelectionThemeData(
      cursorColor: p.fg,
      selectionColor: p.accent.withValues(alpha: 0.28),
      selectionHandleColor: p.accent,
    ),
  );
}
