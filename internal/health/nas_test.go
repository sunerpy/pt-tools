package health

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/nas"
)

func TestNASCheck_NoSources(t *testing.T) {
	now := time.Date(2026, 11, 6, 15, 0, 0, 0, time.Local)
	clock := func() time.Time { return now }
	run := func(list []nas.Status) []Item {
		return NAS(func() []nas.Status { return list }, clock).Run(context.Background())
	}

	got := run([]nas.Status{{ID: nas.LocalID, Name: "本机", Enabled: true, State: nas.StateOK}})
	require.Len(t, got, 1, "本机来源不算")
	assert.Equal(t, Item{
		Key: "nas", Group: GroupNAS, Title: "NAS", Status: StatusSkip, Detail: "还没添加 NAS 来源",
		Hint: "在「系统 → NAS 监控」里添加来源（能接入 TrueNAS），体检会看它的存储池、告警与硬盘温度",
	}, got[0])

	got = run([]nas.Status{{ID: "3", Name: "家里", Kind: nas.KindTrueNAS, Enabled: false, State: nas.StateDisabled}})
	assert.Equal(t, []Item{{
		Key: "nas", Group: GroupNAS, Title: "NAS", Status: StatusSkip, Detail: "NAS 来源都停用了",
		Hint: "在「系统 → NAS 监控」里编辑来源，打开「启用」",
	}}, got, "停用的来源不列")

	got = NAS(nil, clock).Run(context.Background())
	assert.Equal(t, []Item{{Key: "nas", Group: GroupNAS, Title: "NAS", Status: StatusSkip, Detail: "NAS 监控没有启动"}}, got)
}

func TestNASKindLabels(t *testing.T) {
	assert.Equal(t, "（能接入 TrueNAS）", nasKindLabels())
	assert.Equal(t, "TrueNAS", nasKindLabel(nas.KindTrueNAS))
	assert.Equal(t, "glances", nasKindLabel("glances"), "这个版本不认的类型写原样")
	assert.Equal(t, "NAS", nasKindLabel(""))
}

// healthySnapshot 是一份都正常的 TrueNAS 快照。
func healthySnapshot(at time.Time) *nas.Snapshot {
	yes := true
	t38, t41, t55 := 38.0, 41.0, 55.0
	return &nas.Snapshot{
		At:     at,
		System: nas.System{Hostname: "truenas", Platform: "TrueNAS", Version: "25.04.1"},
		Pools:  []nas.Pool{{Name: "UniData", Status: "ONLINE", Healthy: &yes}, {Name: "SSDData", Status: "ONLINE", Healthy: &yes}},
		Disks: []nas.Disk{
			{Name: "sda", Kind: nas.DiskHDD, TempC: &t38},
			{Name: "sdb", Kind: nas.DiskHDD, TempC: &t41},
			{Name: "nvme0n1", Kind: nas.DiskNVMe, TempC: &t55},
			{Name: "sdc", Kind: nas.DiskSSD},
		},
		Alerts: []nas.Alert{{Level: nas.AlertWarning, Title: "存储池 UniData 已用 85%"}, {Level: nas.AlertInfo, Title: "TrueNAS 有新版本可以更新"}},
		Facets: map[string]nas.Facet{
			nas.FacetPools: {Status: nas.FacetOK}, nas.FacetDisks: {Status: nas.FacetOK}, nas.FacetAlerts: {Status: nas.FacetOK},
		},
	}
}

// badSnapshot 是存储池不健康、有严重告警、硬盘偏热的快照：用来测过时的快照不当成现在的样子。
func badSnapshot(at time.Time) *nas.Snapshot {
	s := healthySnapshot(at)
	no, hot := false, 60.0
	s.Pools[0].Healthy = &no
	s.Alerts = append(s.Alerts, nas.Alert{Level: nas.AlertCritical, Title: "存储池 UniData 状态不健康"})
	s.Disks[0].TempC = &hot
	return s
}

func TestNASCheck_Source(t *testing.T) {
	now := time.Date(2026, 11, 6, 15, 0, 0, 0, time.Local)
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	run := func(st nas.Status) Item {
		st.Enabled = true
		if st.ID == "" {
			st.ID = "7"
		}
		if st.Kind == "" {
			st.Kind = nas.KindTrueNAS
		}
		got := NAS(func() []nas.Status { return []nas.Status{st} }, func() time.Time { return now }).Run(context.Background())
		require.Len(t, got, 1)
		return got[0]
	}

	t.Run("都正常", func(t *testing.T) {
		it := run(nas.Status{Name: "家里的 TrueNAS", State: nas.StateOK, LastOKAt: ago(20 * time.Second), Snapshot: healthySnapshot(now.Add(-20 * time.Second))})
		assert.Equal(t, Item{
			Key: "nas:7", Group: GroupNAS, Title: "家里的 TrueNAS", Status: StatusOK,
			Detail: "TrueNAS 25.04.1（truenas） · 2 个存储池都健康 · 4 块硬盘，最高 55 °C · 2 条告警，都不严重 · 11-06 14:59 更新",
		}, it)
	})

	t.Run("没有名字时写类型", func(t *testing.T) {
		snap := healthySnapshot(now)
		snap.Alerts, snap.System = nil, nas.System{}
		for i := range snap.Disks {
			snap.Disks[i].TempC = nil
		}
		snap.Pools[1].Healthy = nil
		snap.Facets[nas.FacetCPU] = nas.Facet{Status: nas.FacetUnsupported}
		it := run(nas.Status{State: nas.StateOK, LastOKAt: &now, Snapshot: snap})
		assert.Equal(t, "TrueNAS", it.Title)
		assert.Equal(t, "TrueNAS · 2 个存储池 · 4 块硬盘 · 没有告警 · 11-06 15:00 更新", it.Detail, "有池读不到健康状态时不说「都健康」")
	})

	t.Run("读不到的几类写上", func(t *testing.T) {
		snap := healthySnapshot(now)
		snap.Facets[nas.FacetPools] = nas.Facet{Status: nas.FacetDenied, Reason: "没有权限"}
		snap.Facets[nas.FacetAlerts] = nas.Facet{Status: nas.FacetError, Reason: "超时"}
		snap.Facets[nas.FacetDisks] = nas.Facet{Status: nas.FacetUnsupported}
		yes := false
		snap.Pools[0].Healthy = &yes // 存储池读不到时不按它判断
		it := run(nas.Status{Name: "n", State: nas.StateOK, LastOKAt: &now, Snapshot: snap})
		assert.Equal(t, StatusOK, it.Status)
		assert.Equal(t, "TrueNAS 25.04.1（truenas） · 读不到存储池、告警 · 11-06 15:00 更新", it.Detail)
	})

	t.Run("存储池不健康、严重告警、硬盘偏热", func(t *testing.T) {
		snap := healthySnapshot(now)
		no := false
		snap.Pools = append(snap.Pools, nas.Pool{Name: "Cold", Status: "DEGRADED", Healthy: &no}, nas.Pool{Name: "Ice", Healthy: &no})
		early, late := now.Add(-2*time.Hour), now.Add(-time.Hour)
		snap.Alerts = append(snap.Alerts,
			nas.Alert{Level: nas.AlertCritical, Title: "没有时间的一条"},
			nas.Alert{Level: nas.AlertCritical, Title: "存储池 Cold 状态不健康（DEGRADED）", At: &late},
			nas.Alert{Level: nas.AlertCritical, Title: "早一点的", At: &early},
		)
		hot, warm, cool := 50.0, 70.0, 69.0
		snap.Disks = append(snap.Disks,
			nas.Disk{Name: "sdd", Kind: nas.DiskHDD, TempC: &hot},
			nas.Disk{Name: "sde", Kind: nas.DiskSSD, TempC: &warm},
			nas.Disk{Name: "sdf", Kind: nas.DiskSSD, TempC: &cool},
			nas.Disk{Name: "x", Kind: nas.DiskUnknown, TempC: &hot},
		)
		it := run(nas.Status{Name: "n", State: nas.StateOK, LastOKAt: &now, Snapshot: snap})
		assert.Equal(t, StatusFail, it.Status)
		assert.Equal(t, "存储池不健康：Cold（DEGRADED）、Ice · 3 条严重告警，最新一条：存储池 Cold 状态不健康（DEGRADED） · "+
			"硬盘偏热：sdd 50 °C、sde 70 °C、x 50 °C", it.Detail)
		assert.Equal(t, "在 TrueNAS 的存储页里看是哪块盘出了问题；换盘或修复以后这里会恢复", it.Hint, "怎么修写最坏的那个")

		snap.Pools = snap.Pools[:2]
		it = run(nas.Status{Name: "n", State: nas.StateOK, LastOKAt: &now, Snapshot: snap})
		assert.Equal(t, StatusWarn, it.Status)
		assert.Contains(t, it.Hint, "告警里看详情")

		snap.Alerts = []nas.Alert{{Level: nas.AlertCritical, Title: "只有一条没有时间的"}}
		it = run(nas.Status{Name: "n", State: nas.StateOK, LastOKAt: &now, Snapshot: snap})
		assert.Contains(t, it.Detail, "1 条严重告警，最新一条：只有一条没有时间的")
	})

	t.Run("认证失败与证书", func(t *testing.T) {
		cases := []struct {
			state, msg, wantDetail, hint string
		}{
			{nas.StateAuthFailed, "认证失败：API Key 不对或已被吊销", "认证失败：API Key 不对或已被吊销", "重新填 API Key 或密码"},
			{nas.StateAuthFailed, "", "认证失败", "改好以前 pt-tools 不会再登录"},
			{nas.StateCertUntrusted, nas.ErrCertUntrusted.Error(), nas.ErrCertUntrusted.Error(), "信任这张证书并再测一次"},
			{nas.StateCertUntrusted, "", "证书不受信任", "核对指纹"},
			{nas.StateCertChanged, "", "证书和信任的那张不一样", "先确认是你自己换了 NAS 的证书"},
		}
		for _, c := range cases {
			// 停住的来源有旧快照也不看
			snap := healthySnapshot(now)
			no := false
			snap.Pools[0].Healthy = &no
			it := run(nas.Status{Name: "n", State: c.state, Message: c.msg, LastOKAt: ago(time.Hour), Snapshot: snap})
			assert.Equal(t, StatusFail, it.Status, c.state)
			assert.Equal(t, c.wantDetail, it.Detail, c.state)
			assert.Contains(t, it.Hint, c.hint, c.state)
		}
	})

	t.Run("不支持与正在连接", func(t *testing.T) {
		it := run(nas.Status{Name: "n", Kind: "synology", State: nas.StateUnsupported, Message: "这个版本不支持「synology」来源"})
		assert.Equal(t, Item{Key: "nas:7", Group: GroupNAS, Title: "n", Status: StatusSkip, Detail: "这个版本不支持「synology」来源"}, it)
		assert.Equal(t, "这个版本不支持这类来源", run(nas.Status{Name: "n", State: nas.StateUnsupported}).Detail)
		it = run(nas.Status{Name: "n", State: nas.StateConnecting})
		assert.Equal(t, Item{Key: "nas:7", Group: GroupNAS, Title: "n", Status: StatusSkip, Detail: "正在连接，还没读到数据"}, it)
	})

	t.Run("连不上", func(t *testing.T) {
		// 刚出错：提醒，快照还新，照样看
		snap := healthySnapshot(now.Add(-90 * time.Second))
		hot := 60.0
		snap.Disks[0].TempC = &hot
		it := run(nas.Status{
			Name: "n", State: nas.StateError, Message: "连不上 TrueNAS：dial tcp 10.0.0.2:443: connect: connection refused",
			FailingSince: ago(time.Minute), LastOKAt: ago(90 * time.Second), Snapshot: snap,
		})
		assert.Equal(t, StatusWarn, it.Status)
		assert.Equal(t, "11-06 14:59 起连不上，正在重试：连不上 TrueNAS：dial tcp 10.0.0.2:443: connect: connection refused · 硬盘偏热：sda 60 °C", it.Detail)
		assert.Contains(t, it.Hint, "超过 3 分钟会记成失败")

		// 超过 3 分钟：失败，快照过时了不看（里面的存储池不健康也不写）
		old := badSnapshot(now.Add(-200 * time.Second))
		it = run(nas.Status{
			Name: "n", State: nas.StateError, Message: "10 秒内没有回应",
			FailingSince: ago(3 * time.Minute), LastOKAt: ago(200 * time.Second), Snapshot: old,
		})
		assert.Equal(t, StatusFail, it.Status)
		assert.Equal(t, "11-06 14:57 起连不上：10 秒内没有回应（最后一次读到是 11-06 14:56）", it.Detail)
		assert.Contains(t, it.Hint, "重新连接")

		// 从没连上过
		it = run(nas.Status{Name: "n", State: nas.StateError, FailingSince: ago(10 * time.Minute)})
		assert.Equal(t, "11-06 14:50 起连不上：出错了", it.Detail)

		// 设置本身用不了：没有在重试
		it = run(nas.Status{Name: "n", State: nas.StateError, Message: "保存的密钥解不开"})
		assert.Equal(t, StatusFail, it.Status)
		assert.Equal(t, "保存的密钥解不开", it.Detail)
		assert.Contains(t, it.Hint, "重新填 API Key")
		assert.Equal(t, "用不了", run(nas.Status{Name: "n", State: nas.StateError}).Detail)
	})

	t.Run("数据过时", func(t *testing.T) {
		// 过时的快照不当成现在的样子：里面的存储池不健康、严重告警与硬盘偏热都不写
		it := run(nas.Status{Name: "n", State: nas.StateOK, LastOKAt: ago(4 * time.Minute), Snapshot: badSnapshot(now.Add(-4 * time.Minute))})
		assert.Equal(t, StatusWarn, it.Status)
		assert.Equal(t, "数据过时了：最后一次读到是 11-06 14:56", it.Detail)
		assert.Contains(t, it.Hint, "重启 pt-tools")
	})

	t.Run("连着但没有快照", func(t *testing.T) {
		it := run(nas.Status{Name: "n", State: nas.StateOK, LastOKAt: &now})
		assert.Equal(t, StatusSkip, it.Status)
		assert.Equal(t, "还没读到数据", it.Detail)
	})

	t.Run("原因里的凭证去掉", func(t *testing.T) {
		it := run(nas.Status{Name: "n", State: nas.StateAuthFailed, Message: "认证失败 token=abcdefghijklmnopqrstuvwxyz0123456789"})
		assert.NotContains(t, it.Detail, "abcdefghijklmnopqrstuvwxyz0123456789")
	})
}

func TestNASCheck_SeveralSourcesAndOrder(t *testing.T) {
	now := time.Date(2026, 11, 6, 15, 0, 0, 0, time.Local)
	list := []nas.Status{
		{ID: nas.LocalID, Name: "本机", Enabled: true, State: nas.StateOK},
		{ID: "2", Name: "甲", Kind: nas.KindTrueNAS, Enabled: true, State: nas.StateConnecting},
		{ID: "3", Name: "乙", Kind: nas.KindTrueNAS, Enabled: false, State: nas.StateDisabled},
		{ID: "4", Name: "丙", Kind: nas.KindTrueNAS, Enabled: true, State: nas.StateOK, LastOKAt: &now, Snapshot: healthySnapshot(now)},
	}
	got := NAS(func() []nas.Status { return list }, func() time.Time { return now }).Run(context.Background())
	require.Len(t, got, 2)
	assert.Equal(t, "nas:2", got[0].Key)
	assert.Equal(t, "nas:4", got[1].Key)
	assert.Equal(t, StatusOK, got[1].Status)

	// 体检页的顺序：NAS 排在磁盘空间后面、时间前面
	items := []Item{{Group: GroupTime}, {Group: GroupNAS}, {Group: GroupDisks}}
	sortItems(items)
	assert.Equal(t, []string{GroupDisks, GroupNAS, GroupTime}, []string{items[0].Group, items[1].Group, items[2].Group})
}
