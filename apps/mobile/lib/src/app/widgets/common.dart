// 页面里通用的小部件：加载与出错、空状态、数值卡片、行组与行卡片、站点图标、海报、连接条。
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../../remote/connection.dart';
import '../../remote/frames.dart';
import '../../remote/session.dart';
import '../../remote/transport.dart';
import '../icons.dart';
import '../providers.dart';
import '../theme.dart';
import 'tunnel_image.dart';

/// 按 AsyncValue 显示：加载中转圈，出错给原因与重试，有数据时用 [data]。
class AsyncBody<T> extends StatelessWidget {
  const AsyncBody({
    super.key,
    required this.value,
    required this.data,
    required this.onRetry,
  });

  final AsyncValue<T> value;
  final Widget Function(T data) data;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return switch (value) {
      AsyncData(:final value) => data(value),
      AsyncError(:final error) when !value.isLoading => ErrorState(
        message: describeError(error, S.of(context)),
        onRetry: onRetry,
      ),
      _ when value.hasValue => data(value.requireValue),
      _ => const Center(child: CircularProgressIndicator()),
    };
  }
}

/// 出错的说明与重试。设备被撤销、主机换了密钥或关掉了远程访问时只说明连接的状态：
/// 不是这一页出错，重试也没有用（连接条上有「重新配对」）。
class ErrorState extends ConsumerWidget {
  const ErrorState({super.key, required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final problem = connectionProblem(
      s,
      ref.watch(connectionStatusProvider).value,
    );
    return ListView(
      // 可以下拉刷新
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.all(32),
      children: [
        Center(
          child: PtIcon(
            problem == null ? PtGlyph.cloudOff : PtGlyph.linkOff,
            size: 40,
            weight: 1.5,
            color: Theme.of(context).colorScheme.onSurfaceVariant,
          ),
        ),
        const SizedBox(height: 14),
        Text(problem ?? s.errorPrefix(message), textAlign: TextAlign.center),
        if (problem == null) ...[
          const SizedBox(height: 12),
          Center(
            child: FilledButton.tonal(onPressed: onRetry, child: Text(s.retry)),
          ),
        ],
      ],
    );
  }
}

/// 连接停在要重新配对或主机关掉了远程访问时的说明；其他状态为 null。
String? connectionProblem(S s, ConnectionStatus? st) => switch (st?.state) {
  LinkState.revoked =>
    st!.goAway == GoAwayReason.keyRotated ? s.connKeyRotated : s.connRevoked,
  LinkState.disabled => s.connDisabled,
  _ => null,
};

/// 把接口的错误写成给人看的一句话（App API 的错误是 {"error","message"}）。
/// 连接上的异常被生成的客户端包成「Exception occurred: GET /torrents」，原因在内层异常里。
String describeError(Object e, S s) {
  if (e is ApiException) {
    final inner = e.innerException;
    if (inner != null) return describeError(inner, s);
    final body = e.message;
    if (body != null && body.isNotEmpty) {
      try {
        final v = jsonDecode(body);
        if (v is Map && v['message'] is String) return v['message'] as String;
      } on FormatException {
        // 不是 JSON，原样给
      }
      return body;
    }
    return 'HTTP ${e.code}';
  }
  if (e is HandshakeException && e.notPaired) return s.connRevoked;
  if (e is SessionClosedException) {
    return switch (e.goAway) {
      GoAwayReason.revoked => s.connRevoked,
      GoAwayReason.keyRotated => s.connKeyRotated,
      GoAwayReason.disabled => s.connDisabled,
      _ => e.message,
    };
  }
  return e.toString().replaceFirst(RegExp(r'^[A-Za-z]+Exception: '), '');
}

class EmptyState extends StatelessWidget {
  const EmptyState({super.key, this.message, this.icon = PtGlyph.inbox});
  final String? message;
  final PtGlyph icon;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context);
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.all(48),
      children: [
        Center(
          child: Container(
            width: 64,
            height: 64,
            decoration: BoxDecoration(
              color: t.colorScheme.surface,
              border: Border.all(color: context.pt.hairline),
              borderRadius: BorderRadius.circular(18),
            ),
            alignment: Alignment.center,
            child: PtIcon(
              icon,
              size: 28,
              color: t.colorScheme.onSurfaceVariant,
            ),
          ),
        ),
        const SizedBox(height: 14),
        Text(
          message ?? S.of(context).empty,
          textAlign: TextAlign.center,
          style: t.textTheme.bodyMedium?.copyWith(
            color: t.colorScheme.onSurfaceVariant,
          ),
        ),
      ],
    );
  }
}

/// 一个数值：名字、值与可选的说明（概览的 KPI 用）。照 Voltip 的数值卡片：每张卡把自己的色相
/// 淡淡地混进卡片面与边框，图标放在同色的小方块里——一眼分得出是哪个数。
class MetricTile extends StatelessWidget {
  const MetricTile({
    super.key,
    required this.label,
    required this.value,
    required this.color,
    this.icon,
    this.note,
  });

  final String label;
  final String value;
  final Color color;
  final PtGlyph? icon;
  final String? note;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context);
    final dark = t.brightness == Brightness.dark;
    final surface = t.colorScheme.surface;
    return DecoratedBox(
      decoration: BoxDecoration(
        color: tint(color, surface, dark ? 0.08 : 0.05),
        border: Border.all(color: tint(color, surface, dark ? 0.28 : 0.22)),
        borderRadius: BorderRadius.circular(cardRadius),
      ),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(14, 12, 14, 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              children: [
                if (icon != null) ...[
                  Container(
                    width: 26,
                    height: 26,
                    decoration: BoxDecoration(
                      color: tint(color, surface, dark ? 0.2 : 0.12),
                      borderRadius: BorderRadius.circular(8),
                    ),
                    alignment: Alignment.center,
                    child: PtIcon(icon!, size: 15, weight: 2, color: color),
                  ),
                  const SizedBox(width: 8),
                ],
                Flexible(
                  child: Text(
                    label,
                    style: t.textTheme.labelMedium?.copyWith(
                      fontSize: 12.5,
                      color: t.colorScheme.onSurfaceVariant,
                    ),
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              value,
              style: t.textTheme.titleLarge?.copyWith(
                fontSize: 21,
                fontWeight: FontWeight.w700,
                fontFeatures: const [FontFeature.tabularFigures()],
              ),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
            if (note != null)
              Padding(
                padding: const EdgeInsets.only(top: 2),
                child: Text(
                  note!,
                  style: t.textTheme.bodySmall?.copyWith(
                    color: t.colorScheme.onSurfaceVariant,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
          ],
        ),
      ),
    );
  }
}

/// 一组行：白卡片加细线，行与行之间一条分隔线（Voltip 手机端的设置列表）；可带一个小标题。
class RowGroup extends StatelessWidget {
  const RowGroup({super.key, this.title, required this.children});
  final String? title;
  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (title != null)
          Padding(
            padding: const EdgeInsets.fromLTRB(4, 0, 4, 8),
            child: Text(
              title!,
              style: t.textTheme.labelLarge?.copyWith(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: t.colorScheme.onSurfaceVariant,
              ),
            ),
          ),
        Card(
          clipBehavior: Clip.antiAlias,
          child: Column(
            children: [
              for (var i = 0; i < children.length; i++) ...[
                if (i > 0) const Divider(height: 1),
                children[i],
              ],
            ],
          ),
        ),
      ],
    );
  }
}

/// 打开一页的行：图标、名字、说明、行尾的箭头（或 [trailing]），整行可点。
class NavRow extends StatelessWidget {
  const NavRow({
    super.key,
    required this.icon,
    required this.title,
    this.subtitle,
    this.trailing,
    this.onTap,
    this.danger = false,
  });

  final PtGlyph icon;
  final String title;
  final String? subtitle;
  final Widget? trailing;
  final VoidCallback? onTap;

  /// 危险的操作（忘掉这台主机）：图标与名字用危险色，没有箭头。
  final bool danger;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context);
    final color = danger ? t.colorScheme.error : null;
    return ListTile(
      minTileHeight: 56,
      leading: PtIcon(icon, size: 20, color: color),
      title: Text(title, style: color == null ? null : TextStyle(color: color)),
      subtitle: subtitle == null ? null : Text(subtitle!),
      trailing:
          trailing ??
          (onTap == null || danger
              ? null
              : PtIcon(
                  PtGlyph.chevronRight,
                  size: 18,
                  color: context.pt.subtle,
                )),
      onTap: onTap,
    );
  }
}

/// 列表里的一张行卡片（种子、搜索结果、任务……）：白卡片加细线，14 的内边距，整张可点。
class RowCard extends StatelessWidget {
  const RowCard({super.key, required this.child, this.onTap, this.padding});
  final Widget child;
  final VoidCallback? onTap;
  final EdgeInsetsGeometry? padding;

  @override
  Widget build(BuildContext context) => Card(
    clipBehavior: Clip.antiAlias,
    child: InkWell(
      onTap: onTap,
      child: Padding(
        padding: padding ?? const EdgeInsets.fromLTRB(14, 12, 14, 12),
        child: child,
      ),
    ),
  );
}

/// 状态的小标签：状态色的字压在它自己 12% 的淡染底上（[PtColors] 的状态色按这个底校过对比度）。
class StatusPill extends StatelessWidget {
  const StatusPill({super.key, required this.text, required this.color});
  final String text;
  final Color color;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
    decoration: BoxDecoration(
      color: color.withValues(alpha: 0.12),
      borderRadius: BorderRadius.circular(6),
    ),
    child: Text(
      text,
      style: TextStyle(color: color, fontSize: 12, fontWeight: FontWeight.w600),
    ),
  );
}

/// 站点图标（经隧道）；取不到时显示首字母。
class SiteIcon extends ConsumerWidget {
  const SiteIcon(this.site, {super.key, this.size = 28});
  final String site;
  final double size;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = ref.watch(connectionProvider);
    final scheme = Theme.of(context).colorScheme;
    final fallback = Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: scheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(size / 4),
      ),
      child: Text(
        site.isEmpty ? '?' : site.substring(0, 1).toUpperCase(),
        style: TextStyle(
          fontSize: size * 0.45,
          fontWeight: FontWeight.w600,
          color: scheme.onSurfaceVariant,
        ),
      ),
    );
    if (c == null) return fallback;
    return ClipRRect(
      borderRadius: BorderRadius.circular(size / 5),
      child: Image(
        image: TunnelImageProvider(
          c.client,
          '/api/app/v1/favicon/${Uri.encodeComponent(site)}',
        ),
        width: size,
        height: size,
        fit: BoxFit.cover,
        errorBuilder: (_, _, _) => fallback,
      ),
    );
  }
}

/// TMDB 海报（经隧道）；没有海报时是一块灰底的图标。
class Poster extends ConsumerWidget {
  const Poster(
    this.posterPath, {
    super.key,
    this.width = 72,
    this.size = 'w342',
  });
  final String? posterPath;
  final double width;
  final String size;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = ref.watch(connectionProvider);
    final path = tmdbImagePath(posterPath, size: size);
    final placeholder = Container(
      color: Theme.of(context).colorScheme.surfaceContainerHighest,
      alignment: Alignment.center,
      child: PtIcon(
        PtGlyph.film,
        size: 22,
        color: Theme.of(context).colorScheme.onSurfaceVariant,
      ),
    );
    return ClipRRect(
      borderRadius: BorderRadius.circular(8),
      child: SizedBox(
        width: width,
        child: AspectRatio(
          aspectRatio: 2 / 3,
          child: (c == null || path == null)
              ? placeholder
              : Image(
                  image: TunnelImageProvider(c.client, path),
                  fit: BoxFit.cover,
                  errorBuilder: (_, _, _) => placeholder,
                ),
        ),
      ),
    );
  }
}

/// 页面顶部的连接条：没连上时说明原因，撤销时给「重新配对」。
class ConnectionBanner extends ConsumerWidget {
  const ConnectionBanner({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final st = ref.watch(connectionStatusProvider).value;
    if (st == null || st.state == LinkState.online) {
      return const SizedBox.shrink();
    }
    final scheme = Theme.of(context).colorScheme;
    final (text, color, action) = switch (st.state) {
      LinkState.connecting => (
        s.connConnecting,
        scheme.secondaryContainer,
        null,
      ),
      LinkState.revoked => (
        connectionProblem(s, st)!,
        scheme.errorContainer,
        s.repair,
      ),
      LinkState.disabled => (
        connectionProblem(s, st)!,
        scheme.tertiaryContainer,
        null,
      ),
      LinkState.failed => (
        s.connFailed(st.message ?? ''),
        scheme.errorContainer,
        s.retry,
      ),
      LinkState.offline || LinkState.online => (
        s.connOffline,
        scheme.surfaceContainerHighest,
        s.retry,
      ),
    };
    return Material(
      color: color,
      child: SafeArea(
        bottom: false,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
          child: Row(
            children: [
              if (st.state == LinkState.connecting)
                const SizedBox(
                  width: 14,
                  height: 14,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              else
                const PtIcon(PtGlyph.info, size: 18),
              const SizedBox(width: 8),
              Expanded(
                child: Text(text, maxLines: 2, overflow: TextOverflow.ellipsis),
              ),
              if (action != null)
                TextButton(
                  onPressed: () {
                    if (st.state == LinkState.revoked) {
                      ref.read(hostProvider.notifier).forget();
                    } else {
                      final c = ref.read(connectionProvider);
                      c?.start();
                      c?.ensure().ignore();
                    }
                  },
                  child: Text(action),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

/// 连接方式的说明（设置页、概览页用）。
String viaLabel(S s, Via? via) => switch (via) {
  Via.direct => s.viaDirect,
  Via.relay => s.viaRelay,
  null => '-',
};
