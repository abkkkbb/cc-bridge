package model

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mrand "math/rand/v2"
)

// Presets for auto-generating canonical identity on account creation.

var envPresets = []CanonicalEnvData{
	{
		Platform: "darwin", PlatformRaw: "darwin", Arch: "arm64",
		NodeVersion: "v24.3.0", Terminal: "iTerm2.app",
		PackageManagers: "npm,pnpm", Runtimes: "node",
		IsClaudeAIAuth: true, Version: "2.1.81", VersionBase: "2.1.81",
		BuildTime: "2026-03-20T21:26:18Z", DeploymentEnvironment: "unknown-darwin", VCS: "git",
	},
	{
		Platform: "darwin", PlatformRaw: "darwin", Arch: "x64",
		NodeVersion: "v22.15.0", Terminal: "Terminal",
		PackageManagers: "npm,yarn", Runtimes: "node",
		IsClaudeAIAuth: true, Version: "2.1.81", VersionBase: "2.1.81",
		BuildTime: "2026-03-20T21:26:18Z", DeploymentEnvironment: "unknown-darwin", VCS: "git",
	},
	{
		Platform: "linux", PlatformRaw: "linux", Arch: "x64",
		NodeVersion: "v24.3.0", Terminal: "xterm-256color",
		PackageManagers: "npm,pnpm", Runtimes: "node",
		IsClaudeAIAuth: true, Version: "2.1.81", VersionBase: "2.1.81",
		BuildTime: "2026-03-20T21:26:18Z", DeploymentEnvironment: "unknown-linux", VCS: "git",
	},
}

var promptPresets = map[string]CanonicalPromptEnvData{
	"darwin": {Platform: "darwin", Shell: "zsh", OSVersion: "Darwin 24.4.0", WorkingDir: "/Users/user/projects"},
	"linux":  {Platform: "linux", Shell: "bash", OSVersion: "Linux 6.5.0-generic", WorkingDir: "/home/user/projects"},
}

var memoryPresets = []int64{
	17179869184,  // 16GB
	34359738368,  // 32GB
	68719476736,  // 64GB
}

// GenerateDeviceID creates a random 64-char hex string.
func GenerateDeviceID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

// GenerateCanonicalIdentity generates all canonical fields for a new account.
func GenerateCanonicalIdentity() (deviceID string, env json.RawMessage, prompt json.RawMessage, process json.RawMessage) {
	deviceID = GenerateDeviceID()

	// Pick random env preset
	preset := envPresets[mrand.IntN(len(envPresets))]
	envBytes, _ := json.Marshal(preset)
	env = envBytes

	// Match prompt to platform
	pe := promptPresets[preset.Platform]
	promptBytes, _ := json.Marshal(pe)
	prompt = promptBytes

	// Random hardware
	mem := memoryPresets[mrand.IntN(len(memoryPresets))]
	proc := CanonicalProcessData{
		ConstrainedMemory: mem,
		RSSRange:          [2]int64{300_000_000, 500_000_000},
		HeapTotalRange:    [2]int64{40_000_000, 80_000_000},
		HeapUsedRange:     [2]int64{100_000_000, 200_000_000},
	}
	procBytes, _ := json.Marshal(proc)
	process = procBytes

	return
}
