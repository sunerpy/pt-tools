package internal

import "sync"

// globalRemovalMutex 串行化「自动删除或暂停种子」与「网页给种子加、去保留标签」。
var globalRemovalMutex sync.Mutex

// RemovalMutex 返回删除锁。自动删种、做种竞争度、刷流删种、免费结束时的删除持锁后重新读下载器里的种子，按最新的标签复查保护才执行；
// 网页的「保留 / 取消保留」持锁加、去标签。所以保留先完成时，之后的删除一定看得到标签；删除先完成时，保留会发现种子已经不在。
// 与 PushMutex 不嵌套持有（持着一把时不去拿另一把），锁里只做「重新读 → 复查 → 执行」这一段。
func RemovalMutex() *sync.Mutex { return &globalRemovalMutex }
