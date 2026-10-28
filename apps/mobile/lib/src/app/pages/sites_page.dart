import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ptt_api/api.dart';

import '../../../l10n/app_localizations.dart';
import '../format.dart';
import '../providers.dart';
import '../widgets/common.dart';
import '../hybrid/mobile_api.dart';
import 'mobile_editor_page.dart';

/// 站点：登录状态（离封号阈值还有几天）、今天的签到、站点上的用户数据与站内信未读数；完全控制的设备可以立即签到。
class SitesPage extends ConsumerStatefulWidget {
  const SitesPage({super.key});
  @override
  ConsumerState<SitesPage> createState() => _SitesPageState();
}

class _SitesPageState extends ConsumerState<SitesPage> {
  String _range = 'today';
  Map<String, dynamic>? _summary;
  String? _statsError;
  bool _statsLoading = false;
  int _statsGeneration = 0;
  @override
  void initState() {
    super.initState();
    _loadSummary();
  }

  Future<void> _loadSummary() async {
    final generation = ++_statsGeneration;
    setState(() => _statsLoading = true);
    try {
      final meta = await ref.read(metaProvider.future);
      if (!mounted || generation != _statsGeneration) return;
      if (!meta.features.contains('stats_export')) return;
      final summary = await mobileApi(ref, '/stats/summary?range=$_range');
      if (mounted && generation == _statsGeneration) {
        setState(() {
          _summary = summary as Map<String, dynamic>;
          _statsError = null;
        });
      }
    } on Object catch (error) {
      if (mounted && generation == _statsGeneration) {
        setState(() => _statsError = describeError(error, S.of(context)));
      }
    } finally {
      if (mounted && generation == _statsGeneration) {
        setState(() => _statsLoading = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final list = ref.watch(sitesProvider);
    final canWrite = ref.watch(canWriteProvider);
    return Scaffold(
      appBar: AppBar(
        title: Text(s.moreSites),
        actions: [
          IconButton(
            icon: const Icon(Icons.ios_share),
            tooltip: s.mobileShare,
            onPressed: () => openMobileEditor(context, ref, 'poster'),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async {
          try {
            ref.invalidate(sitesProvider);
            await ref.read(sitesProvider.future);
            await _loadSummary();
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
                if (ref
                        .watch(metaProvider)
                        .value
                        ?.features
                        .contains('stats_export') ==
                    true)
                  Card(
                    margin: const EdgeInsets.all(12),
                    child: Padding(
                      padding: const EdgeInsets.all(12),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          SegmentedButton<String>(
                            segments: [
                              ButtonSegment(
                                value: 'today',
                                label: Text(s.todayTitle),
                              ),
                              ButtonSegment(
                                value: '7d',
                                label: Text(s.mobileWeek),
                              ),
                              ButtonSegment(
                                value: '30d',
                                label: Text(s.mobileMonth),
                              ),
                            ],
                            selected: {_range},
                            onSelectionChanged: (v) {
                              setState(() {
                                _range = v.first;
                                _summary = null;
                              });
                              _loadSummary();
                            },
                          ),
                          if (_statsLoading) const LinearProgressIndicator(),
                          if (_statsError != null) ...[
                            Text(_statsError!),
                            TextButton(
                              onPressed: _loadSummary,
                              child: Text(s.retry),
                            ),
                          ],
                          if (_summary != null &&
                              !(_summary!['sites'] as List? ?? []).any(
                                (item) => item['hasBaseline'] == true,
                              ))
                            Text(s.mobileNoBaseline),
                          if (_summary != null &&
                              (_summary!['sites'] as List? ?? []).any(
                                (item) => item['hasBaseline'] == true,
                              )) ...[
                            const SizedBox(height: 12),
                            Text(
                              '↑${formatBytes((_summary!['totalUploaded'] as num?)?.toInt() ?? 0)}  ${s.billedDownloaded(formatBytes((_summary!['totalDownloaded'] as num?)?.toInt() ?? 0))}',
                            ),
                            Text(
                              '${s.kpiBonus} ${formatNumber((_summary!['totalBonus'] as num?)?.toDouble() ?? 0)}',
                            ),
                            Text(
                              s.todayBilledNote,
                              style: Theme.of(context).textTheme.bodySmall,
                            ),
                          ],
                        ],
                      ),
                    ),
                  ),
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
        SnackBar(content: Text(s.errorPrefix(describeError(e, s)))),
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
                  Text(s.billedDownloaded(formatBytes(u.downloaded))),
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
