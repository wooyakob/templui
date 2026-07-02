package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// InMemoryStore is an in-memory implementation of the Store interface.
// It stores agents and memories in slices and maps for fast access.
type InMemoryStore struct {
	mu       sync.RWMutex
	agents   map[string]*Agent
	memories map[string]*Memory
	// ordered slices for deterministic iteration
	agentOrder  []string
	memoryOrder []string
}

// NewInMemoryStore creates a new InMemoryStore seeded with sample data.
func NewInMemoryStore() *InMemoryStore {
	s := &InMemoryStore{
		agents:   make(map[string]*Agent),
		memories: make(map[string]*Memory),
	}
	s.seed()
	return s
}

func (s *InMemoryStore) seed() {
	now := time.Now()

	agents := []*Agent{
		{
			ID:          "agent-alpha",
			DocType:     "agent",
			Name:        "Alpha",
			Description: "General-purpose research and summarization agent.",
			CreatedAt:   now.Add(-72 * time.Hour),
		},
		{
			ID:          "agent-beta",
			DocType:     "agent",
			Name:        "Beta",
			Description: "Code generation and debugging specialist.",
			CreatedAt:   now.Add(-48 * time.Hour),
		},
		{
			ID:          "agent-gamma",
			DocType:     "agent",
			Name:        "Gamma",
			Description: "Customer support and FAQ automation agent.",
			CreatedAt:   now.Add(-24 * time.Hour),
		},
	}

	memories := []*Memory{
		{
			ID:         "mem-001",
			DocType:    "memory",
			AgentID:    "agent-alpha",
			AgentName:  "Alpha",
			Content:    "The user prefers concise bullet-point summaries over long paragraphs when asking for research results.",
			MemoryType: MemoryTypeSemantic,
			Tags:       []string{"preferences", "output-format"},
			Importance: 4,
			CreatedAt:  now.Add(-70 * time.Hour),
			UpdatedAt:  now.Add(-70 * time.Hour),
		},
		{
			ID:         "mem-002",
			DocType:    "memory",
			AgentID:    "agent-alpha",
			AgentName:  "Alpha",
			Content:    "User requested a deep-dive on Couchbase vector search capabilities on 2024-03-15. Provided a 5-page summary with benchmark comparisons.",
			MemoryType: MemoryTypeEpisodic,
			Tags:       []string{"couchbase", "vector-search", "research"},
			Importance: 3,
			CreatedAt:  now.Add(-68 * time.Hour),
			UpdatedAt:  now.Add(-68 * time.Hour),
		},
		{
			ID:         "mem-003",
			DocType:    "memory",
			AgentID:    "agent-beta",
			AgentName:  "Beta",
			Content:    "To generate idiomatic Go code: always use table-driven tests, prefer interfaces over concrete types, and run `gofmt` before finalizing output.",
			MemoryType: MemoryTypeProcedural,
			Tags:       []string{"golang", "best-practices", "testing"},
			Importance: 5,
			CreatedAt:  now.Add(-46 * time.Hour),
			UpdatedAt:  now.Add(-46 * time.Hour),
		},
		{
			ID:         "mem-004",
			DocType:    "memory",
			AgentID:    "agent-beta",
			AgentName:  "Beta",
			Content:    "The project uses Go 1.24 with the templ v0.3 templating engine and Tailwind CSS 4.x for styling.",
			MemoryType: MemoryTypeFactual,
			Tags:       []string{"project", "tech-stack"},
			Importance: 5,
			CreatedAt:  now.Add(-44 * time.Hour),
			UpdatedAt:  now.Add(-44 * time.Hour),
		},
		{
			ID:         "mem-005",
			DocType:    "memory",
			AgentID:    "agent-gamma",
			AgentName:  "Gamma",
			Content:    "Escalate to a human agent if the customer mentions billing issues, account suspension, or uses escalation keywords three times in a row.",
			MemoryType: MemoryTypeProcedural,
			Tags:       []string{"escalation", "billing", "support"},
			Importance: 5,
			CreatedAt:  now.Add(-22 * time.Hour),
			UpdatedAt:  now.Add(-22 * time.Hour),
		},
		{
			ID:         "mem-006",
			DocType:    "memory",
			AgentID:    "agent-gamma",
			AgentName:  "Gamma",
			Content:    "Customer Jake asked about the enterprise pricing plan. Provided the current tier breakdown and forwarded a quote request to sales.",
			MemoryType: MemoryTypeEpisodic,
			Tags:       []string{"pricing", "enterprise", "sales"},
			Importance: 2,
			CreatedAt:  now.Add(-20 * time.Hour),
			UpdatedAt:  now.Add(-20 * time.Hour),
		},
		{
			ID:         "mem-007",
			DocType:    "memory",
			AgentID:    "agent-alpha",
			AgentName:  "Alpha",
			Content:    "Couchbase Capella supports multi-model data: key-value, document, SQL++, full-text search, eventing, and vector search in a single platform.",
			MemoryType: MemoryTypeFactual,
			Tags:       []string{"couchbase", "capella", "features"},
			Importance: 4,
			CreatedAt:  now.Add(-2 * time.Hour),
			UpdatedAt:  now.Add(-2 * time.Hour),
		},
		{
			ID:         "mem-008",
			DocType:    "memory",
			AgentID:    "agent-beta",
			AgentName:  "Beta",
			Content:    "When debugging HTMX partial swaps: check that the hx-target selector exists in the DOM before the request fires, and verify the server returns only the fragment, not a full HTML page.",
			MemoryType: MemoryTypeProcedural,
			Tags:       []string{"htmx", "debugging", "frontend"},
			Importance: 3,
			CreatedAt:  now.Add(-30 * time.Minute),
			UpdatedAt:  now.Add(-30 * time.Minute),
		},
	}

	for _, a := range agents {
		s.agents[a.ID] = a
		s.agentOrder = append(s.agentOrder, a.ID)
	}

	for _, m := range memories {
		s.memories[m.ID] = m
		s.memoryOrder = append(s.memoryOrder, m.ID)
	}

	// Sort memories by CreatedAt DESC
	s.sortMemories()
}

// deepCopyMemory returns a copy of m with independent Tags slice and Metadata map.
func deepCopyMemory(m *Memory) *Memory {
	cp := *m
	if m.Tags != nil {
		cp.Tags = make([]string, len(m.Tags))
		copy(cp.Tags, m.Tags)
	}
	if m.Metadata != nil {
		cp.Metadata = make(map[string]interface{}, len(m.Metadata))
		for k, v := range m.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}

// sortMemories re-orders memoryOrder by CreatedAt DESC. Must be called with lock held.
func (s *InMemoryStore) sortMemories() {
	sort.Slice(s.memoryOrder, func(i, j int) bool {
		a := s.memories[s.memoryOrder[i]]
		b := s.memories[s.memoryOrder[j]]
		return a.CreatedAt.After(b.CreatedAt)
	})
}

// CreateMemory persists a new memory.
func (s *InMemoryStore) CreateMemory(_ context.Context, m *Memory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m.ID == "" {
		m.ID = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}
	m.DocType = "memory"
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	s.memories[m.ID] = deepCopyMemory(m)
	s.memoryOrder = append(s.memoryOrder, m.ID)
	s.sortMemories()
	return nil
}

// GetMemory retrieves a single memory by ID.
func (s *InMemoryStore) GetMemory(_ context.Context, id string) (*Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.memories[id]
	if !ok {
		return nil, nil
	}
	return deepCopyMemory(m), nil
}

// ListMemories returns filtered, paginated memories sorted by CreatedAt DESC.
func (s *InMemoryStore) ListMemories(_ context.Context, filter MemoryFilter) ([]*Memory, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []*Memory
	for _, id := range s.memoryOrder {
		m := s.memories[id]
		if filter.AgentID != "" && m.AgentID != filter.AgentID {
			continue
		}
		if filter.Type != "" && m.MemoryType != filter.Type {
			continue
		}
		if filter.Query != "" {
			q := strings.ToLower(filter.Query)
			if !strings.Contains(strings.ToLower(m.Content), q) &&
				!strings.Contains(strings.ToLower(m.AgentName), q) {
				continue
			}
		}
		matched = append(matched, deepCopyMemory(m))
	}

	total := len(matched)

	// Pagination
	if filter.Offset > 0 {
		if filter.Offset >= len(matched) {
			return []*Memory{}, total, nil
		}
		matched = matched[filter.Offset:]
	}
	if filter.Limit > 0 && len(matched) > filter.Limit {
		matched = matched[:filter.Limit]
	}

	return matched, total, nil
}

// UpdateMemory replaces an existing memory.
func (s *InMemoryStore) UpdateMemory(_ context.Context, m *Memory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.memories[m.ID]; !ok {
		return fmt.Errorf("memory %q not found", m.ID)
	}
	m.UpdatedAt = time.Now()
	s.memories[m.ID] = deepCopyMemory(m)
	return nil
}

// DeleteMemory removes a memory by ID.
func (s *InMemoryStore) DeleteMemory(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.memories[id]; !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	delete(s.memories, id)
	for i, oid := range s.memoryOrder {
		if oid == id {
			s.memoryOrder = append(s.memoryOrder[:i], s.memoryOrder[i+1:]...)
			break
		}
	}
	return nil
}

// CreateAgent persists a new agent.
func (s *InMemoryStore) CreateAgent(_ context.Context, a *Agent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if a.ID == "" {
		a.ID = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	a.DocType = "agent"
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}

	cp := *a
	s.agents[cp.ID] = &cp
	s.agentOrder = append(s.agentOrder, cp.ID)
	return nil
}

// GetAgent retrieves a single agent by ID.
func (s *InMemoryStore) GetAgent(_ context.Context, id string) (*Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.agents[id]
	if !ok {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

// ListAgents returns all agents in creation order.
func (s *InMemoryStore) ListAgents(_ context.Context) ([]*Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Agent, 0, len(s.agentOrder))
	for _, id := range s.agentOrder {
		a := s.agents[id]
		cp := *a
		result = append(result, &cp)
	}
	return result, nil
}

// DeleteAgent removes an agent by ID.
func (s *InMemoryStore) DeleteAgent(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.agents[id]; !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	delete(s.agents, id)
	for i, oid := range s.agentOrder {
		if oid == id {
			s.agentOrder = append(s.agentOrder[:i], s.agentOrder[i+1:]...)
			break
		}
	}
	return nil
}

// GetStats computes aggregate statistics across all stored data.
func (s *InMemoryStore) GetStats(_ context.Context) (*Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	breakdown := make(map[MemoryType]int)
	recentCutoff := time.Now().Add(-24 * time.Hour)
	recentCount := 0

	for _, m := range s.memories {
		breakdown[m.MemoryType]++
		if m.CreatedAt.After(recentCutoff) {
			recentCount++
		}
	}

	return &Stats{
		TotalMemories:       len(s.memories),
		TotalAgents:         len(s.agents),
		MemoryTypeBreakdown: breakdown,
		RecentCount:         recentCount,
		StorageBackend:      "inmemory",
	}, nil
}

// GetMemoryCounts returns a map of agentID to memory count in a single pass.
func (s *InMemoryStore) GetMemoryCounts(_ context.Context) (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[string]int)
	for _, m := range s.memories {
		counts[m.AgentID]++
	}
	return counts, nil
}

// Close is a no-op for the in-memory store.
func (s *InMemoryStore) Close() error {
	return nil
}
