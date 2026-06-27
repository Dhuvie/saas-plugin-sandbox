package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	repo := &SQLiteRepository{db: db}
	if err := repo.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return repo, nil
}

func (r *SQLiteRepository) migrate() error {
	pluginsSchema := `
	CREATE TABLE IF NOT EXISTS plugins (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		source_code TEXT NOT NULL,
		compiled_wasm BLOB,
		version INTEGER NOT NULL,
		status TEXT NOT NULL,
		compile_errors TEXT,
		created_at DATETIME NOT NULL
	);`

	executionsSchema := `
	CREATE TABLE IF NOT EXISTS executions (
		id TEXT PRIMARY KEY,
		plugin_id TEXT NOT NULL,
		duration_ms REAL NOT NULL,
		memory_bytes INTEGER NOT NULL,
		fuel_consumed INTEGER NOT NULL,
		status TEXT NOT NULL,
		logs TEXT,
		created_at DATETIME NOT NULL,
		FOREIGN KEY(plugin_id) REFERENCES plugins(id)
	);`

	if _, err := r.db.Exec(pluginsSchema); err != nil {
		return fmt.Errorf("failed to create plugins table: %w", err)
	}

	if _, err := r.db.Exec(executionsSchema); err != nil {
		return fmt.Errorf("failed to create executions table: %w", err)
	}

	return nil
}

func (r *SQLiteRepository) SavePlugin(plugin *Plugin) error {
	query := `
	INSERT INTO plugins (id, name, source_code, compiled_wasm, version, status, compile_errors, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query,
		plugin.ID,
		plugin.Name,
		plugin.SourceCode,
		plugin.CompiledWasm,
		plugin.Version,
		plugin.Status,
		plugin.CompileErrors,
		plugin.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) GetPlugin(id string) (*Plugin, error) {
	query := `
	SELECT id, name, source_code, compiled_wasm, version, status, compile_errors, created_at
	FROM plugins WHERE id = ?`
	
	row := r.db.QueryRow(query, id)
	var plugin Plugin
	err := row.Scan(
		&plugin.ID,
		&plugin.Name,
		&plugin.SourceCode,
		&plugin.CompiledWasm,
		&plugin.Version,
		&plugin.Status,
		&plugin.CompileErrors,
		&plugin.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("plugin not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return &plugin, nil
}

func (r *SQLiteRepository) ListPlugins() ([]*Plugin, error) {
	query := `
	SELECT id, name, source_code, version, status, compile_errors, created_at
	FROM plugins ORDER BY created_at DESC`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plugins []*Plugin
	for rows.Next() {
		var p Plugin
		err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.SourceCode,
			&p.Version,
			&p.Status,
			&p.CompileErrors,
			&p.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, &p)
	}
	return plugins, nil
}

func (r *SQLiteRepository) UpdatePluginStatus(id string, status string, compiledWasm []byte, compileErrors string) error {
	query := `
	UPDATE plugins
	SET status = ?, compiled_wasm = ?, compile_errors = ?, version = version + 1
	WHERE id = ?`
	_, err := r.db.Exec(query, status, compiledWasm, compileErrors, id)
	return err
}

func (r *SQLiteRepository) UpdatePluginCode(id string, name string, sourceCode string) error {
	query := `
	UPDATE plugins
	SET name = ?, source_code = ?, status = 'pending', compile_errors = ''
	WHERE id = ?`
	_, err := r.db.Exec(query, name, sourceCode, id)
	return err
}

func (r *SQLiteRepository) SaveExecution(exec *Execution) error {
	query := `
	INSERT INTO executions (id, plugin_id, duration_ms, memory_bytes, fuel_consumed, status, logs, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.Exec(query,
		exec.ID,
		exec.PluginID,
		exec.DurationMs,
		exec.MemoryBytes,
		exec.FuelConsumed,
		exec.Status,
		exec.Logs,
		exec.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) GetExecutions(pluginID string) ([]*Execution, error) {
	query := `
	SELECT id, plugin_id, duration_ms, memory_bytes, fuel_consumed, status, logs, created_at
	FROM executions WHERE plugin_id = ? ORDER BY created_at DESC`

	rows, err := r.db.Query(query, pluginID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []*Execution
	for rows.Next() {
		var e Execution
		err := rows.Scan(
			&e.ID,
			&e.PluginID,
			&e.DurationMs,
			&e.MemoryBytes,
			&e.FuelConsumed,
			&e.Status,
			&e.Logs,
			&e.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		executions = append(executions, &e)
	}
	return executions, nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}
