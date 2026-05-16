package chatops

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Button struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type CommandSpec struct {
	Name        string
	Description string
	Aliases     []string
	AdminOnly   bool
	Handler     CommandHandler
	RateLimit   *RateLimitSpec
}

type CommandHandler func(ctx context.Context, args []string, src Source) (Reply, error)

type Source struct {
	ChannelType   string
	ChannelConfID uint
	ChannelUserID string
	Username      string
	ChatID        string
	ReplyLang     string
	PtAdmin       bool
	IsAdmin       bool
	BindingID     uint
}

type CommandSource = Source

type Reply struct {
	Text       string
	Buttons    [][]Button
	SilentDrop bool
}

type CommandReply = Reply

type CommandRegistry struct {
	mu      sync.RWMutex
	specs   map[string]CommandSpec
	aliases map[string]string
}

func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		specs:   make(map[string]CommandSpec),
		aliases: make(map[string]string),
	}
}

func (r *CommandRegistry) Register(spec CommandSpec) {
	name := normalizeCommandName(spec.Name)
	if name == "" {
		panic("chatops: command name is empty")
	}
	spec.Name = name
	spec.Aliases = normalizeAliases(spec.Aliases)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.specs[name]; ok {
		panic(fmt.Sprintf("chatops: command %q already registered", name))
	}
	if owner, ok := r.aliases[name]; ok {
		panic(fmt.Sprintf("chatops: command %q conflicts with alias of %q", name, owner))
	}
	for _, alias := range spec.Aliases {
		if _, ok := r.specs[alias]; ok {
			panic(fmt.Sprintf("chatops: alias %q conflicts with command", alias))
		}
		if owner, ok := r.aliases[alias]; ok {
			panic(fmt.Sprintf("chatops: alias %q already registered for %q", alias, owner))
		}
	}

	r.specs[name] = cloneCommandSpec(spec)
	for _, alias := range spec.Aliases {
		r.aliases[alias] = name
	}
}

func (r *CommandRegistry) Get(name string) (CommandSpec, bool) {
	key := normalizeCommandName(name)
	if key == "" {
		return CommandSpec{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if spec, ok := r.specs[key]; ok {
		return cloneCommandSpec(spec), true
	}
	canonical, ok := r.aliases[key]
	if !ok {
		return CommandSpec{}, false
	}
	spec, ok := r.specs[canonical]
	if !ok {
		return CommandSpec{}, false
	}
	return cloneCommandSpec(spec), true
}

func (r *CommandRegistry) List() []CommandSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]CommandSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		result = append(result, cloneCommandSpec(spec))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

var defaultRegistry = NewCommandRegistry()

func RegisterCommand(spec CommandSpec) {
	defaultRegistry.Register(spec)
}

func DefaultRegistry() *CommandRegistry {
	return defaultRegistry
}

func normalizeCommandName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "/")
	return strings.ToLower(name)
}

func normalizeAliases(aliases []string) []string {
	seen := make(map[string]struct{}, len(aliases))
	result := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		alias = normalizeCommandName(alias)
		if alias == "" {
			continue
		}
		if _, ok := seen[alias]; ok {
			panic(fmt.Sprintf("chatops: duplicate alias %q", alias))
		}
		seen[alias] = struct{}{}
		result = append(result, alias)
	}
	return result
}

func cloneCommandSpec(spec CommandSpec) CommandSpec {
	if spec.Aliases != nil {
		spec.Aliases = append([]string(nil), spec.Aliases...)
	}
	return spec
}
