package memory

import "context"

type Store interface {
	CreateMemory(ctx context.Context, m *Memory) error
	GetMemory(ctx context.Context, id string) (*Memory, error)
	ListMemories(ctx context.Context, filter MemoryFilter) ([]*Memory, int, error)
	UpdateMemory(ctx context.Context, m *Memory) error
	DeleteMemory(ctx context.Context, id string) error

	CreateAgent(ctx context.Context, a *Agent) error
	GetAgent(ctx context.Context, id string) (*Agent, error)
	ListAgents(ctx context.Context) ([]*Agent, error)
	DeleteAgent(ctx context.Context, id string) error

	GetStats(ctx context.Context) (*Stats, error)
	GetMemoryCounts(ctx context.Context) (map[string]int, error)

	Close() error
}
