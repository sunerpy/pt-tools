package nas

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// 飞牛的请求（同 pyfnos 记录的名字：资源监控、系统信息与容器带 appcgi. 前缀，存储不带）。
const (
	fnosReqCPU        = "appcgi.resmon.cpu"
	fnosReqMem        = "appcgi.resmon.mem"
	fnosReqNet        = "appcgi.resmon.net"
	fnosReqGen        = "appcgi.resmon.gen"
	fnosReqDiskMon    = "appcgi.resmon.disk"
	fnosReqHostName   = "appcgi.sysinfo.getHostName"
	fnosReqUptime     = "appcgi.sysinfo.getUptime"
	fnosReqStorage    = "stor.general"
	fnosReqStorState  = "stor.state"
	fnosReqDisks      = "stor.listDisk"
	fnosReqContainers = "appcgi.dockermgr.containerList"
)

// fnosPlatform 是快照里的系统名。
const fnosPlatform = "fnOS"

// fnosNoAlerts 是飞牛读不到的一类：pt-tools 现在不读飞牛的通知。
const fnosNoAlerts = "pt-tools 还没接飞牛的通知；存储空间降级时，存储池那里会写"

// fnosReader 发一次采集里的各个请求：记下成功了几个、第一个被拒绝的是什么（全被拒绝时整次采集失败，连接重来）。
type fnosReader struct {
	c     *fnosConn
	ok    int
	first error
}

// get 发一个签名的请求：成功时 ok 为真；飞牛回了失败或回应看不懂时 ok 为假，f 是这一类数据的情况（没有权限或出错）；
// 连接的问题（断了、超时）是 err，整次采集失败。
func (r *fnosReader) get(ctx context.Context, req string, params map[string]any, out any, what string) (f Facet, ok bool, err error) {
	err = r.c.call(ctx, req, params, out)
	switch {
	case err == nil:
		r.ok++
		return Facet{Status: FacetOK}, true, nil
	case fnosTransport(err):
		return Facet{}, false, err
	}
	if r.first == nil {
		r.first = err
	}
	return fnosFacet(err, what), false, nil
}

// fnosFacet 是一类数据没读成时的情况：4352 是账号不是管理员（存储要管理员），容器要管理员时回 52429312，
// 别的照飞牛的原因写。
func fnosFacet(err error, what string) Facet {
	if n := fnosErrno(err); n == fnosErrNotAdmin || n == fnosErrDockerNotAdmin {
		return Facet{Status: FacetDenied, Reason: "这个账号没有读" + what + "的权限：飞牛只让管理员读，见「添加来源」里飞牛的准备步骤"}
	}
	return Facet{Status: FacetError, Reason: err.Error()}
}

// collectFnOS 读一次：轻量采集读 CPU、内存、网络与运行时长，完整采集再读主机信息、存储空间、硬盘与容器。
func collectFnOS(ctx context.Context, c *fnosConn, full bool) (*Snapshot, error) {
	r := &fnosReader{c: c}
	snap := &Snapshot{
		At:     time.Now(),
		System: System{Platform: fnosPlatform},
		Pools:  []Pool{},
		Disks:  []Disk{},
		Alerts: []Alert{},
		Facets: map[string]Facet{},
	}
	snap.Network.Interfaces = []string{}
	steps := []func(context.Context, *fnosReader, *Snapshot) error{fillFnosCPU, fillFnosMemory, fillFnosNetwork, fillFnosUptime}
	if full {
		steps = append(steps, fillFnosHost, fillFnosStorage, fillFnosApps)
	}
	for _, step := range steps {
		if err := step(ctx, r, snap); err != nil {
			return nil, err
		}
	}
	if r.ok == 0 && r.first != nil {
		return nil, fmt.Errorf("飞牛拒绝了这一次的全部请求，下一次重新登录：%w", r.first)
	}
	if full {
		snap.Facets[FacetAlerts] = Facet{Status: FacetUnsupported, Reason: fnosNoAlerts}
	}
	return snap, nil
}

// fnosTestSystem 是测试连接时读的主机信息：主机名与版本必须读到（4352 是账号不是管理员，不让保存）；CPU 型号、核数与运行时长
// 读不到时只缺这几项。
func fnosTestSystem(ctx context.Context, c *fnosConn) (System, error) {
	sys := System{Platform: fnosPlatform}
	var host fnosHostReply
	if err := c.call(ctx, fnosReqHostName, nil, &host); err != nil {
		if fnosErrno(err) == fnosErrNotAdmin {
			return System{}, &AuthError{Reason: fnosNotAdmin}
		}
		return System{}, err
	}
	host.fill(&sys)
	var cpu fnosCPUReply
	if err := c.call(ctx, fnosReqCPU, nil, &cpu); err == nil {
		cpu.fillSystem(&sys)
	} else if fnosTransport(err) {
		return System{}, err
	}
	var up fnosUptimeReply
	if err := c.call(ctx, fnosReqUptime, nil, &up); err == nil {
		sys.UptimeSeconds = up.seconds()
	} else if fnosTransport(err) {
		return System{}, err
	}
	return sys, nil
}

// fnosHostReply 是 appcgi.sysinfo.getHostName：主机名与飞牛的版本（trimVersion）。
type fnosHostReply struct {
	Data struct {
		HostName    string `json:"hostName"`
		TrimVersion string `json:"trimVersion"`
	} `json:"data"`
}

func (h fnosHostReply) fill(sys *System) {
	sys.Hostname = strings.TrimSpace(h.Data.HostName)
	sys.Version = strings.TrimSpace(h.Data.TrimVersion)
}

// fillFnosHost 读主机名与版本（完整采集）。
func fillFnosHost(ctx context.Context, r *fnosReader, snap *Snapshot) error {
	var reply fnosHostReply
	f, ok, err := r.get(ctx, fnosReqHostName, nil, &reply, "主机信息")
	if err != nil {
		return err
	}
	if ok {
		reply.fill(&snap.System)
	}
	snap.Facets[FacetSystem] = f
	return nil
}

// fnosUptimeReply 是 appcgi.sysinfo.getUptime：运行了多少秒。
type fnosUptimeReply struct {
	Data struct {
		Uptime synoNum `json:"uptime"`
	} `json:"data"`
}

func (u fnosUptimeReply) seconds() int64 {
	v, ok := u.Data.Uptime.float()
	if !ok || v <= 0 || v > math.MaxInt64/2 {
		return 0
	}
	return int64(v)
}

// fillFnosUptime 读运行时长（每次）：读不到时沿用上一次的。
func fillFnosUptime(ctx context.Context, r *fnosReader, snap *Snapshot) error {
	var reply fnosUptimeReply
	_, ok, err := r.get(ctx, fnosReqUptime, nil, &reply, "运行时长")
	if err == nil && ok {
		snap.System.UptimeSeconds = reply.seconds()
	}
	return err
}

// fnosCPUReply 是 appcgi.resmon.cpu（飞牛的资源监控）：型号、核数与线程数、各个温度、占用（百分比）与平均负载。
// 数有时写成字符串，同群晖的回应一样宽松地解（synoNum）。
type fnosCPUReply struct {
	Data struct {
		CPU *struct {
			Name   string    `json:"name"`
			Core   synoNum   `json:"core"`
			Thread synoNum   `json:"thread"`
			Temp   []synoNum `json:"temp"`
			Busy   struct {
				All    synoNum `json:"all"`
				IOWait synoNum `json:"iowait"`
			} `json:"busy"`
			LoadAvg struct {
				Avg1  synoNum `json:"avg1min"`
				Avg5  synoNum `json:"avg5min"`
				Avg15 synoNum `json:"avg15min"`
			} `json:"loadavg"`
		} `json:"cpu"`
	} `json:"data"`
}

// fillSystem：型号写 CPU 型号（飞牛装在各种机器上，没有整机型号），核数写线程数（没有时写核数）。
func (r fnosCPUReply) fillSystem(sys *System) {
	cpu := r.Data.CPU
	if cpu == nil {
		return
	}
	sys.Model = strings.TrimSpace(cpu.Name)
	for _, n := range []synoNum{cpu.Thread, cpu.Core} {
		if v, ok := n.float(); ok && v > 0 && v < math.MaxInt32 {
			sys.Cores = int(v)
			return
		}
	}
}

// fillFnosCPU 读 CPU（每次）：占用、IO 等待、负载与温度（几个温度里最高的那个）。
func fillFnosCPU(ctx context.Context, r *fnosReader, snap *Snapshot) error {
	var reply fnosCPUReply
	f, ok, err := r.get(ctx, fnosReqCPU, nil, &reply, "CPU")
	if err != nil {
		return err
	}
	cpu := reply.Data.CPU
	if ok && cpu == nil {
		f = Facet{Status: FacetError, Reason: "飞牛没给 CPU 的数据"}
	}
	if f.Status != FacetOK {
		snap.Facets[FacetCPU], snap.Facets[FacetCPUTemp] = f, f
		return nil
	}
	reply.fillSystem(&snap.System)
	all, ok := cpu.Busy.All.float()
	if !ok {
		snap.Facets[FacetCPU] = Facet{Status: FacetError, Reason: "飞牛没给 CPU 占用"}
	} else {
		snap.CPU.Percent = clampPercent(all)
		if w, ok := cpu.Busy.IOWait.float(); ok {
			w = clampPercent(w)
			snap.CPU.IOWait = &w
		}
		load := func(n synoNum) float64 {
			v, _ := n.float()
			return math.Max(0, v)
		}
		snap.CPU.Load1, snap.CPU.Load5, snap.CPU.Load15 = load(cpu.LoadAvg.Avg1), load(cpu.LoadAvg.Avg5), load(cpu.LoadAvg.Avg15)
		snap.Facets[FacetCPU] = Facet{Status: FacetOK}
	}
	var hottest *float64
	for _, n := range cpu.Temp {
		if t, ok := n.float(); ok && t > 0 && t < 150 && (hottest == nil || t > *hottest) {
			hottest = &t
		}
	}
	if hottest == nil {
		snap.Facets[FacetCPUTemp] = Facet{Status: FacetUnsupported, Reason: "飞牛没给 CPU 温度（虚拟机里多半读不到）"}
		return nil
	}
	snap.CPU.TempC = hottest
	snap.Facets[FacetCPUTemp] = Facet{Status: FacetOK}
	return nil
}

// fnosMemReply 是 appcgi.resmon.mem：内存（字节）。
type fnosMemReply struct {
	Data struct {
		Mem *struct {
			Total     synoNum `json:"total"`
			Available synoNum `json:"available"`
			Used      synoNum `json:"used"`
		} `json:"mem"`
	} `json:"data"`
}

// fillFnosMemory 读内存（每次）：已用用飞牛的 used，同飞牛网页上的「已使用」；没给 used 时用总量减可用。
// fnOS 1.2.1000 真机上 total 含 reserved（固件与内核占着的那块，4 GB 的虚拟机里约 176 MiB），
// total − available 等于 used + reserved，按它算的占用比飞牛自己显示的高几个百分点。
func fillFnosMemory(ctx context.Context, r *fnosReader, snap *Snapshot) error {
	var reply fnosMemReply
	f, ok, err := r.get(ctx, fnosReqMem, nil, &reply, "内存")
	if err != nil {
		return err
	}
	mem := reply.Data.Mem
	switch {
	case !ok:
	case mem == nil:
		f = Facet{Status: FacetError, Reason: "飞牛没给内存的数据"}
	default:
		f = fillFnosMem(mem.Total, mem.Available, mem.Used, snap)
	}
	snap.Facets[FacetMemory] = f
	return nil
}

func fillFnosMem(totalN, availN, usedN synoNum, snap *Snapshot) Facet {
	total, ok := totalN.uint()
	if !ok || total == 0 {
		return Facet{Status: FacetError, Reason: "飞牛没给内存总量"}
	}
	var used uint64
	if u, ok := usedN.uint(); ok {
		used = min(u, total)
	} else if avail, ok := availN.uint(); ok {
		used = total - min(avail, total)
	} else {
		return Facet{Status: FacetError, Reason: "飞牛没给内存用量"}
	}
	// Available 照 Memory 的约定写成 Total − Used（这样 reserved 也算在可用里，飞牛的网页不显示可用）
	snap.Memory = Memory{Total: total, Used: used, Available: total - used}
	return Facet{Status: FacetOK}
}

// fnosGenReply 是 appcgi.resmon.gen 的网速（飞牛首页的「网络」：各网卡合计的每秒字节数）。
type fnosGenReply struct {
	Data struct {
		Item struct {
			NetSpeed *struct {
				Receive  synoNum `json:"receive"`
				Transmit synoNum `json:"transmit"`
			} `json:"netSpeed"`
		} `json:"item"`
	} `json:"data"`
}

// fnosNetReply 是 appcgi.resmon.net：各块网卡（这里只用名字）。
type fnosNetReply struct {
	Data struct {
		Ifs []struct {
			Name string `json:"name"`
		} `json:"ifs"`
	} `json:"data"`
}

// fillFnosNetwork 读网络（每次）：网速用飞牛自己的合计（聚合口与它的成员不会算两遍），网卡名来自网卡列表
// （读不到时只缺名字）。
func fillFnosNetwork(ctx context.Context, r *fnosReader, snap *Snapshot) error {
	var gen fnosGenReply
	f, ok, err := r.get(ctx, fnosReqGen, map[string]any{"item": []string{"netSpeed"}}, &gen, "网络")
	if err != nil {
		return err
	}
	speed := gen.Data.Item.NetSpeed
	if ok && speed == nil {
		f = Facet{Status: FacetError, Reason: "飞牛没给网速"}
	}
	snap.Facets[FacetNetwork] = f
	if f.Status != FacetOK {
		return nil
	}
	snap.Network.RxBps, _ = speed.Receive.uint()
	snap.Network.TxBps, _ = speed.Transmit.uint()
	var ifs fnosNetReply
	_, ok, err = r.get(ctx, fnosReqNet, nil, &ifs, "网卡")
	if err != nil || !ok {
		return err
	}
	for _, i := range ifs.Data.Ifs {
		if name := strings.TrimSpace(i.Name); name != "" && name != "lo" {
			snap.Network.Interfaces = append(snap.Network.Interfaces, name)
		}
	}
	return nil
}

// fnosStorGeneral 是 stor.general：各个存储空间（飞牛的存储池）。frsize 是空闲、fssize 是总量（字节，同 pyfnos 测试里
// frsizeSys 不大于 fssizeSys）。数据在回应的最外层，不在 data 里。
type fnosStorGeneral struct {
	Array *[]fnosStorage `json:"array"`
}

type fnosStorage struct {
	Name       string  `json:"name"`
	UUID       string  `json:"uuid"`
	Mountpoint string  `json:"mountpoint"`
	FrSize     synoNum `json:"frsize"`
	FsSize     synoNum `json:"fssize"`
}

// label 是存储空间在快照里的名字：挂载点（/vol1），没有时用设备名。
func (s fnosStorage) label() string {
	if m := strings.TrimSpace(s.Mountpoint); m != "" {
		return m
	}
	return strings.TrimSpace(s.Name)
}

// fnosStorState 是 stor.state：各个存储空间底下的 md 阵列与成员盘的状态（mdadm 的 array_state、sync_action 与成员的
// state）。
type fnosStorState struct {
	State []struct {
		Name string   `json:"name"`
		UUID string   `json:"uuid"`
		MD   []fnosMD `json:"md"`
	} `json:"state"`
}

type fnosMD struct {
	RaidDisks  synoNum `json:"raidDisks"`
	ArrayState string  `json:"arrayState"`
	SyncAction string  `json:"syncAction"`
	Disk       []struct {
		Name     string `json:"name"`
		ArrState string `json:"arrState"`
	} `json:"disk"`
}

// fnosHealth 是一个阵列的状态：Healthy 为空是不知道。
type fnosHealth struct {
	Status  string
	Healthy *bool
}

func fnosHealthOf(status string, healthy *bool) fnosHealth {
	return fnosHealth{Status: status, Healthy: healthy}
}

// worse 报告 h 是不是比 o 更差：不健康最差，其次是不知道，健康最好。
func (h fnosHealth) worse(o fnosHealth) bool {
	rank := func(x fnosHealth) int {
		switch {
		case x.Healthy == nil:
			return 1
		case !*x.Healthy:
			return 2
		}
		return 0
	}
	return rank(h) > rank(o)
}

// mdHealth 按 mdadm 的状态判断一个阵列：成员有 faulty、同步的成员比阵列要的盘少是降级（备用盘不算）；阵列 broken 是已损毁、inactive 是没在
// 运行；在 recover（重建）是不健康；resync、reshape 时不说健不健康；check、repair（校验）照常是健康；clean、active 而成员都
// 同步是正常。别的状态照原样写、不说健不健康。
func mdHealth(md fnosMD) fnosHealth {
	no, yes := false, true
	state := strings.ToLower(strings.TrimSpace(md.ArrayState))
	sync := strings.ToLower(strings.TrimSpace(md.SyncAction))
	inSync, faulty, spare := 0, 0, 0
	for _, d := range md.Disk {
		switch st := strings.ToLower(d.ArrState); {
		case strings.Contains(st, "faulty"):
			faulty++
		case strings.Contains(st, "in_sync"):
			inSync++
		case strings.Contains(st, "spare"):
			spare++
		}
	}
	// 阵列要几块盘：raidDisks；没给时是除了备用盘以外的成员数
	want := len(md.Disk) - spare
	if n, ok := md.RaidDisks.float(); ok && n > 0 && n < math.MaxInt32 {
		want = int(n)
	}
	switch {
	case state == "broken":
		return fnosHealthOf("已损毁", &no)
	case state == "inactive":
		return fnosHealthOf("没在运行", &no)
	case faulty > 0 || (len(md.Disk) > 0 && inSync < want):
		if sync == "recover" {
			return fnosHealthOf("重建中", &no)
		}
		return fnosHealthOf("降级", &no)
	case sync == "recover":
		return fnosHealthOf("重建中", &no)
	case sync == "resync":
		return fnosHealthOf("同步中", nil)
	case sync == "reshape":
		return fnosHealthOf("调整中", nil)
	}
	switch state {
	case "clean", "active", "active-idle", "write-pending", "read-auto":
		if sync == "check" || sync == "repair" {
			return fnosHealthOf("校验中", &yes)
		}
		return fnosHealthOf("正常", &yes)
	}
	return fnosHealthOf(md.ArrayState, nil)
}

// fnosPartRe 是分区的名字：成员盘写的可能是分区（sdb3、nvme0n1p2），算硬盘时去掉分区号。
var fnosPartRe = regexp.MustCompile(`^(nvme\d+n\d+)p\d+$|^((?:sd|vd|hd|xvd)[a-z]+)\d+$`)

func fnosDiskOf(member string) string {
	name := strings.TrimPrefix(strings.TrimSpace(member), "/dev/")
	if m := fnosPartRe.FindStringSubmatch(name); m != nil {
		return m[1] + m[2]
	}
	return name
}

// fnosStorageView 是读到的存储：各个存储空间与它们的健康、每块硬盘在哪个存储空间里。
type fnosStorageView struct {
	pools  []Pool
	diskOf map[string]string
}

func fnosPools(list []fnosStorage, st *fnosStorState) fnosStorageView {
	view := fnosStorageView{pools: make([]Pool, 0, len(list)), diskOf: map[string]string{}}
	for _, s := range list {
		size, _ := s.FsSize.uint()
		free, _ := s.FrSize.uint()
		free = min(free, size)
		p := Pool{Name: s.label(), Size: size, Free: free, Used: size - free}
		if st != nil {
			for _, x := range st.State {
				if (s.UUID != "" && x.UUID == s.UUID) || (s.UUID == "" && x.Name != "" && x.Name == s.Name) {
					p.Status, p.Healthy, p.Disks = view.apply(p.Name, x.MD)
					break
				}
			}
		}
		view.pools = append(view.pools, p)
	}
	return view
}

// apply 合并一个存储空间底下的各个阵列：状态取最差的那个；硬盘数是成员盘去重以后的块数，并记下每块硬盘在哪个存储空间里。
func (v *fnosStorageView) apply(pool string, mds []fnosMD) (string, *bool, int) {
	var worst *fnosHealth
	seen := map[string]bool{}
	for _, md := range mds {
		h := mdHealth(md)
		if worst == nil || h.worse(*worst) {
			worst = &h
		}
		for _, d := range md.Disk {
			disk := fnosDiskOf(d.Name)
			if disk == "" || seen[disk] {
				continue
			}
			seen[disk] = true
			if _, ok := v.diskOf[disk]; !ok {
				v.diskOf[disk] = pool
			}
		}
	}
	if worst == nil {
		return "", nil, 0
	}
	return worst.Status, worst.Healthy, len(seen)
}

// fnosDiskList 是 stor.listDisk：各块硬盘（大小是字节；回应里的序列号不读也不存）。数据在最外层。
type fnosDiskList struct {
	Disk *[]struct {
		Name      string  `json:"name"`
		Size      synoNum `json:"size"`
		ModelName string  `json:"modelName"`
		Type      string  `json:"type"`
		Protocol  string  `json:"protocol"`
	} `json:"disk"`
}

// fnosDiskMon 是 appcgi.resmon.disk：各块硬盘的温度（0 是读不到，NVMe 常这样）。
type fnosDiskMon struct {
	Data struct {
		Disk []struct {
			Name string  `json:"name"`
			Temp synoNum `json:"temp"`
		} `json:"disk"`
	} `json:"data"`
}

// fnosDiskKind 是硬盘的类型：接口或类型里有 NVMe、名字以 nvme 开头的是 NVMe；类型是 SSD、HDD 的照写；都没有时不知道。
func fnosDiskKind(name, typ, protocol string) string {
	t, p := strings.ToLower(typ), strings.ToLower(protocol)
	switch {
	case strings.Contains(t, "nvme") || strings.Contains(p, "nvme") || strings.HasPrefix(strings.ToLower(name), "nvme"):
		return DiskNVMe
	case strings.Contains(t, "ssd"):
		return DiskSSD
	case strings.Contains(t, "hdd"):
		return DiskHDD
	}
	return DiskUnknown
}

// fillFnosStorage 读存储（完整采集）：存储空间与健康、硬盘与温度。存储的接口要管理员：账号不是管理员时这两类记 denied。
// 健康或温度读不到时只缺这一项。
func fillFnosStorage(ctx context.Context, r *fnosReader, snap *Snapshot) error {
	var gen fnosStorGeneral
	f, ok, err := r.get(ctx, fnosReqStorage, nil, &gen, "存储空间")
	if err != nil {
		return err
	}
	if ok && gen.Array == nil {
		f = Facet{Status: FacetError, Reason: "飞牛没给存储空间的列表"}
	}
	view := fnosStorageView{diskOf: map[string]string{}}
	if f.Status == FacetOK {
		list := *gen.Array
		var st *fnosStorState
		if len(list) > 0 {
			names, uuids := make([]string, 0, len(list)), make([]string, 0, len(list))
			for _, s := range list {
				names, uuids = append(names, s.Name), append(uuids, s.UUID)
			}
			var state fnosStorState
			_, ok, err = r.get(ctx, fnosReqStorState, map[string]any{"name": names, "uuid": uuids}, &state, "存储空间的状态")
			if err != nil {
				return err
			}
			if ok {
				st = &state
			}
		}
		view = fnosPools(list, st)
		snap.Pools = view.pools
	}
	snap.Facets[FacetPools] = f
	return fillFnosDisks(ctx, r, snap, view.diskOf)
}

func fillFnosDisks(ctx context.Context, r *fnosReader, snap *Snapshot, diskOf map[string]string) error {
	var list fnosDiskList
	f, ok, err := r.get(ctx, fnosReqDisks, map[string]any{"noHotSpare": true}, &list, "硬盘")
	if err != nil {
		return err
	}
	if ok && list.Disk == nil {
		f = Facet{Status: FacetError, Reason: "飞牛没给硬盘的列表"}
	}
	snap.Facets[FacetDisks] = f
	if f.Status != FacetOK {
		return nil
	}
	temps := map[string]float64{}
	var mon fnosDiskMon
	_, ok, err = r.get(ctx, fnosReqDiskMon, nil, &mon, "硬盘温度")
	if err != nil {
		return err
	}
	if ok {
		for _, d := range mon.Data.Disk {
			if t, ok := d.Temp.float(); ok && t > 0 && t < 150 {
				temps[d.Name] = t
			}
		}
	}
	for _, d := range *list.Disk {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		disk := Disk{Name: name, Model: strings.TrimSpace(d.ModelName), Kind: fnosDiskKind(name, d.Type, d.Protocol), Pool: diskOf[name]}
		disk.Size, _ = d.Size.uint()
		if t, ok := temps[name]; ok {
			disk.TempC = &t
		}
		snap.Disks = append(snap.Disks, disk)
	}
	return nil
}

// fnosContainers 是 appcgi.dockermgr.containerList：Docker 的容器（数据在最外层的 rsp 里）。飞牛的容器列表不说哪些能更新。
// fnOS 1.2.1000 真机上没有容器时（Docker 没启动也一样）回 succ、不带 rsp，飞牛的网页写「暂无容器」。
type fnosContainers struct {
	Rsp *[]struct {
		State string `json:"State"`
	} `json:"rsp"`
}

// fillFnosApps 读容器（完整采集），当成应用：运行中、共几个。
func fillFnosApps(ctx context.Context, r *fnosReader, snap *Snapshot) error {
	var reply fnosContainers
	f, ok, err := r.get(ctx, fnosReqContainers, map[string]any{"all": true}, &reply, "容器")
	if err != nil {
		return err
	}
	snap.Facets[FacetApps] = f
	if !ok {
		return nil
	}
	snap.Apps = Apps{UpdatesUnknown: true}
	if reply.Rsp == nil {
		return nil
	}
	snap.Apps.Total = len(*reply.Rsp)
	for _, c := range *reply.Rsp {
		if strings.EqualFold(c.State, "running") {
			snap.Apps.Running++
		}
	}
	return nil
}
