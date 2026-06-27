# Isolated WebAssembly Tenant Plugin Runtime for SaaS Environments
This repository implements a high-performance, secure, and resource-constrained execution platform designed to run untrusted guest scripts within SaaS environments. The runtime leverages Rust compilation targeting WebAssembly (Wasm) and utilizes the Wasmtime virtual engine embedded in a Go host environment to enforce strict resource ceilings and prevent escape attempts.

---

## Architectural Specifications

### 1. Memory Virtualization and Constraint Enforcements
Guest plugins run inside a dedicated virtual memory layout consisting of contiguous 64KB pages. The system enforces a hard memory limit of 5 megabytes (5,242,880 bytes). When the guest allocator attempts to grow the heap beyond this limit via memory.grow instructions, the Wasmtime runtime intercepts the system call and returns a failure code. This triggers a panic inside the guest's allocator, raising an unreachable execution trap that is captured by the Go host.

### 2. Instruction-Level CPU Fuel Metering
To mitigate execution hang or infinite loops, the compiler modifies the compiled Wasm bytecode to inject decrementing counters at the start of loop headers, function entries, and jump targets. A budget of 10,000,000 instruction units is allocated per execution. Each compiled instruction executed by Wasmtime decrements this budget. If the budget reaches zero before the execution completes, Wasmtime halts the virtual thread, raises an out of fuel trap, and returns CPU control to Go.

### 3. POSIX and WASI Isolation
The execution engine implements strict isolation from the host operating system. The WebAssembly System Interface (WASI) context is initialized with empty configuration sets:
- **No File Directories**: No folder directories are mapped to the guest. Filesystem syscalls (such as path_open) immediately return POSIX error 44 (Permission Denied).
- **No Socket Bindings**: Network connections are completely blocked by omitting socket drivers from the virtual runtime context.
- **Piped Logging**: Standard output (stdout) and standard error (stderr) streams are redirected into isolated memory buffers, allowing Go to record guest stdout logs while preventing any host device interactions.

### 4. Binary Memory Marshaling Protocol
Since Go and Rust utilize different allocators and struct layouts, data transfers across the guest-host boundary are executed via a custom memory translation protocol:
- **Allocation**: The Go host invokes the guest's `allocate` function, which reserves bytes in the Wasm heap and returns the pointer offset.
- **Memory Write**: Go writes the JSON argument bytes directly to Wasm linear memory at the designated pointer offset.
- **Execution**: Go invokes the guest's `process` function, passing the offset pointer and payload length.
- **Packed u64 Return**: The guest processes the data and returns a packed 64-bit unsigned integer containing:
  - Upper 32-bits: Starting pointer offset of the output buffer.
  - Lower 32-bits: Length of the output buffer.
- **Deallocation**: Go reads the output bytes from Wasm memory, and then calls the guest's `deallocate` function to reclaim the heap space, preventing memory leaks during warm executions.

---

## Directory Layout

- `compiler/`: Background compilation queue manager and Cargo project blueprints.
- `compiler/template/`: Rust guest source project files containing memory allocation templates.
- `db/`: Database repository layer abstracting SQLite, PostgreSQL, and MongoDB.
- `sandbox/`: Wasmtime engine configuration and resource limit wrappers.
- `frontend/`: React Vite dashboard, Monaco Editor integrations, and Recharts performance logs charts.
- `test_plugins/`: Verification source files checking memory, infinite loops, and sandbox escape restrictions.

---

## Quick Start and Local Deployment

### 1. Toolchain Path Configurations
To use the local user-space Go, Rust, and GCC compilers in PowerShell, run:
```powershell
. .\use_toolchain.ps1
```

### 2. Compilation and Server Run
Build the binary and start the API server:
```powershell
go build -o wasm-host.exe main.go
.\wasm-host.exe
```

### 3. Frontend Start
Start the development server for the user dashboard:
```powershell
cd frontend
npm install
npm run dev
```
Open `http://localhost:5173` to access the React interface.

---

## Production Deployment and Containerization

The project is packaged with a multi-stage Dockerfile that builds the React application, builds the Go host binary, installs the Rust toolchain with the Wasm target inside the production image, and exposes a single unified HTTP server:
```dockerfile
docker build -t wasm-tenant-isolation .
docker run -p 8080:8080 wasm-tenant-isolation
```
If the directory `./frontend/dist` is present at server startup, the Go host automatically serves the static React application from `/`.

---

## API Reference

### 1. Create Plugin
- **Endpoint**: `POST /api/plugins`
- **Request Body**:
  ```json
  {
    "name": "data_processor",
    "source_code": "fn handler(input: &str) -> String { ... }"
  }
  ```
- **Response**: The created plugin object metadata with status set to `pending`.

### 2. Update Plugin
- **Endpoint**: `PUT /api/plugins/:id`
- **Request Body**: Updated name and source code.
- **Response**: Updates metadata and enqueues Wasm compilation.

### 3. Execute Plugin
- **Endpoint**: `POST /api/plugins/:id/execute`
- **Request Body**:
  ```json
  {
    "input_json": "{\"points\": 650}"
  }
  ```
- **Response**:
  ```json
  {
    "output": "{\"points\": 650, \"status\": \"premium\"}",
    "status": "success",
    "duration_ms": 1.06,
    "memory_bytes": 1114112,
    "fuel_consumed": 15340,
    "logs": "STDOUT:\nProcessing input...\n"
  }
  ```
