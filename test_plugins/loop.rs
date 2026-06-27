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
}
