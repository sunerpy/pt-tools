package chatops

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 命令没做成（下载器报错、服务不可用）时多数处理器回一条失败文本、返回 nil error：
// 原来审计按 error 判断，一律记成 success。现在 Reply.Failed 为真的记为 error:command_failed。
func TestProcess_FailedReplyAuditedAsError(t *testing.T) {
	f := newChain(t, CommandSpec{Name: "pause", Handler: func(context.Context, []string, Source) (Reply, error) {
		return Reply{Text: "暂停失败: 下载器不可达", Failed: true}, nil
	}})
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true, PtAdmin: true}

	require.NoError(t, f.chain.Process(context.Background(), mkMsg("/pause abc")))

	assert.Equal(t, "error:command_failed", lastResult(f.audit.snapshot()))
	reply, ok := f.replier.lastReply()
	require.True(t, ok)
	assert.Contains(t, reply.Text, "暂停失败", "失败原因照常回给用户")
}

// 会话步骤的处理器同理。
func TestProcess_FailedSessionReplyAuditedAsError(t *testing.T) {
	f := newChain(t)
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true}
	f.sessions.Set("telegram", 7, "u-999", SessionState{Step: "addrss:url", Handler: func(context.Context, []string, Source) (Reply, error) {
		return Reply{Text: "添加 RSS 订阅失败", Failed: true}, nil
	}}, time.Minute)

	require.NoError(t, f.chain.Process(context.Background(), mkMsg("https://example.com/rss")))

	assert.Equal(t, "error:session_failed", lastResult(f.audit.snapshot()))
}

type failingAudit struct{ err error }

func (a failingAudit) Record(context.Context, AuditEntry) error { return a.err }

// 审计写入失败原来被直接丢掉：远端操作已经做了，却既没有审计记录也没有日志。
func TestProcess_AuditWriteFailureIsLogged(t *testing.T) {
	f := newChain(t, CommandSpec{Name: "status", Handler: func(context.Context, []string, Source) (Reply, error) {
		return Reply{Text: "ok"}, nil
	}})
	f.bindings.exists = true
	f.bindings.binding = BindingInfo{ID: 1, ConfID: 7, Allowed: true}
	f.chain.auditSvc = failingAudit{err: errors.New("database is locked")}
	var mu sync.Mutex
	var logs []string
	f.chain.SetLogf(func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, fmt.Sprintf(format, args...))
	})

	require.NoError(t, f.chain.Process(context.Background(), mkMsg("/status")))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0], "审计写入失败")
	assert.Contains(t, logs[0], "status")
	assert.Contains(t, logs[0], "database is locked")
}
