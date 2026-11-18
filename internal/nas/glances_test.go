package nas

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlancesKind(t *testing.T) {
	k, ok := LookupKind(KindGlances)
	require.True(t, ok)
	assert.Equal(t, "Glances", k.Label)
	assert.False(t, k.Experimental)
	assert.Equal(t, UsernameOptional, k.Username)
	assert.False(t, k.RequiresSecret(""), "不开认证时不要密码")
	assert.True(t, k.RequiresSecret("admin"), "填了用户名就要密码")
	assert.Equal(t, PlainWithoutCredentials, k.Plain)

	cases := []struct{ in, want string }{
		{"http://192.168.1.10", "http://192.168.1.10:61208"},
		{"http://NAS.local/", "http://nas.local:61208"},
		{"http://192.168.1.10:8080", "http://192.168.1.10:8080"},
		{"http://[fd00::10]", "http://[fd00::10]:61208"},
		{"https://glances.lan", "https://glances.lan"},
		{"https://glances.lan:8443", "https://glances.lan:8443"},
	}
	for _, c := range cases {
		u, err := ParseURL(KindGlances, c.in)
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, u.String(), c.in)
	}
	_, err := ParseURL(KindGlances, "http://192.168.1.10:61208/api/4")
	require.ErrorIs(t, err, errURLShape)
}

func TestGlancesCollectRealFixtures(t *testing.T) {
	f := newFakeGlances(t, false)
	g := f.source(SourceConfig{}, nil)

	snap, err := g.Collect(context.Background(), true)
	require.NoError(t, err)

	assert.Equal(t, System{
		Hostname: "nas-glances", Platform: "Ubuntu 24.04", Version: "7.0.0-1014-aws",
		Model: "Intel(R) Xeon(R) 6975P-C", Cores: 32, UptimeSeconds: 9*86400 + 11*3600 + 27*60,
	}, snap.System)
	assert.InDelta(t, 34.8, snap.CPU.Percent, 1e-9)
	require.NotNil(t, snap.CPU.IOWait)
	assert.InDelta(t, 3.0, *snap.CPU.IOWait, 1e-9)
	assert.InDelta(t, 19.09765625, snap.CPU.Load1, 1e-9)
	assert.InDelta(t, 13.99951171875, snap.CPU.Load5, 1e-9)
	assert.InDelta(t, 12.10107421875, snap.CPU.Load15, 1e-9)
	assert.Nil(t, snap.CPU.TempC, "云主机的 NVMe 报 -273 °C，没有 CPU 温度")
	assert.Equal(t, Memory{Total: 66320371712, Available: 37016018944, Used: 66320371712 - 37016018944}, snap.Memory)

	assert.Equal(t, []string{"enp39s0"}, snap.Network.Interfaces, "lo、docker0、veth 不算")
	assert.Equal(t, uint64(70226), snap.Network.RxBps)
	assert.Equal(t, uint64(756596), snap.Network.TxBps)

	assert.Equal(t, []Pool{{Name: "/data", Size: 518933680128, Used: 191310761984, Free: 327606140928}}, snap.Pools,
		"Docker 挂进来的单个文件（/etc/hosts 等）不算，同一块盘只算一次")
	assert.Empty(t, snap.Disks, "-273 °C 不是真的温度")
	assert.Equal(t, Apps{Running: 2, Total: 2, UpdatesUnknown: true}, snap.Apps)
	assert.Empty(t, snap.Alerts)

	assert.Equal(t, map[string]Facet{
		FacetSystem:  {Status: FacetOK},
		FacetCPU:     {Status: FacetOK},
		FacetCPUTemp: {Status: FacetUnsupported, Reason: glancesNoCPUTemp},
		FacetMemory:  {Status: FacetOK},
		FacetNetwork: {Status: FacetOK},
		FacetPools:   {Status: FacetOK},
		FacetDisks:   {Status: FacetUnsupported, Reason: glancesNoDisks},
		FacetAlerts:  {Status: FacetUnsupported, Reason: glancesRaidOff},
		FacetApps:    {Status: FacetOK},
	}, snap.Facets)
	assert.Equal(t,
		[]string{"cpu", "system", "quicklook", "mem", "load", "network", "sensors", "uptime", "fs", "containers", "raid"},
		f.paths())
	for _, r := range f.requests() {
		assert.Empty(t, r.auth, "没填密码时不发 Authorization")
	}
}

func TestGlancesLightCollectSkipsFullPlugins(t *testing.T) {
	f := newFakeGlances(t, false)
	g := f.source(SourceConfig{}, nil)

	snap, err := g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, []string{"cpu", "mem", "load", "network", "sensors", "uptime"}, f.paths())
	assert.ElementsMatch(t, []string{FacetCPU, FacetCPUTemp, FacetMemory, FacetNetwork}, keysOf(snap.Facets))
	assert.Equal(t, int64(9*86400+11*3600+27*60), snap.System.UptimeSeconds)
	assert.Equal(t, 32, snap.System.Cores, "核数从 cpu 的 cpucore 来")
	assert.Empty(t, snap.Pools)
	assert.Empty(t, snap.Disks)

	merged := mergeLight(&Snapshot{System: System{Hostname: "nas"}, Pools: []Pool{{Name: "/volume1"}}, Facets: map[string]Facet{FacetPools: {Status: FacetOK}}}, snap)
	assert.Equal(t, "nas", merged.System.Hostname, "轻量采集不换主机信息")
	assert.Equal(t, []Pool{{Name: "/volume1"}}, merged.Pools)
}

func keysOf(m map[string]Facet) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// 一台像 NAS 的 Glances：群晖式的 btrfs 存储空间、ZFS 池、几种温度传感器、mdadm 阵列、几个网卡与容器。
func TestGlancesCollectNASLike(t *testing.T) {
	f := newFakeGlances(t, false)
	f.set("sensors", http.StatusOK, `[
		{"label":"Package id 0","unit":"C","value":48,"warning":80,"critical":100,"type":"temperature_core","key":"label"},
		{"label":"Core 0","unit":"C","value":51,"type":"temperature_core","key":"label"},
		{"label":"Composite","unit":"C","value":41.85,"warning":84.85,"critical":84.85,"type":"temperature_core","key":"label"},
		{"label":"Composite 1","unit":"C","value":38,"type":"temperature_core","key":"label"},
		{"label":"drivetemp 0","unit":"C","value":35,"type":"temperature_core","key":"label"},
		{"label":"/dev/sdb","unit":"C","value":37,"type":"temperature_hdd","key":"label"},
		{"label":"acpitz 0","unit":"C","value":27.8,"type":"temperature_core","key":"label"},
		{"label":"CPU Fan","unit":"R","value":1200,"type":"fan_speed","key":"label"},
		{"label":"Composite 2","unit":"C","value":null,"type":"temperature_core","key":"label"}
	]`)
	f.set("fs", http.StatusOK, `[
		{"device_name":"overlay","fs_type":"overlay","mnt_point":"/","size":100,"used":50,"free":50,"key":"mnt_point"},
		{"device_name":"/dev/mapper/cachedev_0","fs_type":"btrfs","mnt_point":"/volume1/docker","size":8000,"used":3000,"free":5000,"key":"mnt_point"},
		{"device_name":"/dev/mapper/cachedev_0","fs_type":"btrfs","mnt_point":"/volume1","size":8000,"used":3000,"free":5000,"key":"mnt_point"},
		{"device_name":"/dev/mapper/cachedev_1","fs_type":"btrfs","mnt_point":"/volume2","size":4000,"used":1000,"free":3000,"key":"mnt_point"},
		{"device_name":"/dev/mapper/cachedev_1","fs_type":"btrfs","mnt_point":"/volume2/@appstore","size":4000,"used":1000,"free":3000,"key":"mnt_point"},
		{"device_name":"tank/media","fs_type":"zfs","mnt_point":"/mnt/tank/media","size":900,"used":600,"free":300,"key":"mnt_point"},
		{"device_name":"tank","fs_type":"zfs","mnt_point":"/mnt/tank","size":310,"used":10,"free":300,"key":"mnt_point"},
		{"device_name":"tank/backup","fs_type":"zfs","mnt_point":"/mnt/tank/backup","size":390,"used":90,"free":300,"key":"mnt_point"},
		{"device_name":"shm","fs_type":"tmpfs","mnt_point":"/dev/shm","size":64,"used":0,"free":64,"key":"mnt_point"},
		{"device_name":"/dev/sda1","fs_type":"ext4","mnt_point":"/etc/hosts","size":200,"used":100,"free":100,"key":"mnt_point"},
		{"device_name":"/dev/sdc1","fs_type":"xfs","mnt_point":"/srv/empty","size":0,"used":0,"free":0,"key":"mnt_point"}
	]`)
	f.set("raid", http.StatusOK, `{
		"md2":{"status":"active","type":"raid5","components":{"sda5":"0","sdb5":"1","sdc5":"2"},"available":4,"used":3,"config":"UUU_"},
		"md0":{"status":"active","type":"raid1","components":{"sda1":"0","sdb1":"1"},"available":"2","used":"2","config":"UU"},
		"md1":{"status":"inactive","type":"raid1","components":{},"available":null,"used":null,"config":""},
		"md3":{"status":"active","type":"raid0","available":2,"used":1,"config":"U_"}
	}`)
	f.set("network", http.StatusOK, `[
		{"interface_name":"lo","bytes_recv_rate_per_sec":900,"bytes_sent_rate_per_sec":900},
		{"interface_name":"eth0","bytes_recv_rate_per_sec":1000,"bytes_sent_rate_per_sec":100,"is_up":true},
		{"interface_name":"eth1","bytes_recv_rate_per_sec":2000,"bytes_sent_rate_per_sec":200},
		{"interface_name":"eth2","bytes_recv_rate_per_sec":4000,"bytes_sent_rate_per_sec":400,"is_up":false},
		{"interface_name":"bond0","bytes_recv_rate_per_sec":3000,"bytes_sent_rate_per_sec":300},
		{"interface_name":"ovs_eth0","bytes_recv_rate_per_sec":1000,"bytes_sent_rate_per_sec":100},
		{"interface_name":"eth0.100","bytes_recv_rate_per_sec":50,"bytes_sent_rate_per_sec":5},
		{"interface_name":"br0","bytes_recv_rate_per_sec":1000,"bytes_sent_rate_per_sec":100},
		{"interface_name":"docker0","bytes_recv_rate_per_sec":10,"bytes_sent_rate_per_sec":10}
	]`)
	f.set("containers", http.StatusOK, `[
		{"name":"jellyfin","status":"running","engine":"docker"},
		{"name":"qbittorrent","status":"Running","engine":"docker"},
		{"name":"old","status":"exited","engine":"docker"},
		{"name":"paused","status":"paused","engine":"podman"}
	]`)
	g := f.source(SourceConfig{}, nil)

	snap, err := g.Collect(context.Background(), true)
	require.NoError(t, err)

	require.NotNil(t, snap.CPU.TempC)
	assert.InDelta(t, 48, *snap.CPU.TempC, 1e-9, "Package id 0 是整颗 CPU 的温度")
	require.Len(t, snap.Disks, 4)
	assert.Equal(t, "NVMe 1", snap.Disks[0].Name)
	assert.Equal(t, DiskNVMe, snap.Disks[0].Kind)
	assert.InDelta(t, 41.85, *snap.Disks[0].TempC, 1e-9)
	assert.Equal(t, "NVMe 2", snap.Disks[1].Name)
	assert.Equal(t, Disk{Name: "drivetemp 0", Kind: DiskUnknown, TempC: snap.Disks[2].TempC}, snap.Disks[2])
	assert.InDelta(t, 35, *snap.Disks[2].TempC, 1e-9)
	assert.Equal(t, "/dev/sdb", snap.Disks[3].Name)
	assert.Equal(t, FacetOK, snap.Facets[FacetDisks].Status)

	assert.Equal(t, []Pool{
		{Name: "/volume1", Size: 8000, Used: 3000, Free: 5000},
		{Name: "/volume2", Size: 4000, Used: 1000, Free: 3000},
		{Name: "tank", Size: 700 + 300, Used: 700, Free: 300},
	}, snap.Pools, "btrfs 同一块盘的几个挂载点只算一次、用短的名字（先挂哪个都一样）；ZFS 按池合计；内存盘、空的与单个文件不算")

	assert.Equal(t, []string{"eth0", "eth1"}, snap.Network.Interfaces, "网桥、聚合口、VLAN、没连上的网卡不算")
	assert.Equal(t, uint64(3000), snap.Network.RxBps)
	assert.Equal(t, uint64(300), snap.Network.TxBps)

	assert.Equal(t, Apps{Running: 2, Total: 4, UpdatesUnknown: true}, snap.Apps)

	assert.Equal(t, []Alert{
		{Level: AlertCritical, Title: "RAID 阵列 md1（raid1）没有在运行：inactive"},
		{Level: AlertCritical, Title: "RAID 阵列 md2（raid5）降级：4 块盘里 3 块在用"},
	}, snap.Alerts, "RAID 0 没有冗余，不算降级")
	assert.Equal(t, Facet{Status: FacetOK}, snap.Facets[FacetAlerts])
}

func TestGlancesCPUTemp(t *testing.T) {
	temp := func(v float64) *float64 { return &v }
	cases := []struct {
		name string
		list []glSensor
		want float64
		ok   bool
	}{
		{"各核里最高的", []glSensor{{Label: "Core 0", Unit: "C", Value: temp(40)}, {Label: "Core 1", Unit: "C", Value: temp(47)}, {Label: "Core 0 1", Unit: "C", Value: temp(45)}}, 47, true},
		{"AMD 的 Tctl", []glSensor{{Label: "Tccd1", Unit: "C", Value: temp(60)}, {Label: "Tctl", Unit: "C", Value: temp(55)}}, 55, true},
		{"标签为空的芯片（树莓派）", []glSensor{{Label: "cpu_thermal 0", Unit: "C", Value: temp(52)}}, 52, true},
		{"以 CPU 开头的标签", []glSensor{{Label: "CPU", Unit: "C", Value: temp(44)}, {Label: "CPU Fan", Unit: "R", Value: temp(900)}}, 44, true},
		{"主板与硬盘的不算", []glSensor{{Label: "acpitz 0", Unit: "C", Value: temp(27)}, {Label: "Composite", Unit: "C", Value: temp(40)}}, 0, false},
		{"无效的读数", []glSensor{{Label: "Package id 0", Unit: "C", Value: temp(-273)}, {Label: "Core 0", Unit: "C", Value: temp(0)}, {Label: "Core 1", Unit: "C", Value: temp(200)}, {Label: "Tctl", Unit: "C"}}, 0, false},
		{"华氏度不算", []glSensor{{Label: "Package id 0", Unit: "F", Value: temp(120)}}, 0, false},
	}
	for _, c := range cases {
		got, ok := glancesCPUTemp(c.list)
		assert.Equal(t, c.ok, ok, c.name)
		assert.InDelta(t, c.want, got, 1e-9, c.name)
	}
}

func TestGlancesNetworkFallback(t *testing.T) {
	f := newFakeGlances(t, false)
	// FreeBSD 的网卡名（ixl0、vtnet0）不像 Linux：退回「除了虚拟网卡都算」；老的 4.x 没有 *_rate_per_sec，用这段时间的字节数除以间隔
	f.set("network", http.StatusOK, `[
		{"interface_name":"lo0","bytes_recv":100,"bytes_sent":100,"time_since_update":2},
		{"interface_name":"ixl0","bytes_recv":600,"bytes_sent":60,"time_since_update":2},
		{"interface_name":"vtnet0","bytes_recv":400,"bytes_sent":40,"time_since_update":2},
		{"interface_name":"ixl0.10","bytes_recv":2000,"bytes_sent":200,"time_since_update":2},
		{"interface_name":"bridge0","bytes_recv":1000,"bytes_sent":100,"time_since_update":2,"is_up":false}
	]`)
	g := f.source(SourceConfig{}, nil)
	snap, err := g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, []string{"ixl0", "vtnet0"}, snap.Network.Interfaces, "VLAN（ixl0.10）的流量也在 ixl0 里，不算两遍")
	assert.Equal(t, uint64(500), snap.Network.RxBps)
	assert.Equal(t, uint64(50), snap.Network.TxBps)

	f.set("network", http.StatusOK, `[{"interface_name":"lo","bytes_recv_rate_per_sec":10},{"interface_name":"docker0","bytes_recv_rate_per_sec":10},{"interface_name":"veth1"}]`)
	snap, err = g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, FacetUnsupported, snap.Facets[FacetNetwork].Status)
	assert.Contains(t, snap.Facets[FacetNetwork].Reason, "network_mode: host")
	assert.Empty(t, snap.Network.Interfaces)
}

// 容器没用 host 网络时（真机 4.5.8 用默认的 bridge 网络跑：主机名是容器 ID，网卡只有 lo 与 eth0），读到的是 Glances
// 容器自己的流量：网络这一类记作读不到并写怎么改，不把它当成 NAS 的流量。
func TestGlancesBridgedNetwork(t *testing.T) {
	bridged := `[{"interface_name":"lo","bytes_recv_rate_per_sec":5},{"interface_name":"eth0","bytes_recv_rate_per_sec":1200,"bytes_sent_rate_per_sec":800}]`
	f := newFakeGlances(t, false)
	f.set("system", http.StatusOK, `{"os_name":"Linux","hostname":"3f2a9c1b7d4e","platform":"64bit","linux_distro":"Alpine Linux 3.24.2","os_version":"6.1.0"}`)
	f.set("network", http.StatusOK, bridged)
	g := f.source(SourceConfig{}, nil)

	snap, err := g.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: glancesBridgedNet}, snap.Facets[FacetNetwork], "第一次采集就认出来（主机信息先读）")
	assert.Zero(t, snap.Network.RxBps)
	assert.Empty(t, snap.Network.Interfaces)
	assert.Equal(t, FacetOK, snap.Facets[FacetCPU].Status, "别的照常读")

	snap, err = g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, FacetUnsupported, snap.Facets[FacetNetwork].Status, "轻量采集不读主机信息，用上一次读到的主机名")

	// 主机名是真名字：直接装在 NAS 上的 Glances 只有一块 eth0 也是 NAS 的流量
	f.set("system", http.StatusOK, `{"os_name":"Linux","hostname":"nas","linux_distro":"Debian GNU/Linux 12 (bookworm)","os_version":"6.1.0"}`)
	snap, err = g.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, FacetOK, snap.Facets[FacetNetwork].Status)
	assert.Equal(t, []string{"eth0"}, snap.Network.Interfaces)
	assert.Equal(t, uint64(1200), snap.Network.RxBps)
}

func TestGlancesBridged(t *testing.T) {
	net := func(names ...string) []glNet {
		out := make([]glNet, 0, len(names))
		for _, n := range names {
			out = append(out, glNet{Name: n})
		}
		return out
	}
	const id = "3f2a9c1b7d4e"
	cases := []struct {
		name     string
		hostname string
		list     []glNet
		want     bool
	}{
		{"容器自己的一块网卡", id, net("lo", "eth0"), true},
		{"容器接了两个网络", id, net("lo", "eth0", "eth1"), true},
		{"内核在每个命名空间里建的隧道网卡不看", id, net("lo", "eth0", "tunl0", "sit0", "ip6tnl0"), true},
		{"host 网络：看得到 docker0 与 veth", id, net("lo", "eth0", "docker0", "veth1a2b"), false},
		{"host 网络：物理网卡不叫 eth", id, net("lo", "enp3s0"), false},
		{"主机名不是容器 ID", "nas", net("lo", "eth0"), false},
		{"主机名是大写的十六进制（不是 Docker 给的）", "3F2A9C1B7D4E", net("lo", "eth0"), false},
		{"还不知道主机名", "", net("lo", "eth0"), false},
		{"只有 lo", id, net("lo"), false},
		{"没有网卡", id, nil, false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, glancesBridged(c.hostname, c.list), c.name)
	}
}

func TestGlancesCPUColdStart(t *testing.T) {
	f := newFakeGlances(t, false)
	cold := `{"total":0.0,"idle":0.0,"iowait":0.0,"cpucore":4,"time_since_update":26.1}`
	f.set("cpu", http.StatusOK, cold, glancesReply{status: http.StatusOK, body: `{"total":12.5,"idle":87.5,"iowait":0.5,"cpucore":4}`})
	g := f.source(SourceConfig{}, nil)
	var waited []time.Duration
	g.wait = func(_ context.Context, d time.Duration) error {
		waited = append(waited, d)
		return nil
	}
	snap, err := g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{glancesCPURetry}, waited, "Glances 刚启动时第一次读 CPU 全是 0，等一下再读")
	assert.InDelta(t, 12.5, snap.CPU.Percent, 1e-9)
	assert.Equal(t, []string{"cpu", "cpu", "mem", "load", "network", "sensors", "uptime"}, f.paths())

	// 再读一次还是 0：这一类记作出错，不当成 0%
	f.set("cpu", http.StatusOK, cold)
	snap, err = g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, Facet{Status: FacetError, Reason: "Glances 还没算出 CPU 占用"}, snap.Facets[FacetCPU])
	assert.Zero(t, snap.CPU.Percent)

	// 等的时候采集被取消
	g.wait = func(ctx context.Context, d time.Duration) error { return context.Canceled }
	_, err = g.Collect(context.Background(), false)
	require.ErrorIs(t, err, context.Canceled)

	// 真的空闲（占用 0、空闲 100）不是刚启动：不再读一次，记 0%
	waited = nil
	g.wait = func(_ context.Context, d time.Duration) error {
		waited = append(waited, d)
		return nil
	}
	f.set("cpu", http.StatusOK, `{"total":0.0,"idle":100.0,"iowait":0.0,"cpucore":4}`)
	snap, err = g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Empty(t, waited)
	assert.Equal(t, Facet{Status: FacetOK}, snap.Facets[FacetCPU])
	assert.Zero(t, snap.CPU.Percent)
}

func TestGlancesPluginErrors(t *testing.T) {
	f := newFakeGlances(t, false)
	f.set("sensors", http.StatusBadRequest, `{"detail":"Unknown plugin sensors (available plugins: ['cpu'])"}`)
	f.set("fs", http.StatusNotFound, `{"detail":"Cannot get plugin fs (permission denied)"}`)
	f.set("containers", http.StatusOK, `<html>not json</html>`)
	f.set("mem", http.StatusOK, `{"total":0,"available":0}`)
	f.set("load", http.StatusBadRequest, `{"detail":"Unknown plugin load"}`)
	f.set("uptime", http.StatusOK, `"unknown"`)
	f.set("quicklook", http.StatusBadRequest, `{"detail":"Unknown plugin quicklook"}`)
	f.set("raid", http.StatusOK, `{}`)
	g := f.source(SourceConfig{}, nil)

	snap, err := g.Collect(context.Background(), true)
	require.NoError(t, err, "某个插件读不成时只记在那一类上")
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: "Glances 关着 sensors 插件，读不到温度"}, snap.Facets[FacetCPUTemp])
	assert.Equal(t, snap.Facets[FacetCPUTemp], snap.Facets[FacetDisks])
	assert.Equal(t, FacetError, snap.Facets[FacetPools].Status)
	assert.Equal(t, "Glances 读 fs 回了 HTTP 404：Cannot get plugin fs (permission denied)", snap.Facets[FacetPools].Reason)
	assert.Equal(t, FacetError, snap.Facets[FacetApps].Status)
	assert.Contains(t, snap.Facets[FacetApps].Reason, "回的不是 Glances 的数据")
	assert.Equal(t, Facet{Status: FacetError, Reason: "Glances 没给内存总量"}, snap.Facets[FacetMemory])
	assert.Zero(t, snap.CPU.Load1, "负载读不到时留 0")
	assert.Zero(t, snap.System.UptimeSeconds, "看不懂的运行时长记 0（合并时沿用上一次的）")
	assert.Empty(t, snap.System.Model, "quicklook 关着时只缺 CPU 型号")
	assert.Equal(t, "nas-glances", snap.System.Hostname)
	assert.Equal(t, 32, snap.System.Cores, "quicklook 关着时核数用 cpu 的")
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: glancesNoRaid}, snap.Facets[FacetAlerts])

	f.set("system", http.StatusBadRequest, `{"detail":"Unknown plugin system"}`)
	f.set("containers", http.StatusOK, `[]`)
	f.set("raid", http.StatusInternalServerError, `oops`)
	f.set("network", http.StatusBadRequest, `{"detail":"Unknown plugin network"}`)
	f.set("fs", http.StatusOK, `[{"device_name":"overlay","fs_type":"overlay","mnt_point":"/","size":1,"used":1,"free":0}]`)
	snap, err = g.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: "Glances 关着 system 插件，读不到主机信息"}, snap.Facets[FacetSystem])
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: glancesNoApps}, snap.Facets[FacetApps])
	assert.Equal(t, Facet{Status: FacetError, Reason: "Glances 读 raid 回了 HTTP 500：oops"}, snap.Facets[FacetAlerts])
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: "Glances 关着 network 插件，读不到网卡"}, snap.Facets[FacetNetwork])
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: glancesNoPools}, snap.Facets[FacetPools])

	f.set("cpu", http.StatusBadRequest, `{"detail":"Unknown plugin cpu"}`)
	snap, err = g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, FacetUnsupported, snap.Facets[FacetCPU].Status)
}

func TestGlancesBodyLimit(t *testing.T) {
	f := newFakeGlances(t, false)
	f.set("containers", http.StatusOK, "["+strings.Repeat(" ", glancesMaxBody)+"]")
	g := f.source(SourceConfig{}, nil)
	snap, err := g.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, Facet{Status: FacetError, Reason: "Glances 读 containers 回了 HTTP 200：回应太大"}, snap.Facets[FacetApps])
}

func TestGlancesAuth(t *testing.T) {
	f := newFakeGlances(t, false)
	f.user, f.pass = "glances", "pw-secret"

	// 开了认证、没填账号：认证失败，提示要填
	g := f.source(SourceConfig{}, nil)
	_, err := g.Collect(context.Background(), true)
	var ae *AuthError
	require.ErrorAs(t, err, &ae)
	require.ErrorIs(t, err, ErrAuth)
	assert.Equal(t, "这台 Glances 开了认证，要填用户名（默认 glances）和密码", ae.Reason)
	assert.Equal(t, []string{"cpu"}, f.paths(), "认证失败时不再接着读")

	// 只填了密码：用户名按 glances
	f.resetRequests()
	g = f.source(SourceConfig{Secret: "pw-secret", InsecureAck: true}, nil)
	snap, err := g.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, "nas-glances", snap.System.Hostname)
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("glances:pw-secret"))
	for _, r := range f.requests() {
		assert.Equal(t, want, r.auth, r.path)
	}

	// 用户名改过
	f.user = "admin"
	f.resetRequests()
	g = f.source(SourceConfig{Username: "admin", Secret: "pw-secret", InsecureAck: true}, nil)
	_, err = g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:pw-secret")), f.requests()[0].auth)

	// 密码不对
	g = f.source(SourceConfig{Username: "admin", Secret: "wrong", InsecureAck: true}, nil)
	_, err = g.Collect(context.Background(), false)
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "Glances 说用户名或密码不对", ae.Reason)
	assert.NotContains(t, err.Error(), "wrong")
}

func TestGlancesPlainCredentialsNeedAck(t *testing.T) {
	_, err := newGlances(SourceConfig{URL: "http://192.168.1.10:61208", Secret: "pw"}, nil)
	require.ErrorIs(t, err, ErrPlainNeedsAck)
	_, err = newGlances(SourceConfig{URL: "http://192.168.1.10:61208", Username: "admin"}, nil)
	require.ErrorIs(t, err, ErrPlainNeedsAck)
	_, err = newGlances(SourceConfig{URL: "http://192.168.1.10:61208", Secret: "pw", InsecureAck: true}, nil)
	require.NoError(t, err)
	_, err = newGlances(SourceConfig{URL: "https://192.168.1.10", Secret: "pw"}, nil)
	require.NoError(t, err, "https 不用确认")
	_, err = newGlances(SourceConfig{URL: "http://192.168.1.10:61208"}, nil)
	require.NoError(t, err, "不带账号时 http 直接可用")
	_, err = newGlances(SourceConfig{URL: "://bad"}, nil)
	require.Error(t, err)
}

func TestGlancesNoAPIAndV5(t *testing.T) {
	f := newFakeGlances(t, false)
	f.set("cpu", http.StatusNotFound, `{"detail":"Not Found"}`)
	g := f.source(SourceConfig{}, nil)
	_, err := g.Collect(context.Background(), false)
	require.ErrorIs(t, err, errGlancesNoAPI)

	// 测试连接：/api/4/status 404，再看 /api/5/status
	f.set("status", http.StatusNotFound, `{"detail":"Not Found"}`)
	_, err = testGlances(context.Background(), TestInput{URL: f.url()}, nil)
	require.ErrorIs(t, err, errGlancesNoAPI)
	f.setPath("/api/5/status", glancesReply{status: http.StatusOK, body: `{"version":"5.0.0b1"}`})
	_, err = testGlances(context.Background(), TestInput{URL: f.url()}, nil)
	require.ErrorIs(t, err, errGlancesV5)

	f.set("status", http.StatusInternalServerError, `oops`)
	_, err = testGlances(context.Background(), TestInput{URL: f.url()}, nil)
	require.EqualError(t, err, "Glances 的 /api/4/status 回了 HTTP 500")

	_, err = testGlances(context.Background(), TestInput{}, nil)
	require.EqualError(t, err, "要填地址")
}

func TestGlancesTestConnection(t *testing.T) {
	f := newFakeGlances(t, false)
	f.user, f.pass = "glances", "pw-secret"
	res, err := testGlances(context.Background(), TestInput{URL: f.url(), Secret: "pw-secret"}, nil)
	require.NoError(t, err)
	assert.Equal(t, System{Hostname: "nas-glances", Platform: "Ubuntu 24.04", Version: "7.0.0-1014-aws", Model: "Intel(R) Xeon(R) 6975P-C", Cores: 32}, res.System)
	reqs := f.requests()
	require.Len(t, reqs, 3)
	assert.Equal(t, fakeGlancesReq{path: "/api/4/status"}, reqs[0], "先不带账号认 Glances 4")
	assert.Equal(t, "/api/4/system", reqs[1].path)
	assert.NotEmpty(t, reqs[1].auth)

	// 老版本的 status 也要认证（回 401）：照样接着带账号读
	f.set("status", http.StatusUnauthorized, `{"detail":"Not authenticated"}`)
	_, err = testGlances(context.Background(), TestInput{URL: f.url(), Secret: "pw-secret"}, nil)
	require.NoError(t, err)

	_, err = testGlances(context.Background(), TestInput{URL: f.url(), Secret: "wrong"}, nil)
	require.ErrorIs(t, err, ErrAuth)

	// 系统名写在 os_name 里（不是 Linux 发行版）
	f.set("system", http.StatusOK, `{"hostname":"freenas","os_name":"FreeBSD","os_version":"13.1","linux_distro":""}`)
	res, err = testGlances(context.Background(), TestInput{URL: f.url(), Secret: "pw-secret"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "FreeBSD", res.System.Platform)
}

func TestGlancesTLSPin(t *testing.T) {
	f := newFakeGlances(t, true)
	u := f.url()

	// 系统根证书不认、也没钉住：握手就断，假服务一个请求都没收到
	_, err := testGlances(context.Background(), TestInput{URL: u, Secret: "pw"}, nil)
	var ce *CertError
	require.ErrorAs(t, err, &ce)
	require.ErrorIs(t, err, ErrCertUntrusted)
	assert.Empty(t, f.requests(), "证书不认时不发任何请求（也就不发密码）")
	pin := ce.Cert.SHA256

	res, err := testGlances(context.Background(), TestInput{URL: u, Secret: "pw", PinSHA256: pin}, nil)
	require.NoError(t, err)
	assert.Equal(t, "nas-glances", res.System.Hostname)

	g := f.source(SourceConfig{Secret: "pw", PinSHA256: strings.Repeat("0", 64)}, nil)
	_, err = g.Collect(context.Background(), false)
	require.ErrorIs(t, err, ErrCertChanged)

	g = f.source(SourceConfig{Secret: "pw"}, f.roots())
	_, err = g.Collect(context.Background(), false)
	require.NoError(t, err, "根证书认的时候不用钉住")
}

func TestGlancesRedirectNotFollowed(t *testing.T) {
	var hits int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer other.Close()
	f := newFakeGlances(t, false)
	f.user, f.pass = "glances", "pw-secret"
	f.setPath("/api/4/cpu", glancesReply{status: http.StatusFound, header: map[string]string{"Location": other.URL + "/api/4/cpu"}})
	g := f.source(SourceConfig{Secret: "pw-secret", InsecureAck: true}, nil)
	_, err := g.Collect(context.Background(), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "重定向（HTTP 302")
	assert.Zero(t, hits, "不跟着重定向走（会把密码带到别的主机）")

	assert.Contains(t, err.Error(), "不像 Glances 4 的接口", "转到别的主机的 http 地址不是「改成 https」的问题")

	f.set("status", http.StatusMovedPermanently, ``)
	_, err = testGlances(context.Background(), TestInput{URL: f.url()}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "重定向（HTTP 301")

	// 真机见过的：地址指到了别的网页（登录页），回 302 到 /login
	f.setPath("/api/4/status", glancesReply{status: http.StatusFound, header: map[string]string{"Location": "/login"}})
	_, err = testGlances(context.Background(), TestInput{URL: f.url()}, nil)
	require.Error(t, err)
	assert.Equal(t, "这个地址回了重定向（HTTP 302，到 /login），不像 Glances 4 的接口：地址或端口对吗？Glances 默认端口是 61208", err.Error())

	// 转到 https：多半是反向代理把 http 转成了 https
	f.setPath("/api/4/status", glancesReply{status: http.StatusMovedPermanently, header: map[string]string{"Location": "https://nas.lan/api/4/status"}})
	_, err = testGlances(context.Background(), TestInput{URL: f.url()}, nil)
	require.Error(t, err)
	assert.Equal(t, "Glances 回了重定向（HTTP 301，到 https://nas.lan/api/4/status）：地址是不是要改成 https://", err.Error())
}

func TestGlancesUnreachable(t *testing.T) {
	f := newFakeGlances(t, false)
	g := f.source(SourceConfig{}, nil)
	f.ts.Close()
	_, err := g.Collect(context.Background(), false)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "连不上 Glances："), err.Error())
	assert.NotContains(t, err.Error(), `Get "`, "不带 Go 加的「Get 地址」前缀")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = g.Collect(ctx, false)
	require.ErrorIs(t, err, context.Canceled)
}

func TestParseGlancesUptime(t *testing.T) {
	cases := map[string]int64{
		"9 days, 11:22:59": 9*86400 + 11*3600 + 22*60 + 59,
		"1 day, 0:01:02":   86400 + 62,
		"7:49:45":          7*3600 + 49*60 + 45,
		" 0:00:05 ":        5,
		"a week":           0,
		"":                 0,
		"1:2:3":            0,
	}
	for in, want := range cases {
		assert.Equal(t, want, parseGlancesUptime(in), in)
	}
}

func TestFlexInt(t *testing.T) {
	cases := []struct {
		in string
		n  int
		ok bool
	}{{`3`, 3, true}, {`"4"`, 4, true}, {`2.0`, 2, true}, {`null`, 0, false}, {`""`, 0, false}, {`"x"`, 0, false}, {`-1`, 0, false}}
	for _, c := range cases {
		var f flexInt
		require.NoError(t, f.UnmarshalJSON([]byte(c.in)), c.in)
		assert.Equal(t, c.ok, f.ok, c.in)
		assert.Equal(t, c.n, f.n, c.in)
	}
}

func TestRaidProblem(t *testing.T) {
	n := func(v int) flexInt { return flexInt{n: v, ok: true} }
	assert.Empty(t, raidProblem("md0", glRaid{Status: "active", Type: "raid1", Available: n(2), Used: n(2), Config: "UU"}))
	assert.Equal(t, "RAID 阵列 md0（raid1）降级：有盘掉线（U_）", raidProblem("md0", glRaid{Status: "active", Type: "raid1", Config: "U_"}),
		"没给盘数时看 config 里的 _")
	assert.Equal(t, "RAID 阵列 md9（RAID）没有在运行：inactive", raidProblem("md9", glRaid{Status: "inactive"}))
	assert.Empty(t, raidProblem("md3", glRaid{Status: "active", Type: "raid0", Available: n(2), Used: n(1)}))
}

func TestGlancesHTTPErrorText(t *testing.T) {
	assert.Equal(t, "Glances 读 fs 回了 HTTP 500", (&glancesHTTPError{plugin: "fs", status: 500}).Error())
	assert.Equal(t, strings.Repeat("a", 120)+"…", glancesDetail([]byte(strings.Repeat("a", 200))))
	assert.Equal(t, "boom", glancesDetail([]byte(`{"detail":"boom"}`)))
	assert.False(t, glancesFatal(&glancesHTTPError{}))
	assert.False(t, glancesFatal(errGlancesNotJSON))
	assert.True(t, glancesFatal(errGlancesNoAPI))
	assert.True(t, glancesFatal(errors.New("连不上")))
}

func TestGlancesKindConstructors(t *testing.T) {
	k, ok := LookupKind(KindGlances)
	require.True(t, ok)
	src, err := k.New(SourceConfig{URL: "http://192.168.1.10:61208"})
	require.NoError(t, err)
	require.NoError(t, src.Close())
	_, err = k.New(SourceConfig{URL: "http://192.168.1.10:61208", Secret: "pw"})
	require.ErrorIs(t, err, ErrPlainNeedsAck)
	_, err = k.Test(context.Background(), TestInput{})
	require.EqualError(t, err, "要填地址")
}

// 读到一半时任何一个插件回 401（例如密码刚在 NAS 上改了）：整次采集失败，不拼一份半新半旧的快照。
func TestGlancesAuthFailsMidCollection(t *testing.T) {
	for _, p := range []string{"mem", "load", "network", "sensors", "uptime", "system", "quicklook", "fs", "containers", "raid"} {
		f := newFakeGlances(t, false)
		f.set(p, http.StatusUnauthorized, `{"detail":"Incorrect authentication"}`)
		g := f.source(SourceConfig{}, nil)
		_, err := g.Collect(context.Background(), true)
		require.ErrorIs(t, err, ErrAuth, p)
	}
}

func TestGlancesNumbers(t *testing.T) {
	assert.Zero(t, clampPercent(math.NaN()))
	assert.InDelta(t, 100, clampPercent(130), 1e-9)
	assert.Zero(t, clampPercent(-3))
	assert.Zero(t, toBytes(math.NaN()))
	assert.Zero(t, toBytes(-5))
	assert.Equal(t, uint64(math.MaxUint64), toBytes(1e30))
	assert.Zero(t, glancesRate(nil, nil, nil))
	d, s := 10.0, 0.0
	assert.Zero(t, glancesRate(nil, &d, &s), "间隔是 0 时不除")
}

// 可用比总量还大（数不对）时可用按总量截、已用记 0，不减成一个极大的数
func TestGlancesMemoryAvailableAboveTotal(t *testing.T) {
	f := newFakeGlances(t, false)
	f.set("mem", http.StatusOK, `{"total":1000,"available":1200}`)
	g := f.source(SourceConfig{}, nil)
	snap, err := g.Collect(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, Memory{Total: 1000, Available: 1000}, snap.Memory)
	assert.Equal(t, Facet{Status: FacetOK}, snap.Facets[FacetMemory])
}

func TestGlancesMorePluginBranches(t *testing.T) {
	f := newFakeGlances(t, false)
	f.set("mem", http.StatusBadRequest, `{"detail":"Unknown plugin mem"}`)
	f.set("uptime", http.StatusBadRequest, `{"detail":"Unknown plugin uptime"}`)
	f.set("fs", http.StatusOK, `[
		{"device_name":"","fs_type":"nfs4","mnt_point":"/mnt/share","size":100,"used":40,"free":60},
		{"device_name":"","fs_type":"nfs4","mnt_point":"/mnt/share2","size":100,"used":40,"free":60},
		{"device_name":"/tank","fs_type":"zfs","mnt_point":"/mnt/x","size":10,"used":5,"free":5},
		{"device_name":"/dev/sdd1","fs_type":"ext4","mnt_point":"  ","size":10,"used":5,"free":5},
		{"device_name":"/dev/sde1","fs_type":"xfs","mnt_point":"/srv/odd","size":5,"used":0,"free":0}
	]`)
	g := f.source(SourceConfig{}, nil)
	snap, err := g.Collect(context.Background(), true)
	require.NoError(t, err)
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: "Glances 关着 mem 插件，读不到内存"}, snap.Facets[FacetMemory])
	assert.Zero(t, snap.System.UptimeSeconds)
	assert.Equal(t, []Pool{
		{Name: "/mnt/share", Size: 100, Used: 40, Free: 60},
		{Name: "/mnt/share2", Size: 100, Used: 40, Free: 60},
		{Name: "/srv/odd", Size: 5},
	}, snap.Pools, "没有设备名的按挂载点分开；ZFS 的数据集名不以池名开头时不算")
}
