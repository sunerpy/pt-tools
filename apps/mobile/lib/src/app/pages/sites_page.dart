import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
import '../widgets/common.dart';

/// 站点：登录状态（离封号阈值还有几天）、今天的签到、站点上的用户数据与站内信未读数；完全控制的设备可以立即签到。
class SitesPage extends ConsumerWidget {
  const SitesPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final list = ref.watch(sitesProvider);
    final canWrite = ref.watch(canWriteProvider);
    return Scaffold(
      appBar: AppBar(title: Text(s.moreSites)),
      body: RefreshIndicator(
        onRefresh: () async {
          try {
            ref.invalidate(sitesProvider);
            await ref.read(sitesProvider.future);
          } on Object {
            // 页面显示错误
          }
        },
        child: AsyncBody<AppSiteList>(
          value: list,
          onRetry: () => ref.invalidate(sitesProvider),
          data: (l) {
            // 启用的站点在前；没启用的收起来
            final on = l.items.where((x) => x.enabled).toList();
            final off = l.items.where((x) => !x.enabled).toList();
            return ListView(
              padding: const EdgeInsets.only(bottom: 24),
              children: [
                if (l.userError != null && l.userError!.isNotEmpty)
                  ListTile(
                    leading: const Icon(Icons.warning_amber),
                    title: Text(s.siteUserError(l.userError!)),
                  ),
                if (on.isEmpty) ListTile(title: Text(s.noEnabledSites)),
                for (final site in on)
                  _SiteTile(site: site, canWrite: canWrite),
                if (off.isNotEmpty)
                  ExpansionTile(
                    title: Text(s.disabledSites(off.length)),
                    children: [
                      for (final site in off)
                        ListTile(
                          dense: true,
                          leading: SiteIcon(site.name, size: 24),
                          title: Text(
                            site.displayName.isEmpty
                                ? site.name
                                : site.displayName,
                          ),
                        ),
                    ],
                  ),
              ],
            );
          },
        ),
      ),
    );
  }
}

class _SiteTile extends ConsumerStatefulWidget {
  const _SiteTile({required this.site, required this.canWrite});
  final AppSite site;
  final bool canWrite;

  @override
  ConsumerState<_SiteTile> createState() => _SiteTileState();
}

class _SiteTileState extends ConsumerState<_SiteTile> {
  bool _busy = false;

  Future<void> _attend() async {
    final s = S.of(context);
    final messenger = ScaffoldMessenger.of(context);
    setState(() => _busy = true);
    try {
      final a = await ref.read(apiProvider)!.attendSite(widget.site.name);
      messenger.showSnackBar(
        SnackBar(
          content: Text(
            a?.message?.isNotEmpty == true ? a!.message! : s.attendOk,
          ),
        ),
      );
      ref.invalidate(sitesProvider);
    } on Object catch (e) {
      messenger.showSnackBar(
        SnackBar(content: Text(s.errorPrefix(describeError(e)))),
      );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final t = Theme.of(context);
    final site = widget.site;
    final u = site.user;
    final att = site.attendance;
    final attText = switch (att.status) {
      'signed' || 'already' => s.attendSigned,
      'failed' => s.attendFailed,
      'unsupported' => s.attendUnsupported,
      _ => att.supported ? s.attendPending : s.attendUnsupported,
    };
    final login = site.login.tier == 'unknown'
        ? s.loginUnknown
        : s.loginDays(site.login.daysRemaining);
    final warn = const [
      '7d',
      '3d',
      'banned-imminent',
    ].contains(site.login.tier);
    return Card(
      margin: const EdgeInsets.fromLTRB(12, 8, 12, 0),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 10, 8, 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                SiteIcon(site.name, size: 28),
                const SizedBox(width: 10),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        site.displayName.isEmpty ? site.name : site.displayName,
                        style: t.textTheme.titleMedium,
                      ),
                      Text(
                        site.enabled ? login : s.siteDisabled,
                        style: TextStyle(
                          color: warn ? t.colorScheme.error : null,
                          fontSize: 12,
                        ),
                      ),
                    ],
                  ),
                ),
                if (u != null && u.unreadMessages > 0)
                  Badge(label: Text(s.unreadMessages(u.unreadMessages))),
                if (widget.canWrite &&
                    att.supported &&
                    att.enabled &&
                    att.status != 'signed' &&
                    att.status != 'already')
                  TextButton(
                    onPressed: _busy ? null : _attend,
                    child: _busy
                        ? const SizedBox(
                            width: 16,
                            height: 16,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : Text(s.attend),
                  ),
              ],
            ),
            if (u != null) ...[
              const SizedBox(height: 8),
              Wrap(
                spacing: 12,
                runSpacing: 4,
                children: [
                  Text(
                    '${u.username}${u.level != null && u.level!.isNotEmpty ? ' · ${u.level}' : ''}',
                    style: t.textTheme.bodySmall,
                  ),
                  Text('↑${formatBytes(u.uploaded)}'),
                  Text('↓${formatBytes(u.downloaded)}'),
                  Text('${s.kpiRatio} ${formatRatio(u.ratio)}'),
                  Text('${s.kpiBonus} ${formatNumber(u.bonus)}'),
                  Text('${s.kpiSeeding} ${u.seeding}'),
                ],
              ),
            ],
            if (att.supported) ...[
              const SizedBox(height: 4),
              Text(
                '$attText${att.message != null && att.message!.isNotEmpty ? '：${att.message}' : ''}',
                style: t.textTheme.bodySmall,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ],
        ),
      ),
    );
  }
}
