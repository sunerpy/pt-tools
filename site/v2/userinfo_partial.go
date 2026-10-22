package v2

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"slices"
	"strings"
	"time"
)

// 取用户信息的容错：一页没取到（Cloudflare 质询、连接被重置、超时、HTTP 出错）不让整次失败——用户名到手了就回
// 取到的部分（PartialUserInfoError 写明缺哪些字段），保存时缺的字段沿用上一次的（UserInfoService.FetchAndSave）。
// 一时的网络错误与质询先在请求这一层重试（fetchUserInfoPage）。

// PartialUserInfoError 是取用户信息只取到一部分：用户名到手了，有的页面没取到。和它一起返回的 UserInfo 是取到的部分；
// Missing 是没取到的字段（站点定义里的字段名），Err 是第一个失败的原因。
type PartialUserInfoError struct {
	Missing []string
	Err     error
}

func (e *PartialUserInfoError) Error() string {
	return fmt.Sprintf("部分用户信息没取到（%s）: %v", strings.Join(e.Missing, ", "), e.Err)
}

func (e *PartialUserInfoError) Unwrap() error { return e.Err }

// IgnorePartial 把「只取到一部分」的错误换成 nil：用户名到手了，登录与站点状态照样能判断。其余错误原样返回。
func IgnorePartial(info UserInfo, err error) error {
	var partial *PartialUserInfoError
	if errors.As(err, &partial) && strings.TrimSpace(info.Username) != "" {
		return nil
	}
	return err
}

// coreUserInfoFields 是统计离不开的字段：没取到又没有上一次的可沿用时，不保存这次的结果（免得统计里冒出 0）。
var coreUserInfoFields = []string{"uploaded", "downloaded"}

// keepFreshFields 是不沿用上一次的字段：最近访问时间按这次访问算（登录探测据此记最近动向）。
var keepFreshFields = []string{"lastAccessAt", "lastAccess"}

// copyUserInfoField 把 src 里站点定义字段名对应的值抄到 dst（和 setUserInfoField 的字段名一样）。
func copyUserInfoField(dst *UserInfo, src UserInfo, field string) {
	switch field {
	case "id":
		dst.UserID = src.UserID
	case "name", "username":
		dst.Username = src.Username
	case "uploaded":
		dst.Uploaded = src.Uploaded
	case "downloaded":
		dst.Downloaded = src.Downloaded
	case "ratio":
		dst.Ratio = src.Ratio
	case "bonus":
		dst.Bonus = src.Bonus
	case "levelName", "rank", "class":
		dst.LevelName, dst.Rank = src.LevelName, src.Rank
	case "seedingBonus":
		dst.SeedingBonus = src.SeedingBonus
	case "bonusPerHour":
		dst.BonusPerHour = src.BonusPerHour
	case "seedingBonusPerHour":
		dst.SeedingBonusPerHour = src.SeedingBonusPerHour
	case "joinTime", "joinDate":
		dst.JoinDate = src.JoinDate
	case "lastLoginAt", "lastLogin":
		dst.LastLogin = src.LastLogin
	case "messageCount", "unreadMessageCount":
		dst.UnreadMessageCount = src.UnreadMessageCount
	case "hnrUnsatisfied":
		dst.HnRUnsatisfied = src.HnRUnsatisfied
	case "hnrPreWarning":
		dst.HnRPreWarning = src.HnRPreWarning
	case "seeding", "seederCount":
		dst.Seeding, dst.SeederCount = src.Seeding, src.SeederCount
	case "leeching", "leecherCount":
		dst.Leeching, dst.LeecherCount = src.Leeching, src.LeecherCount
	case "uploads":
		dst.Uploads = src.Uploads
	case "trueUploaded":
		dst.TrueUploaded = src.TrueUploaded
	case "trueDownloaded":
		dst.TrueDownloaded = src.TrueDownloaded
	case "seederSize", "seedingSize":
		dst.SeederSize = src.SeederSize
	case "leecherSize":
		dst.LeecherSize = src.LeecherSize
	}
}

// mergeMissingUserInfo 用上一次的数据补上这次没取到的字段（最近访问时间除外）；补了哪些字段一并返回。
// 上传量、下载量都补过时分享率也按上一次的。
func mergeMissingUserInfo(info, prev UserInfo, missing []string) (UserInfo, []string) {
	var kept []string
	for _, f := range missing {
		if slices.Contains(keepFreshFields, f) {
			continue
		}
		copyUserInfoField(&info, prev, f)
		kept = append(kept, f)
	}
	if slices.Contains(missing, "uploaded") && slices.Contains(missing, "downloaded") && !slices.Contains(missing, "ratio") && info.Ratio == 0 {
		info.Ratio = prev.Ratio
	}
	return info, kept
}

// missesCore 报告缺的字段里有没有统计离不开的。
func missesCore(missing []string) bool {
	for _, f := range coreUserInfoFields {
		if slices.Contains(missing, f) {
			return true
		}
	}
	return false
}

// FetchFailureReason 是取用户信息失败的原因分类（日志里用）：Cloudflare 质询、登录失效、限流、超时、网络错误等。
func FetchFailureReason(err error) string {
	var partial *PartialUserInfoError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &partial):
		return "部分页面没取到"
	case errors.Is(err, ErrCloudflareChallenge):
		return "Cloudflare 质询"
	case errors.Is(err, ErrSessionExpired), errors.Is(err, ErrInvalidCredentials):
		return "登录失效"
	case errors.Is(err, Err2FARequired):
		return "要二次验证"
	case errors.Is(err, ErrRateLimited):
		return "被限流"
	case errors.Is(err, context.DeadlineExceeded):
		return "超时"
	case errors.Is(err, ErrNetworkError):
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "connection reset"):
			return "网络错误（连接被重置）"
		case strings.Contains(msg, "no such host"):
			return "网络错误（域名解析不了）"
		case strings.Contains(msg, "connection refused"):
			return "网络错误（连接被拒绝）"
		case strings.Contains(msg, "timeout"):
			return "超时"
		}
		return "网络错误"
	case errors.Is(err, ErrEmptyUsername):
		return "没取到用户名"
	}
	var status *HTTPStatusError
	if errors.As(err, &status) {
		return fmt.Sprintf("HTTP %d", status.Code)
	}
	return "其他错误"
}

// isTransientNetError 报告是不是一时的网络错误（连接被重置、断开、超时）：值得再试。域名解析不了、证书不对重试也没用。
func isTransientNetError(err error) bool {
	if !errors.Is(err, ErrNetworkError) {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false
	}
	var certErr *tls.CertificateVerificationError
	return !errors.As(err, &certErr)
}

// 取用户信息时每一页的重试（测试里调小）。
var (
	// userInfoAttemptTimeout 是一次请求最多等多久，和站点 HTTP 客户端自己的上限（30 秒）一样：线路慢时一页就要二三十秒
	// （2026-10-22 从生产的 truenas 请求 SSD、HDSky 的登录页要 6 到 25 秒），等得太短会把回得慢的页面掐断再重试、
	// 越试越慢；一页卡住时也留出重试的时间
	userInfoAttemptTimeout = 30 * time.Second
	// userInfoNetRetries 是一时的网络错误再试几次；userInfoRetryBase 是第一次重试前等多久（之后加倍，另加随机的一点）
	userInfoNetRetries = 2
	userInfoRetryBase  = time.Second
	// userInfoChallengeWait 是遇到 Cloudflare 质询以后等多久再试一次（质询多是瞬时的风控）
	userInfoChallengeWait = 3 * time.Second
	// userInfoMinAttempt 是重试前剩下的时间至少要有多少，不够就不试了（线路慢时几秒钟取不完一页）
	userInfoMinAttempt = 10 * time.Second
)

// jitter 在 d 上加 0 到 d/2 的随机时长，几个请求不会同时重试。
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return d + time.Duration(rand.Int64N(int64(d/2)+1))
}

// sleepContext 等 d；ctx 先结束时回 false。
func sleepContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// retryWait 是第 n 次失败以后要不要再试、等多久：一时的网络错误与站点出错（5xx）最多再试 userInfoNetRetries 次，
// 质询再试一次。
func retryWait(err error, netTries, challengeTries int) (time.Duration, bool) {
	var status *HTTPStatusError
	switch {
	case errors.Is(err, ErrCloudflareChallenge):
		return jitter(userInfoChallengeWait), challengeTries < 1
	case isTransientNetError(err), errors.As(err, &status) && status.Code >= 500:
		return jitter(userInfoRetryBase << netTries), netTries < userInfoNetRetries
	}
	return 0, false
}

// withAttemptTimeout 给一次请求加上时限（不超过 ctx 自己的）。
func withAttemptTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if userInfoAttemptTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, userInfoAttemptTimeout)
}
