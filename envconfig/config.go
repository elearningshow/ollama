package envconfig

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidHostPort = errors.New("invalid port specified in OLLAMA_HOST")

// Debug returns true if the OLLAMA_DEBUG environment variable is set to a truthy value.
func Debug() bool {
	if s := getenv("OLLAMA_DEBUG"); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			// non-empty value is truthy
			return true
		}

		return b
	}

	return false
}

// Host returns the scheme and host. Host can be configured via the OLLAMA_HOST environment variable.
// Default is scheme "http" and host "127.0.0.1:11434"
func Host() *url.URL {
	defaultPort := "11434"

	s := os.Getenv("OLLAMA_HOST")
	s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "\"'"))
	scheme, hostport, ok := strings.Cut(s, "://")
	switch {
	case !ok:
		scheme, hostport = "http", s
	case scheme == "http":
		defaultPort = "80"
	case scheme == "https":
		defaultPort = "443"
	}

	// trim trailing slashes
	hostport = strings.TrimRight(hostport, "/")

	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = "127.0.0.1", defaultPort
		if ip := net.ParseIP(strings.Trim(hostport, "[]")); ip != nil {
			host = ip.String()
		} else if hostport != "" {
			host = hostport
		}
	}

	if n, err := strconv.ParseInt(port, 10, 32); err != nil || n > 65535 || n < 0 {
		return &url.URL{
			Scheme: scheme,
			Host:   net.JoinHostPort(host, defaultPort),
		}
	}

	return &url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, port),
	}
}

// Origins returns a list of allowed origins. Origins can be configured via the OLLAMA_ORIGINS environment variable.
func Origins() (origins []string) {
	if s := getenv("OLLAMA_ORIGINS"); s != "" {
		origins = strings.Split(s, ",")
	}

	for _, origin := range []string{"localhost", "127.0.0.1", "0.0.0.0"} {
		origins = append(origins,
			fmt.Sprintf("http://%s", origin),
			fmt.Sprintf("https://%s", origin),
			fmt.Sprintf("http://%s", net.JoinHostPort(origin, "*")),
			fmt.Sprintf("https://%s", net.JoinHostPort(origin, "*")),
		)
	}

	origins = append(origins,
		"app://*",
		"file://*",
		"tauri://*",
	)

	return origins
}

// Models returns the path to the models directory. Models directory can be configured via the OLLAMA_MODELS environment variable.
// Default is $HOME/.ollama/models
func Models() string {
	if s, ok := os.LookupEnv("OLLAMA_MODELS"); ok {
		return s
	}

	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	return filepath.Join(home, ".ollama", "models")
}

// KeepAlive returns the duration that models stay loaded in memory. KeepAlive can be configured via the OLLAMA_KEEP_ALIVE environment variable.
// Negative values are treated as infinite. Zero is treated as no keep alive.
// Default is 5 minutes.
func KeepAlive() (keepAlive time.Duration) {
	keepAlive = 5 * time.Minute
	if s := os.Getenv("OLLAMA_KEEP_ALIVE"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			keepAlive = d
		} else if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			keepAlive = time.Duration(n) * time.Second
		}
	}

	if keepAlive < 0 {
		return time.Duration(math.MaxInt64)
	}

	return keepAlive
}

func Bool(k string) func() bool {
	return func() bool {
		if s := getenv(k); s != "" {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return true
			}

			return b
		}

		return false
	}
}

var (
	// FlashAttention enables the experimental flash attention feature.
	FlashAttention = Bool("OLLAMA_FLASH_ATTENTION")
	// NoHistory disables readline history.
	NoHistory = Bool("OLLAMA_NOHISTORY")
	// NoPrune disables pruning of model blobs on startup.
	NoPrune = Bool("OLLAMA_NOPRUNE")
	// SchedSpread allows scheduling models across all GPUs.
	SchedSpread = Bool("OLLAMA_SCHED_SPREAD")
	// IntelGPU enables experimental Intel GPU detection.
	IntelGPU = Bool("OLLAMA_INTEL_GPU")
)

func String(s string) func() string {
	return func() string {
		return getenv(s)
	}
}

var (
	LLMLibrary = String("OLLAMA_LLM_LIBRARY")
	TmpDir     = String("OLLAMA_TMPDIR")

	CudaVisibleDevices    = String("CUDA_VISIBLE_DEVICES")
	HipVisibleDevices     = String("HIP_VISIBLE_DEVICES")
	RocrVisibleDevices    = String("ROCR_VISIBLE_DEVICES")
	GpuDeviceOrdinal      = String("GPU_DEVICE_ORDINAL")
	HsaOverrideGfxVersion = String("HSA_OVERRIDE_GFX_VERSION")
)

func RunnersDir() (p string) {
	if p := getenv("OLLAMA_RUNNERS_DIR"); p != "" {
		return p
	}

	if runtime.GOOS != "windows" {
		return
	}

	defer func() {
		if p == "" {
			slog.Error("unable to locate llm runner directory. Set OLLAMA_RUNNERS_DIR to the location of 'ollama_runners'")
		}
	}()

	// On Windows we do not carry the payloads inside the main executable
	exe, err := os.Executable()
	if err != nil {
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	var paths []string
	for _, root := range []string{filepath.Dir(exe), cwd} {
		paths = append(paths,
			root,
			filepath.Join(root, "windows-"+runtime.GOARCH),
			filepath.Join(root, "dist", "windows-"+runtime.GOARCH),
		)
	}

	// Try a few variations to improve developer experience when building from source in the local tree
	for _, path := range paths {
		candidate := filepath.Join(path, "ollama_runners")
		if _, err := os.Stat(candidate); err == nil {
			p = candidate
			break
		}
	}

	return p
}

var (
	// Set via OLLAMA_MAX_LOADED_MODELS in the environment
	MaxRunners int
	// Set via OLLAMA_MAX_QUEUE in the environment
	MaxQueuedRequests int
	// Set via OLLAMA_MAX_VRAM in the environment
	MaxVRAM uint64
	// Set via OLLAMA_NUM_PARALLEL in the environment
	NumParallel int
)

type EnvVar struct {
	Name        string
	Value       any
	Description string
}

func AsMap() map[string]EnvVar {
	ret := map[string]EnvVar{
		"OLLAMA_DEBUG":             {"OLLAMA_DEBUG", Debug(), "Show additional debug information (e.g. OLLAMA_DEBUG=1)"},
		"OLLAMA_FLASH_ATTENTION":   {"OLLAMA_FLASH_ATTENTION", FlashAttention(), "Enabled flash attention"},
		"OLLAMA_HOST":              {"OLLAMA_HOST", Host(), "IP Address for the ollama server (default 127.0.0.1:11434)"},
		"OLLAMA_KEEP_ALIVE":        {"OLLAMA_KEEP_ALIVE", KeepAlive(), "The duration that models stay loaded in memory (default \"5m\")"},
		"OLLAMA_LLM_LIBRARY":       {"OLLAMA_LLM_LIBRARY", LLMLibrary(), "Set LLM library to bypass autodetection"},
		"OLLAMA_MAX_LOADED_MODELS": {"OLLAMA_MAX_LOADED_MODELS", MaxRunners, "Maximum number of loaded models per GPU"},
		"OLLAMA_MAX_QUEUE":         {"OLLAMA_MAX_QUEUE", MaxQueuedRequests, "Maximum number of queued requests"},
		"OLLAMA_MAX_VRAM":          {"OLLAMA_MAX_VRAM", MaxVRAM, "Maximum VRAM"},
		"OLLAMA_MODELS":            {"OLLAMA_MODELS", Models(), "The path to the models directory"},
		"OLLAMA_NOHISTORY":         {"OLLAMA_NOHISTORY", NoHistory(), "Do not preserve readline history"},
		"OLLAMA_NOPRUNE":           {"OLLAMA_NOPRUNE", NoPrune(), "Do not prune model blobs on startup"},
		"OLLAMA_NUM_PARALLEL":      {"OLLAMA_NUM_PARALLEL", NumParallel, "Maximum number of parallel requests"},
		"OLLAMA_ORIGINS":           {"OLLAMA_ORIGINS", Origins(), "A comma separated list of allowed origins"},
		"OLLAMA_RUNNERS_DIR":       {"OLLAMA_RUNNERS_DIR", RunnersDir(), "Location for runners"},
		"OLLAMA_SCHED_SPREAD":      {"OLLAMA_SCHED_SPREAD", SchedSpread(), "Always schedule model across all GPUs"},
		"OLLAMA_TMPDIR":            {"OLLAMA_TMPDIR", TmpDir(), "Location for temporary files"},
	}
	if runtime.GOOS != "darwin" {
		ret["CUDA_VISIBLE_DEVICES"] = EnvVar{"CUDA_VISIBLE_DEVICES", CudaVisibleDevices(), "Set which NVIDIA devices are visible"}
		ret["HIP_VISIBLE_DEVICES"] = EnvVar{"HIP_VISIBLE_DEVICES", HipVisibleDevices(), "Set which AMD devices are visible"}
		ret["ROCR_VISIBLE_DEVICES"] = EnvVar{"ROCR_VISIBLE_DEVICES", RocrVisibleDevices(), "Set which AMD devices are visible"}
		ret["GPU_DEVICE_ORDINAL"] = EnvVar{"GPU_DEVICE_ORDINAL", GpuDeviceOrdinal(), "Set which AMD devices are visible"}
		ret["HSA_OVERRIDE_GFX_VERSION"] = EnvVar{"HSA_OVERRIDE_GFX_VERSION", HsaOverrideGfxVersion(), "Override the gfx used for all detected AMD GPUs"}
		ret["OLLAMA_INTEL_GPU"] = EnvVar{"OLLAMA_INTEL_GPU", IntelGPU(), "Enable experimental Intel GPU detection"}
	}
	return ret
}

func Values() map[string]string {
	vals := make(map[string]string)
	for k, v := range AsMap() {
		vals[k] = fmt.Sprintf("%v", v.Value)
	}
	return vals
}

// getenv returns an environment variable stripped of leading and trailing quotes or spaces
func getenv(key string) string {
	return strings.Trim(os.Getenv(key), "\"' ")
}

func init() {
	// default values
	NumParallel = 0 // Autoselect
	MaxRunners = 0  // Autoselect
	MaxQueuedRequests = 512

	LoadConfig()
}

func LoadConfig() {
	userLimit := getenv("OLLAMA_MAX_VRAM")
	if userLimit != "" {
		avail, err := strconv.ParseUint(userLimit, 10, 64)
		if err != nil {
			slog.Error("invalid setting, ignoring", "OLLAMA_MAX_VRAM", userLimit, "error", err)
		} else {
			MaxVRAM = avail
		}
	}

	if onp := getenv("OLLAMA_NUM_PARALLEL"); onp != "" {
		val, err := strconv.Atoi(onp)
		if err != nil {
			slog.Error("invalid setting, ignoring", "OLLAMA_NUM_PARALLEL", onp, "error", err)
		} else {
			NumParallel = val
		}
	}

	maxRunners := getenv("OLLAMA_MAX_LOADED_MODELS")
	if maxRunners != "" {
		m, err := strconv.Atoi(maxRunners)
		if err != nil {
			slog.Error("invalid setting, ignoring", "OLLAMA_MAX_LOADED_MODELS", maxRunners, "error", err)
		} else {
			MaxRunners = m
		}
	}

	if onp := os.Getenv("OLLAMA_MAX_QUEUE"); onp != "" {
		p, err := strconv.Atoi(onp)
		if err != nil || p <= 0 {
			slog.Error("invalid setting, ignoring", "OLLAMA_MAX_QUEUE", onp, "error", err)
		} else {
			MaxQueuedRequests = p
		}
	}
}
