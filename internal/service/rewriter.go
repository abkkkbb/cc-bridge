package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	mrand "math/rand/v2"
	"regexp"
	"strings"

	"cc2api/internal/model"
)

// headerWireCasing maps lowercase header keys to their real wire format
// as observed in Claude CLI traffic captures. Go's HTTP server canonicalizes
// all header keys (e.g. "anthropic-beta" → "Anthropic-Beta"); this map
// restores the original casing when forwarding to upstream.
var headerWireCasing = map[string]string{
	"accept":                                     "Accept",
	"user-agent":                                 "User-Agent",
	"x-stainless-retry-count":                    "X-Stainless-Retry-Count",
	"x-stainless-timeout":                        "X-Stainless-Timeout",
	"x-stainless-lang":                           "X-Stainless-Lang",
	"x-stainless-package-version":                "X-Stainless-Package-Version",
	"x-stainless-os":                             "X-Stainless-OS",
	"x-stainless-arch":                           "X-Stainless-Arch",
	"x-stainless-runtime":                        "X-Stainless-Runtime",
	"x-stainless-runtime-version":                "X-Stainless-Runtime-Version",
	"x-stainless-helper-method":                  "x-stainless-helper-method",
	"anthropic-dangerous-direct-browser-access":   "anthropic-dangerous-direct-browser-access",
	"anthropic-version":                           "anthropic-version",
	"anthropic-beta":                              "anthropic-beta",
	"x-app":                                       "x-app",
	"content-type":                                "content-type",
	"accept-language":                             "accept-language",
	"sec-fetch-mode":                              "sec-fetch-mode",
	"accept-encoding":                             "accept-encoding",
	"authorization":                               "authorization",
	"x-claude-code-session-id":                    "X-Claude-Code-Session-Id",
	"x-client-request-id":                         "x-client-request-id",
	"content-length":                              "content-length",
	"x-anthropic-billing-header":                  "x-anthropic-billing-header",
}

// resolveWireCasing converts a Go canonical key to its real wire casing.
func resolveWireCasing(key string) string {
	if wk, ok := headerWireCasing[strings.ToLower(key)]; ok {
		return wk
	}
	return key
}

// Rewriter handles all anti-detection rewriting for requests.
// Two modes: Replace (CC client) and Inject (pure API).
type Rewriter struct{}

func NewRewriter() *Rewriter { return &Rewriter{} }

// ClientType distinguishes request origin.
type ClientType int

const (
	ClientTypeClaudeCode ClientType = iota
	ClientTypeAPI
)

const defaultVersion = "2.1.81"

// mergeAnthropicBeta merges required beta tokens with incoming client beta tokens,
// deduplicating and preserving order (required first, then extras from client).
func mergeAnthropicBeta(required, incoming string) string {
	seen := make(map[string]bool)
	var tokens []string
	for _, t := range strings.Split(required, ",") {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			tokens = append(tokens, t)
		}
	}
	for _, t := range strings.Split(incoming, ",") {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			tokens = append(tokens, t)
		}
	}
	return strings.Join(tokens, ",")
}

// betaHeaderForModel returns the correct anthropic-beta value based on model.
// Haiku models must NOT include claude-code beta.
func betaHeaderForModel(modelID string) string {
	lower := strings.ToLower(modelID)
	if strings.Contains(lower, "haiku") {
		return "oauth-2025-04-20,interleaved-thinking-2025-05-14"
	}
	return "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14"
}

// --- Header rewriting ---

// RewriteHeaders processes outgoing headers for anti-detection.
// Removes hop-by-hop and auth headers, normalizes User-Agent and billing header.
func (rw *Rewriter) RewriteHeaders(headers map[string]string, account *model.Account, clientType ClientType, modelID string) map[string]string {
	env := rw.parseEnv(account)
	version := env.Version
	if version == "" {
		version = defaultVersion
	}

	out := make(map[string]string, len(headers))

	if clientType == ClientTypeAPI {
		// API mode: use a fixed set of headers that match real Claude CLI.
		// Do NOT forward any client headers — they come from browsers/third-party
		// apps and contain detectable fingerprints (Sec-Ch-Ua, Sec-Fetch-*, etc.).
		out["Accept"] = "application/json"
		out["User-Agent"] = fmt.Sprintf("claude-cli/%s (external, cli)", version)
		out["anthropic-beta"] = betaHeaderForModel(modelID)
		out["anthropic-version"] = "2023-06-01"
		out["anthropic-dangerous-direct-browser-access"] = "true"
		out["x-app"] = "cli"
		out["content-type"] = "application/json"
		out["accept-encoding"] = "gzip, deflate, br, zstd"
		out["x-anthropic-billing-header"] = fmt.Sprintf("cc_version=%s.000; cc_entrypoint=cli;", version)
		out["X-Stainless-Lang"] = "js"
		out["X-Stainless-Package-Version"] = "0.70.0"
		out["X-Stainless-OS"] = "Linux"
		out["X-Stainless-Arch"] = "arm64"
		out["X-Stainless-Runtime"] = "node"
		out["X-Stainless-Runtime-Version"] = "v24.13.0"
		out["X-Stainless-Retry-Count"] = "0"
		out["X-Stainless-Timeout"] = "600"
	} else {
		// CC client mode: whitelist + rewrite. The client already sends correct
		// Claude CLI headers, just filter and normalize casing.
		allowedHeaders := map[string]bool{
			"accept": true, "user-agent": true, "content-type": true,
			"accept-encoding": true, "accept-language": true,
			"anthropic-beta": true, "anthropic-version": true,
			"anthropic-dangerous-direct-browser-access": true,
			"x-app": true, "sec-fetch-mode": true,
			"x-stainless-retry-count": true, "x-stainless-timeout": true,
			"x-stainless-lang": true, "x-stainless-package-version": true,
			"x-stainless-os": true, "x-stainless-arch": true,
			"x-stainless-runtime": true, "x-stainless-runtime-version": true,
			"x-stainless-helper-method": true,
			"x-claude-code-session-id": true, "x-client-request-id": true,
			"x-anthropic-billing-header": true,
		}

		for k, v := range headers {
			lower := strings.ToLower(k)
			if !allowedHeaders[lower] {
				continue
			}
			wireKey := resolveWireCasing(k)
			switch lower {
			case "user-agent":
				out[wireKey] = fmt.Sprintf("claude-cli/%s (external, cli)", version)
			case "x-anthropic-billing-header":
				out[wireKey] = rewriteBillingHeader(v, version)
			default:
				out[wireKey] = v
			}
		}

		// Ensure OAuth-required headers are present
		if _, ok := out["anthropic-dangerous-direct-browser-access"]; !ok {
			out["anthropic-dangerous-direct-browser-access"] = "true"
		}
		// Merge client beta with required betas, model-aware
		out["anthropic-beta"] = mergeAnthropicBeta(betaHeaderForModel(modelID), out["anthropic-beta"])
	}

	return out
}

var ccVersionRegex = regexp.MustCompile(`cc_version=[\d.]+\.[a-f0-9]{3}`)

func rewriteBillingHeader(v, version string) string {
	return ccVersionRegex.ReplaceAllString(v, fmt.Sprintf("cc_version=%s.000", version))
}

// --- Body rewriting ---

// RewriteBody rewrites the request body based on endpoint and client type.
func (rw *Rewriter) RewriteBody(body []byte, path string, account *model.Account, clientType ClientType) []byte {
	if len(body) == 0 {
		return body
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body // not JSON, pass through
	}

	switch {
	case strings.HasPrefix(path, "/v1/messages"):
		stripEmptyTextBlocks(parsed)
		rw.rewriteMessages(parsed, account, clientType)
	case strings.Contains(path, "/event_logging/batch"):
		rw.rewriteEventBatch(parsed, account)
	default:
		rw.rewriteGenericIdentity(parsed, account)
	}

	out, err := json.Marshal(parsed)
	if err != nil {
		return body
	}
	return out
}

// rewriteMessages handles /v1/messages body.
func (rw *Rewriter) rewriteMessages(body map[string]any, account *model.Account, clientType ClientType) {
	env := rw.parseEnv(account)
	promptEnv := rw.parsePromptEnv(account)

	if clientType == ClientTypeClaudeCode {
		// Replace mode: rewrite existing metadata.user_id
		rw.rewriteMetadataUserID(body, account)
		// Rewrite system prompt <env> block
		rw.rewriteSystemPrompt(body, promptEnv, env.Version)
	} else {
		// Inject mode: add metadata.user_id if missing
		rw.injectMetadataUserID(body, account)
		// Don't touch system prompt for API calls
	}
}

// rewriteMetadataUserID replaces device_id in existing metadata.user_id (CC client mode).
func (rw *Rewriter) rewriteMetadataUserID(body map[string]any, account *model.Account) {
	metadata, ok := body["metadata"].(map[string]any)
	if !ok {
		return
	}
	userIDStr, ok := metadata["user_id"].(string)
	if !ok || userIDStr == "" {
		return
	}

	// Try JSON format
	var uid map[string]any
	if err := json.Unmarshal([]byte(userIDStr), &uid); err == nil {
		uid["device_id"] = account.DeviceID
		// Preserve account_uuid and session_id
		newBytes, _ := json.Marshal(uid)
		metadata["user_id"] = string(newBytes)
		return
	}

	// Legacy format: user_{device}_account_{uuid}_session_{uuid}
	// Replace device_id portion
	parts := strings.SplitN(userIDStr, "_account_", 2)
	if len(parts) == 2 {
		metadata["user_id"] = "user_" + account.DeviceID + "_account_" + parts[1]
	}
}

// injectMetadataUserID creates metadata.user_id for pure API calls.
func (rw *Rewriter) injectMetadataUserID(body map[string]any, account *model.Account) {
	metadata, ok := body["metadata"].(map[string]any)
	if !ok {
		metadata = make(map[string]any)
		body["metadata"] = metadata
	}

	if _, exists := metadata["user_id"]; exists {
		// Already has user_id, rewrite it instead
		rw.rewriteMetadataUserID(body, account)
		return
	}

	uid := map[string]any{
		"device_id":    account.DeviceID,
		"account_uuid": "",
		"session_id":   generateSessionUUID(),
	}
	uidBytes, _ := json.Marshal(uid)
	metadata["user_id"] = string(uidBytes)
}

// --- System prompt rewriting (CC client mode only) ---

var (
	platformRegex   = regexp.MustCompile(`Platform:\s*\S+`)
	shellRegex      = regexp.MustCompile(`Shell:\s*\S+`)
	osVersionRegex  = regexp.MustCompile(`OS Version:\s*[^\n<]+`)
	workingDirRegex = regexp.MustCompile(`((?:Primary )?[Ww]orking directory:\s*)/\S+`)
	homePathRegex   = regexp.MustCompile(`/(?:Users|home)/[^/\s]+/`)
	promptCCVersion = regexp.MustCompile(`cc_version=[\d.]+\.[a-f0-9]{3}`)
)

func (rw *Rewriter) rewriteSystemPrompt(body map[string]any, pe model.CanonicalPromptEnvData, version string) {
	if version == "" {
		version = defaultVersion
	}

	rewrite := func(text string) string {
		text = platformRegex.ReplaceAllString(text, "Platform: "+pe.Platform)
		text = shellRegex.ReplaceAllString(text, "Shell: "+pe.Shell)
		text = osVersionRegex.ReplaceAllString(text, "OS Version: "+pe.OSVersion)
		text = workingDirRegex.ReplaceAllString(text, "${1}"+pe.WorkingDir)
		homePrefix := pe.WorkingDir
		if idx := nthIndex(pe.WorkingDir, '/', 3); idx > 0 {
			homePrefix = pe.WorkingDir[:idx+1]
		}
		text = homePathRegex.ReplaceAllString(text, homePrefix)
		text = promptCCVersion.ReplaceAllString(text, fmt.Sprintf("cc_version=%s.000", version))
		return text
	}

	// Rewrite body.system (string or array of text blocks)
	switch sys := body["system"].(type) {
	case string:
		body["system"] = rewrite(sys)
	case []any:
		for _, item := range sys {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					block["text"] = rewrite(text)
				}
			}
		}
	}

	// Rewrite messages that may contain <system-reminder> with env info
	if messages, ok := body["messages"].([]any); ok {
		for _, msg := range messages {
			if m, ok := msg.(map[string]any); ok {
				rw.rewriteMessageContent(m, rewrite)
			}
		}
	}
}

func (rw *Rewriter) rewriteMessageContent(msg map[string]any, rewriteFn func(string) string) {
	switch content := msg["content"].(type) {
	case string:
		msg["content"] = rewriteFn(content)
	case []any:
		for _, item := range content {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					block["text"] = rewriteFn(text)
				}
			}
		}
	}
}

// --- Event logging batch rewriting ---

func (rw *Rewriter) rewriteEventBatch(body map[string]any, account *model.Account) {
	env := rw.parseEnv(account)
	proc := rw.parseProcess(account)

	events, ok := body["events"].([]any)
	if !ok {
		return
	}

	canonicalEnv := buildCanonicalEnvMap(env)

	for _, event := range events {
		e, ok := event.(map[string]any)
		if !ok {
			continue
		}

		// Replace identity fields
		if _, ok := e["device_id"]; ok {
			e["device_id"] = account.DeviceID
		}
		if _, ok := e["email"]; ok {
			e["email"] = account.Email
		}

		// Delete proxy traces
		delete(e, "baseUrl")
		delete(e, "base_url")
		delete(e, "gateway")

		// Replace env object entirely
		if _, ok := e["env"]; ok {
			e["env"] = canonicalEnv
		}

		// Replace process data
		if p, ok := e["process"]; ok {
			e["process"] = rw.rewriteProcess(p, proc)
		}

		// Rewrite additional_metadata (base64 encoded)
		if am, ok := e["additional_metadata"].(string); ok {
			e["additional_metadata"] = rewriteAdditionalMetadata(am)
		}
	}
}

func buildCanonicalEnvMap(env model.CanonicalEnvData) map[string]any {
	return map[string]any{
		"platform":                 env.Platform,
		"platform_raw":            env.PlatformRaw,
		"arch":                    env.Arch,
		"node_version":            env.NodeVersion,
		"terminal":                env.Terminal,
		"package_managers":        env.PackageManagers,
		"runtimes":                env.Runtimes,
		"is_running_with_bun":     false,
		"is_ci":                   false,
		"is_claubbit":             false,
		"is_claude_code_remote":   false,
		"is_local_agent_mode":     false,
		"is_conductor":            false,
		"is_github_action":        false,
		"is_claude_code_action":   false,
		"is_claude_ai_auth":       env.IsClaudeAIAuth,
		"version":                 env.Version,
		"version_base":            env.VersionBase,
		"build_time":              env.BuildTime,
		"deployment_environment":  env.DeploymentEnvironment,
		"vcs":                     env.VCS,
	}
}

// --- Process (hardware) fingerprint rewriting ---

func (rw *Rewriter) rewriteProcess(original any, proc model.CanonicalProcessData) any {
	// Process can be base64-encoded JSON string or object
	switch p := original.(type) {
	case string:
		decoded, err := base64.StdEncoding.DecodeString(p)
		if err != nil {
			return original
		}
		var obj map[string]any
		if err := json.Unmarshal(decoded, &obj); err != nil {
			return original
		}
		rewriteProcessFields(obj, proc)
		out, _ := json.Marshal(obj)
		return base64.StdEncoding.EncodeToString(out)
	case map[string]any:
		rewriteProcessFields(p, proc)
		return p
	default:
		return original
	}
}

func rewriteProcessFields(obj map[string]any, proc model.CanonicalProcessData) {
	obj["constrainedMemory"] = proc.ConstrainedMemory
	obj["rss"] = randomInRange(proc.RSSRange[0], proc.RSSRange[1])
	obj["heapTotal"] = randomInRange(proc.HeapTotalRange[0], proc.HeapTotalRange[1])
	obj["heapUsed"] = randomInRange(proc.HeapUsedRange[0], proc.HeapUsedRange[1])
}

// --- Generic identity rewriting (policy_limits, settings, etc.) ---

func (rw *Rewriter) rewriteGenericIdentity(body map[string]any, account *model.Account) {
	if _, ok := body["device_id"]; ok {
		body["device_id"] = account.DeviceID
	}
	if _, ok := body["email"]; ok {
		body["email"] = account.Email
	}
}

// --- Base64 additional_metadata rewriting ---

func rewriteAdditionalMetadata(encoded string) string {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return encoded
	}
	var obj map[string]any
	if err := json.Unmarshal(decoded, &obj); err != nil {
		return encoded
	}
	delete(obj, "baseUrl")
	delete(obj, "base_url")
	delete(obj, "gateway")
	out, _ := json.Marshal(obj)
	return base64.StdEncoding.EncodeToString(out)
}

// --- Helpers ---

func (rw *Rewriter) parseEnv(account *model.Account) model.CanonicalEnvData {
	var env model.CanonicalEnvData
	_ = json.Unmarshal(account.CanonicalEnv, &env)
	return env
}

func (rw *Rewriter) parsePromptEnv(account *model.Account) model.CanonicalPromptEnvData {
	var pe model.CanonicalPromptEnvData
	_ = json.Unmarshal(account.CanonicalPrompt, &pe)
	return pe
}

func (rw *Rewriter) parseProcess(account *model.Account) model.CanonicalProcessData {
	var proc model.CanonicalProcessData
	_ = json.Unmarshal(account.CanonicalProcess, &proc)
	return proc
}

func randomInRange(min, max int64) int64 {
	if max <= min {
		return min
	}
	return min + mrand.Int64N(max-min)
}

func generateSessionUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func nthIndex(s string, c byte, n int) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			count++
			if count == n {
				return i
			}
		}
	}
	return -1
}

// stripEmptyTextBlocks removes empty text content blocks ({"type":"text","text":""})
// from messages and system to prevent upstream 400 errors.
func stripEmptyTextBlocks(body map[string]any) {
	var filterBlocks func([]any) []any
	filterBlocks = func(blocks []any) []any {
		result := make([]any, 0, len(blocks))
		for _, item := range blocks {
			block, ok := item.(map[string]any)
			if !ok {
				result = append(result, item)
				continue
			}
			if block["type"] == "text" {
				text, _ := block["text"].(string)
				if text == "" {
					continue // skip empty text block
				}
			}
			// Also check nested content in tool_result blocks
			if block["type"] == "tool_result" {
				if content, ok := block["content"].([]any); ok {
					block["content"] = filterBlocks(content)
				}
			}
			result = append(result, item)
		}
		return result
	}

	// Filter system blocks
	if sys, ok := body["system"].([]any); ok {
		body["system"] = filterBlocks(sys)
	}

	// Filter message content blocks
	if messages, ok := body["messages"].([]any); ok {
		for _, msg := range messages {
			m, ok := msg.(map[string]any)
			if !ok {
				continue
			}
			if content, ok := m["content"].([]any); ok {
				m["content"] = filterBlocks(content)
			}
		}
	}
}

// DetectClientType determines if request is from Claude Code or pure API.
func DetectClientType(userAgent string, body map[string]any) ClientType {
	if strings.HasPrefix(strings.ToLower(userAgent), "claude-cli/") {
		return ClientTypeClaudeCode
	}
	// Check for metadata.user_id presence (CC client marker)
	if metadata, ok := body["metadata"].(map[string]any); ok {
		if _, ok := metadata["user_id"]; ok {
			return ClientTypeClaudeCode
		}
	}
	return ClientTypeAPI
}

