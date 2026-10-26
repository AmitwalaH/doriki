package manifest

// envAllowlist holds the prefixes of the environment variables a manifest may capture
// (DESIGN.md section 4.3; PROJECT.md section 6 names this file as their one home). A variable
// outside them is dropped, value and all. It is a variable so a test can swap in a sentinel
// and prove nothing else decides.
var envAllowlist = []string{"VLLM_", "HF_", "SGLANG_", "OLLAMA_", "LLAMA_", "CUDA_", "NCCL_"}
