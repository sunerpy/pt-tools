// MIT License
// Copyright (c) 2025 pt-tools

package chatops

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandRegistry_RegisterAndGetWithAliases(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{
		Name:    "status",
		Aliases: []string{"st", "stat"},
		Handler: func(context.Context, []string, Source) (Reply, error) { return Reply{}, nil },
	})

	spec, ok := r.Get("status")
	require.True(t, ok)
	assert.Equal(t, "status", spec.Name)

	spec, ok = r.Get("st")
	require.True(t, ok)
	assert.Equal(t, "status", spec.Name, "alias must resolve to canonical command")

	spec, ok = r.Get("/STAT")
	require.True(t, ok)
	assert.Equal(t, "status", spec.Name, "normalization strips slash and lowercases")

	_, ok = r.Get("")
	assert.False(t, ok)

	_, ok = r.Get("nonexistent")
	assert.False(t, ok)
}

func TestCommandRegistry_Register_EmptyNamePanics(t *testing.T) {
	r := NewCommandRegistry()
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "   "}) })
}

func TestCommandRegistry_Register_DuplicatePanics(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "dup"})
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "dup"}) })
}

func TestCommandRegistry_Register_NameConflictsWithAlias(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "a", Aliases: []string{"b"}})
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "b"}) },
		"registering a command whose name equals an existing alias must panic")
}

func TestCommandRegistry_Register_AliasConflictsWithCommand(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "a"})
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "c", Aliases: []string{"a"}}) })
}

func TestCommandRegistry_Register_AliasConflictsWithAlias(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(CommandSpec{Name: "a", Aliases: []string{"x"}})
	assert.Panics(t, func() { r.Register(CommandSpec{Name: "b", Aliases: []string{"x"}}) })
}

func TestNormalizeAliases_DuplicatePanics(t *testing.T) {
	assert.Panics(t, func() {
		_ = normalizeAliases([]string{"dup", "/DUP"})
	}, "duplicate aliases (after normalization) must panic")
}

func TestNormalizeAliases_SkipsEmpty(t *testing.T) {
	got := normalizeAliases([]string{"", "  ", "keep"})
	assert.Equal(t, []string{"keep"}, got)
}

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
