// 页面里通用的小部件：加载与出错、空状态、数值卡片、站点图标、海报、连接条。
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../l10n/app_localizations.dart';
import '../../remote/connection.dart';
import '../../remote/transport.dart';
import '../providers.dart';
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
        message: describeError(error),
        onRetry: onRetry,
      ),
      _ when value.hasValue => data(value.requireValue),
      _ => const Center(child: CircularProgressIndicator()),
    };
  }
}

class ErrorState extends StatelessWidget {
  const ErrorState({super.key, required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return ListView(
      // 可以下拉刷新
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.all(32),
      children: [
        Icon(
          Icons.cloud_off,
          size: 48,
          color: Theme.of(context).colorScheme.outline,
        ),
        const SizedBox(height: 12),
        Text(s.errorPrefix(message), textAlign: TextAlign.center),
        const SizedBox(height: 12),
        Center(
          child: FilledButton.tonal(onPressed: onRetry, child: Text(s.retry)),
        ),
      ],
    );
  }
}

class EmptyState extends StatelessWidget {
  const EmptyState({super.key, this.message, this.icon = Icons.inbox_outlined});
  final String? message;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.all(48),
      children: [
        Icon(icon, size: 48, color: Theme.of(context).colorScheme.outline),
        const SizedBox(height: 12),
        Text(message ?? S.of(context).empty, textAlign: TextAlign.center),
      ],
    );
  }
}

/// 一个数值：名字、值与可选的说明，带一条彩色的边（概览的 KPI 用）。
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
  final IconData? icon;
  final String? note;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context);
    return Card(
      margin: EdgeInsets.zero,
      clipBehavior: Clip.antiAlias,
      child: Container(
        decoration: BoxDecoration(
          border: Border(left: BorderSide(color: color, width: 4)),
        ),
        padding: const EdgeInsets.fromLTRB(12, 10, 12, 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              children: [
                if (icon != null) ...[
                  Icon(icon, size: 16, color: color),
                  const SizedBox(width: 6),
                ],
                Flexible(
                  child: Text(
                    label,
                    style: t.textTheme.labelMedium,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              value,
              style: t.textTheme.titleLarge?.copyWith(
                fontWeight: FontWeight.w600,
              ),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
            if (note != null)
              Text(
                note!,
                style: t.textTheme.bodySmall,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
          ],
        ),
      ),
    );
  }
}

/// 站点图标（经隧道）；取不到时显示首字母。
class SiteIcon extends ConsumerWidget {
  const SiteIcon(this.site, {super.key, this.size = 28});
  final String site;
  final double size;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = ref.watch(connectionProvider);
    final fallback = CircleAvatar(
      radius: size / 2,
      child: Text(
        site.isEmpty ? '?' : site.substring(0, 1).toUpperCase(),
        style: TextStyle(fontSize: size * 0.45),
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
      child: Icon(
        Icons.movie_outlined,
        color: Theme.of(context).colorScheme.outline,
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
        st.goAway == 'key_rotated' ? s.connKeyRotated : s.connRevoked,
        scheme.errorContainer,
        s.repair,
      ),
      LinkState.disabled => (s.connDisabled, scheme.tertiaryContainer, null),
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
                const Icon(Icons.info_outline, size: 18),
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
