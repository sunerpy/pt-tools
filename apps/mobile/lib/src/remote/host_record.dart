// 配对过的主机：配对链接里的 hostId、主机公钥、relay 与直连地址，加上这台设备的私钥与主机记下的设备信息。
// 存在安全存储里（私钥只在手机上）。
import 'dart:convert';
import 'dart:typed_data';

import 'bytes.dart';

class HostRecord {
  const HostRecord({
    required this.hostId,
    required this.hostKey,
    required this.devicePrivate,
    required this.relays,
    this.direct,
    required this.deviceId,
    required this.deviceName,
    required this.scopes,
    required this.pairedAt,
    this.hostVersion = '',
  });

  final String hostId;
  final Uint8List hostKey;
  final Uint8List devicePrivate;
  final List<String> relays;
  final String? direct;
  final int deviceId;
  final String deviceName;
  final List<String> scopes;
  final DateTime pairedAt;
  final String hostVersion;

  /// 这台设备能不能做写操作（完全控制）。
  bool get canWrite => scopes.contains('app:write');

  HostRecord copyWith({
    List<String>? scopes,
    String? hostVersion,
    List<String>? relays,
    String? direct,
  }) => HostRecord(
    hostId: hostId,
    hostKey: hostKey,
    devicePrivate: devicePrivate,
    relays: relays ?? this.relays,
    direct: direct ?? this.direct,
    deviceId: deviceId,
    deviceName: deviceName,
    scopes: scopes ?? this.scopes,
    pairedAt: pairedAt,
    hostVersion: hostVersion ?? this.hostVersion,
  );

  Map<String, Object?> toJson() => {
    'v': 1,
    'host_id': hostId,
    'host_key': encodeKey(hostKey),
    'device_private': encodeKey(devicePrivate),
    'relays': relays,
    'direct': direct,
    'device_id': deviceId,
    'device_name': deviceName,
    'scopes': scopes,
    'paired_at': pairedAt.toUtc().toIso8601String(),
    'host_version': hostVersion,
  };

  static HostRecord fromJson(Map<String, dynamic> j) => HostRecord(
    hostId: j['host_id'] as String,
    hostKey: decodeKey(j['host_key'] as String, length: 32),
    devicePrivate: decodeKey(j['device_private'] as String, length: 32),
    relays: (j['relays'] as List).cast<String>(),
    direct: j['direct'] as String?,
    deviceId: j['device_id'] as int,
    deviceName: j['device_name'] as String,
    scopes: (j['scopes'] as List).cast<String>(),
    pairedAt: DateTime.parse(j['paired_at'] as String),
    hostVersion: (j['host_version'] as String?) ?? '',
  );

  String encode() => jsonEncode(toJson());
  static HostRecord decode(String s) =>
      fromJson(jsonDecode(s) as Map<String, dynamic>);
}
