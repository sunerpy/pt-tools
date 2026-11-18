package crypto

// 备份恢复用的三个函数（internal/backup）。生产代码只在启动时恢复备份时调用：
// 换上备份里的 secret.key 以后要从磁盘重新读取（ReloadKey）；后面任何一步失败都要回到换以前的样子，
// 而重新读取会把「本进程新生成了密钥」的标记清掉（loadKeyLocked 先把 keyGenerated 置为 false），
// 回滚不能靠「再读一次」，只能把换以前记下的状态原样放回去（SnapshotKeyState、RestoreKeyState）。

// KeyState 是内存里密钥状态的原样快照：加密器、密钥文件路径、是否本进程新生成、读取错误、是否已经读过。
type KeyState struct {
	encryptor *AESGCMEncryptor
	path      string
	generated bool
	loadErr   error
	loaded    bool
}

// SnapshotKeyState 原样记下内存里的密钥状态（还没读过时先读一次，和 KeyStatus 一样）。
func SnapshotKeyState() KeyState {
	ensureKey()
	keyMu.Lock()
	defer keyMu.Unlock()
	return KeyState{encryptor: encryptor, path: keyFilePath, generated: keyGenerated, loadErr: keyLoadErr, loaded: keyLoaded}
}

// RestoreKeyState 把 SnapshotKeyState 记下的状态原样放回去，不读磁盘。
func RestoreKeyState(s KeyState) {
	keyMu.Lock()
	defer keyMu.Unlock()
	encryptor, keyFilePath, keyGenerated, keyLoadErr, keyLoaded = s.encryptor, s.path, s.generated, s.loadErr, s.loaded
}

// ReloadKey 按启动时的规则重新读取密钥（环境变量优先，其次 ~/.pt-tools/secret.key，都没有时生成）。
func ReloadKey() {
	initKey()
}

// DecryptWithKey 用给定的 AES-256 密钥解 Encrypt 产生的密文，不碰进程里的密钥：
// 校验备份时要用备份里的密钥解它的数据库。
func DecryptWithKey(key []byte, cipherStr string) ([]byte, error) {
	return decryptWith(key, cipherStr)
}
