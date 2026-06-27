package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
	"wasm-sandbox/compiler"
	"wasm-sandbox/db"
	"wasm-sandbox/sandbox"

	"github.com/google/uuid"
)

type App struct {
	repo     db.Repository
	worker   *compiler.CompileWorker
	engine   *sandbox.Engine
}

func main() {
	fmt.Println("==========================================")
	fmt.Println("Starting Wasm Sandboxed Plugin Host Server")
	fmt.Println("==========================================")

	var repo db.Repository
	var err error
	pgConnStr := os.Getenv("DATABASE_URL")
	
	if pgConnStr != "" {
		fmt.Println("[Init] PostgreSQL URL detected. Connecting to Postgres...")
		repo, err = db.NewPostgresRepository(pgConnStr)
	} else {
		dbPath := "sandbox.db"
		fmt.Printf("[Init] DATABASE_URL not set. Falling back to local SQLite: %s\n", dbPath)
		repo, err = db.NewSQLiteRepository(dbPath)
	}
	
	if err != nil {
		log.Fatalf("[Fatal] Failed to initialize database: %v\n", err)
	}
	defer repo.Close()

	engine := sandbox.NewEngine()

	worker, err := compiler.NewCompileWorker(repo, 100)
	if err != nil {
		log.Fatalf("[Fatal] Failed to initialize compilation worker: %v\n", err)
	}
	worker.Start()
	fmt.Println("[Init] Compiler Worker queue started in the background")

	app := &App{
		repo:   repo,
		worker: worker,
		engine: engine,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/plugins", app.handleCreatePlugin)
	mux.HandleFunc("PUT /api/plugins/{id}", app.handleUpdatePlugin)
	mux.HandleFunc("GET /api/plugins", app.handleListPlugins)
	mux.HandleFunc("GET /api/plugins/{id}", app.handleGetPlugin)
	mux.HandleFunc("POST /api/plugins/{id}/execute", app.handleExecutePlugin)
	mux.HandleFunc("GET /api/plugins/{id}/metrics", app.handleGetMetrics)

	if _, err := os.Stat("./frontend/dist"); err == nil {
		fmt.Println("[Init] Serving React frontend from ./frontend/dist")
		fs := http.FileServer(http.Dir("./frontend/dist"))
		mux.Handle("GET /", fs)
	}

	handler := enableCORS(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("[Server] REST API listening on http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("[Fatal] Server failed: %v\n", err)
	}
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

type CreatePluginRequest struct {
	Name       string `json:"name"`
	SourceCode string `json:"source_code"`
}

func (app *App) handleCreatePlugin(w http.ResponseWriter, r *http.Request) {
	var req CreatePluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body JSON", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.SourceCode == "" {
		http.Error(w, "Fields 'name' and 'source_code' are required", http.StatusBadRequest)
		return
	}

	plugin := &db.Plugin{
		ID:         uuid.New().String(),
		Name:       req.Name,
		SourceCode: req.SourceCode,
		Version:    1,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}

	if err := app.repo.SavePlugin(plugin); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save plugin: %v", err), http.StatusInternalServerError)
		return
	}

	app.worker.QueueCompile(plugin.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(plugin)
}

type UpdatePluginRequest struct {
	Name       string `json:"name"`
	SourceCode string `json:"source_code"`
}

func (app *App) handleUpdatePlugin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "Missing plugin ID parameter", http.StatusBadRequest)
		return
	}

	_, err := app.repo.GetPlugin(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Plugin not found: %v", err), http.StatusNotFound)
		return
	}

	var req UpdatePluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body JSON", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.SourceCode == "" {
		http.Error(w, "Fields 'name' and 'source_code' are required", http.StatusBadRequest)
		return
	}

	if err := app.repo.UpdatePluginCode(id, req.Name, req.SourceCode); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update plugin: %v", err), http.StatusInternalServerError)
		return
	}

	app.worker.QueueCompile(id)

	updatedPlugin, err := app.repo.GetPlugin(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to retrieve updated plugin: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedPlugin)
}

func (app *App) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	plugins, err := app.repo.ListPlugins()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list plugins: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plugins)
}

func (app *App) handleGetPlugin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "Missing plugin ID parameter", http.StatusBadRequest)
		return
	}

	plugin, err := app.repo.GetPlugin(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Plugin not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plugin)
}

type ExecuteRequest struct {
	InputJSON string `json:"input_json"`
}

func (app *App) handleExecutePlugin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "Missing plugin ID parameter", http.StatusBadRequest)
		return
	}

	plugin, err := app.repo.GetPlugin(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Plugin not found: %v", err), http.StatusNotFound)
		return
	}

	if plugin.Status != "compiled" {
		http.Error(w, fmt.Sprintf("Plugin is not ready for execution (status: %s)", plugin.Status), http.StatusConflict)
		return
	}

	var req ExecuteRequest
	bodyBytes, err := io.ReadAll(r.Body)
	if err == nil && len(bodyBytes) > 0 {
		var jsonReq ExecuteRequest
		if err := json.Unmarshal(bodyBytes, &jsonReq); err == nil {
			req = jsonReq
		} else {
			req.InputJSON = string(bodyBytes)
		}
	}

	if req.InputJSON == "" {
		req.InputJSON = "{}"
	}

	res, err := app.engine.Run(plugin.ID, plugin.CompiledWasm, req.InputJSON)
	if err != nil {
		http.Error(w, fmt.Sprintf("Internal sandbox execution engine error: %v", err), http.StatusInternalServerError)
		return
	}

	exec := &db.Execution{
		ID:           uuid.New().String(),
		PluginID:     plugin.ID,
		DurationMs:   float64(res.Duration.Microseconds()) / 1000.0,
		MemoryBytes:  res.MemoryUsed,
		FuelConsumed: res.FuelConsumed,
		Status:       res.Status,
		Logs:         res.Logs,
		CreatedAt:    time.Now(),
	}

	if err := app.repo.SaveExecution(exec); err != nil {
		fmt.Printf("[Error] Failed to save execution metrics: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Output       string  `json:"output"`
		Status       string  `json:"status"`
		DurationMs   float64 `json:"duration_ms"`
		MemoryBytes  int64   `json:"memory_bytes"`
		FuelConsumed int64   `json:"fuel_consumed"`
		Logs         string  `json:"logs"`
	}{
		Output:       res.Output,
		Status:       res.Status,
		DurationMs:   exec.DurationMs,
		MemoryBytes:  exec.MemoryBytes,
		FuelConsumed: exec.FuelConsumed,
		Logs:         exec.Logs,
	})
}

func (app *App) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "Missing plugin ID parameter", http.StatusBadRequest)
		return
	}

	executions, err := app.repo.GetExecutions(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch metrics: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(executions)
}
