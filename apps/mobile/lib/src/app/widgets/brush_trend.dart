// 刷流收益的柱状图：一根柱子是一段（一天、一周、一个月、一个季度或一年）的上传量。
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';

import '../theme.dart';

/// 图上的一段。
class BrushTrendPoint {
  const BrushTrendPoint({
    required this.short,
    required this.long,
    required this.value,
    required this.valueText,
    required this.detail,
  });

  /// 柱子下面的短标签
  final String short;

  /// 读数里的完整写法
  final String long;
  final int value;

  /// 读数里的主数字（已格式化）
  final String valueText;

  /// 读数里的次要信息
  final String detail;
}

/// 柱子从 0 起算、单一色相；柱子上面两行读数写选中那一段，默认是当前这一段（最后一根）。
/// 点一下或左右拖动换成手指下的那一段。读数固定两行、不折行：高度一变，图就跟着挪。
/// 读屏按柱子逐个读出「哪一段、上传多少」。
class BrushTrendChart extends StatefulWidget {
  const BrushTrendChart({
    super.key,
    required this.points,
    required this.color,
    this.height = 96,
  });

  final List<BrushTrendPoint> points;
  final Color color;
  final double height;

  @override
  State<BrushTrendChart> createState() => _BrushTrendChartState();
}

class _BrushTrendChartState extends State<BrushTrendChart> {
  /// 选中的那一段；null 是当前这一段（最后一根）
  int? _sel;

  @override
  void didUpdateWidget(BrushTrendChart old) {
    super.didUpdateWidget(old);
    // 换了周期（段数变了）回到当前这一段
    if (old.points.length != widget.points.length) _sel = null;
  }

  int get _selected {
    final n = widget.points.length;
    return (_sel ?? n - 1).clamp(0, math.max(0, n - 1));
  }

  void _pick(double dx, double width) {
    final n = widget.points.length;
    if (n == 0 || width <= 0) return;
    final i = (dx / width * n).floor().clamp(0, n - 1);
    if (i != _selected) setState(() => _sel = i);
  }

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context);
    final pt = context.pt;
    final points = widget.points;
    if (points.isEmpty) return const SizedBox.shrink();
    final cur = points[_selected];
    final muted = t.colorScheme.onSurfaceVariant;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text.rich(
          TextSpan(
            children: [
              TextSpan(
                text: cur.valueText,
                style: t.textTheme.titleMedium?.copyWith(
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                  fontFeatures: const [FontFeature.tabularFigures()],
                ),
              ),
              const TextSpan(text: '  '),
              TextSpan(
                text: cur.long,
                style: t.textTheme.bodySmall?.copyWith(
                  fontSize: 13,
                  color: muted,
                ),
              ),
            ],
          ),
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          key: const ValueKey('brush-trend-readout'),
        ),
        const SizedBox(height: 2),
        Text(
          cur.detail,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: t.textTheme.bodySmall?.copyWith(fontSize: 12, color: muted),
          key: const ValueKey('brush-trend-detail'),
        ),
        const SizedBox(height: 10),
        LayoutBuilder(
          builder: (context, c) => GestureDetector(
            key: const ValueKey('brush-trend-plot'),
            behavior: HitTestBehavior.opaque,
            onTapDown: (d) => _pick(d.localPosition.dx, c.maxWidth),
            onHorizontalDragUpdate: (d) =>
                _pick(d.localPosition.dx, c.maxWidth),
            child: CustomPaint(
              size: Size(c.maxWidth, widget.height + _axisHeight),
              painter: _TrendPainter(
                points: points,
                selected: _selected,
                color: widget.color,
                baseline: pt.hairline,
                label: muted,
                labelOn: t.colorScheme.onSurface,
                plotHeight: widget.height,
                textScaler: MediaQuery.textScalerOf(context),
                fontFamily: t.textTheme.bodySmall?.fontFamily,
              ),
            ),
          ),
        ),
      ],
    );
  }
}

const _axisHeight = 22.0;

class _TrendPainter extends CustomPainter {
  _TrendPainter({
    required this.points,
    required this.selected,
    required this.color,
    required this.baseline,
    required this.label,
    required this.labelOn,
    required this.plotHeight,
    required this.textScaler,
    this.fontFamily,
  });

  final List<BrushTrendPoint> points;
  final int selected;
  final Color color;
  final Color baseline;
  final Color label;
  final Color labelOn;
  final double plotHeight;
  final TextScaler textScaler;
  final String? fontFamily;

  static const _gap = 2.0;

  double _colWidth(Size size) =>
      (size.width - _gap * (points.length - 1)) / points.length;

  /// 标签放不下就隔几根写一个：最多写 6 个（手机比网页窄），最后一根（当前这一段）总是写，
  /// 离它太近的不写，免得挤在一起。与网页端的 BrushTrendChart 同一个算法。
  bool _showLabel(int i) {
    final n = points.length;
    final every = math.max(1, (n / 6).ceil());
    if (i == n - 1) return true;
    if (n - 1 - i < every) return false;
    return i % every == 0;
  }

  @override
  void paint(Canvas canvas, Size size) {
    final n = points.length;
    final colW = _colWidth(size);
    final barW = math.min(colW, 28.0);
    final maxV = points.fold<int>(0, (m, p) => math.max(m, p.value));
    for (var i = 0; i < n; i++) {
      final v = points[i].value;
      // 0 只画 2px 的底（「这一段没有收益」），正值按占最大值的比例、最矮 3px
      final h = v <= 0 || maxV <= 0
          ? 2.0
          : math.max(3.0, v / maxV * plotHeight);
      final left = i * (colW + _gap) + (colW - barW) / 2;
      final rect = Rect.fromLTWH(left, plotHeight - h, barW, h);
      final alpha = i == selected ? 1.0 : 0.5;
      final paint = Paint()
        ..shader = LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            color.withValues(alpha: alpha),
            color.withValues(alpha: alpha * 0.45),
          ],
        ).createShader(rect);
      canvas.drawRRect(
        RRect.fromRectAndCorners(
          rect,
          topLeft: const Radius.circular(3),
          topRight: const Radius.circular(3),
        ),
        paint,
      );
    }
    canvas.drawLine(
      Offset(0, plotHeight + 0.5),
      Offset(size.width, plotHeight + 0.5),
      Paint()
        ..color = baseline
        ..strokeWidth = 1,
    );
    for (var i = 0; i < n; i++) {
      if (!_showLabel(i)) continue;
      final on = i == selected;
      final tp = TextPainter(
        text: TextSpan(
          text: points[i].short,
          style: TextStyle(
            fontSize: 11,
            fontFamily: fontFamily,
            color: on ? labelOn : label,
            fontWeight: on ? FontWeight.w600 : FontWeight.w400,
          ),
        ),
        textDirection: TextDirection.ltr,
        textScaler: textScaler,
        maxLines: 1,
      )..layout();
      // 居中在柱子下面；第一个与最后一个不伸出图的左右边
      final center = i * (colW + _gap) + colW / 2;
      final x = (center - tp.width / 2)
          .clamp(0.0, math.max(0.0, size.width - tp.width))
          .toDouble();
      tp.paint(canvas, Offset(x, plotHeight + 6));
    }
  }

  @override
  SemanticsBuilderCallback get semanticsBuilder => (size) {
    final colW = _colWidth(size);
    return [
      for (var i = 0; i < points.length; i++)
        CustomPainterSemantics(
          rect: Rect.fromLTWH(i * (colW + _gap), 0, colW, plotHeight),
          properties: SemanticsProperties(
            label: '${points[i].long} ${points[i].valueText}',
            selected: i == selected,
            textDirection: TextDirection.ltr,
          ),
        ),
    ];
  };

  @override
  bool shouldRepaint(_TrendPainter old) =>
      old.points != points ||
      old.selected != selected ||
      old.color != color ||
      old.label != label ||
      old.labelOn != labelOn ||
      old.baseline != baseline ||
      old.plotHeight != plotHeight ||
      old.textScaler != textScaler;

  @override
  bool shouldRebuildSemantics(_TrendPainter old) =>
      old.points != points || old.selected != selected;
}
