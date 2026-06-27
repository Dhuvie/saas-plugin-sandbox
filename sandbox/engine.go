package sandbox

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bytecodealliance/wasmtime-go/v20"
)

type Engine struct {
	wasmEngine *wasmtime.Engine
	modules    map[string]*wasmtime.Module
	mu         sync.RWMutex
}

type ExecutionResult struct {
	Output       string        `json:"output"`
	Logs         string        `json:"logs"`
	Duration     time.Duration `json:"duration"`
	FuelConsumed int64         `json:"fuel_consumed"`
	MemoryUsed   int64         `json:"memory_used"`
	Status       string        `json:"status"`
}

func NewEngine() *Engine {
	config := wasmtime.NewConfig()
	config.SetConsumeFuel(true)
	config.SetCraneliftOptLevel(wasmtime.OptLevelSpeed)

	return &Engine{
		wasmEngine: wasmtime.NewEngineWithConfig(config),
		modules:    make(map[string]*wasmtime.Module),
	}
}

func (e *Engine) Compile(pluginID string, wasmBytes []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.modules[pluginID]; ok {
		return nil
	}

	module, err := wasmtime.NewModule(e.wasmEngine, wasmBytes)
	if err != nil {
		return fmt.Errorf("failed to compile Wasm: %w", err)
	}

	e.modules[pluginID] = module
	return nil
}

func (e *Engine) GetModule(pluginID string, wasmBytes []byte) (*wasmtime.Module, error) {
	e.mu.RLock()
	mod, ok := e.modules[pluginID]
	e.mu.RUnlock()

	if ok {
		return mod, nil
	}

	if err := e.Compile(pluginID, wasmBytes); err != nil {
		return nil, err
	}

	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.modules[pluginID], nil
}

func (e *Engine) Run(pluginID string, wasmBytes []byte, inputJSON string) (*ExecutionResult, error) {
	startTime := time.Now()
	var memoryUsed int64

	module, err := e.GetModule(pluginID, wasmBytes)
	if err != nil {
		return &ExecutionResult{
			Status: "runtime_error",
			Logs:   fmt.Sprintf("Compilation/Load error: %v", err),
		}, nil
	}

	tmpStdout, err := os.CreateTemp("", "wasm-stdout-*.log")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp stdout file: %w", err)
	}
	tmpStdoutPath := tmpStdout.Name()
	defer os.Remove(tmpStdoutPath)
	defer tmpStdout.Close()

	tmpStderr, err := os.CreateTemp("", "wasm-stderr-*.log")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp stderr file: %w", err)
	}
	tmpStderrPath := tmpStderr.Name()
	defer os.Remove(tmpStderrPath)
	defer tmpStderr.Close()

	wasiConfig := wasmtime.NewWasiConfig()
	wasiConfig.SetStdoutFile(tmpStdoutPath)
	wasiConfig.SetStderrFile(tmpStderrPath)

	store := wasmtime.NewStore(e.wasmEngine)
	store.SetWasi(wasiConfig)

	const maxMemory = 5 * 1024 * 1024
	store.Limiter(maxMemory, -1, -1, -1, -1)

	const maxFuel = 10_000_000
	err = store.SetFuel(maxFuel)
	if err != nil {
		return nil, fmt.Errorf("failed to configure CPU fuel: %w", err)
	}

	readLogs := func() string {
		stdoutBytes, _ := os.ReadFile(tmpStdoutPath)
		stderrBytes, _ := os.ReadFile(tmpStderrPath)
		var logs strings.Builder
		if len(stdoutBytes) > 0 {
			logs.WriteString("STDOUT:\n")
			logs.WriteString(string(stdoutBytes))
		}
		if len(stderrBytes) > 0 {
			if logs.Len() > 0 {
				logs.WriteString("\n")
			}
			logs.WriteString("STDERR:\n")
			logs.WriteString(string(stderrBytes))
		}
		return logs.String()
	}

	getMetrics := func(status string, output string, execErr error) *ExecutionResult {
		duration := time.Since(startTime)
		
		var fuelConsumed int64
		if remainingFuel, err := store.GetFuel(); err == nil {
			fuelConsumed = int64(maxFuel - remainingFuel)
		}

		logs := readLogs()
		if execErr != nil {
			if len(logs) > 0 {
				logs = fmt.Sprintf("%s\n\nRUNTIME ERROR: %v", logs, execErr)
			} else {
				logs = fmt.Sprintf("RUNTIME ERROR: %v", execErr)
			}
		}

		return &ExecutionResult{
			Output:       output,
			Logs:         logs,
			Duration:     duration,
			FuelConsumed: fuelConsumed,
			MemoryUsed:   memoryUsed,
			Status:       status,
		}
	}

	linker := wasmtime.NewLinker(e.wasmEngine)
	err = linker.DefineWasi()
	if err != nil {
		return getMetrics("runtime_error", "", fmt.Errorf("linker error: %w", err)), nil
	}

	instance, err := linker.Instantiate(store, module)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "memory") || strings.Contains(errMsg, "limit") {
			return getMetrics("out_of_memory", "", err), nil
		}
		return getMetrics("runtime_error", "", err), nil
	}

	memoryExtern := instance.GetExport(store, "memory")
	if memoryExtern != nil && memoryExtern.Memory() != nil {
		memoryUsed = int64(memoryExtern.Memory().Size(store) * 64 * 1024)
	}

	allocate := instance.GetFunc(store, "allocate")
	deallocate := instance.GetFunc(store, "deallocate")
	process := instance.GetFunc(store, "process")
	memory := memoryExtern.Memory()

	if allocate == nil || deallocate == nil || process == nil || memory == nil {
		return getMetrics("runtime_error", "", fmt.Errorf("wasm module missing required exports (allocate, deallocate, process, or memory)")), nil
	}

	inputBytes := []byte(inputJSON)
	allocResult, err := allocate.Call(store, int32(len(inputBytes)))
	if err != nil {
		return handleExecutionError(err, getMetrics, memoryUsed), nil
	}

	inputPtr := toInt32(allocResult)
	if inputPtr == 0 && len(inputBytes) > 0 {
		return getMetrics("out_of_memory", "", fmt.Errorf("failed to allocate memory in Wasm guest")), nil
	}

	memBytes := memory.UnsafeData(store)
	copy(memBytes[inputPtr:inputPtr+int32(len(inputBytes))], inputBytes)

	memoryUsed = int64(memory.Size(store) * 64 * 1024)

	res, err := process.Call(store, inputPtr, int32(len(inputBytes)))
	if err != nil {
		return handleExecutionError(err, getMetrics, memoryUsed), nil
	}

	packedResult := res.(int64)
	outPtr := uint32(uint64(packedResult) >> 32)
	outLen := uint32(uint64(packedResult) & 0xFFFFFFFF)

	memoryUsed = int64(memory.Size(store) * 64 * 1024)

	var outputJSON string
	if outLen > 0 {
		memBytes = memory.UnsafeData(store)
		outBytes := make([]byte, outLen)
		copy(outBytes, memBytes[outPtr:outPtr+outLen])
		outputJSON = string(outBytes)

		_, err = deallocate.Call(store, int32(outPtr), int32(outLen))
		if err != nil {
			fmt.Printf("[Sandbox] Warning: deallocate output failed: %v\n", err)
		}
	}

	metrics := getMetrics("success", outputJSON, nil)
	metrics.MemoryUsed = memoryUsed
	return metrics, nil
}

func handleExecutionError(err error, getMetrics func(string, string, error) *ExecutionResult, memoryUsed int64) *ExecutionResult {
	errMsg := err.Error()
	status := "runtime_error"
	if strings.Contains(errMsg, "fuel") {
		status = "out_of_fuel"
	} else if strings.Contains(errMsg, "memory") || strings.Contains(errMsg, "limit") || strings.Contains(errMsg, "out of bounds") {
		status = "out_of_memory"
	}
	
	res := getMetrics(status, "", err)
	
	if res.Status == "runtime_error" && (strings.Contains(res.Logs, "memory allocation") || strings.Contains(res.Logs, "alloc") || strings.Contains(res.Logs, "out of memory")) {
		res.Status = "out_of_memory"
	}
	
	res.MemoryUsed = memoryUsed
	return res
}

func toInt32(v interface{}) int32 {
	switch val := v.(type) {
	case int32:
		return val
	case int64:
		return int32(val)
	case float64:
		return int32(val)
	default:
		return 0
	}
}
