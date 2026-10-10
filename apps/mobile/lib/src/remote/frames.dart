// 隧道帧：[type u8][req_id u32 大端][payload]，每条 Noise 传输消息的明文恰好是一个帧（docs/design/remote-access.md）。
import 'dart:convert';
import 'dart:typed_data';

/// 各层的大小（v1 冻结）。
const maxNoiseMessage = 65535;
const tagLen = 16;
const maxPlaintext = maxNoiseMessage - tagLen;
const frameHeaderLen = 5;
const maxFramePayload = maxPlaintext - frameHeaderLen;
const maxPingPayload = 64;
const maxReasonLen = 256;
const maxRequestBody = 16 << 20;

/// 隧道帧的类型。
abstract final class FrameType {
  static const reqHead = 0x01;
  static const reqBody = 0x02;
  static const reqEnd = 0x03;
  static const cancel = 0x04;
  static const respHead = 0x11;
  static const respBody = 0x12;
  static const respEnd = 0x13;
  static const ping = 0x20;
  static const pong = 0x21;
  static const goAway = 0x30;
}

/// GOAWAY 的原因。
abstract final class GoAwayReason {
  static const revoked = 'revoked';
  static const scopeChanged = 'scope_changed';
  static const disabled = 'disabled';
  static const keyRotated = 'key_rotated';
  static const paired = 'paired';
  static const pairingClosed = 'pairing_closed';
  static const shutdown = 'shutdown';
  static const protocolError = 'protocol_error';
}

class FrameException implements Exception {
  const FrameException(this.message);
  final String message;
  @override
  String toString() => 'FrameException: $message';
}

class Frame {
  const Frame(this.type, this.id, this.payload);

  final int type;
  final int id;
  final Uint8List payload;

  static void _check(int type, int id, int n, List<int> payload) {
    if (n > maxFramePayload) {
      throw FrameException('payload $n 字节，超过 $maxFramePayload');
    }
    if (id < 0 || id > 0xffffffff) throw const FrameException('请求编号不对');
    switch (type) {
      case FrameType.reqHead ||
          FrameType.reqBody ||
          FrameType.respHead ||
          FrameType.respBody:
        if (id == 0 || n == 0) {
          throw FrameException('类型 0x${type.toRadixString(16)} 要有请求编号与内容');
        }
      case FrameType.reqEnd || FrameType.respEnd:
        if (id == 0 || n != 0) {
          throw FrameException('类型 0x${type.toRadixString(16)} 要有请求编号、不带内容');
        }
      case FrameType.cancel:
        if (id == 0 || n > maxReasonLen || !_validUtf8(payload)) {
          throw const FrameException('CANCEL 要有请求编号，原因最多 256 字节 UTF-8');
        }
      case FrameType.ping || FrameType.pong:
        if (id != 0 || n > maxPingPayload) {
          throw const FrameException('PING、PONG 的编号是 0，内容最多 64 字节');
        }
      case FrameType.goAway:
        if (id != 0 || n == 0 || n > maxReasonLen) {
          throw const FrameException('GOAWAY 的编号是 0，内容 1 到 256 字节');
        }
      default:
        throw FrameException('不认识的类型 0x${type.toRadixString(16)}');
    }
  }

  static bool _validUtf8(List<int> b) {
    try {
      utf8.decode(b);
      return true;
    } on FormatException {
      return false;
    }
  }

  Uint8List encode() {
    _check(type, id, payload.length, payload);
    final out = Uint8List(frameHeaderLen + payload.length);
    out[0] = type;
    ByteData.sublistView(out).setUint32(1, id);
    out.setRange(frameHeaderLen, out.length, payload);
    return out;
  }

  static Frame parse(Uint8List b) {
    if (b.length < frameHeaderLen || b.length > maxPlaintext) {
      throw FrameException('长度 ${b.length} 字节');
    }
    final type = b[0];
    final id = ByteData.sublistView(b).getUint32(1);
    final payload = Uint8List.sublistView(b, frameHeaderLen);
    _check(type, id, payload.length, payload);
    return Frame(type, id, payload);
  }

  static Frame json(int type, int id, Object value) =>
      Frame(type, id, Uint8List.fromList(utf8.encode(jsonEncode(value))));
}

/// REQ_HEAD 的内容。
class RequestHead {
  const RequestHead(this.method, this.path, this.headers);
  final String method;
  final String path;
  final Map<String, String> headers;

  Map<String, Object> toJson() => {
    'method': method,
    'path': path,
    if (headers.isNotEmpty) 'headers': headers,
  };
}

/// RESP_HEAD 的内容。
class ResponseHead {
  const ResponseHead(this.status, this.headers);
  final int status;
  final Map<String, String> headers;

  static ResponseHead parse(List<int> payload) {
    final v = jsonDecode(utf8.decode(payload));
    if (v is! Map || v['status'] is! int) throw const FrameException('回应头格式不对');
    final h = <String, String>{};
    final raw = v['headers'];
    if (raw is Map) {
      for (final e in raw.entries) {
        if (e.key is String && e.value is String) {
          h[(e.key as String).toLowerCase()] = e.value as String;
        }
      }
    }
    return ResponseHead(v['status'] as int, h);
  }
}

/// 主机只转发这几个请求头（其余丢掉）。
const requestHeaderAllow = [
  'Accept',
  'Accept-Language',
  'Content-Type',
  'If-Modified-Since',
  'If-None-Match',
];
