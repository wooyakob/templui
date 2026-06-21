// Agent Memory System — standalone web server
// Extends the templUI docs server's asset infrastructure.
// Run: go run ./cmd/memory
// Env: COUCHBASE_CONN_STR, COUCHBASE_USERNAME, COUCHBASE_PASSWORD, COUCHBASE_BUCKET
//      PORT (default 8091), GO_ENV (default development)
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/joho/godotenv"

	"github.com/templui/templui/assets"
	"github.com/templui/templui/components"
	memstore "github.com/templui/templui/internal/memory"
	"github.com/templui/templui/internal/middleware"
	"github.com/templui/templui/internal/ui/memorypages"
	"github.com/templui/templui/static"
)

func main() {
	_ = godotenv.Load()

	store := initStore()
	defer store.Close()

	mux := http.NewServeMux()

	setupAssets(mux)
	setupStaticFiles(mux)
	setupMemoryRoutes(mux, store)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}

	wrappedMux := middleware.WithURLPathValue(
		middleware.CacheControlMiddleware(mux),
	)

	log.Printf("Agent Memory System running at http://localhost:%s/memory", port)
	if err := http.ListenAndServe(":"+port, wrappedMux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// initStore connects to Couchbase if credentials are provided, otherwise falls back to in-memory.
func initStore() memstore.Store {
	connStr := os.Getenv("COUCHBASE_CONN_STR")
	username := os.Getenv("COUCHBASE_USERNAME")
	password := os.Getenv("COUCHBASE_PASSWORD")
	bucket := os.Getenv("COUCHBASE_BUCKET")

	if connStr != "" && username != "" && password != "" && bucket != "" {
		s, err := memstore.NewCouchbaseStore(connStr, username, password, bucket)
		if err != nil {
			log.Printf("Couchbase connection failed (%v); falling back to in-memory store", err)
		} else {
			log.Printf("Connected to Couchbase bucket %q at %s", bucket, connStr)
			return s
		}
	} else {
		log.Println("Couchbase credentials not set; using in-memory store (data will not persist)")
	}
	return memstore.NewInMemoryStore()
}

// ---- Asset serving ----

func setupAssets(mux *http.ServeMux) {
	isDev := os.Getenv("GO_ENV") != "production"

	assetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isDev {
			w.Header().Set("Cache-Control", "no-store")
			http.FileServer(http.Dir("./assets")).ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		http.FileServer(http.FS(assets.Assets)).ServeHTTP(w, r)
	})
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", assetHandler))

	jsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/components/js/")
		w.Header().Set("Content-Type", "application/javascript")
		if isDev {
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, "./components/"+path)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		data, err := components.TemplFiles.ReadFile(path)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		w.Write(data)
	})
	mux.Handle("GET /components/js/", jsHandler)
}

func setupStaticFiles(mux *http.ServeMux) {
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		content, _ := static.Files.ReadFile("robots.txt")
		w.Header().Set("Content-Type", "text/plain")
		w.Write(content)
	})
}

// ---- Memory app routes ----

func setupMemoryRoutes(mux *http.ServeMux, store memstore.Store) {
	// Redirect / to /memory
	mux.Handle("GET /{$}", http.RedirectHandler("/memory", http.StatusSeeOther))

	// Dashboard
	mux.HandleFunc("GET /memory", func(w http.ResponseWriter, r *http.Request) {
		dashboardHandler(w, r, store)
	})

	// Memories list
	mux.HandleFunc("GET /memory/memories", func(w http.ResponseWriter, r *http.Request) {
		memoriesListHandler(w, r, store)
	})
	// Create memory form
	mux.HandleFunc("GET /memory/memories/new", func(w http.ResponseWriter, r *http.Request) {
		newMemoryFormHandler(w, r, store)
	})
	// Create memory
	mux.HandleFunc("POST /memory/memories", func(w http.ResponseWriter, r *http.Request) {
		createMemoryHandler(w, r, store)
	})
	// Memory detail
	mux.HandleFunc("GET /memory/memories/{id}", func(w http.ResponseWriter, r *http.Request) {
		memoryDetailHandler(w, r, store)
	})
	// Edit memory form
	mux.HandleFunc("GET /memory/memories/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		editMemoryFormHandler(w, r, store)
	})
	// Update memory (POST with _method=PUT)
	mux.HandleFunc("POST /memory/memories/{id}", func(w http.ResponseWriter, r *http.Request) {
		updateMemoryHandler(w, r, store)
	})
	// Delete memory (HTMX sends DELETE)
	mux.HandleFunc("DELETE /memory/memories/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteMemoryHandler(w, r, store)
	})

	// Agents list
	mux.HandleFunc("GET /memory/agents", func(w http.ResponseWriter, r *http.Request) {
		agentsListHandler(w, r, store)
	})
	// Create agent form
	mux.HandleFunc("GET /memory/agents/new", func(w http.ResponseWriter, r *http.Request) {
		newAgentFormHandler(w, r, store)
	})
	// Create agent
	mux.HandleFunc("POST /memory/agents", func(w http.ResponseWriter, r *http.Request) {
		createAgentHandler(w, r, store)
	})
	// Delete agent
	mux.HandleFunc("DELETE /memory/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteAgentHandler(w, r, store)
	})

	// HTMX partial: memories table
	mux.HandleFunc("GET /memory/api/memories", func(w http.ResponseWriter, r *http.Request) {
		memoriesAPIHandler(w, r, store)
	})
}

// ---- Handlers ----

func dashboardHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()

	stats, err := store.GetStats(ctx)
	if err != nil {
		log.Printf("GetStats error: %v", err)
		stats = &memstore.Stats{StorageBackend: "unknown"}
	}

	recentMemories, _, err := store.ListMemories(ctx, memstore.MemoryFilter{Limit: 5})
	if err != nil {
		log.Printf("ListMemories error: %v", err)
		recentMemories = nil
	}

	templ.Handler(memorypages.Dashboard(stats, recentMemories, stats.StorageBackend)).ServeHTTP(w, r)
}

func memoriesListHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	q := r.URL.Query()

	filter := memstore.MemoryFilter{
		AgentID: q.Get("agentId"),
		Type:    memstore.MemoryType(q.Get("type")),
		Query:   q.Get("q"),
		Limit:   50,
	}

	memories, total, err := store.ListMemories(ctx, filter)
	if err != nil {
		log.Printf("ListMemories error: %v", err)
		http.Error(w, "Failed to load memories", http.StatusInternalServerError)
		return
	}

	agents, err := store.ListAgents(ctx)
	if err != nil {
		log.Printf("ListAgents error: %v", err)
		agents = nil
	}

	stats, _ := store.GetStats(ctx)
	backend := "inmemory"
	if stats != nil {
		backend = stats.StorageBackend
	}

	templ.Handler(memorypages.MemoriesList(memories, agents, filter, total, backend)).ServeHTTP(w, r)
}

func memoriesAPIHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	q := r.URL.Query()

	filter := memstore.MemoryFilter{
		AgentID: q.Get("agentId"),
		Type:    memstore.MemoryType(q.Get("type")),
		Query:   q.Get("q"),
		Limit:   50,
	}

	memories, total, err := store.ListMemories(ctx, filter)
	if err != nil {
		log.Printf("ListMemories API error: %v", err)
		http.Error(w, "Failed to load memories", http.StatusInternalServerError)
		return
	}

	templ.Handler(memorypages.MemoriesTable(memories, total)).ServeHTTP(w, r)
}

func newMemoryFormHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	agents, _ := store.ListAgents(ctx)
	stats, _ := store.GetStats(ctx)
	backend := "inmemory"
	if stats != nil {
		backend = stats.StorageBackend
	}
	templ.Handler(memorypages.MemoryForm(agents, nil, false, backend)).ServeHTTP(w, r)
}

func createMemoryHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	agentID := r.FormValue("agentId")
	content := strings.TrimSpace(r.FormValue("content"))
	memType := memstore.MemoryType(r.FormValue("memoryType"))
	tagsRaw := r.FormValue("tags")
	importanceStr := r.FormValue("importance")

	if agentID == "" || content == "" {
		http.Error(w, "Agent and content are required", http.StatusBadRequest)
		return
	}

	importance, _ := strconv.Atoi(importanceStr)
	if importance < 1 {
		importance = 3
	}
	if importance > 5 {
		importance = 5
	}

	tags := parseTags(tagsRaw)

	// Look up agent name
	agent, err := store.GetAgent(ctx, agentID)
	if err != nil || agent == nil {
		http.Error(w, "Agent not found", http.StatusBadRequest)
		return
	}

	m := &memstore.Memory{
		AgentID:    agentID,
		AgentName:  agent.Name,
		Content:    content,
		MemoryType: memType,
		Tags:       tags,
		Importance: importance,
		CreatedAt:  time.Now(),
	}

	if err := store.CreateMemory(ctx, m); err != nil {
		log.Printf("CreateMemory error: %v", err)
		http.Error(w, "Failed to create memory", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/memory/memories/%s", m.ID), http.StatusSeeOther)
}

func memoryDetailHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	id := r.PathValue("id")

	m, err := store.GetMemory(ctx, id)
	if err != nil {
		log.Printf("GetMemory error: %v", err)
		http.Error(w, "Failed to load memory", http.StatusInternalServerError)
		return
	}
	if m == nil {
		http.Error(w, "Memory not found", http.StatusNotFound)
		return
	}

	stats, _ := store.GetStats(ctx)
	backend := "inmemory"
	if stats != nil {
		backend = stats.StorageBackend
	}

	templ.Handler(memorypages.MemoryDetail(m, backend)).ServeHTTP(w, r)
}

func editMemoryFormHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	id := r.PathValue("id")

	m, err := store.GetMemory(ctx, id)
	if err != nil || m == nil {
		http.Error(w, "Memory not found", http.StatusNotFound)
		return
	}

	agents, _ := store.ListAgents(ctx)
	stats, _ := store.GetStats(ctx)
	backend := "inmemory"
	if stats != nil {
		backend = stats.StorageBackend
	}

	templ.Handler(memorypages.MemoryForm(agents, m, true, backend)).ServeHTTP(w, r)
}

func updateMemoryHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	id := r.PathValue("id")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	// Only handle PUT method override
	if r.FormValue("_method") != "PUT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	existing, err := store.GetMemory(ctx, id)
	if err != nil || existing == nil {
		http.Error(w, "Memory not found", http.StatusNotFound)
		return
	}

	agentID := r.FormValue("agentId")
	content := strings.TrimSpace(r.FormValue("content"))
	memType := memstore.MemoryType(r.FormValue("memoryType"))
	tagsRaw := r.FormValue("tags")
	importanceStr := r.FormValue("importance")

	importance, _ := strconv.Atoi(importanceStr)
	if importance < 1 {
		importance = 3
	}
	if importance > 5 {
		importance = 5
	}

	// Update agent name if agentId changed
	agentName := existing.AgentName
	if agentID != existing.AgentID {
		agent, err := store.GetAgent(ctx, agentID)
		if err == nil && agent != nil {
			agentName = agent.Name
		}
	}

	existing.AgentID = agentID
	existing.AgentName = agentName
	existing.Content = content
	existing.MemoryType = memType
	existing.Tags = parseTags(tagsRaw)
	existing.Importance = importance

	if err := store.UpdateMemory(ctx, existing); err != nil {
		log.Printf("UpdateMemory error: %v", err)
		http.Error(w, "Failed to update memory", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/memory/memories/%s", id), http.StatusSeeOther)
}

func deleteMemoryHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	id := r.PathValue("id")

	if err := store.DeleteMemory(ctx, id); err != nil {
		log.Printf("DeleteMemory error: %v", err)
		http.Error(w, "Failed to delete memory", http.StatusInternalServerError)
		return
	}

	// HTMX: redirect via header
	w.Header().Set("HX-Redirect", "/memory/memories")
	w.WriteHeader(http.StatusOK)
}

func agentsListHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()

	agents, err := store.ListAgents(ctx)
	if err != nil {
		log.Printf("ListAgents error: %v", err)
		http.Error(w, "Failed to load agents", http.StatusInternalServerError)
		return
	}

	// Build memory count map
	memoryCounts := make(map[string]int)
	for _, a := range agents {
		_, count, err := store.ListMemories(ctx, memstore.MemoryFilter{AgentID: a.ID})
		if err == nil {
			memoryCounts[a.ID] = count
		}
	}

	stats, _ := store.GetStats(ctx)
	backend := "inmemory"
	if stats != nil {
		backend = stats.StorageBackend
	}

	templ.Handler(memorypages.AgentsList(agents, memoryCounts, backend)).ServeHTTP(w, r)
}

func newAgentFormHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	stats, _ := store.GetStats(r.Context())
	backend := "inmemory"
	if stats != nil {
		backend = stats.StorageBackend
	}
	templ.Handler(memorypages.AgentForm(backend)).ServeHTTP(w, r)
}

func createAgentHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))

	if name == "" {
		http.Error(w, "Agent name is required", http.StatusBadRequest)
		return
	}

	a := &memstore.Agent{
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
	}

	if err := store.CreateAgent(ctx, a); err != nil {
		log.Printf("CreateAgent error: %v", err)
		http.Error(w, "Failed to create agent", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/memory/agents", http.StatusSeeOther)
}

func deleteAgentHandler(w http.ResponseWriter, r *http.Request, store memstore.Store) {
	ctx := r.Context()
	id := r.PathValue("id")

	if err := store.DeleteAgent(ctx, id); err != nil {
		log.Printf("DeleteAgent error: %v", err)
		http.Error(w, "Failed to delete agent", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/memory/agents")
	w.WriteHeader(http.StatusOK)
}

// parseTags splits a comma-separated tags string into a slice, trimming whitespace.
func parseTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

