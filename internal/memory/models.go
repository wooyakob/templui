package memory

import "time"

type MemoryType string

const (
	MemoryTypeEpisodic   MemoryType = "episodic"
	MemoryTypeSemantic   MemoryType = "semantic"
	MemoryTypeProcedural MemoryType = "procedural"
	MemoryTypeFactual    MemoryType = "factual"
)

func (t MemoryType) Label() string {
	switch t {
	case MemoryTypeEpisodic:
		return "Episodic"
	case MemoryTypeSemantic:
		return "Semantic"
	case MemoryTypeProcedural:
		return "Procedural"
	case MemoryTypeFactual:
		return "Factual"
	default:
		return string(t)
	}
}

func (t MemoryType) BadgeClass() string {
	switch t {
	case MemoryTypeEpisodic:
		return "bg-blue-100 text-blue-700 border-blue-200 dark:bg-blue-900/30 dark:text-blue-400 dark:border-blue-800"
	case MemoryTypeSemantic:
		return "bg-green-100 text-green-700 border-green-200 dark:bg-green-900/30 dark:text-green-400 dark:border-green-800"
	case MemoryTypeProcedural:
		return "bg-orange-100 text-orange-700 border-orange-200 dark:bg-orange-900/30 dark:text-orange-400 dark:border-orange-800"
	case MemoryTypeFactual:
		return "bg-purple-100 text-purple-700 border-purple-200 dark:bg-purple-900/30 dark:text-purple-400 dark:border-purple-800"
	default:
		return "bg-muted text-muted-foreground"
	}
}

type Memory struct {
	ID         string                 `json:"id"`
	DocType    string                 `json:"docType"`
	AgentID    string                 `json:"agentId"`
	AgentName  string                 `json:"agentName"`
	Content    string                 `json:"content"`
	MemoryType MemoryType             `json:"memoryType"`
	Tags       []string               `json:"tags"`
	Importance int                    `json:"importance"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"createdAt"`
	UpdatedAt  time.Time              `json:"updatedAt"`
}

func (m *Memory) Truncate(n int) string {
	if len(m.Content) <= n {
		return m.Content
	}
	return m.Content[:n] + "..."
}

type Agent struct {
	ID          string    `json:"id"`
	DocType     string    `json:"docType"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type MemoryFilter struct {
	AgentID string
	Type    MemoryType
	Query   string
	Limit   int
	Offset  int
}

type Stats struct {
	TotalMemories       int
	TotalAgents         int
	MemoryTypeBreakdown map[MemoryType]int
	RecentCount         int
	StorageBackend      string
}
