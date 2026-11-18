// 数字的显示：字节、速度、分享率、时间。
import 'package:intl/intl.dart';

const _units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];

/// 字节数，例如 1.5 GiB。
String formatBytes(num bytes, {int digits = 1}) {
  if (bytes < 0) return '-';
  var v = bytes.toDouble();
  var i = 0;
  while (v >= 1024 && i < _units.length - 1) {
    v /= 1024;
    i++;
  }
  return i == 0
      ? '${v.toInt()} B'
      : '${v.toStringAsFixed(v >= 100 ? 0 : digits)} ${_units[i]}';
}

/// 速度，例如 2.3 MiB/s；0 显示成 0。
String formatSpeed(num bytesPerSecond) =>
    bytesPerSecond <= 0 ? '0' : '${formatBytes(bytesPerSecond)}/s';

/// 分享率：两位小数，无穷大（只上传）显示成 ∞。
String formatRatio(num ratio) {
  if (ratio.isInfinite || ratio > 1e6) return '∞';
  return ratio.toStringAsFixed(2);
}

/// 魔力等较大的数：千分位，最多一位小数。
String formatNumber(num v) => NumberFormat('#,##0.#').format(v);

/// Unix 时间戳（秒）的本地时间：今天只显示时分，今年显示月日时分，更早显示年月日。
String formatTime(int unixSeconds, {DateTime? now}) {
  if (unixSeconds <= 0) return '-';
  final t = DateTime.fromMillisecondsSinceEpoch(unixSeconds * 1000);
  final n = now ?? DateTime.now();
  if (t.year == n.year && t.month == n.month && t.day == n.day) {
    return DateFormat('HH:mm').format(t);
  }
  if (t.year == n.year) return DateFormat('MM-dd HH:mm').format(t);
  return DateFormat('yyyy-MM-dd').format(t);
}

/// 剩余时间（秒），例如 1h 20m；负数（无法估计）显示成 ∞。
String formatEta(int seconds) {
  if (seconds < 0 || seconds >= 8640000) return '∞';
  final d = Duration(seconds: seconds);
  if (d.inDays > 0) return '${d.inDays}d ${d.inHours % 24}h';
  if (d.inHours > 0) return '${d.inHours}h ${d.inMinutes % 60}m';
  if (d.inMinutes > 0) return '${d.inMinutes}m';
  return '${d.inSeconds}s';
}
