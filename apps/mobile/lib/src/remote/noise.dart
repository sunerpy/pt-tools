// Noise_IK_25519_ChaChaPoly_BLAKE2s（Noise 规范第 34 版），App 是发起方、pt-tools 是响应方。
// 只实现 IK 这一种模式；两个角色都实现，测试用 Noise 的公开向量（cacophony、snow）逐字节核对。
import 'dart:convert';
import 'dart:typed_data';

import 'package:cryptography/cryptography.dart';
import 'package:cryptography/dart.dart';

const protocolName = 'Noise_IK_25519_ChaChaPoly_BLAKE2s';
const dhLen = 32;
const hashLen = 32;
const _tagLen = 16;

const _blake2s = DartBlake2s();
const _aead = DartChacha20.poly1305Aead();
const _x25519 = DartX25519();

/// Noise 握手或解密失败。
class NoiseException implements Exception {
  const NoiseException(this.message);
  final String message;
  @override
  String toString() => 'NoiseException: $message';
}

Uint8List _hash(List<int> data) =>
    Uint8List.fromList(_blake2s.hashSync(data).bytes);

/// HMAC-BLAKE2s，按 RFC 2104（块长 64 字节）。cryptography 包的 Hmac(Blake2s()) 与标准 HMAC 的结果不同，不能用。
Uint8List _hmacHash(List<int> key, List<int> data) {
  const block = 64;
  final k = key.length > block ? _hash(key) : key;
  final ipad = Uint8List(block);
  final opad = Uint8List(block);
  for (var i = 0; i < block; i++) {
    final b = i < k.length ? k[i] : 0;
    ipad[i] = b ^ 0x36;
    opad[i] = b ^ 0x5c;
  }
  final inner = _hash([...ipad, ...data]);
  return _hash([...opad, ...inner]);
}

/// HKDF(ck, ikm) 的两个输出（规范 4.3）。
(Uint8List, Uint8List) _hkdf2(List<int> ck, List<int> ikm) {
  final temp = _hmacHash(ck, ikm);
  final o1 = _hmacHash(temp, const [1]);
  final o2 = _hmacHash(temp, [...o1, 2]);
  return (o1, o2);
}

/// X25519 密钥对：私钥 32 字节（使用时按 RFC 7748 夹紧），公钥 32 字节。
class DhKey {
  DhKey._(this.privateKey, this.publicKey);

  final Uint8List privateKey;
  final Uint8List publicKey;

  /// 由私钥算出公钥。
  static Future<DhKey> fromPrivate(List<int> priv) async {
    if (priv.length != dhLen) throw const NoiseException('X25519 私钥长度不对');
    final kp = await _x25519.newKeyPairFromSeed(priv);
    final pub = await kp.extractPublicKey();
    return DhKey._(Uint8List.fromList(priv), Uint8List.fromList(pub.bytes));
  }

  /// 新的随机密钥对。
  static Future<DhKey> generate() async {
    final kp = await _x25519.newKeyPair();
    final data = await kp.extract();
    return fromPrivate(data.bytes);
  }

  Uint8List dh(List<int> remotePublic) {
    if (remotePublic.length != dhLen) {
      throw const NoiseException('X25519 公钥长度不对');
    }
    final secret = _x25519.sharedSecretSync(
      keyPairData: SimpleKeyPairData(
        privateKey,
        publicKey: SimplePublicKey(publicKey, type: KeyPairType.x25519),
        type: KeyPairType.x25519,
      ),
      remotePublicKey: SimplePublicKey(remotePublic, type: KeyPairType.x25519),
    );
    return Uint8List.fromList((secret as SecretKeyData).bytes);
  }
}

/// CipherState：ChaChaPoly，nonce 是 4 个 0 字节加 8 字节小端的计数。加密与解密都是同步的，调用顺序就是 nonce 顺序。
class CipherState {
  CipherState([List<int>? key])
    : _k = key == null ? null : Uint8List.fromList(key);

  final Uint8List? _k;
  int _n = 0;

  bool get hasKey => _k != null;

  /// 下一个 nonce（测试用）。
  int get nonce => _n;

  static Uint8List _nonceBytes(int n) {
    // 不用 setUint64：网页上没有 64 位整数。计数到不了 2^53。
    final b = Uint8List(12);
    var lo = n % 0x100000000;
    var hi = n ~/ 0x100000000;
    for (var i = 0; i < 4; i++) {
      b[4 + i] = lo & 0xff;
      lo >>= 8;
      b[8 + i] = hi & 0xff;
      hi >>= 8;
    }
    return b;
  }

  Uint8List encryptWithAd(List<int> ad, List<int> plaintext) {
    final k = _k;
    if (k == null) return Uint8List.fromList(plaintext);
    final box = _aead.encryptSync(
      plaintext,
      secretKey: SecretKeyData(k),
      nonce: _nonceBytes(_n),
      aad: ad,
    );
    _n++;
    return Uint8List.fromList([...box.cipherText, ...box.mac.bytes]);
  }

  Uint8List decryptWithAd(List<int> ad, List<int> ciphertext) {
    final k = _k;
    if (k == null) return Uint8List.fromList(ciphertext);
    if (ciphertext.length < _tagLen) throw const NoiseException('密文太短');
    final ct = ciphertext.sublist(0, ciphertext.length - _tagLen);
    final mac = Mac(ciphertext.sublist(ciphertext.length - _tagLen));
    try {
      final pt = _aead.decryptSync(
        SecretBox(ct, nonce: _nonceBytes(_n), mac: mac),
        secretKey: SecretKeyData(k),
        aad: ad,
      );
      _n++;
      return Uint8List.fromList(pt);
    } on SecretBoxAuthenticationError {
      throw const NoiseException('解密失败');
    }
  }
}

class _SymmetricState {
  _SymmetricState() {
    final name = ascii.encode(protocolName);
    _h = name.length <= hashLen
        ? Uint8List.fromList([
            ...name,
            ...List.filled(hashLen - name.length, 0),
          ])
        : _hash(name);
    _ck = Uint8List.fromList(_h);
  }

  late Uint8List _ck;
  late Uint8List _h;
  CipherState _cs = CipherState();

  void mixKey(List<int> ikm) {
    final (ck, k) = _hkdf2(_ck, ikm);
    _ck = ck;
    _cs = CipherState(k);
  }

  void mixHash(List<int> data) {
    _h = _hash([..._h, ...data]);
  }

  Uint8List encryptAndHash(List<int> plaintext) {
    final ct = _cs.encryptWithAd(_h, plaintext);
    mixHash(ct);
    return ct;
  }

  Uint8List decryptAndHash(List<int> ciphertext) {
    final pt = _cs.decryptWithAd(_h, ciphertext);
    mixHash(ciphertext);
    return pt;
  }

  (CipherState, CipherState) split() {
    final (k1, k2) = _hkdf2(_ck, const []);
    return (CipherState(k1), CipherState(k2));
  }
}

/// IK 的握手状态。发起方：writeMessage → readMessage；响应方：readMessage → writeMessage。
/// 两条消息都处理完以后 [complete] 为真，[send]、[recv] 是传输用的 CipherState。
class NoiseIK {
  NoiseIK._(this.initiator, this._s, this._rs, this._e);

  final bool initiator;
  final DhKey _s;
  Uint8List? _rs;
  DhKey? _e;
  Uint8List? _re;
  final _ss = _SymmetricState();
  int _step = 0;
  CipherState? _send;
  CipherState? _recv;

  /// 发起方：s 是自己的静态密钥，rs 是对方的静态公钥（扫码拿到的主机公钥）。e 只在测试里给（固定的临时密钥）。
  static NoiseIK initiatorOf({
    required DhKey s,
    required List<int> rs,
    required List<int> prologue,
    DhKey? e,
  }) {
    if (rs.length != dhLen) throw const NoiseException('主机公钥长度不对');
    final hs = NoiseIK._(true, s, Uint8List.fromList(rs), e);
    hs._ss.mixHash(prologue);
    hs._ss.mixHash(rs);
    return hs;
  }

  /// 响应方（测试用；主机是 Go 实现）。
  static NoiseIK responderOf({
    required DhKey s,
    required List<int> prologue,
    DhKey? e,
  }) {
    final hs = NoiseIK._(false, s, null, e);
    hs._ss.mixHash(prologue);
    hs._ss.mixHash(s.publicKey);
    return hs;
  }

  bool get complete => _step == 2;

  /// 对方的静态公钥（响应方读完第一条消息以后才有）。
  Uint8List? get remoteStatic => _rs;

  /// 握手哈希（两条消息之后）。
  Uint8List get handshakeHash => Uint8List.fromList(_ss._h);

  CipherState get send => _send ?? (throw const NoiseException('握手还没完成'));
  CipherState get recv => _recv ?? (throw const NoiseException('握手还没完成'));

  Future<Uint8List> writeMessage(List<int> payload) async {
    final e = _e ??= await DhKey.generate();
    final out = BytesBuilder(copy: false);
    if (initiator && _step == 0) {
      // -> e, es, s, ss
      out.add(e.publicKey);
      _ss.mixHash(e.publicKey);
      _ss.mixKey(e.dh(_rs!));
      out.add(_ss.encryptAndHash(_s.publicKey));
      _ss.mixKey(_s.dh(_rs!));
      out.add(_ss.encryptAndHash(payload));
      _step = 1;
    } else if (!initiator && _step == 1) {
      // <- e, ee, se
      out.add(e.publicKey);
      _ss.mixHash(e.publicKey);
      _ss.mixKey(e.dh(_re!));
      _ss.mixKey(e.dh(_rs!));
      out.add(_ss.encryptAndHash(payload));
      _finish();
    } else {
      throw const NoiseException('现在不该写握手消息');
    }
    return out.takeBytes();
  }

  Future<Uint8List> readMessage(List<int> message) async {
    if (!initiator && _step == 0) {
      // -> e, es, s, ss
      if (message.length < dhLen + dhLen + _tagLen + _tagLen) {
        throw const NoiseException('第一条握手消息太短');
      }
      _re = Uint8List.fromList(message.sublist(0, dhLen));
      _ss.mixHash(_re!);
      _ss.mixKey(_s.dh(_re!));
      _rs = _ss.decryptAndHash(message.sublist(dhLen, dhLen + dhLen + _tagLen));
      _ss.mixKey(_s.dh(_rs!));
      final payload = _ss.decryptAndHash(
        message.sublist(dhLen + dhLen + _tagLen),
      );
      _step = 1;
      return payload;
    }
    if (initiator && _step == 1) {
      // <- e, ee, se
      if (message.length < dhLen + _tagLen) {
        throw const NoiseException('第二条握手消息太短');
      }
      _re = Uint8List.fromList(message.sublist(0, dhLen));
      _ss.mixHash(_re!);
      final e = _e!;
      _ss.mixKey(e.dh(_re!));
      _ss.mixKey(_s.dh(_re!));
      final payload = _ss.decryptAndHash(message.sublist(dhLen));
      _finish();
      return payload;
    }
    throw const NoiseException('现在不该读握手消息');
  }

  void _finish() {
    final (c1, c2) = _ss.split();
    // c1 加密发起方发出的消息，c2 加密响应方发出的消息
    _send = initiator ? c1 : c2;
    _recv = initiator ? c2 : c1;
    _step = 2;
  }
}
