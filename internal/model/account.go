package model

import (
	"encoding/json"
	"time"
)

type AccountStatus string

const (
	AccountStatusActive   AccountStatus = "active"
	AccountStatusError    AccountStatus = "error"
	AccountStatusDisabled AccountStatus = "disabled"
)

type Account struct {
	ID        int64         `json:"id" db:"id"`
	Name      string        `json:"name" db:"name"`
	Email     string        `json:"email" db:"email"`
	Status    AccountStatus `json:"status" db:"status"`

	Token    string `json:"token" db:"token"`
	ProxyURL     string `json:"proxy_url,omitempty" db:"proxy_url"`

	// Auto-generated canonical identity (stored as JSON)
	DeviceID         string          `json:"device_id" db:"device_id"`
	CanonicalEnv     json.RawMessage `json:"canonical_env" db:"canonical_env"`
	CanonicalPrompt  json.RawMessage `json:"canonical_prompt_env" db:"canonical_prompt_env"`
	CanonicalProcess json.RawMessage `json:"canonical_process" db:"canonical_process"`

	Concurrency int `json:"concurrency" db:"concurrency"`
	Priority    int `json:"priority" db:"priority"`

	RateLimitedAt    *time.Time `json:"rate_limited_at,omitempty" db:"rate_limited_at"`
	RateLimitResetAt *time.Time `json:"rate_limit_reset_at,omitempty" db:"rate_limit_reset_at"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

func (a *Account) IsSchedulable() bool {
	if a.Status != AccountStatusActive {
		return false
	}
	if a.RateLimitResetAt != nil && time.Now().Before(*a.RateLimitResetAt) {
		return false
	}
	return true
}

// CanonicalEnvData holds the 19+ env dimensions for anti-detection.
type CanonicalEnvData struct {
	Platform               string `json:"platform"`
	PlatformRaw            string `json:"platform_raw"`
	Arch                   string `json:"arch"`
	NodeVersion            string `json:"node_version"`
	Terminal               string `json:"terminal"`
	PackageManagers        string `json:"package_managers"`
	Runtimes               string `json:"runtimes"`
	IsRunningWithBun       bool   `json:"is_running_with_bun"`
	IsCI                   bool   `json:"is_ci"`
	IsClaubbit             bool   `json:"is_claubbit"`
	IsClaudeCodeRemote     bool   `json:"is_claude_code_remote"`
	IsLocalAgentMode       bool   `json:"is_local_agent_mode"`
	IsConductor            bool   `json:"is_conductor"`
	IsGitHubAction         bool   `json:"is_github_action"`
	IsClaudeCodeAction     bool   `json:"is_claude_code_action"`
	IsClaudeAIAuth         bool   `json:"is_claude_ai_auth"`
	Version                string `json:"version"`
	VersionBase            string `json:"version_base"`
	BuildTime              string `json:"build_time"`
	DeploymentEnvironment  string `json:"deployment_environment"`
	VCS                    string `json:"vcs"`
}

// CanonicalPromptEnvData holds prompt-level environment rewrite data.
type CanonicalPromptEnvData struct {
	Platform   string `json:"platform"`
	Shell      string `json:"shell"`
	OSVersion  string `json:"os_version"`
	WorkingDir string `json:"working_dir"`
}

// CanonicalProcessData holds hardware fingerprint config.
type CanonicalProcessData struct {
	ConstrainedMemory int64    `json:"constrained_memory"`
	RSSRange          [2]int64 `json:"rss_range"`
	HeapTotalRange    [2]int64 `json:"heap_total_range"`
	HeapUsedRange     [2]int64 `json:"heap_used_range"`
}
