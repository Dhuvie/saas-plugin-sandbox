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
}
