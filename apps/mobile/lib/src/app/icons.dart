// App 的图标：Voltip 的描边图标（24 的画布、1.75 的线宽、圆头圆角，颜色跟着 IconTheme），
// 不用 Material 那套粗细、实心与描边混着的图标。路径照 Voltip 的 packages/ui/src/components/Icon.tsx
// （同一作者），pt-tools 才有的几个（种子、RSS、下载器、邮件……）按同样的画法补上。
import 'package:flutter/widgets.dart';

/// 一个图标的路径（SVG 的 path data，画布 24 × 24）。
enum PtGlyph {
  home('M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z'),
  dashboard('M4 4h7v9H4zM13 4h7v5h-7zM13 11h7v9h-7zM4 15h7v5H4z'),
  torrents('M7 20V4M3 8l4-4 4 4M17 4v16M13 16l4 4 4-4'),
  search('M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM20 20l-4-4'),
  film('M4 4h16v16H4zM8 4v16M16 4v16M4 8h4M4 12h16M4 16h4M16 8h4M16 16h4'),
  more(
    'M5 11a1 1 0 1 0 0 2 1 1 0 0 0 0-2zM12 11a1 1 0 1 0 0 2 1 1 0 0 0 0-2zM19 11a1 1 0 1 0 0 2 1 1 0 0 0 0-2z',
  ),
  moreVertical(
    'M12 4a1 1 0 1 0 0 2 1 1 0 0 0 0-2zM12 11a1 1 0 1 0 0 2 1 1 0 0 0 0-2zM12 18a1 1 0 1 0 0 2 1 1 0 0 0 0-2z',
  ),
  settings(
    'M12 8.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7zM19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z',
  ),
  plus('M12 5v14M5 12h14'),
  minus('M5 12h14'),
  x('M6 6l12 12M18 6 6 18'),
  check('M5 12l5 5L20 7'),
  chevronRight('M9 6l6 6-6 6'),
  chevronLeft('M15 6l-6 6 6 6'),
  chevronDown('M6 9l6 6 6-6'),
  arrowRight('M5 12h14M13 6l6 6-6 6'),
  arrowUp('M12 19V5M6 11l6-6 6 6'),
  arrowDown('M12 5v14M6 13l6 6 6-6'),
  filter('M4 6h16M7 12h10M10 18h4'),
  refresh('M20 12a8 8 0 1 1-2.3-5.7M20 4v5h-5'),
  cycle(
    'M20 11a8 8 0 0 0-14.3-4.9L4 8M4 4v4h4M4 13a8 8 0 0 0 14.3 4.9L20 16M20 20v-4h-4',
  ),
  rss(
    'M5 11a8 8 0 0 1 8 8M5 5a14 14 0 0 1 14 14M6 18a1 1 0 1 0-2 0 1 1 0 0 0 2 0z',
  ),
  server('M4 4h16v6H4zM4 14h16v6H4zM7.5 7h1M7.5 17h1'),
  serverOff('M4 4h16v6H4zM4 14h16v6H4zM7.5 7h1M7.5 17h1M3 3l18 18'),
  globe(
    'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18',
  ),
  sparkles(
    'M12 3l1.8 4.7L18 9.5l-4.2 1.8L12 16l-1.8-4.7L6 9.5l4.2-1.8zM19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8zM5 15l.6 1.4L7 17l-1.4.6L5 19l-.6-1.4L3 17l1.4-.6z',
  ),
  chatPlus('M4 5h16v11H9l-5 4zM12 7.5v5M9.5 10h5'),
  terminal('M4 6l6 6-6 6M12 18h8'),
  stop('M6 6h12v12H6z'),
  send('M21 3 3 10l7 3.5L14 21zM21 3 10 13.5'),
  share('M12 15V3M8 7l4-4 4 4M5 11v9h14v-9'),
  upload('M12 16V4M6 10l6-6 6 6M4 20h16'),
  download('M12 3v12M7 10l5 5 5-5M4 21h16'),
  cloudUp(
    'M7 18a4 4 0 0 1-.5-8A6 6 0 0 1 18 9a4.5 4.5 0 0 1-.5 9zM12 16v-5M9.5 13.5 12 11l2.5 2.5',
  ),
  cloudOff('M7 18a4 4 0 0 1-.5-8A6 6 0 0 1 18 9a4.5 4.5 0 0 1-.5 9zM3 3l18 18'),
  link(
    'M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1',
  ),
  linkOff(
    'M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1M3 3l18 18',
  ),
  broadcast(
    'M13 12a1 1 0 1 0-2 0 1 1 0 0 0 2 0zM8.5 8.5a5 5 0 0 0 0 7M15.5 8.5a5 5 0 0 1 0 7M5.6 5.6a9 9 0 0 0 0 12.8M18.4 5.6a9 9 0 0 1 0 12.8',
  ),
  monitor('M3 4h18v12H3zM8 20h8M12 16v4'),
  phone(
    'M8 2h8a2 2 0 0 1 2 2v16a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2zM11 18h2',
  ),
  shield('M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z'),
  shieldCheck('M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6zM9 12l2 2 4-4'),
  alert('M12 3 2 21h20zM12 10v5M12 18h.01'),
  info('M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 11v5M12 8h.01'),
  errorCircle('M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7.5v5.5M12 16.5h.01'),
  checkCircle('M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM8 12l3 3 5-6'),
  minusCircle('M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM8 12h8'),
  star(
    'M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3 6.4 20.2l1.1-6.2L3 9.6l6.2-.9z',
  ),
  scale(
    'M12 4v16M8 20h8M5 7h14M5 7l-3 7M5 7l3 7M2 14a3 3 0 0 0 6 0M19 7l-3 7M19 7l3 7M16 14a3 3 0 0 0 6 0',
  ),
  mail('M3 5h18v14H3zM3 6l9 7 9-7'),
  inbox('M4 13h4l1.5 3h5l1.5-3h4M4 13l2.5-8h11L20 13v7H4z'),
  bookmark('M6 3h12v18l-6-4-6 4z'),
  library('M4 6h16v12H4zM10 9.5l4.5 2.5-4.5 2.5z'),
  sliders('M4 7h10M18 7h2M4 17h4M12 17h8M14 5v4M8 15v4'),
  qr(
    'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h3v3h-3zM18 18h3v3h-3zM18 14h3M14 18v3',
  ),
  scan('M4 8V4h4M16 4h4v4M20 16v4h-4M8 20H4v-4M7 12h10'),
  paste('M9 3h6v3H9zM9 4.5H6V21h12V4.5h-3M12 10v7M9 14l3 3 3-3'),
  trash('M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v6M14 11v6'),
  edit('M4 20h4l10-10-4-4L4 16zM13 7l4 4'),
  play('M7 4l12 8-12 8z'),
  pause('M7 4h4v16H7zM13 4h4v16h-4z'),
  clock('M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 2'),
  history('M3 12a9 9 0 1 0 3-6.7M3 4v5h5M12 7v5l3 2'),
  sun(
    'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4',
  ),
  moon('M20 14.5A8.5 8.5 0 1 1 9.5 4a7 7 0 0 0 10.5 10.5z'),
  paper('M6 3h9l3 3v15H6zM15 3v3h3M9 11h6M9 15h6'),
  gauge('M4 14a8 8 0 1 1 16 0M12 14l4-4M12 14h.01'),
  palette(
    'M12 3a9 9 0 0 0 0 18c1.2 0 1.7-1 1.2-2-.6-1.2.2-2.5 1.5-2.5H17a4 4 0 0 0 4-4c0-5.2-4-9.5-9-9.5zM7.5 12h.01M10 7.5h.01M15 7.5h.01M17.5 11h.01',
  ),
  user('M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0'),
  image('M4 5h16v14H4zM4 16l5-5 4 4 2.5-2.5L20 16M15 9h.01'),
  external('M14 4h6v6M20 4l-9 9M18 14v6H4V6h6');

  const PtGlyph(this.data);

  /// SVG path data on a 24 × 24 canvas.
  final String data;
}

/// 一个描边图标。大小与颜色默认取 [IconTheme]（与 [Icon] 一样），所以放在 [IconButton]、
/// [ListTile]、[NavigationBar] 里会跟着它们的选中、禁用颜色走。没有 [semanticLabel] 时对读屏隐藏。
class PtIcon extends StatelessWidget {
  const PtIcon(
    this.glyph, {
    super.key,
    this.size,
    this.color,
    this.weight = 1.75,
    this.semanticLabel,
  });

  final PtGlyph glyph;
  final double? size;
  final Color? color;

  /// 线宽，按 24 的画布算（Voltip 是 1.75）。
  final double weight;
  final String? semanticLabel;

  @override
  Widget build(BuildContext context) {
    final theme = IconTheme.of(context);
    final s = size ?? theme.size ?? 24;
    var c = color ?? theme.color ?? const Color(0xFF000000);
    final opacity = theme.opacity;
    if (color == null && opacity != null && opacity < 1) {
      c = c.withValues(alpha: c.a * opacity);
    }
    final painted = SizedBox(
      width: s,
      height: s,
      child: CustomPaint(painter: _GlyphPainter(glyphPath(glyph), c, weight)),
    );
    if (semanticLabel == null) return ExcludeSemantics(child: painted);
    return Semantics(label: semanticLabel, child: painted);
  }
}

class _GlyphPainter extends CustomPainter {
  _GlyphPainter(this.path, this.color, this.weight);
  final Path path;
  final Color color;
  final double weight;

  @override
  void paint(Canvas canvas, Size size) {
    final k = size.shortestSide / 24;
    canvas.save();
    canvas.scale(k);
    canvas.drawPath(
      path,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = weight
        ..strokeCap = StrokeCap.round
        ..strokeJoin = StrokeJoin.round
        ..isAntiAlias = true
        ..color = color,
    );
    canvas.restore();
  }

  @override
  bool shouldRepaint(_GlyphPainter old) =>
      old.path != path || old.color != color || old.weight != weight;
}

final _paths = <PtGlyph, Path>{};

/// [glyph] 解析好的路径（解析一次以后缓存）。
Path glyphPath(PtGlyph glyph) =>
    _paths.putIfAbsent(glyph, () => parseSvgPath(glyph.data));

/// 把 SVG 的 path data 转成 [Path]：M L H V C S Q T A Z（大小写都认）与省略命令的连续坐标。
/// 读不懂的字符抛 [FormatException]（图标是写死的，测试里会读一遍所有图标）。
Path parseSvgPath(String d) {
  final path = Path();
  final r = _PathReader(d);
  var cmd = '';
  var x = 0.0, y = 0.0; // 当前点
  var sx = 0.0, sy = 0.0; // 子路径起点
  double? cx, cy; // 上一段曲线的第二个控制点（S、T 用来求反射）
  var prev = '';
  while (true) {
    r.skipSeparators();
    if (r.done) break;
    if (r.atCommand) {
      cmd = r.command();
    } else if (cmd.isEmpty) {
      throw FormatException('path data must start with a command', d);
    } else if (cmd == 'M') {
      cmd = 'L'; // M 后面省略命令的坐标是 L
    } else if (cmd == 'm') {
      cmd = 'l';
    }
    final rel = cmd == cmd.toLowerCase();
    final ox = rel ? x : 0.0, oy = rel ? y : 0.0;
    switch (cmd.toUpperCase()) {
      case 'M':
        x = ox + r.number();
        y = oy + r.number();
        sx = x;
        sy = y;
        path.moveTo(x, y);
        cx = cy = null;
      case 'L':
        x = ox + r.number();
        y = oy + r.number();
        path.lineTo(x, y);
        cx = cy = null;
      case 'H':
        x = ox + r.number();
        path.lineTo(x, y);
        cx = cy = null;
      case 'V':
        y = (rel ? y : 0.0) + r.number();
        path.lineTo(x, y);
        cx = cy = null;
      case 'C':
        final x1 = ox + r.number(), y1 = oy + r.number();
        final x2 = ox + r.number(), y2 = oy + r.number();
        x = ox + r.number();
        y = oy + r.number();
        path.cubicTo(x1, y1, x2, y2, x, y);
        cx = x2;
        cy = y2;
      case 'S':
        final smooth = 'CcSs'.contains(prev) && cx != null;
        final x1 = smooth ? 2 * x - cx : x;
        final y1 = smooth ? 2 * y - cy! : y;
        final x2 = ox + r.number(), y2 = oy + r.number();
        x = ox + r.number();
        y = oy + r.number();
        path.cubicTo(x1, y1, x2, y2, x, y);
        cx = x2;
        cy = y2;
      case 'Q':
        final x1 = ox + r.number(), y1 = oy + r.number();
        x = ox + r.number();
        y = oy + r.number();
        path.quadraticBezierTo(x1, y1, x, y);
        cx = x1;
        cy = y1;
      case 'T':
        final smooth = 'QqTt'.contains(prev) && cx != null;
        final x1 = smooth ? 2 * x - cx : x;
        final y1 = smooth ? 2 * y - cy! : y;
        x = ox + r.number();
        y = oy + r.number();
        path.quadraticBezierTo(x1, y1, x, y);
        cx = x1;
        cy = y1;
      case 'A':
        final rx = r.number().abs(), ry = r.number().abs();
        final rotation = r.number();
        final large = r.flag();
        final sweep = r.flag();
        x = ox + r.number();
        y = oy + r.number();
        // SVG 的 sweep-flag 1 是屏幕上顺时针（y 向下），与 Flutter 的 clockwise 同向
        path.arcToPoint(
          Offset(x, y),
          radius: Radius.elliptical(rx, ry),
          rotation: rotation,
          largeArc: large,
          clockwise: sweep,
        );
        cx = cy = null;
      case 'Z':
        path.close();
        x = sx;
        y = sy;
        cx = cy = null;
      default:
        throw FormatException('unsupported path command $cmd', d);
    }
    prev = cmd;
  }
  return path;
}

class _PathReader {
  _PathReader(this.s);
  final String s;
  int i = 0;

  bool get done => i >= s.length;

  bool get atCommand => !done && 'MmLlHhVvCcSsQqTtAaZz'.contains(s[i]);

  String command() => s[i++];

  void skipSeparators() {
    while (!done &&
        (s[i] == ' ' || s[i] == ',' || s[i] == '\n' || s[i] == '\t')) {
      i++;
    }
  }

  bool flag() {
    skipSeparators();
    if (done || (s[i] != '0' && s[i] != '1')) {
      throw FormatException('expected an arc flag', s, i);
    }
    return s[i++] == '1';
  }

  double number() {
    skipSeparators();
    final start = i;
    if (!done && (s[i] == '-' || s[i] == '+')) i++;
    var dot = false;
    while (!done) {
      final c = s[i];
      if (c == '.') {
        if (dot) break; // 「1.5.5」是 1.5 与 .5
        dot = true;
      } else if (c.codeUnitAt(0) < 48 || c.codeUnitAt(0) > 57) {
        break;
      }
      i++;
    }
    if (!done && (s[i] == 'e' || s[i] == 'E')) {
      i++;
      if (!done && (s[i] == '-' || s[i] == '+')) i++;
      while (!done && s[i].codeUnitAt(0) >= 48 && s[i].codeUnitAt(0) <= 57) {
        i++;
      }
    }
    final v = double.tryParse(s.substring(start, i));
    if (v == null) throw FormatException('expected a number', s, start);
    return v;
  }
}
