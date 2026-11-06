package nas

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"
)

// KindTrueNAS 是 TrueNAS 来源：经 wss://主机/api/current 的 JSON-RPC 2.0（TrueNAS 25.04 起），用只读管理员账号的 API Key
// 登录，每条连接登录一次。证书不认时在握手阶段就断开、不发 Key；只收 https://（经 http 用过的 Key 会被 TrueNAS 吊销）。
const KindTrueNAS = "truenas"

func init() {
	RegisterKind(Kind{
		Name:     KindTrueNAS,
		Label:    "TrueNAS",
		Username: UsernameNone,
		Plain:    PlainNever,
		New: func(cfg SourceConfig) (Source, error) {
			t, err := newTrueNAS(cfg, nil)
			if err != nil {
				return nil, err
			}
			return t, nil
		},
		Test: func(ctx context.Context, in TestInput) (*TestResult, error) { return testTrueNAS(ctx, in, nil) },
	})
}

const (
	// trueNASWindow 是轻量采集取报表的时长（秒）：和轻量采集的间隔一样长，CPU 与网速是这段时间的平均
	trueNASWindow = 30
	// trueNASLag 是报表最新的数据比 TrueNAS 的时钟晚几秒
	trueNASLag = 5
	// trueNASHistory 是第一次采集补上的历史有多长（秒），trueNASStep 是每拍的间隔（同轻量采集）
	trueNASHistory = HistoryLen * trueNASStep
	trueNASStep    = 30
	// kbitToBytes 把报表里网卡的千比特每秒换成字节每秒
	kbitToBytes = 125
)

var (
	errTrueNASHTTP   = errors.New("TrueNAS 只能用 https:// 连接（API Key 经 http 用过会被 TrueNAS 吊销）")
	errTrueNASNoKey  = errors.New("要填 TrueNAS 的 API Key")
	errTrueNASReauth = errors.New("TrueNAS 说这条连接没登录（登录失效了），下次采集重新登录")
)

// trueNAS 是一个 TrueNAS 来源：持有一条登录过的连接，断了或出错以后下次采集重连。
type trueNAS struct {
	url   *url.URL
	pin   string
	key   string
	roots *x509.CertPool
	now   func() time.Time

	mu   sync.Mutex
	conn *rpcConn
	// ifaces 是上一次完整采集读到的、连着的物理网卡；netFacet 不为空时是读网卡列表没读成的原因（轻量采集沿用）
	ifaces   []string
	netFacet *Facet
	// history 是补上的过去几拍（还没交给采集器的）；backfilled 为真是已经补过了（每个来源只补一次）
	history    []Sample
	backfilled bool
}

func newTrueNAS(cfg SourceConfig, roots *x509.CertPool) (*trueNAS, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("地址不对：%w", err)
	}
	if u.Scheme != "https" {
		return nil, errTrueNASHTTP
	}
	if strings.TrimSpace(cfg.Secret) == "" {
		return nil, errTrueNASNoKey
	}
	return &trueNAS{url: u, pin: cfg.PinSHA256, key: cfg.Secret, roots: roots, now: time.Now}, nil
}

// testTrueNAS 是「测试连接」：连上、登录、读 system.info，然后断开。
func testTrueNAS(ctx context.Context, in TestInput, roots *x509.CertPool) (*TestResult, error) {
	if in.URL == nil {
		return nil, errors.New("要填地址")
	}
	if strings.TrimSpace(in.Secret) == "" {
		return nil, errTrueNASNoKey
	}
	c, err := dialRPC(ctx, in.URL, in.PinSHA256, roots, in.Secret)
	if err != nil {
		return nil, err
	}
	defer c.close()
	if err := c.login(ctx, in.Secret); err != nil {
		return nil, err
	}
	var info tnSystemInfo
	if err := c.call(ctx, "system.info", nil, &info); err != nil {
		return nil, systemInfoError(err)
	}
	return &TestResult{System: info.system()}, nil
}

// Collect 采集一次：没有连接（或者断了）时先连上、登录。出错时断开连接，下次重连。
func (t *trueNAS) Collect(ctx context.Context, full bool) (*Snapshot, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil && t.conn.broken() {
		t.conn.close()
		t.conn = nil
	}
	if t.conn == nil {
		c, err := dialRPC(ctx, t.url, t.pin, t.roots, t.key)
		if err != nil {
			return nil, err
		}
		if err := c.login(ctx, t.key); err != nil {
			c.close()
			return nil, err
		}
		t.conn = c
	}
	snap, err := t.collect(ctx, t.conn, full)
	if err != nil {
		t.conn.close()
		t.conn = nil
		return nil, err
	}
	return snap, nil
}

// Close 断开连接。
func (t *trueNAS) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		t.conn.close()
		t.conn = nil
	}
	return nil
}

// TakeHistory 交出第一次采集补上的过去 30 分钟（每 30 秒一拍），之后是 nil。
func (t *trueNAS) TakeHistory() []Sample {
	t.mu.Lock()
	defer t.mu.Unlock()
	h := t.history
	t.history = nil
	return h
}

// collect 分两轮发请求：第一轮是 system.info（报表的时间要按 TrueNAS 的时钟算）与完整采集的各项，第二轮是报表与数据集。
func (t *trueNAS) collect(ctx context.Context, c *rpcConn, full bool) (*Snapshot, error) {
	var (
		info   tnSystemInfo
		ifaces []tnInterface
		pools  []tnPool
		disks  []tnDisk
		temps  map[string]*float64
		alerts []tnAlert
		apps   []tnApp
	)
	sys := &rpcCall{method: "system.info", out: &info}
	var ifc, pc, dc, tc, ac, apc *rpcCall
	if full {
		ifc = &rpcCall{method: "interface.query", out: &ifaces}
		pc = &rpcCall{method: "pool.query", out: &pools}
		dc = &rpcCall{method: "disk.query", params: []any{[]any{}, map[string]any{"extra": map[string]any{"pools": true}}}, out: &disks}
		tc = &rpcCall{method: "disk.temperatures", out: &temps}
		ac = &rpcCall{method: "alert.list", out: &alerts}
		apc = &rpcCall{method: "app.query", out: &apps}
	}
	first := []*rpcCall{sys, ifc, pc, dc, tc, ac, apc}
	c.batch(ctx, first...)
	local := t.now()
	if err := firstTransportError(first...); err != nil {
		return nil, err
	}
	if sys.err != nil {
		return nil, systemInfoError(sys.err)
	}
	server := local
	if info.Datetime.Date > 0 {
		server = time.UnixMilli(info.Datetime.Date)
	}
	if full {
		t.setInterfaces(ifaces, ifc.err)
	}

	end := server.Unix() - trueNASLag
	window := map[string]any{"aggregate": true, "start": end - trueNASWindow, "end": end}
	var cpuMem, cpuTemp, net, histCPU, histNet []tnGraph
	var datasets []tnDataset
	cm := &rpcCall{method: "reporting.get_data", params: []any{graphs("cpu", "memory"), window}, out: &cpuMem}
	tp := &rpcCall{method: "reporting.get_data", params: []any{graphs("cputemp"), window}, out: &cpuTemp}
	var nc, dsc, hc, hn *rpcCall
	if len(t.ifaces) > 0 {
		nc = &rpcCall{method: "reporting.get_data", params: []any{ifaceGraphs(t.ifaces), window}, out: &net}
	}
	if full && pc.err == nil && len(pools) > 0 {
		names := make([]any, 0, len(pools))
		for _, p := range pools {
			names = append(names, p.Name)
		}
		filter := []any{[]any{"id", "in", names}}
		opts := map[string]any{"extra": map[string]any{"properties": []string{"used", "available"}, "retrieve_children": false}}
		dsc = &rpcCall{method: "pool.dataset.query", params: []any{filter, opts}, out: &datasets}
	}
	if !t.backfilled {
		span := map[string]any{"aggregate": true, "start": end - trueNASHistory, "end": end}
		hc = &rpcCall{method: "reporting.get_data", params: []any{graphs("cpu", "memory"), span}, out: &histCPU}
		if len(t.ifaces) > 0 {
			hn = &rpcCall{method: "reporting.get_data", params: []any{ifaceGraphs(t.ifaces), span}, out: &histNet}
		}
	}
	second := []*rpcCall{cm, tp, nc, dsc, hc, hn}
	c.batch(ctx, second...)
	if err := firstTransportError(second...); err != nil {
		return nil, err
	}

	snap := &Snapshot{System: info.system(), Facets: map[string]Facet{FacetSystem: {Status: FacetOK}}}
	if len(info.Loadavg) >= 3 {
		snap.CPU.Load1, snap.CPU.Load5, snap.CPU.Load15 = info.Loadavg[0], info.Loadavg[1], info.Loadavg[2]
	}
	snap.Memory.Total = info.Physmem
	t.fillCPUMemory(snap, cpuMem, cm.err)
	fillCPUTemp(snap, cpuTemp, tp.err)
	t.fillNetwork(snap, net, nc)
	if full {
		fillPools(snap, pools, pc.err, datasets, dsc, disks, dc.err)
		fillDisks(snap, disks, dc.err, temps, tc.err)
		fillAlerts(snap, alerts, ac.err)
		fillApps(snap, apps, apc.err)
	}
	if !t.backfilled {
		if hc.err == nil && (hn == nil || hn.err == nil) {
			t.history = historySamples(histCPU, histNet, info.Physmem, end, local.Sub(server))
		}
		t.backfilled = true
	}
	return snap, nil
}

// setInterfaces 记下连着的物理网卡（不算网桥、VLAN 与聚合口，免得同一份流量算两遍）。
func (t *trueNAS) setInterfaces(list []tnInterface, err error) {
	t.ifaces, t.netFacet = nil, nil
	if err != nil {
		f := facetFor(err, "网卡")
		t.netFacet = &f
		return
	}
	for _, i := range list {
		if i.Type == "PHYSICAL" && i.State.LinkState == "LINK_STATE_UP" && i.Name != "" {
			t.ifaces = append(t.ifaces, i.Name)
		}
	}
	if len(t.ifaces) == 0 {
		t.netFacet = &Facet{Status: FacetUnsupported, Reason: "TrueNAS 上没有连着的物理网卡"}
	}
}

// systemInfoError 是读不到 system.info 时的错误：账号没有只读权限停住（改权限以后点「重新连接」），登录失效了重连。
func systemInfoError(err error) error {
	var re *rpcError
	if errors.As(err, &re) {
		switch {
		case re.denied():
			return &AuthError{Reason: "这个 API Key 的账号没有只读管理员（READONLY_ADMIN）权限，读不到主机信息"}
		case re.unauthenticated():
			return errTrueNASReauth
		}
	}
	return err
}

// facetFor 是一类数据没读成时的情况：账号没权限、这个版本没有这个方法，或者这一次出错了。
func facetFor(err error, what string) Facet {
	var re *rpcError
	if errors.As(err, &re) {
		switch {
		case re.denied():
			return Facet{Status: FacetDenied, Reason: "这个 API Key 的账号没有读" + what + "的权限"}
		case re.missing():
			return Facet{Status: FacetUnsupported, Reason: "这个版本的 TrueNAS 读不到" + what}
		}
	}
	return Facet{Status: FacetError, Reason: err.Error()}
}

func (t *trueNAS) fillCPUMemory(snap *Snapshot, list []tnGraph, err error) {
	if err != nil {
		snap.Facets[FacetCPU] = facetFor(err, "报表")
		snap.Facets[FacetMemory] = facetFor(err, "报表")
		return
	}
	cpu, mem := findGraph(list, "cpu"), findGraph(list, "memory")
	if v, ok := cpu.mean("cpu"); ok {
		snap.CPU.Percent = math.Min(100, math.Max(0, v))
		snap.Facets[FacetCPU] = Facet{Status: FacetOK}
	} else {
		snap.Facets[FacetCPU] = Facet{Status: FacetError, Reason: "TrueNAS 的报表里没有最近 30 秒的 CPU 数据"}
	}
	if v, ok := mem.last("available"); ok && snap.Memory.Total > 0 {
		avail := uint64(math.Max(0, v))
		avail = min(avail, snap.Memory.Total)
		snap.Memory.Available, snap.Memory.Used = avail, snap.Memory.Total-avail
		snap.Facets[FacetMemory] = Facet{Status: FacetOK}
	} else {
		snap.Facets[FacetMemory] = Facet{Status: FacetError, Reason: "TrueNAS 的报表里没有最近 30 秒的内存数据"}
	}
}

// fillCPUTemp 用报表里 cputemp 的 cpu 一列（没有时取各核里最高的）；都是 0 是读不到温度（虚拟机里常见）。
func fillCPUTemp(snap *Snapshot, list []tnGraph, err error) {
	if err != nil {
		snap.Facets[FacetCPUTemp] = facetFor(err, "CPU 温度")
		return
	}
	g := findGraph(list, "cputemp")
	temp, ok := g.mean("cpu")
	if !ok || temp <= 0 {
		temp = 0
		for _, col := range g.Legend {
			if col == "time" || col == "cpu" {
				continue
			}
			if v, ok := g.mean(col); ok && v > temp {
				temp = v
			}
		}
	}
	if temp <= 0 {
		snap.Facets[FacetCPUTemp] = Facet{Status: FacetUnsupported, Reason: "TrueNAS 读不到 CPU 温度（虚拟机里常见）"}
		return
	}
	snap.CPU.TempC = &temp
	snap.Facets[FacetCPUTemp] = Facet{Status: FacetOK}
}

func (t *trueNAS) fillNetwork(snap *Snapshot, list []tnGraph, nc *rpcCall) {
	switch {
	case t.netFacet != nil:
		snap.Facets[FacetNetwork] = *t.netFacet
		return
	case nc == nil:
		snap.Facets[FacetNetwork] = Facet{Status: FacetUnsupported, Reason: "TrueNAS 上没有连着的物理网卡"}
		return
	case nc.err != nil:
		snap.Facets[FacetNetwork] = facetFor(nc.err, "报表")
		return
	}
	var rx, tx float64
	for _, g := range list {
		if g.Name != "interface" {
			continue
		}
		r, okR := g.mean("received")
		s, okS := g.mean("sent")
		if !okR && !okS {
			continue
		}
		rx += math.Max(0, r)
		tx += math.Max(0, s)
		if g.Identifier != nil {
			snap.Network.Interfaces = append(snap.Network.Interfaces, *g.Identifier)
		}
	}
	if len(snap.Network.Interfaces) == 0 {
		snap.Facets[FacetNetwork] = Facet{Status: FacetError, Reason: "TrueNAS 的报表里没有最近 30 秒的网卡数据"}
		return
	}
	snap.Network.RxBps, snap.Network.TxBps = uint64(rx*kbitToBytes), uint64(tx*kbitToBytes)
	snap.Facets[FacetNetwork] = Facet{Status: FacetOK}
}

// fillPools 的容量按存储池的根数据集算（used + available，扣掉了校验盘）；读不到根数据集的池按 pool.query 的裸容量。
func fillPools(snap *Snapshot, pools []tnPool, err error, datasets []tnDataset, dsc *rpcCall, disks []tnDisk, diskErr error) {
	if err != nil {
		snap.Facets[FacetPools] = facetFor(err, "存储池")
		return
	}
	roots := map[string]tnDataset{}
	if dsc != nil && dsc.err == nil {
		for _, d := range datasets {
			roots[d.ID] = d
		}
	}
	members := map[string]int{}
	if diskErr == nil {
		for _, d := range disks {
			if d.Pool != "" {
				members[d.Pool]++
			}
		}
	}
	snap.Pools = make([]Pool, 0, len(pools))
	for _, p := range pools {
		out := Pool{Name: p.Name, Status: p.Status, Size: p.Size, Used: p.Allocated, Free: p.Free, Disks: members[p.Name]}
		if p.Healthy != nil {
			h := *p.Healthy
			out.Healthy = &h
		}
		if d, ok := roots[p.Name]; ok && d.Used.Parsed != nil && d.Available.Parsed != nil {
			out.Used, out.Free = *d.Used.Parsed, *d.Available.Parsed
			out.Size = out.Used + out.Free
		}
		snap.Pools = append(snap.Pools, out)
	}
	snap.Facets[FacetPools] = Facet{Status: FacetOK}
}

// fillDisks 列出硬盘：类型按名字（nvme 开头）与 TrueNAS 给的 HDD、SSD 分；温度是 0 或读不到时为空。序列号不要。
func fillDisks(snap *Snapshot, disks []tnDisk, err error, temps map[string]*float64, tempErr error) {
	if err != nil {
		snap.Facets[FacetDisks] = facetFor(err, "硬盘")
		return
	}
	snap.Disks = make([]Disk, 0, len(disks))
	for _, d := range disks {
		out := Disk{Name: d.Name, Model: d.Model, Kind: diskKind(d.Name, d.Type), Size: d.Size, Pool: d.Pool}
		if tempErr == nil {
			if v, ok := temps[d.Name]; ok && v != nil && *v > 0 {
				c := *v
				out.TempC = &c
			}
		}
		snap.Disks = append(snap.Disks, out)
	}
	snap.Facets[FacetDisks] = Facet{Status: FacetOK}
}

func diskKind(name, typ string) string {
	switch {
	case strings.HasPrefix(name, "nvme"):
		return DiskNVMe
	case typ == "HDD":
		return DiskHDD
	case typ == "SSD":
		return DiskSSD
	default:
		return DiskUnknown
	}
}

// fillAlerts 列出没被忽略的告警：级别按 TrueNAS 的 CRITICAL 等分成三级，标题见 trueNASAlertTitle。
func fillAlerts(snap *Snapshot, alerts []tnAlert, err error) {
	if err != nil {
		snap.Facets[FacetAlerts] = facetFor(err, "告警")
		return
	}
	snap.Alerts = make([]Alert, 0, len(alerts))
	for _, a := range alerts {
		if a.Dismissed {
			continue
		}
		out := Alert{Level: alertLevel(a.Level), Title: trueNASAlertTitle(a)}
		if a.Datetime.Date > 0 {
			at := time.UnixMilli(a.Datetime.Date)
			out.At = &at
		}
		snap.Alerts = append(snap.Alerts, out)
	}
	snap.Facets[FacetAlerts] = Facet{Status: FacetOK}
}

func alertLevel(level string) string {
	switch level {
	case "CRITICAL", "ALERT", "EMERGENCY", "ERROR":
		return AlertCritical
	case "WARNING":
		return AlertWarning
	default:
		return AlertInfo
	}
}

// fillApps 数应用：运行中的、共几个、有更新的（应用或镜像有新版本都算）。
func fillApps(snap *Snapshot, apps []tnApp, err error) {
	if err != nil {
		snap.Facets[FacetApps] = facetFor(err, "应用")
		return
	}
	snap.Apps.Total = len(apps)
	for _, a := range apps {
		if a.State == "RUNNING" {
			snap.Apps.Running++
		}
		if a.UpgradeAvailable || a.ImageUpdatesAvailable {
			snap.Apps.Upgradable++
		}
	}
	snap.Facets[FacetApps] = Facet{Status: FacetOK}
}

// historySamples 把 30 分钟的报表按 30 秒一拍合成历史：CPU、可用内存与各网卡的网速取每拍里的平均（几块网卡的平均加起来），
// 时间换成本机的时钟（offset = 本机 - TrueNAS）。一拍里没有 CPU 或内存数据的不要。
func historySamples(cpuMem, net []tnGraph, physmem uint64, end int64, offset time.Duration) []Sample {
	if physmem == 0 {
		return nil
	}
	n := trueNASHistory / trueNASStep
	start := end - trueNASHistory
	cpu := bucketMeans(findGraph(cpuMem, "cpu"), "cpu", start, n)
	mem := bucketMeans(findGraph(cpuMem, "memory"), "available", start, n)
	rx, tx := make([]float64, n), make([]float64, n)
	for _, g := range net {
		if g.Name != "interface" {
			continue
		}
		r, s := bucketMeans(g, "received", start, n), bucketMeans(g, "sent", start, n)
		for k := range n {
			if !math.IsNaN(r[k]) {
				rx[k] += math.Max(0, r[k])
			}
			if !math.IsNaN(s[k]) {
				tx[k] += math.Max(0, s[k])
			}
		}
	}
	var out []Sample
	for k := range n {
		if math.IsNaN(cpu[k]) || math.IsNaN(mem[k]) {
			continue
		}
		avail := math.Min(float64(physmem), math.Max(0, mem[k]))
		out = append(out, Sample{
			At:  time.Unix(start+int64(k+1)*trueNASStep, 0).Add(offset),
			CPU: math.Min(100, math.Max(0, cpu[k])),
			Mem: (float64(physmem) - avail) * 100 / float64(physmem),
			Rx:  uint64(rx[k] * kbitToBytes),
			Tx:  uint64(tx[k] * kbitToBytes),
		})
	}
	return out
}

// bucketMeans 是一列在每一拍（从 start 起每 trueNASStep 秒）里的平均；没有数据的拍是 NaN。
func bucketMeans(g tnGraph, col string, start int64, n int) []float64 {
	out := make([]float64, n)
	for k := range out {
		out[k] = math.NaN()
	}
	i := g.col(col)
	if i < 0 {
		return out
	}
	sum, cnt := make([]float64, n), make([]int, n)
	for _, row := range g.Data {
		if len(row) <= i || row[0] == nil || row[i] == nil {
			continue
		}
		k := (int64(*row[0]) - start) / trueNASStep
		if *row[0] < float64(start) || k >= int64(n) {
			continue
		}
		sum[k] += *row[i]
		cnt[k]++
	}
	for k := range out {
		if cnt[k] > 0 {
			out[k] = sum[k] / float64(cnt[k])
		}
	}
	return out
}

func graphs(names ...string) []any {
	out := make([]any, 0, len(names))
	for _, n := range names {
		out = append(out, map[string]any{"name": n})
	}
	return out
}

func ifaceGraphs(ifaces []string) []any {
	out := make([]any, 0, len(ifaces))
	for _, n := range ifaces {
		out = append(out, map[string]any{"name": "interface", "identifier": n})
	}
	return out
}
