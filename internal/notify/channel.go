package notify

import (
	"context"

	"github.com/sunerpy/pt-tools/models"
)

/*
 * 连通态取值。
 *
 * 为什么不能直接拿 Healthy() 当「已连接」：这四个适配器里 Healthy() 的含义都只是
 * **构造/启动成功**，不是对端真的接上了 ——
 *   · QQ：监听端口一绑上就 healthy=true，而 NapCat 还没握手时发送会明确失败
 *     （「QQ 通道未连接 (NapCat 尚未握手)」）；适配器自己的测试注释也写了这一点；
 *   · Telegram：bot 造出来就 healthy=true，没做任何网络确认；
 *   · Webhook：只判 config != nil；
 *   · WeCom：恒为 true（它只是个出站 URL，没有长连接可言）。
 * 所以把 Healthy() 映射成「已连接」会在界面上说一件不存在的事。
 */
const (
	// LinkConnected 表示对端确实接上了（只有能判断这件事的适配器才会返回它）。
	LinkConnected = "connected"
	// LinkWaiting 表示实例在跑、但对端还没接上（QQ 绑好端口等 NapCat 握手就是这个态）。
	LinkWaiting = "waiting"
)

// LinkStater 由**能判断对端是否真的接上**的适配器实现，是可选能力。
// 不实现它的适配器，上层只会说「运行中」，不会说「已连接」。
type LinkStater interface {
	LinkState() string
}

type Channel interface {
	Type() string
	Init(ctx context.Context, conf *models.NotificationConf) error
	SupportsInbound() bool
	Send(ctx context.Context, n Notification) error
	OnInbound(handler InboundHandler)
	Close(ctx context.Context) error
	Healthy() bool
}
