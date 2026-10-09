// 配对过的主机存在安全存储里（Android Keystore / iOS Keychain；网页版是加密的 localStorage）。
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../remote/host_record.dart';

/// 主机记录的读写（测试里换成内存的）。
abstract interface class HostStore {
  Future<HostRecord?> load();
  Future<void> save(HostRecord host);
  Future<void> clear();
}

class SecureHostStore implements HostStore {
  SecureHostStore([FlutterSecureStorage? storage])
    : _s = storage ?? const FlutterSecureStorage();

  static const _key = 'pt-tools.host.v1';
  final FlutterSecureStorage _s;

  @override
  Future<HostRecord?> load() async {
    final raw = await _s.read(key: _key);
    if (raw == null || raw.isEmpty) return null;
    try {
      return HostRecord.decode(raw);
    } on Object {
      // 读不懂的记录（例如版本不对）当作没有配对
      return null;
    }
  }

  @override
  Future<void> save(HostRecord host) =>
      _s.write(key: _key, value: host.encode());

  @override
  Future<void> clear() => _s.delete(key: _key);
}

class MemoryHostStore implements HostStore {
  MemoryHostStore([this.host]);
  HostRecord? host;

  @override
  Future<HostRecord?> load() async => host;

  @override
  Future<void> save(HostRecord h) async => host = h;

  @override
  Future<void> clear() async => host = null;
}
