import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'src/app/app.dart';
import 'src/app/appearance.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // 网页版（只用于预览与自动化验收）一直生成无障碍树：读屏软件与测试脚本都靠它找到按钮和输入框
  if (kIsWeb) SemanticsBinding.instance.ensureSemantics();
  // 先读出主题再画第一帧，免得一打开先闪一下默认主题
  final theme = await loadInitialTheme(SecureAppearanceStore());
  runApp(
    ProviderScope(
      overrides: [initialThemeChoiceProvider.overrideWithValue(theme)],
      // 连接的重试由 HostConnection 自己做；接口出错时页面给「重试」，不要 Riverpod 自动重试
      retry: (_, _) => null,
      child: const PtToolsApp(),
    ),
  );
}
