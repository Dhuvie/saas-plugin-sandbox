fn handler(_input: &str) -> String {
    match std::fs::read_to_string("C:\\Windows\\system.ini") {
        Ok(content) => format!("SUCCESS: Read file content (length: {})", content.len()),
        Err(e) => format!("BLOCKED: Failed to read file: {}", e),
    }
}
