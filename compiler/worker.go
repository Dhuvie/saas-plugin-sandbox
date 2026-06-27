package compiler

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"wasm-sandbox/db"
)

const guestBoilerplateSDK = `
use std::slice;
use std::mem;

#[no_mangle]
pub extern "C" fn allocate(size: usize) -> *mut u8 {
    let mut buf = Vec::with_capacity(size);
    let ptr = buf.as_mut_ptr();
    mem::forget(buf);
    ptr
}

#[no_mangle]
pub unsafe extern "C" fn deallocate(ptr: *mut u8, size: usize) {
    let _ = Vec::from_raw_parts(ptr, size, size);
}

#[no_mangle]
pub unsafe extern "C" fn process(ptr: *mut u8, len: usize) -> u64 {
    let input_slice = slice::from_raw_parts(ptr, len);
    let input_str = match std::str::from_utf8(input_slice) {
        Ok(s) => s,
        Err(_) => return 0,
    };
    let output_str = handler(input_str);
    let out_bytes = output_str.into_bytes();
    let out_ptr = out_bytes.as_ptr() as u64;
    let out_len = out_bytes.len() as u64;
    mem::forget(out_bytes);
    (out_ptr << 32) | out_len
}
`

type CompileWorker struct {
	repo         db.Repository
	compileQueue chan string
	workingDir   string
}

func NewCompileWorker(repo db.Repository, queueSize int) (*CompileWorker, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	return &CompileWorker{
		repo:         repo,
		compileQueue: make(chan string, queueSize),
		workingDir:   wd,
	}, nil
}

func (w *CompileWorker) QueueCompile(pluginID string) {
	w.compileQueue <- pluginID
}

func (w *CompileWorker) Start() {
	go func() {
		for id := range w.compileQueue {
			w.compilePlugin(id)
		}
	}()
}

func (w *CompileWorker) compilePlugin(id string) {
	fmt.Printf("[Compiler] Starting compilation for plugin ID: %s\n", id)
	
	plugin, err := w.repo.GetPlugin(id)
	if err != nil {
		fmt.Printf("[Compiler] Error fetching plugin %s: %v\n", id, err)
		return
	}

	tempBuildName := fmt.Sprintf("temp_build_%s", id)
	tempBuildDir := filepath.Join(w.workingDir, ".build", tempBuildName)
	
	defer func() {
		os.RemoveAll(tempBuildDir)
		fmt.Printf("[Compiler] Cleaned up temporary directory: %s\n", tempBuildDir)
	}()

	err = w.setupTempCargoProject(tempBuildDir, plugin.SourceCode)
	if err != nil {
		w.repo.UpdatePluginStatus(id, "failed", nil, fmt.Sprintf("Setup build failed: %v", err))
		return
	}

	compileOutput, err := w.runCargoBuild(tempBuildDir)
	if err != nil {
		fmt.Printf("[Compiler] Build failed for plugin %s\n", id)
		w.repo.UpdatePluginStatus(id, "failed", nil, compileOutput)
		return
	}

	wasmPath := filepath.Join(tempBuildDir, "target", "wasm32-wasip1", "release", "wasm_plugin.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		wasmPath = filepath.Join(tempBuildDir, "target", "wasm32-wasip1", "release", "wasm_plugin.wasm")
		wasmBytes, err = os.ReadFile(wasmPath)
		if err != nil {
			w.repo.UpdatePluginStatus(id, "failed", nil, fmt.Sprintf("Failed to read compiled wasm file: %v", err))
			return
		}
	}

	err = w.repo.UpdatePluginStatus(id, "compiled", wasmBytes, "")
	if err != nil {
		fmt.Printf("[Compiler] Error saving compiled Wasm for %s: %v\n", id, err)
		return
	}

	fmt.Printf("[Compiler] Plugin %s successfully compiled (%d bytes)\n", id, len(wasmBytes))
}

func (w *CompileWorker) setupTempCargoProject(dir string, sourceCode string) error {
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return err
	}

	templateCargoPath := filepath.Join(w.workingDir, "compiler", "template", "Cargo.toml")
	destCargoPath := filepath.Join(dir, "Cargo.toml")
	
	if err := copyFile(templateCargoPath, destCargoPath); err != nil {
		return fmt.Errorf("failed to copy Cargo.toml template: %w", err)
	}

	destLibPath := filepath.Join(srcDir, "lib.rs")
	fullCode := sourceCode + "\n" + guestBoilerplateSDK
	
	if err := os.WriteFile(destLibPath, []byte(fullCode), 0644); err != nil {
		return fmt.Errorf("failed to write src/lib.rs: %w", err)
	}

	return nil
}

func (w *CompileWorker) runCargoBuild(dir string) (string, error) {
	toolchainDir := filepath.Join(w.workingDir, ".toolchain")
	cargoHome := filepath.Join(toolchainDir, "cargo")
	rustupHome := filepath.Join(toolchainDir, "rustup")
	goBin := filepath.Join(toolchainDir, "go", "bin")
	mingwBin := filepath.Join(toolchainDir, "w64devkit", "w64devkit", "bin")

	cargoExe := filepath.Join(cargoHome, "bin", "cargo.exe")
	
	cmd := exec.Command(cargoExe, "build", "--target", "wasm32-wasip1", "--release")
	cmd.Dir = dir

	env := os.Environ()
	env = append(env, fmt.Sprintf("CARGO_HOME=%s", cargoHome))
	env = append(env, fmt.Sprintf("RUSTUP_HOME=%s", rustupHome))
	
	origPath := os.Getenv("PATH")
	newPath := fmt.Sprintf("%s\\bin;%s;%s;%s", cargoHome, goBin, mingwBin, origPath)
	env = append(env, fmt.Sprintf("PATH=%s", newPath))
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	return string(out), err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}
