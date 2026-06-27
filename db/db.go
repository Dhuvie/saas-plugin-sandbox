package db

import (
	"time"
)

type Plugin struct {
	ID            string    `json:"id" bson:"_id"`
	Name          string    `json:"name" bson:"name"`
	SourceCode    string    `json:"source_code" bson:"source_code"`
	CompiledWasm  []byte    `json:"-" bson:"compiled_wasm"`
	Version       int       `json:"version" bson:"version"`
	Status        string    `json:"status" bson:"status"`
	CompileErrors string    `json:"compile_errors" bson:"compile_errors"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
}

type Execution struct {
	ID           string    `json:"id" bson:"_id"`
	PluginID     string    `json:"plugin_id" bson:"plugin_id"`
	DurationMs   float64   `json:"duration_ms" bson:"duration_ms"`
	MemoryBytes  int64     `json:"memory_bytes" bson:"memory_bytes"`
	FuelConsumed int64     `json:"fuel_consumed" bson:"fuel_consumed"`
	Status       string    `json:"status" bson:"status"`
	Logs         string    `json:"logs" bson:"logs"`
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
}

type Repository interface {
	SavePlugin(plugin *Plugin) error
	GetPlugin(id string) (*Plugin, error)
	ListPlugins() ([]*Plugin, error)
	UpdatePluginStatus(id string, status string, compiledWasm []byte, compileErrors string) error
	UpdatePluginCode(id string, name string, sourceCode string) error
	SaveExecution(exec *Execution) error
	GetExecutions(pluginID string) ([]*Execution, error)
	DeletePlugin(id string) error
	Close() error
}
