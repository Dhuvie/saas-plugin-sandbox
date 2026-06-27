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

fn handler(input: &str) -> String {
    if let Ok(mut val) = serde_json::from_str::<serde_json::Value>(input) {
        if let Some(obj) = val.as_object_mut() {
            obj.insert("status".to_string(), serde_json::Value::String("premium".to_string()));
            if let Some(points) = obj.get("points").and_then(|p| p.as_i64()) {
                obj.insert("tier".to_string(), serde_json::Value::String(
                    if points > 500 { "gold".to_string() } else { "silver".to_string() }
                ));
            }
        }
        serde_json::to_string(&val).unwrap_or_else(|_| "{}".to_string())
    } else {
        r#"{"error": "Invalid JSON"}"#.to_string()
    }
}
