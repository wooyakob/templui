package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/couchbase/gocb/v2"
)

// CouchbaseStore is a Couchbase-backed implementation of the Store interface.
type CouchbaseStore struct {
	cluster    *gocb.Cluster
	bucket     *gocb.Bucket
	collection *gocb.Collection
	bucketName string
}

// NewCouchbaseStore connects to a Couchbase cluster and returns a ready-to-use store.
// connStr is the connection string (e.g. "couchbase://localhost"), username and password
// are the cluster credentials, and bucketName is the bucket to store documents in.
func NewCouchbaseStore(connStr, username, password, bucketName string) (*CouchbaseStore, error) {
	cluster, err := gocb.Connect(connStr, gocb.ClusterOptions{
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("couchbase connect: %w", err)
	}

	if err := cluster.WaitUntilReady(10*time.Second, nil); err != nil {
		return nil, fmt.Errorf("couchbase wait until ready: %w", err)
	}

	bucket := cluster.Bucket(bucketName)
	if err := bucket.WaitUntilReady(10*time.Second, nil); err != nil {
		return nil, fmt.Errorf("couchbase bucket wait until ready: %w", err)
	}

	collection := bucket.DefaultCollection()

	return &CouchbaseStore{
		cluster:    cluster,
		bucket:     bucket,
		collection: collection,
		bucketName: bucketName,
	}, nil
}

// docKey returns the full document key for a memory.
func memoryKey(id string) string {
	return "memory::" + id
}

// agentKey returns the full document key for an agent.
func agentKey(id string) string {
	return "agent::" + id
}

// CreateMemory persists a new memory document.
func (s *CouchbaseStore) CreateMemory(_ context.Context, m *Memory) error {
	if m.ID == "" {
		m.ID = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}
	m.DocType = "memory"
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	_, err := s.collection.Insert(memoryKey(m.ID), m, nil)
	if err != nil {
		return fmt.Errorf("CreateMemory insert: %w", err)
	}
	return nil
}

// GetMemory retrieves a memory by ID. Returns nil, nil when not found.
func (s *CouchbaseStore) GetMemory(_ context.Context, id string) (*Memory, error) {
	result, err := s.collection.Get(memoryKey(id), nil)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetMemory get: %w", err)
	}

	var m Memory
	if err := result.Content(&m); err != nil {
		return nil, fmt.Errorf("GetMemory decode: %w", err)
	}
	return &m, nil
}

// ListMemories returns memories matching the given filter, sorted by createdAt DESC.
func (s *CouchbaseStore) ListMemories(_ context.Context, filter MemoryFilter) ([]*Memory, int, error) {
	// Build dynamic WHERE clause
	conditions := []string{"docType = 'memory'"}
	params := map[string]interface{}{}

	if filter.AgentID != "" {
		conditions = append(conditions, "agentId = $agentId")
		params["agentId"] = filter.AgentID
	}
	if filter.Type != "" {
		conditions = append(conditions, "memoryType = $memoryType")
		params["memoryType"] = string(filter.Type)
	}
	if filter.Query != "" {
		conditions = append(conditions, "(LOWER(content) LIKE $query OR LOWER(agentName) LIKE $query)")
		params["query"] = "%" + strings.ToLower(filter.Query) + "%"
	}

	where := strings.Join(conditions, " AND ")

	// Count query
	countSQL := fmt.Sprintf("SELECT COUNT(*) AS cnt FROM `%s` WHERE %s", s.bucketName, where)
	countResult, err := s.cluster.Query(countSQL, &gocb.QueryOptions{
		NamedParameters: params,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("ListMemories count query: %w", err)
	}

	var countRow struct {
		Cnt int `json:"cnt"`
	}
	if countResult.Next() {
		if err := countResult.Row(&countRow); err != nil {
			return nil, 0, fmt.Errorf("ListMemories count decode: %w", err)
		}
	}
	if err := countResult.Err(); err != nil {
		return nil, 0, fmt.Errorf("ListMemories count result: %w", err)
	}
	total := countRow.Cnt

	// Data query
	dataSQL := fmt.Sprintf(
		"SELECT m.* FROM `%s` AS m WHERE %s ORDER BY m.createdAt DESC",
		s.bucketName, where,
	)

	queryParams := make(map[string]interface{}, len(params))
	for k, v := range params {
		queryParams[k] = v
	}

	if filter.Limit > 0 {
		dataSQL += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		dataSQL += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.cluster.Query(dataSQL, &gocb.QueryOptions{
		NamedParameters: queryParams,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("ListMemories data query: %w", err)
	}

	var memories []*Memory
	for rows.Next() {
		var m Memory
		if err := rows.Row(&m); err != nil {
			return nil, 0, fmt.Errorf("ListMemories row decode: %w", err)
		}
		memories = append(memories, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("ListMemories rows error: %w", err)
	}

	return memories, total, nil
}

// UpdateMemory replaces an existing memory document.
func (s *CouchbaseStore) UpdateMemory(_ context.Context, m *Memory) error {
	m.UpdatedAt = time.Now()
	_, err := s.collection.Replace(memoryKey(m.ID), m, nil)
	if err != nil {
		return fmt.Errorf("UpdateMemory replace: %w", err)
	}
	return nil
}

// DeleteMemory removes a memory document by ID.
func (s *CouchbaseStore) DeleteMemory(_ context.Context, id string) error {
	_, err := s.collection.Remove(memoryKey(id), nil)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("memory %q not found", id)
		}
		return fmt.Errorf("DeleteMemory remove: %w", err)
	}
	return nil
}

// CreateAgent persists a new agent document.
func (s *CouchbaseStore) CreateAgent(_ context.Context, a *Agent) error {
	if a.ID == "" {
		a.ID = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	a.DocType = "agent"
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}

	_, err := s.collection.Insert(agentKey(a.ID), a, nil)
	if err != nil {
		return fmt.Errorf("CreateAgent insert: %w", err)
	}
	return nil
}

// GetAgent retrieves an agent by ID. Returns nil, nil when not found.
func (s *CouchbaseStore) GetAgent(_ context.Context, id string) (*Agent, error) {
	result, err := s.collection.Get(agentKey(id), nil)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetAgent get: %w", err)
	}

	var a Agent
	if err := result.Content(&a); err != nil {
		return nil, fmt.Errorf("GetAgent decode: %w", err)
	}
	return &a, nil
}

// ListAgents returns all agent documents sorted by createdAt ASC.
func (s *CouchbaseStore) ListAgents(_ context.Context) ([]*Agent, error) {
	sql := fmt.Sprintf(
		"SELECT a.* FROM `%s` AS a WHERE a.docType = 'agent' ORDER BY a.createdAt ASC",
		s.bucketName,
	)

	rows, err := s.cluster.Query(sql, nil)
	if err != nil {
		return nil, fmt.Errorf("ListAgents query: %w", err)
	}

	var agents []*Agent
	for rows.Next() {
		var a Agent
		if err := rows.Row(&a); err != nil {
			return nil, fmt.Errorf("ListAgents row decode: %w", err)
		}
		agents = append(agents, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListAgents rows error: %w", err)
	}

	return agents, nil
}

// DeleteAgent removes an agent document by ID.
func (s *CouchbaseStore) DeleteAgent(_ context.Context, id string) error {
	_, err := s.collection.Remove(agentKey(id), nil)
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("agent %q not found", id)
		}
		return fmt.Errorf("DeleteAgent remove: %w", err)
	}
	return nil
}

// GetStats computes aggregate statistics by running COUNT queries.
func (s *CouchbaseStore) GetStats(_ context.Context) (*Stats, error) {
	// Total memories
	totalMemSQL := fmt.Sprintf(
		"SELECT COUNT(*) AS cnt FROM `%s` WHERE docType = 'memory'",
		s.bucketName,
	)
	totalMem, err := s.queryCount(totalMemSQL)
	if err != nil {
		return nil, fmt.Errorf("GetStats total memories: %w", err)
	}

	// Total agents
	totalAgentSQL := fmt.Sprintf(
		"SELECT COUNT(*) AS cnt FROM `%s` WHERE docType = 'agent'",
		s.bucketName,
	)
	totalAgents, err := s.queryCount(totalAgentSQL)
	if err != nil {
		return nil, fmt.Errorf("GetStats total agents: %w", err)
	}

	// Recent (last 24 hours)
	recentCutoff := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	recentSQL := fmt.Sprintf(
		"SELECT COUNT(*) AS cnt FROM `%s` WHERE docType = 'memory' AND createdAt >= '%s'",
		s.bucketName, recentCutoff,
	)
	recentCount, err := s.queryCount(recentSQL)
	if err != nil {
		return nil, fmt.Errorf("GetStats recent count: %w", err)
	}

	// Memory type breakdown
	breakdownSQL := fmt.Sprintf(
		"SELECT memoryType, COUNT(*) AS cnt FROM `%s` WHERE docType = 'memory' GROUP BY memoryType",
		s.bucketName,
	)
	rows, err := s.cluster.Query(breakdownSQL, nil)
	if err != nil {
		return nil, fmt.Errorf("GetStats breakdown query: %w", err)
	}

	breakdown := make(map[MemoryType]int)
	for rows.Next() {
		var row struct {
			MemoryType string `json:"memoryType"`
			Cnt        int    `json:"cnt"`
		}
		if err := rows.Row(&row); err != nil {
			return nil, fmt.Errorf("GetStats breakdown row decode: %w", err)
		}
		breakdown[MemoryType(row.MemoryType)] = row.Cnt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetStats breakdown rows error: %w", err)
	}

	return &Stats{
		TotalMemories:       totalMem,
		TotalAgents:         totalAgents,
		MemoryTypeBreakdown: breakdown,
		RecentCount:         recentCount,
		StorageBackend:      "couchbase",
	}, nil
}

// queryCount is a helper that runs a COUNT(*) AS cnt query and returns the integer result.
func (s *CouchbaseStore) queryCount(sql string) (int, error) {
	rows, err := s.cluster.Query(sql, nil)
	if err != nil {
		return 0, err
	}
	var row struct {
		Cnt int `json:"cnt"`
	}
	if rows.Next() {
		if err := rows.Row(&row); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return row.Cnt, nil
}

// Close shuts down the Couchbase cluster connection.
func (s *CouchbaseStore) Close() error {
	return s.cluster.Close(nil)
}

// isNotFound checks whether the gocb error represents a document-not-found condition.
func isNotFound(err error) bool {
	return errors.Is(err, gocb.ErrDocumentNotFound)
}
