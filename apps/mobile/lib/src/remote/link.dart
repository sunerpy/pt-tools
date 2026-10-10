// 配对链接（v1）：pttools://pair?v=1&h=<hostId>&k=<主机 X25519 公钥>&s=<配对密钥>&r=<relay 地址，逗号分隔>&d=<直连地址>
import 'dart:typed_data';

import 'bytes.dart';

const linkVersion = 1;
const maxRelays = 4;
const _maxUrlLen = 256;

class LinkException implements Exception {
  const LinkException(this.message, {this.unsupportedVersion = false});
  final String message;

  /// 链接的版本不认识：App 要提示升级
  final bool unsupportedVersion;
  @override
  String toString() => 'LinkException: $message';
}

final _hostIdRe = RegExp(r'^[a-z2-7]{26}$');

bool validHostId(String s) => _hostIdRe.hasMatch(s);

class PairingLink {
  const PairingLink({
    required this.hostId,
    required this.hostKey,
    required this.secret,
    this.relays = const [],
    this.direct,
  });

  final String hostId;
  final Uint8List hostKey;
  final Uint8List secret;
  final List<String> relays;
  final String? direct;

  static PairingLink parse(String raw) {
    final Uri u;
    try {
      u = Uri.parse(raw.trim());
    } on FormatException catch (e) {
      throw LinkException('不是链接：${e.message}');
    }
    if (u.scheme != 'pttools' ||
        u.host != 'pair' ||
        (u.path != '' && u.path != '/') ||
        u.userInfo != '' ||
        u.hasFragment) {
      throw const LinkException('不是 pttools://pair 链接');
    }
    final Map<String, List<String>> q;
    try {
      q = u.queryParametersAll;
    } on FormatException {
      throw const LinkException('查询串不对');
    }
    for (final k in const ['v', 'h', 'k', 's', 'r', 'd']) {
      if ((q[k]?.length ?? 0) > 1) throw LinkException('参数 $k 重复');
    }
    String? one(String k) => q[k]?.first;
    final v = one('v');
    if (v != '$linkVersion') {
      if (v == null || v.isEmpty) throw const LinkException('没有版本');
      throw LinkException('链接版本 $v 不认识', unsupportedVersion: true);
    }
    final hostId = one('h') ?? '';
    if (!validHostId(hostId)) throw const LinkException('hostId 不对');
    final Uint8List hostKey;
    final Uint8List secret;
    try {
      hostKey = decodeKey(one('k') ?? '', length: 32);
    } on FormatException {
      throw const LinkException('主机公钥不对');
    }
    try {
      secret = decodeKey(one('s') ?? '', length: 32);
    } on FormatException {
      throw const LinkException('配对密钥不对');
    }
    final relays = <String>[];
    final r = one('r') ?? '';
    if (r.isNotEmpty) {
      for (final part in r.split(',')) {
        final relay = normalizeRelayUrl(part);
        if (!relays.contains(relay)) relays.add(relay);
      }
      if (relays.length > maxRelays) throw const LinkException('relay 超过 4 个');
    }
    String? direct;
    final d = one('d') ?? '';
    if (d.isNotEmpty) direct = normalizeDirectUrl(d);
    if (relays.isEmpty && direct == null) {
      throw const LinkException('既没有 relay 也没有直连地址');
    }
    return PairingLink(
      hostId: hostId,
      hostKey: hostKey,
      secret: secret,
      relays: relays,
      direct: direct,
    );
  }
}

/// relay 地址：ws:// 或 wss://，可以带路径前缀；去掉末尾的 /。
String normalizeRelayUrl(String raw) =>
    _normalizeUrl(raw, 'relay 地址', const ['ws', 'wss']);

/// 直连地址：http:// 或 https://，可以带反向代理的子路径；去掉末尾的 /。
String normalizeDirectUrl(String raw) =>
    _normalizeUrl(raw, '直连地址', const ['http', 'https']);

String _normalizeUrl(String raw, String what, List<String> schemes) {
  final s = raw.trim();
  if (s.isEmpty || s.length > _maxUrlLen) {
    throw LinkException('$what的长度要在 1 到 $_maxUrlLen 个字符之间');
  }
  if (RegExp(r'[,%?# \t\r\n\\]').hasMatch(s)) {
    throw LinkException('$what里不能有逗号、百分号、问号、井号、空白与反斜杠');
  }
  final Uri u;
  try {
    u = Uri.parse(s);
  } on FormatException {
    throw LinkException('$what不对');
  }
  final scheme = u.scheme.toLowerCase();
  if (!schemes.contains(scheme)) {
    throw LinkException('$what要以 ${schemes[0]}:// 或 ${schemes[1]}:// 开头');
  }
  if (u.userInfo.isNotEmpty ||
      u.host.isEmpty ||
      !s.toLowerCase().startsWith('$scheme://')) {
    throw LinkException('$what要写成 ${schemes[1]}://主机[:端口][/路径]');
  }
  if (u.hasPort && (u.port < 1 || u.port > 65535)) {
    throw LinkException('$what的端口不对');
  }
  // 主机与端口原样小写（Uri 会去掉默认端口，这里按原文保留）
  final rest = s.substring(scheme.length + 3);
  final slash = rest.indexOf('/');
  final authority = (slash < 0 ? rest : rest.substring(0, slash)).toLowerCase();
  var path = slash < 0 ? '' : rest.substring(slash);
  while (path.endsWith('/')) {
    path = path.substring(0, path.length - 1);
  }
  return '$scheme://$authority$path';
}
