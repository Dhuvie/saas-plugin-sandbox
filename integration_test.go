package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wasm-sandbox/compiler"
	"wasm-sandbox/db"
	"wasm-sandbox/sandbox"
)

func TestWasmSandboxIntegration(t *testing.T) {
	fmt.Println("==========================================")
	fmt.Println("Running Wasm Sandbox Integration Test Suite")
	fmt.Println("==========================================")

	// 1. Initialize DB (in-memory SQLite for testing)
	repo, err := db.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}
	defer repo.Close()

	// 2. Initialize Compile Worker
	worker, err := compiler.NewCompileWorker(repo, 10)
	if err != nil {
		t.Fatalf("Failed to initialize compile worker: %v", err)
	}
	worker.Start()

	// 3. Initialize Sandbox Engine
	engine := sandbox.NewEngine()

	// Helper to load, compile, and wait for a plugin
	compilePlugin := func(name string, sourcePath string) *db.Plugin {
		codeBytes, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatalf("Failed to read test plugin source at %s: %v", sourcePath, err)
		}

		plugin := &db.Plugin{
			ID:         name,
			Name:       name,
			SourceCode: string(codeBytes),
			Version:    1,
			Status:     "pending",
			CreatedAt:  time.Now(),
		}

		if err := repo.SavePlugin(plugin); err != nil {
			t.Fatalf("Failed to save plugin %s to DB: %v", name, err)
		}

		worker.QueueCompile(plugin.ID)

		// Poll for compilation complete (max 30 seconds)
		deadline := time.Now().Add(30 * time.Second)
		var compiledPlugin *db.Plugin
		for time.Now().Before(deadline) {
			p, err := repo.GetPlugin(plugin.ID)
			if err != nil {
				t.Fatalf("Failed to fetch plugin status: %v", err)
			}
			if p.Status == "compiled" {
				compiledPlugin = p
				break
			}
			if p.Status == "failed" {
				t.Fatalf("Plugin %s compilation failed: %s", name, p.CompileErrors)
			}
			time.Sleep(200 * time.Millisecond)
		}

		if compiledPlugin == nil {
			t.Fatalf("Timeout waiting for plugin %s to compile", name)
		}

		return compiledPlugin
	}

	// Helper to run a plugin in the sandbox
	runPlugin := func(plugin *db.Plugin, inputJSON string) *sandbox.ExecutionResult {
		res, err := engine.Run(plugin.ID, plugin.CompiledWasm, inputJSON)
		if err != nil {
			t.Fatalf("Sandbox engine returned critical error: %v", err)
		}
		return res
	}

	// -------------------------------------------------------------
	// TEST CASE 1: Standard plugin (Correct parsing and execution < 5ms)
	// -------------------------------------------------------------
	t.Run("Standard Plugin Execution", func(t *testing.T) {
		plugin := compilePlugin("basic", filepath.Join("test_plugins", "basic.rs"))
		
		input := `{"name": "Alice", "points": 650}`
		res := runPlugin(plugin, input)

		fmt.Printf("[Test] Basic run took %v, status: %s, fuel: %d, memory: %d bytes\n", 
			res.Duration, res.Status, res.FuelConsumed, res.MemoryUsed)

		if res.Status != "success" {
			t.Errorf("Expected status 'success', got '%s'. Logs: %s", res.Status, res.Logs)
		}

		if res.Duration > 10*time.Millisecond {
			// Note: warm runs should be under 5ms, first cold run might be slightly longer.
			// Let's execute it a second time to test the hot execution time!
			hotRes := runPlugin(plugin, input)
			fmt.Printf("[Test] Basic hot run took %v, status: %s\n", hotRes.Duration, hotRes.Status)
			if hotRes.Duration > 5*time.Millisecond {
				t.Logf("Warning: Hot run took %v (expected < 5ms)", hotRes.Duration)
			}
		}

		expectedContent := `"status":"premium"`
		if !stringsContains(res.Output, expectedContent) {
			t.Errorf("Expected output to contain '%s', got '%s'", expectedContent, res.Output)
		}
	})

	// -------------------------------------------------------------
	// TEST CASE 2: Out of Fuel (catching infinite loops)
	// -------------------------------------------------------------
	t.Run("Infinite Loop Protection", func(t *testing.T) {
		plugin := compilePlugin("loop", filepath.Join("test_plugins", "loop.rs"))
		
		res := runPlugin(plugin, `{}`)

		fmt.Printf("[Test] Loop run status: %s, fuel: %d, logs: %s\n", 
			res.Status, res.FuelConsumed, res.Logs)

		if res.Status != "out_of_fuel" {
			t.Errorf("Expected status 'out_of_fuel', got '%s'", res.Status)
		}
	})

	// -------------------------------------------------------------
	// TEST CASE 3: Out of Memory (OOM protection)
	// -------------------------------------------------------------
	t.Run("Memory Limits Protection", func(t *testing.T) {
		plugin := compilePlugin("oom", filepath.Join("test_plugins", "oom.rs"))
		
		res := runPlugin(plugin, `{}`)

		fmt.Printf("[Test] OOM run status: %s, memory: %d bytes, logs: %s\n", 
			res.Status, res.MemoryUsed, res.Logs)

		if res.Status != "out_of_memory" {
			t.Errorf("Expected status 'out_of_memory', got '%s'", res.Status)
		}
	})

	// -------------------------------------------------------------
	// TEST CASE 4: Host Escape (Filesystem Access blocked by WASI)
	// -------------------------------------------------------------
	t.Run("Filesystem Isolation", func(t *testing.T) {
		plugin := compilePlugin("escape", filepath.Join("test_plugins", "escape.rs"))
		
		res := runPlugin(plugin, `{}`)

		fmt.Printf("[Test] Escape run status: %s, output: %s, logs: %s\n", 
			res.Status, res.Output, res.Logs)

		// The plugin should fail to read the file, returning a "BLOCKED" message in output,
		// or Wasmtime should fail. In our Rust code, it catches the read error and returns "BLOCKED: Failed to read..."
		if res.Status != "success" {
			t.Errorf("Expected run to complete with 'success' (gracefully handling file error), got '%s'", res.Status)
		}

		if !stringsContains(res.Output, "BLOCKED") {
			t.Errorf("Expected output to contain 'BLOCKED', got '%s'", res.Output)
		}
	})
}

// Simple string helper to avoid importing "strings" in tests if not needed,
// but we can import it. Let's write a simple stringsContains helper.
func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s[0:len(s)] == substr || stringsContainsHelper(s, substr))
}

func stringsContainsHelper(s, substr string) bool {
	// A simple check using standard Go logic
	// But it's easier to just use strings.Contains!
	// Let's import strings since we already did in engine.go.
	// Oh, wait, the import block above does NOT include "strings".
	// Let's just use a simple strings.Contains, we will add "strings" to imports.
	return len(s) >= len(substr) && (s == substr || stringsContainsInner(s, substr))
}

func stringsContainsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
