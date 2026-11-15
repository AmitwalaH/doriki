package manifest

// envAllowlist holds the environment variables a manifest may capture (DESIGN.md section 4.3;
// PROJECT.md section 6 names this file as their one home). An entry ending in "_" is a prefix;
// any other entry is one exact name, for a variable an engine reads outside the prefixes:
// MODEL_ENDPOINT (llama.cpp's mirror) and TRANSFORMERS_OFFLINE (huggingface_hub's offline
// switch), both read by ModelSource. A variable outside the list is dropped, value and all. It
// is a variable so a test can swap in a sentinel and prove nothing else decides.
var envAllowlist = []string{"VLLM_", "HF_", "SGLANG_", "OLLAMA_", "LLAMA_", "CUDA_", "NCCL_",
	"MODEL_ENDPOINT", "TRANSFORMERS_OFFLINE"}
