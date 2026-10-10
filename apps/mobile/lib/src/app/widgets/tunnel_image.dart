// 经隧道加载的图片（站点图标、TMDB 海报）：图片不直接连外网，都由 pt-tools 去取。
import 'dart:async';
import 'dart:ui' as ui;

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:http/http.dart' as http;

class TunnelImageProvider extends ImageProvider<TunnelImageProvider> {
  const TunnelImageProvider(this.client, this.path);

  final http.Client client;

  /// App API 的路径，例如 /api/app/v1/favicon/hdtime
  final String path;

  @override
  Future<TunnelImageProvider> obtainKey(ImageConfiguration configuration) =>
      SynchronousFuture(this);

  @override
  ImageStreamCompleter loadImage(
    TunnelImageProvider key,
    ImageDecoderCallback decode,
  ) => OneFrameImageStreamCompleter(_load(key, decode));

  Future<ImageInfo> _load(
    TunnelImageProvider key,
    ImageDecoderCallback decode,
  ) async {
    final resp = await client.get(Uri.parse('https://pt-tools$path'));
    if (resp.statusCode != 200 || resp.bodyBytes.isEmpty) {
      throw StateError('图片 ${resp.statusCode}');
    }
    final buffer = await ui.ImmutableBuffer.fromUint8List(resp.bodyBytes);
    final codec = await decode(buffer);
    final frame = await codec.getNextFrame();
    return ImageInfo(image: frame.image);
  }

  // 同一个路径就是同一张图（重连以后缓存照样能用）
  @override
  bool operator ==(Object other) =>
      other is TunnelImageProvider && other.path == path;

  @override
  int get hashCode => path.hashCode;
}

/// TMDB 海报的路径（poster_path 形如 /abc.jpg）。
String? tmdbImagePath(String? posterPath, {String size = 'w342'}) {
  if (posterPath == null || posterPath.isEmpty) return null;
  final file = posterPath.startsWith('/')
      ? posterPath.substring(1)
      : posterPath;
  if (!RegExp(r'^[A-Za-z0-9_-]+\.(jpg|jpeg|png|webp)$').hasMatch(file)) {
    return null;
  }
  return '/api/app/v1/images/tmdb/$size/$file';
}
