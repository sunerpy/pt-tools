import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'src/app/app.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  // 网页版（只用于预览与自动化验收）一直生成无障碍树：读屏软件与测试脚本都靠它找到按钮和输入框
  if (kIsWeb) SemanticsBinding.instance.ensureSemantics();
  runApp(
    ProviderScope(
      // 连接的重试由 HostConnection 自己做；接口出错时页面给「重试」，不要 Riverpod 自动重试
      retry: (_, _) => null,
      child: const PtToolsApp(),
    ),
  );
}
