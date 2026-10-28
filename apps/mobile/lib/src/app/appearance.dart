// 外观：主题的选择存在本机（与配对记录同一个安全存储，只是一个短字符串），启动时先读出来再画第一帧，
// 免得一打开先闪一下默认主题。
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'theme.dart';

/// 主题选择的读写（测试里换成内存的）。
abstract interface class AppearanceStore {
  Future<ThemeChoice> loadTheme();
  Future<void> saveTheme(ThemeChoice choice);
}

class SecureAppearanceStore implements AppearanceStore {
  SecureAppearanceStore([FlutterSecureStorage? storage])
    : _s = storage ?? const FlutterSecureStorage();

  static const _key = 'pt-tools.theme.v1';
  final FlutterSecureStorage _s;

  @override
  Future<ThemeChoice> loadTheme() async =>
      ThemeChoice.parse(await _s.read(key: _key));

  @override
  Future<void> saveTheme(ThemeChoice choice) =>
      _s.write(key: _key, value: choice.name);
}

class MemoryAppearanceStore implements AppearanceStore {
  MemoryAppearanceStore([this.choice = ThemeChoice.system]);
  ThemeChoice choice;

  @override
  Future<ThemeChoice> loadTheme() async => choice;

  @override
  Future<void> saveTheme(ThemeChoice c) async => choice = c;
}

/// 启动时读出来的选择（main 里覆盖）；读不到时跟随系统。
Future<ThemeChoice> loadInitialTheme(AppearanceStore store) async {
  try {
    return await store.loadTheme().timeout(const Duration(seconds: 2));
  } on Object {
    // 安全存储读不出来（或者太慢）时不挡着启动
    return ThemeChoice.system;
  }
}

final appearanceStoreProvider = Provider<AppearanceStore>(
  (ref) => SecureAppearanceStore(),
);

final initialThemeChoiceProvider = Provider<ThemeChoice>(
  (ref) => ThemeChoice.system,
);

final themeChoiceProvider = NotifierProvider<ThemeChoiceNotifier, ThemeChoice>(
  ThemeChoiceNotifier.new,
);

class ThemeChoiceNotifier extends Notifier<ThemeChoice> {
  @override
  ThemeChoice build() => ref.read(initialThemeChoiceProvider);

  /// 换主题：界面马上换，再存下来（存不进去也不退回，下次启动按默认）。
  Future<void> set(ThemeChoice choice) async {
    if (choice == state) return;
    state = choice;
    try {
      await ref.read(appearanceStoreProvider).saveTheme(choice);
    } on Object {
      // 只影响下次启动
    }
  }
}
