import 'dart:convert';
import 'dart:typed_data';

/// 十六进制（测试向量用）。
String toHex(List<int> b) =>
    b.map((x) => x.toRadixString(16).padLeft(2, '0')).join();

Uint8List fromHex(String s) {
  if (s.length.isOdd) throw const FormatException('十六进制长度不对');
  final out = Uint8List(s.length ~/ 2);
  for (var i = 0; i < out.length; i++) {
    out[i] = int.parse(s.substring(i * 2, i * 2 + 2), radix: 16);
  }
  return out;
}

/// base64url，无填充（协议里文本形式的二进制值都用它）。
String encodeKey(List<int> b) => base64Url.encode(b).replaceAll('=', '');

/// 解 base64url（无填充），只接受规范写法：重新编码要和原文一样（最后一个字符的填充位为 0）。
Uint8List decodeKey(String s, {int? length}) {
  if (s.isEmpty ||
      s.contains('=') ||
      !RegExp(r'^[A-Za-z0-9_-]+$').hasMatch(s)) {
    throw const FormatException('不是 base64url');
  }
  final padded = s + '=' * ((4 - s.length % 4) % 4);
  final Uint8List out;
  try {
    out = base64Url.decode(padded);
  } on FormatException {
    throw const FormatException('不是 base64url');
  }
  if (encodeKey(out) != s) throw const FormatException('不是规范的 base64url');
  if (length != null && out.length != length) {
    throw FormatException('长度不是 $length 字节');
  }
  return out;
}

/// 两段字节是否相同。
bool bytesEqual(List<int> a, List<int> b) {
  if (a.length != b.length) return false;
  var d = 0;
  for (var i = 0; i < a.length; i++) {
    d |= a[i] ^ b[i];
  }
  return d == 0;
}
