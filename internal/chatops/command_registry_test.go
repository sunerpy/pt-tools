package chatops

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Register_Get(t *testing.T) {
	r := NewCommandRegistry()
	handler := func(ctx context.Context, args []string, src Source) (Reply, error) {
		return Reply{Text: "ok"}, nil
	}

	r.Register(CommandSpec{Name: "status", Description: "show status", Handler: handler})

	spec, ok := r.Get("status")
	require.True(t, ok)
	assert.Equal(t, "status", spec.Name)
	assert.Equal(t, "show status", spec.Description)
	assert.Len(t, r.List(), 1)
}

func TestRegistry_AliasResolves(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "status", Aliases: []string{"s", "/STATUS"}})

	spec, ok := r.Get("/s")
	require.True(t, ok)
	assert.Equal(t, "status", spec.Name)

	spec, ok = r.Get("STATUS")
	require.True(t, ok)
	assert.Equal(t, "status", spec.Name)
}

func TestRegistry_DuplicateRegister_Panic(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "status", Aliases: []string{"st"}})

	assert.Panics(t, func() {
		r.Register(CommandSpec{Name: "/status"})
	})
	assert.Panics(t, func() {
		r.Register(CommandSpec{Name: "tasks", Aliases: []string{"st"}})
	})
}

func TestRegistry_ConcurrentRace(t *testing.T) {
	r := NewCommandRegistry()
	for i := range 32 {
		r.Register(CommandSpec{Name: fmt.Sprintf("cmd_%d", i), Aliases: []string{fmt.Sprintf("c%d", i)}})
	}

	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for range 100 {
				spec, ok := r.Get(fmt.Sprintf("/c%d", i))
				require.True(t, ok)
				assert.Equal(t, fmt.Sprintf("cmd_%d", i), spec.Name)
				_ = r.List()
			}
		}(i)
	}
	wg.Wait()
}
