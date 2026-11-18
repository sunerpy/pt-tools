package assistant

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestDeviceBrushConfirmationWithoutConfigScope(t *testing.T) {
	e := newEnv(t)
	actor := Actor{Owner: "device:9", Name: "Android", Entry: models.AssistantEntryApp, CanWrite: true}
	e.llm.push(call("call_brush", "change_brush_task", `{"op":"update","id":9,"values":{"enabled":false}}`), reply{text: "已暂停。"})
	conv := e.conversation(actor)
	res := e.send(actor, conv, "暂停刷流任务 9")
	events, _ := e.until(actor, conv, 0, pendingConfirmation)
	confirmation := *events[len(events)-1].Confirmation
	assert.Equal(t, "change_brush_task", confirmation.Tool)
	assert.Equal(t, "暂停刷流任务 9", confirmation.Summary)
	assert.Empty(t, e.backend.writes(), "confirmation has not been approved")
	_, err := e.svc.Decide(context.Background(), actor, confirmation.ID, true)
	require.NoError(t, err)
	fin, _ := e.waitRun(actor, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)
	require.Len(t, e.backend.writes(), 1)
	assert.Equal(t, "/brush/tasks/9", e.backend.writes()[0].Path)
	_, err = e.svc.Decide(context.Background(), actor, confirmation.ID, true)
	assert.Error(t, err)
	assert.Len(t, e.backend.writes(), 1, "repeated approval cannot repeat the mutation")
}
