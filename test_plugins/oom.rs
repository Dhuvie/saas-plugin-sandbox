fn handler(_input: &str) -> String {
    let mut chunks = Vec::new();
    println!("OOM Test: Starting allocation loop...");
    
    for i in 1..=20 {
        let chunk = vec![0u8; 1024 * 1024];
        println!("OOM Test: Successfully allocated chunk {}", i);
        
        chunks.push(chunk);
        std::hint::black_box(&chunks);
    }
    
    format!("SUCCESS: Allocated {} MB!", chunks.len())
}
