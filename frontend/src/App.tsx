import { useState, useEffect, useRef } from 'react';
import Editor from '@monaco-editor/react';
import { 
  Play, Cpu, Code2, Activity, Plus, RefreshCw, 
  AlertTriangle, CheckCircle2, Shield, Flame, TerminalSquare, AreaChart as ChartIcon, FileCode, AlertCircle, HelpCircle, HardDrive, Trash2
} from 'lucide-react';
import { ResponsiveContainer, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip } from 'recharts';

interface Plugin {
  id: string;
  name: string;
  source_code: string;
  version: number;
  status: 'pending' | 'compiled' | 'failed';
  compile_errors: string;
  created_at: string;
}

interface Execution {
  id: string;
  plugin_id: string;
  duration_ms: number;
  memory_bytes: number;
  fuel_consumed: number;
  status: 'success' | 'runtime_error' | 'out_of_memory' | 'out_of_fuel';
  logs: string;
  created_at: string;
}

interface RunResponse {
  output: string;
  status: 'success' | 'runtime_error' | 'out_of_memory' | 'out_of_fuel';
  duration_ms: number;
  memory_bytes: number;
  fuel_consumed: number;
  logs: string;
}

const TEMPLATES = [
  {
    name: 'Basic JSON Processor',
    description: 'Parses JSON payload, injects metadata attributes, and returns the modified object.',
    code: `
use serde_json::Value;

fn handler(input: &str) -> String {
    if let Ok(mut val) = serde_json::from_str::<Value>(input) {
        if let Some(obj) = val.as_object_mut() {

            obj.insert("status".to_string(), Value::String("premium".to_string()));
            obj.insert("processed_by".to_string(), Value::String("wasm_sandbox".to_string()));
        }
        serde_json::to_string(&val).unwrap_or_else(|_| "{}".to_string())
    } else {
        r#"{"error": "Invalid input JSON"}"#.to_string()
    }
}`
  },
  {
    name: 'Infinite Loop Protection',
    description: 'Spins up an infinite loop. Wasmtime fuel limits catch and terminate it safely.',
    code: `


fn handler(_input: &str) -> String {
    println!("Infinite Loop Started...");
    let mut x: u64 = 0;
    loop {
        x = x.wrapping_add(1);
        std::hint::black_box(&x);
        

        if x % 1000 == 0 {
            println!("Looping... count = {}", x);
        }
    }
}`
  },
  {
    name: 'Memory Limit Trigger (OOM)',
    description: 'Attempts to allocate 20MB in chunks. Terminated by host when exceeding 5MB.',
    code: `


fn handler(_input: &str) -> String {
    let mut chunks = Vec::new();
    println!("OOM Test: Starting allocation loop...");
    
    for i in 1..=20 {

        let chunk = vec![0u8; 1024 * 1024];
        println!("Successfully allocated chunk {}", i);
        
        chunks.push(chunk);
        std::hint::black_box(&chunks);
    }
    
    format!("SUCCESS: Allocated {} MB!", chunks.len())
}`
  },
  {
    name: 'Host Escape (Sandbox Isolation)',
    description: 'Attempts host file system access. Blocked immediately by strict sandbox WASI rules.',
    code: `


fn handler(_input: &str) -> String {
    println!("Attempting sandbox escape...");
    

    match std::fs::read_to_string("C:\\\\Windows\\\\system.ini") {
        Ok(content) => format!("SUCCESS: Read file content (length: {})", content.len()),
        Err(e) => format!("BLOCKED: Failed to read file: {}", e),
    }
}`
  }
];

const DEFAULT_PAYLOAD = `{
  "name": "Acme SaaS",
  "plan": "Enterprise",
  "active_users": 1420,
  "points": 650
}`;

const API_BASE = 'http://localhost:8080';

export default function App() {
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [selectedPlugin, setSelectedPlugin] = useState<Plugin | null>(null);
  const [editorCode, setEditorCode] = useState<string>('');
  const [pluginName, setPluginName] = useState<string>('');
  

  const [showCreateModal, setShowCreateModal] = useState<boolean>(false);
  const [newPluginName, setNewPluginName] = useState<string>('');
  const [newPluginTemplateIndex, setNewPluginTemplateIndex] = useState<number>(0);
  

  const [inputPayload, setInputPayload] = useState<string>(DEFAULT_PAYLOAD);
  const [activeTab, setActiveTab] = useState<'console' | 'terminal' | 'analytics'>('console');
  const [isCompiling, setIsCompiling] = useState<boolean>(false);
  const [isRunning, setIsRunning] = useState<boolean>(false);
  const [execResult, setExecResult] = useState<RunResponse | null>(null);
  const [metricsData, setMetricsData] = useState<Execution[]>([]);
  const [backendConnected, setBackendConnected] = useState<boolean>(true);

  const pollingRef = useRef<{ [key: string]: any }>({});


  const fetchPlugins = async (selectFirst = false) => {
    try {
      const res = await fetch(`${API_BASE}/api/plugins`);
      if (!res.ok) throw new Error('API Error');
      const data: Plugin[] = await res.json();
      setPlugins(data);
      setBackendConnected(true);
      
      if (selectFirst && data.length > 0) {
        handleSelectPlugin(data[0]);
      } else if (selectedPlugin) {

        const updated = data.find(p => p.id === selectedPlugin.id);
        if (updated) {
          setSelectedPlugin(updated);
        }
      }
    } catch (err) {
      console.error('Failed to fetch plugins:', err);
      setBackendConnected(false);
    }
  };


  const handleSelectPlugin = (plugin: Plugin) => {
    setSelectedPlugin(plugin);
    setEditorCode(plugin.source_code);
    setPluginName(plugin.name);
    setExecResult(null);
    setMetricsData([]);

    fetchMetrics(plugin.id);
  };


  const fetchMetrics = async (id: string) => {
    try {
      const res = await fetch(`${API_BASE}/api/plugins/${id}/metrics`);
      if (!res.ok) throw new Error('API Error');
      const data: Execution[] = await res.json();

      setMetricsData([...data].reverse());
    } catch (err) {
      console.error('Failed to fetch metrics:', err);
    }
  };


  const handleCreatePlugin = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newPluginName.trim()) return;

    try {
      const template = TEMPLATES[newPluginTemplateIndex];
      const res = await fetch(`${API_BASE}/api/plugins`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: newPluginName.trim(),
          source_code: template.code
        })
      });

      if (!res.ok) throw new Error('Failed to create plugin');
      const created: Plugin = await res.json();
      
      setPlugins(prev => [created, ...prev]);
      setShowCreateModal(false);
      setNewPluginName('');
      handleSelectPlugin(created);
      

      startPolling(created.id);
    } catch (err) {
      alert('Error creating plugin: ' + err);
    }
  };


  const handleSaveAndCompile = async () => {
    if (!selectedPlugin) return;
    setIsCompiling(true);
    setActiveTab('terminal');

    try {
      const res = await fetch(`${API_BASE}/api/plugins/${selectedPlugin.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: pluginName,
          source_code: editorCode
        })
      });

      if (!res.ok) throw new Error('Failed to update plugin');
      const updated: Plugin = await res.json();
      

      setSelectedPlugin(updated);
      setPlugins(prev => prev.map(p => p.id === updated.id ? updated : p));
      

      startPolling(updated.id);
    } catch (err) {
      console.error('Update failed:', err);
      setIsCompiling(false);
    }
  };

  const handleDeletePlugin = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (!confirm('Are you sure you want to delete this plugin and all its execution history?')) return;

    try {
      const res = await fetch(`${API_BASE}/api/plugins/${id}`, {
        method: 'DELETE'
      });

      if (!res.ok) {
        const errorText = await res.text();
        throw new Error(errorText || 'Failed to delete plugin');
      }

      setPlugins(prev => prev.filter(p => p.id !== id));
      if (selectedPlugin?.id === id) {
        setSelectedPlugin(null);
        setEditorCode('');
        setPluginName('');
        setMetricsData([]);
        setExecResult(null);
      }
    } catch (err) {
      alert('Error deleting plugin: ' + err);
    }
  };


  const handleRunExecution = async () => {
    if (!selectedPlugin || selectedPlugin.status !== 'compiled') return;
    setIsRunning(true);
    setActiveTab('console');
    setExecResult(null);

    try {
      const res = await fetch(`${API_BASE}/api/plugins/${selectedPlugin.id}/execute`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ input_json: inputPayload })
      });

      if (!res.ok) {
        const errorText = await res.text();
        throw new Error(errorText || 'Execution failed');
      }

      const data: RunResponse = await res.json();
      setExecResult(data);
      

      fetchMetrics(selectedPlugin.id);
    } catch (err: any) {
      console.error('Execution run failed:', err);
      setExecResult({
        status: 'runtime_error',
        output: '',
        duration_ms: 0,
        memory_bytes: 0,
        fuel_consumed: 0,
        logs: `CRITICAL EXECUTION ERROR:\n${err.message}`
      });
    } finally {
      setIsRunning(false);
    }
  };


  const startPolling = (id: string) => {
    if (pollingRef.current[id]) {
      clearInterval(pollingRef.current[id]);
    }

    const interval = setInterval(async () => {
      try {
        const res = await fetch(`${API_BASE}/api/plugins/${id}`);
        if (!res.ok) return;
        const plugin: Plugin = await res.json();
        
        if (plugin.status !== 'pending') {

          clearInterval(interval);
          delete pollingRef.current[id];
          
          if (selectedPlugin && selectedPlugin.id === id) {
            setSelectedPlugin(plugin);
            setIsCompiling(false);
          }
          setPlugins(prev => prev.map(p => p.id === id ? plugin : p));
        }
      } catch (err) {
        console.error('Polling status error:', err);
      }
    }, 1500);

    pollingRef.current[id] = interval;
  };


  useEffect(() => {
    return () => {
      Object.values(pollingRef.current).forEach(clearInterval);
    };
  }, []);


  useEffect(() => {
    fetchPlugins(true);
    const apiInterval = setInterval(() => {
      fetchPlugins();
    }, 10000);
    return () => clearInterval(apiInterval);
  }, []);


  useEffect(() => {
    if (selectedPlugin?.status === 'pending') {
      setIsCompiling(true);
      startPolling(selectedPlugin.id);
    } else {
      setIsCompiling(false);
    }
  }, [selectedPlugin?.id]);


  const handleLoadTemplate = (code: string) => {
    if (window.confirm('Replace current editor code with template? Any unsaved edits will be lost.')) {
      setEditorCode(code);
    }
  };


  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const mb = bytes / (1024 * 1024);
    return `${mb.toFixed(2)} MB`;
  };


  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'compiled': return <CheckCircle2 className="icon" style={{ color: 'var(--success)' }} />;
      case 'pending': return <RefreshCw className="icon animate-spin" style={{ color: 'var(--warning)' }} />;
      case 'failed': return <AlertTriangle className="icon" style={{ color: 'var(--danger)' }} />;
      default: return <HelpCircle className="icon" />;
    }
  };

  const getExecutionBadgeClass = (status: string) => {
    switch (status) {
      case 'success': return 'badge badge-compiled';
      case 'out_of_fuel': return 'badge badge-failed';
      case 'out_of_memory': return 'badge badge-failed';
      default: return 'badge badge-failed';
    }
  };


  const renderTerminalLog = (log: string) => {
    if (!log) return <div className="terminal-line text-muted">No logs recorded.</div>;
    return log.split('\n').map((line, idx) => {
      let className = 'terminal-line';
      if (line.includes('error:') || line.includes('error[') || line.includes('failed') || line.includes('RUNTIME ERROR:') || line.includes('caused by:')) {
        className += ' terminal-error';
      } else if (line.includes('warning:')) {
        className += ' terminal-warn';
      } else if (line.includes('Finished') || line.includes('Successfully') || line.includes('SUCCESS:')) {
        className += ' terminal-success';
      }
      return (
        <div key={idx} className={className}>
          {line}
        </div>
      );
    });
  };

  return (
    <div className="app-container">
      {}
      <aside className="sidebar" style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <div style={{ padding: '1.5rem', borderBottom: '1px solid var(--border-color)', display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
          <Shield className="icon" style={{ color: 'var(--primary)', width: '28px', height: '28px' }} />
          <div>
            <h1 style={{ fontSize: '1.2rem', fontWeight: 800, letterSpacing: '-0.02em', background: 'linear-gradient(90deg, #f8fafc, #94a3b8)', WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent' }}>
              WASM SANDBOX
            </h1>
            <span style={{ fontSize: '0.65rem', color: 'var(--text-muted)', fontWeight: 600, letterSpacing: '0.1em', textTransform: 'uppercase' }}>
              SaaS Plugin Engine
            </span>
          </div>
        </div>

        {}
        <div style={{ padding: '0.5rem 1.5rem', borderBottom: '1px solid var(--border-color)', display: 'flex', alignItems: 'center', gap: '0.5rem', fontSize: '0.75rem' }}>
          <span className={`badge ${backendConnected ? 'badge-compiled' : 'badge-failed'}`} style={{ width: '8px', height: '8px', padding: 0, borderRadius: '50%' }}></span>
          <span style={{ color: backendConnected ? 'var(--text-secondary)' : 'var(--danger)', fontWeight: 500 }}>
            {backendConnected ? 'Connected to Host API' : 'Host API Disconnected'}
          </span>
          <button 
            onClick={() => fetchPlugins()} 
            style={{ marginLeft: 'auto', background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex', alignItems: 'center' }}
            title="Refresh plugins"
          >
            <RefreshCw size={12} />
          </button>
        </div>

        {}
        <div style={{ padding: '1.25rem 1.5rem' }}>
          <button className="btn btn-primary" style={{ width: '100%', padding: '0.65rem' }} onClick={() => setShowCreateModal(true)}>
            <Plus size={16} />
            New Rust Plugin
          </button>
        </div>

        {}
        <div style={{ flex: 1, overflowY: 'auto', padding: '0 1.5rem 1.5rem' }}>
          <div style={{ fontSize: '0.75rem', fontWeight: 600, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.05em', marginBottom: '0.75rem' }}>
            Active Plugins ({plugins.length})
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
            {plugins.map((plugin) => (
              <div 
                key={plugin.id} 
                className={`plugin-item ${selectedPlugin?.id === plugin.id ? 'active' : ''}`}
                onClick={() => handleSelectPlugin(plugin)}
              >
                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.25rem', flex: 1, minWidth: 0 }}>
                  <div style={{ fontWeight: 600, fontSize: '0.9rem', color: 'var(--text-primary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {plugin.name}
                  </div>
                  <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                    v{plugin.version} • {new Date(plugin.created_at).toLocaleTimeString()}
                  </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.25rem' }}>
                  <span className={`badge badge-${plugin.status}`}>
                    {plugin.status}
                  </span>
                  <button 
                    className="delete-btn" 
                    title="Delete plugin"
                    onClick={(e) => handleDeletePlugin(plugin.id, e)}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            ))}
            {plugins.length === 0 && (
              <div style={{ textAlign: 'center', padding: '2rem 1rem', color: 'var(--text-muted)', fontSize: '0.85rem' }}>
                No plugins found. Create one to begin.
              </div>
            )}
          </div>
        </div>
      </aside>

      {}
      <main className="main-content">
        {}
        <header className="app-header">
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
            <FileCode size={20} style={{ color: 'var(--primary)' }} />
            <input 
              type="text" 
              value={pluginName} 
              onChange={(e) => setPluginName(e.target.value)}
              disabled={!selectedPlugin}
              style={{
                background: 'transparent',
                border: 'none',
                color: 'var(--text-primary)',
                fontSize: '1.25rem',
                fontWeight: 700,
                outline: 'none',
                width: '300px',
                borderBottom: selectedPlugin ? '1px dashed var(--border-color)' : 'none'
              }}
              placeholder="Select or create a plugin..."
            />
          </div>
          {selectedPlugin && (
            <div style={{ display: 'flex', alignItems: 'center', gap: '1rem' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                {getStatusIcon(selectedPlugin.status)}
                <span style={{ fontSize: '0.85rem', fontWeight: 600, textTransform: 'capitalize', color: 'var(--text-secondary)' }}>
                  Compiler Status: {selectedPlugin.status}
                </span>
              </div>
              <button 
                className="btn btn-primary" 
                onClick={handleSaveAndCompile}
                disabled={isCompiling || !backendConnected}
                style={{ padding: '0.5rem 1rem', fontSize: '0.85rem' }}
              >
                <RefreshCw size={14} className={isCompiling ? 'animate-spin' : ''} />
                {isCompiling ? 'Compiling cargo...' : 'Deploy & Recompile'}
              </button>
            </div>
          )}
        </header>

        {selectedPlugin ? (
          <div className="dashboard-grid">
            {}
            <div className="glass-card" style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
              <div className="card-header" style={{ padding: '0.75rem 1.25rem' }}>
                <div className="card-title">
                  <Code2 size={16} />
                  Rust Guest Handler Code
                </div>
                
                {}
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontWeight: 500 }}>Load Demo Template:</span>
                  <select 
                    style={{
                      background: 'rgba(9, 12, 21, 0.8)',
                      color: 'var(--text-secondary)',
                      border: '1px solid var(--border-color)',
                      borderRadius: '6px',
                      padding: '0.25rem 0.5rem',
                      fontSize: '0.75rem',
                      cursor: 'pointer',
                      outline: 'none'
                    }}
                    onChange={(e) => {
                      if (e.target.value !== '') {
                        handleLoadTemplate(e.target.value);
                        e.target.value = '';
                      }
                    }}
                    defaultValue=""
                  >
                    <option value="" disabled>-- Select Template --</option>
                    {TEMPLATES.map((t, idx) => (
                      <option key={idx} value={t.code}>{t.name}</option>
                    ))}
                  </select>
                </div>
              </div>
              
              <div className="card-body" style={{ flex: 1, padding: 0 }}>
                <div className="editor-wrapper" style={{ height: '100%', width: '100%', borderRadius: 0, border: 'none' }}>
                  <Editor
                    height="100%"
                    defaultLanguage="rust"
                    theme="vs-dark"
                    value={editorCode}
                    onChange={(val) => setEditorCode(val || '')}
                    options={{
                      minimap: { enabled: false },
                      fontSize: 13,
                      fontFamily: 'var(--font-mono)',
                      automaticLayout: true,
                      padding: { top: 15, bottom: 15 },
                      scrollbar: {
                        verticalScrollbarSize: 8,
                        horizontalScrollbarSize: 8
                      }
                    }}
                  />
                </div>
              </div>
            </div>

            {}
            <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem', overflowY: 'auto', height: '100%' }}>
              
              {}
              <div className="metrics-row">
                {}
                <div className="metric-card latency">
                  <span className="metric-label">Execution Time</span>
                  <span className="metric-value">
                    <Activity size={18} style={{ color: 'var(--success)', marginRight: '4px' }} />
                    {execResult ? `${execResult.duration_ms.toFixed(2)}` : '0.00'}
                    <span style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginLeft: '2px' }}>ms</span>
                  </span>
                  <span className="metric-sub">
                    {execResult && execResult.duration_ms < 5.0 ? (
                      <span style={{ color: 'var(--success)' }}>Warm Run &lt;5ms Overhead</span>
                    ) : execResult ? (
                      <span style={{ color: 'var(--warning)' }}>Cold compile / heavy logic</span>
                    ) : 'Host target execution overhead'}
                  </span>
                </div>

                {}
                <div className="metric-card memory">
                  <span className="metric-label">Memory Footprint</span>
                  <span className="metric-value">
                    <HardDrive size={18} style={{ color: 'var(--primary)', marginRight: '4px' }} />
                    {execResult ? `${(execResult.memory_bytes / (1024 * 1024)).toFixed(2)}` : '0.00'}
                    <span style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginLeft: '2px' }}>MB</span>
                  </span>
                  <div className="progress-bar-container">
                    <div 
                      className={`progress-bar ${execResult && execResult.memory_bytes > 4 * 1024 * 1024 ? 'danger' : 'success'}`} 
                      style={{ width: execResult ? `${Math.min(100, (execResult.memory_bytes / (5 * 1024 * 1024)) * 100)}%` : '0%' }}
                    ></div>
                  </div>
                  <span className="metric-sub">Limit: 5.00 MB linear heap</span>
                </div>

                {}
                <div className="metric-card cpu">
                  <span className="metric-label">Fuel Metering</span>
                  <span className="metric-value">
                    <Flame size={18} style={{ color: 'var(--warning)', marginRight: '4px' }} />
                    {execResult ? execResult.fuel_consumed.toLocaleString() : '0'}
                    <span style={{ fontSize: '0.7rem', color: 'var(--text-muted)', display: 'block', fontWeight: 500 }}>instructions</span>
                  </span>
                  <span className="metric-sub">Limit: 10,000,000 instructions</span>
                </div>
              </div>

              {}
              <div className="glass-card" style={{ flex: 1, minHeight: '380px', display: 'flex', flexDirection: 'column' }}>
                <div className="tab-container" style={{ padding: 0, paddingLeft: '1rem', background: 'rgba(9, 12, 21, 0.4)' }}>
                  <button 
                    className={`tab-btn ${activeTab === 'console' ? 'active' : ''}`}
                    onClick={() => setActiveTab('console')}
                  >
                    <Play size={14} style={{ marginRight: '6px', display: 'inline' }} />
                    Sandbox Execution Console
                  </button>
                  <button 
                    className={`tab-btn ${activeTab === 'terminal' ? 'active' : ''}`}
                    onClick={() => setActiveTab('terminal')}
                  >
                    <TerminalSquare size={14} style={{ marginRight: '6px', display: 'inline' }} />
                    Cargo Compile Log
                  </button>
                  <button 
                    className={`tab-btn ${activeTab === 'analytics' ? 'active' : ''}`}
                    onClick={() => setActiveTab('analytics')}
                  >
                    <ChartIcon size={14} style={{ marginRight: '6px', display: 'inline' }} />
                    Performance Analytics
                  </button>
                </div>

                <div className="card-body" style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                  {}
                  {activeTab === 'console' && (
                    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', gap: '1rem' }}>
                      {}
                      <div>
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.5rem' }}>
                          <span style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-secondary)' }}>
                            JSON Input Payload (Guest Argument)
                          </span>
                          <button 
                            className="btn btn-success"
                            onClick={handleRunExecution}
                            disabled={isRunning || selectedPlugin.status !== 'compiled'}
                            style={{ padding: '0.35rem 0.85rem', fontSize: '0.75rem', borderRadius: '6px' }}
                          >
                            <Play size={12} />
                            {isRunning ? 'Running sandbox...' : 'Execute Sandbox'}
                          </button>
                        </div>
                        <textarea
                          value={inputPayload}
                          onChange={(e) => setInputPayload(e.target.value)}
                          rows={4}
                          style={{
                            width: '100%',
                            background: '#090c15',
                            border: '1px solid var(--border-color)',
                            borderRadius: '8px',
                            color: '#a5b4fc',
                            fontFamily: 'var(--font-mono)',
                            fontSize: '0.85rem',
                            padding: '0.75rem',
                            outline: 'none',
                            resize: 'none'
                          }}
                        />
                      </div>

                      {}
                      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                        <div style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-secondary)', marginBottom: '0.5rem' }}>
                          Sandbox Engine Output Console
                        </div>
                        
                        <div className="terminal-console" style={{ flex: 1, display: 'flex', flexDirection: 'column', padding: '0.75rem' }}>
                          {execResult ? (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem', height: '100%' }}>
                              <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', borderBottom: '1px solid var(--border-color)', paddingBottom: '0.5rem' }}>
                                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontWeight: 600 }}>STATUS:</span>
                                <span className={getExecutionBadgeClass(execResult.status)}>
                                  {execResult.status}
                                </span>
                              </div>

                              {execResult.output && (
                                <div>
                                  <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontWeight: 600, marginBottom: '0.25rem' }}>GUEST FUNCTION RETURN payload:</div>
                                  <div style={{ background: 'rgba(255,255,255,0.03)', padding: '0.5rem', borderRadius: '6px', border: '1px solid rgba(255,255,255,0.05)', color: '#38bdf8', whiteSpace: 'pre-wrap' }}>
                                    {execResult.output}
                                  </div>
                                </div>
                              )}

                              <div>
                                <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', fontWeight: 600, marginBottom: '0.25rem' }}>STDERR / LOG PIPELINE:</div>
                                <div style={{ paddingLeft: '0.25rem' }}>
                                  {renderTerminalLog(execResult.logs)}
                                </div>
                              </div>
                            </div>
                          ) : (
                            <div style={{ color: 'var(--text-muted)', textAlign: 'center', padding: '3rem 1rem' }}>
                              Click "Execute Sandbox" above to trigger a memory-marshaled WASM invocation.
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  )}

                  {}
                  {activeTab === 'terminal' && (
                    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.5rem' }}>
                        <span style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-secondary)' }}>
                          Cargo compilation logs (`cargo build --target wasm32-wasip1 --release`)
                        </span>
                        {selectedPlugin.status === 'failed' && (
                          <span style={{ color: 'var(--danger)', fontSize: '0.75rem', display: 'flex', alignItems: 'center', gap: '0.25rem', fontWeight: 600 }}>
                            <AlertCircle size={14} /> Compilation Failed
                          </span>
                        )}
                      </div>
                      <div className="terminal-console" style={{ whiteSpace: 'pre-wrap' }}>
                        {selectedPlugin.status === 'pending' ? (
                          <div className="terminal-line terminal-warn animate-pulse">
                            [Cargo compiler worker compiling Wasm plugin target...]
                            Running: cargo build --target wasm32-wasip1 --release
                            Gathering intermediate outputs...
                          </div>
                        ) : selectedPlugin.status === 'failed' ? (
                          renderTerminalLog(selectedPlugin.compile_errors)
                        ) : (
                          <div className="terminal-line terminal-success">
                            Cargo target compile build finished successfully.
                            Target binary output generated: target/wasm32-wasip1/release/wasm_plugin.wasm
                            Bytes size stored in database: v{selectedPlugin.version}
                            Engine preloaded module cached safely. Ready to run!
                          </div>
                        )}
                      </div>
                    </div>
                  )}

                  {}
                  {activeTab === 'analytics' && (
                    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', gap: '0.5rem' }}>
                      <span style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-secondary)' }}>
                        Execution Latency (ms) Historic Telemetry Trace
                      </span>
                      
                      <div style={{ width: '100%', height: '180px', background: 'rgba(9, 12, 21, 0.6)', borderRadius: '12px', border: '1px solid var(--border-color)', padding: '0.75rem' }}>
                        {metricsData.length > 0 ? (
                          <ResponsiveContainer width="100%" height="100%">
                            <AreaChart data={metricsData} margin={{ top: 10, right: 10, left: -25, bottom: 0 }}>
                              <defs>
                                <linearGradient id="colorDuration" x1="0" y1="0" x2="0" y2="1">
                                  <stop offset="5%" stopColor="var(--success)" stopOpacity={0.4}/>
                                  <stop offset="95%" stopColor="var(--success)" stopOpacity={0}/>
                                </linearGradient>
                              </defs>
                              <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.05)" />
                              <XAxis 
                                dataKey="created_at" 
                                stroke="var(--text-muted)" 
                                fontSize={9} 
                                tickFormatter={(tick) => new Date(tick).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                              />
                              <YAxis stroke="var(--text-muted)" fontSize={9} unit="ms" />
                              <Tooltip 
                                contentStyle={{ background: '#0d1127', borderColor: 'var(--border-color)', color: 'var(--text-primary)' }}
                                labelFormatter={(label) => new Date(label).toLocaleString()}
                                formatter={(value: any) => [`${parseFloat(value).toFixed(3)} ms`, 'Execution Latency']}
                              />
                              <Area type="monotone" dataKey="duration_ms" stroke="var(--success)" fillOpacity={1} fill="url(#colorDuration)" />
                            </AreaChart>
                          </ResponsiveContainer>
                        ) : (
                          <div style={{ color: 'var(--text-muted)', textAlign: 'center', paddingTop: '4rem', fontSize: '0.85rem' }}>
                            Execute the Wasm plugin sandbox to generate timeseries telemetry.
                          </div>
                        )}
                      </div>

                      {}
                      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', border: '1px solid var(--border-color)', borderRadius: '8px', background: 'rgba(9, 12, 21, 0.4)' }}>
                        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '0.75rem', textAlign: 'left' }}>
                          <thead>
                            <tr style={{ background: 'rgba(255,255,255,0.03)', borderBottom: '1px solid var(--border-color)', color: 'var(--text-muted)' }}>
                              <th style={{ padding: '0.5rem' }}>Time</th>
                              <th style={{ padding: '0.5rem' }}>Status</th>
                              <th style={{ padding: '0.5rem' }}>Latency</th>
                              <th style={{ padding: '0.5rem' }}>Memory</th>
                              <th style={{ padding: '0.5rem' }}>Instructions</th>
                            </tr>
                          </thead>
                          <tbody>
                            {metricsData.map((m, idx) => (
                              <tr key={m.id || idx} style={{ borderBottom: '1px solid rgba(255,255,255,0.02)', color: 'var(--text-secondary)' }}>
                                <td style={{ padding: '0.5rem' }}>{new Date(m.created_at).toLocaleTimeString()}</td>
                                <td style={{ padding: '0.5rem' }}>
                                  <span className={getExecutionBadgeClass(m.status)} style={{ padding: '0.1rem 0.4rem', fontSize: '0.65rem' }}>
                                    {m.status}
                                  </span>
                                </td>
                                <td style={{ padding: '0.5rem', color: m.duration_ms < 5.0 ? 'var(--success)' : 'var(--warning)', fontWeight: 600 }}>
                                  {m.duration_ms.toFixed(2)} ms
                                </td>
                                <td style={{ padding: '0.5rem' }}>{formatBytes(m.memory_bytes)}</td>
                                <td style={{ padding: '0.5rem' }}>{m.fuel_consumed.toLocaleString()}</td>
                              </tr>
                            ))}
                            {metricsData.length === 0 && (
                              <tr>
                                <td colSpan={5} style={{ textAlign: 'center', padding: '1rem', color: 'var(--text-muted)' }}>No logs recorded.</td>
                              </tr>
                            )}
                          </tbody>
                        </table>
                      </div>

                    </div>
                  )}
                </div>
              </div>

            </div>
          </div>
        ) : (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center', gap: '1rem', color: 'var(--text-muted)' }}>
            <Cpu size={48} className="animate-pulse" style={{ color: 'var(--primary)', opacity: 0.6 }} />
            <h3 style={{ fontSize: '1.25rem', fontWeight: 600, color: 'var(--text-primary)' }}>No Plugin Selected</h3>
            <p style={{ maxWidth: '400px', textAlign: 'center', fontSize: '0.85rem' }}>
              Select an existing Rust plugin from the sidebar or click "New Rust Plugin" to configure a fresh micro-script.
            </p>
          </div>
        )}
      </main>

      {}
      {showCreateModal && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          width: '100vw',
          height: '100vh',
          background: 'rgba(4, 5, 12, 0.85)',
          backdropFilter: 'blur(8px)',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          zIndex: 999
        }}>
          <div className="glass-card" style={{ width: '480px', maxHeight: '90vh' }}>
            <div className="card-header">
              <div className="card-title">
                <Plus size={18} style={{ color: 'var(--primary)' }} />
                Configure New Guest Plugin
              </div>
            </div>
            <form onSubmit={handleCreatePlugin} className="card-body" style={{ gap: '1.25rem' }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                <label style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-secondary)' }}>Plugin Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. currency_converter"
                  value={newPluginName}
                  onChange={(e) => setNewPluginName(e.target.value)}
                  style={{
                    background: '#090c15',
                    border: '1px solid var(--border-color)',
                    borderRadius: '8px',
                    color: 'var(--text-primary)',
                    padding: '0.65rem 0.85rem',
                    fontSize: '0.9rem',
                    outline: 'none'
                  }}
                />
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                <label style={{ fontSize: '0.8rem', fontWeight: 600, color: 'var(--text-secondary)' }}>Starter Template</label>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem', maxHeight: '200px', overflowY: 'auto' }}>
                  {TEMPLATES.map((t, idx) => (
                    <div 
                      key={idx}
                      onClick={() => setNewPluginTemplateIndex(idx)}
                      style={{
                        padding: '0.75rem',
                        border: '1px solid',
                        borderColor: newPluginTemplateIndex === idx ? 'var(--primary)' : 'var(--border-color)',
                        background: newPluginTemplateIndex === idx ? 'rgba(99, 102, 241, 0.1)' : 'rgba(9, 12, 21, 0.4)',
                        borderRadius: '8px',
                        cursor: 'pointer',
                        transition: 'all 0.2s'
                      }}
                    >
                      <div style={{ fontWeight: 600, fontSize: '0.85rem', color: newPluginTemplateIndex === idx ? 'var(--primary)' : 'var(--text-primary)' }}>
                        {t.name}
                      </div>
                      <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                        {t.description}
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              <div style={{ display: 'flex', gap: '1rem', marginTop: '0.5rem', justifyContent: 'flex-end' }}>
                <button 
                  type="button" 
                  className="btn" 
                  style={{ background: 'rgba(255,255,255,0.05)', color: 'var(--text-secondary)' }}
                  onClick={() => setShowCreateModal(false)}
                >
                  Cancel
                </button>
                <button type="submit" className="btn btn-primary">
                  Create & Compile
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
