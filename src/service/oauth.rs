use crate::error::AppError;
use crate::model::account::CanonicalEnvData;
use crate::tlsfp::make_request_client;
use chrono::{DateTime, Utc};
use serde::Deserialize;
use serde_json::Value;
use tracing::{info, warn};

const OAUTH_TOKEN_URL: &str = "https://platform.claude.com/v1/oauth/token";
const OAUTH_CLIENT_ID: &str = "9d1c250a-e61b-44d9-88ed-5944d1962f5e";
const OAUTH_SCOPES: &[&str] = &[
    "user:profile",
    "user:inference",
    "user:sessions:claude_code",
    "user:mcp_servers",
    "user:file_upload",
];

#[derive(Debug, Clone)]
pub struct RefreshedOAuthTokens {
    pub access_token: String,
    pub refresh_token: String,
    pub expires_at: DateTime<Utc>,
}

#[derive(Deserialize)]
struct OAuthRefreshResponse {
    access_token: String,
    #[serde(default)]
    refresh_token: String,
    #[serde(default)]
    expires_in: i64,
}

/// 通过轻量级 API 调用验证 Setup Token。
pub struct TokenTester;

impl TokenTester {
    pub fn new() -> Self {
        Self
    }

    /// 通过发送最小消息请求验证 Setup Token 有效性。
    pub async fn test_token(
        &self,
        token: &str,
        proxy_url: &str,
        canonical_env: &Value,
    ) -> Result<(), AppError> {
        let env: CanonicalEnvData =
            serde_json::from_value(canonical_env.clone()).unwrap_or_default();
        let version = if env.version.is_empty() {
            crate::config::CLAUDE_CODE_VERSION
        } else {
            &env.version
        };
        let stainless_os = match env.platform.as_str() {
            "darwin" => "Mac OS X",
            "win32" => "Windows",
            _ => "Linux",
        };

        let body = serde_json::json!({
            "model": "claude-haiku-4-5-20251001",
            "max_tokens": 1,
            "messages": [{"role": "user", "content": "hi"}]
        });

        let client = make_request_client(proxy_url);

        let resp = client
            .post("https://api.anthropic.com/v1/messages?beta=true")
            .header("Authorization", format!("Bearer {}", token))
            .header("Content-Type", "application/json")
            .header("Accept", "application/json")
            .header("anthropic-version", "2023-06-01")
            .header("anthropic-beta", "oauth-2025-04-20,interleaved-thinking-2025-05-14,redact-thinking-2026-02-12,context-management-2025-06-27,prompt-caching-scope-2026-01-05")
            .header("anthropic-dangerous-direct-browser-access", "true")
            .header("User-Agent", format!("claude-cli/{} (external, cli)", version))
            .header("x-app", "cli")
            .header("accept-encoding", "br, gzip, deflate")
            .header("accept-language", "*")
            .header("sec-fetch-mode", "cors")
            .header("X-Stainless-Lang", "js")
            .header("X-Stainless-Package-Version", "0.81.0")
            .header("X-Stainless-OS", stainless_os)
            .header("X-Stainless-Arch", &env.arch)
            .header("X-Stainless-Runtime", "node")
            .header("X-Stainless-Runtime-Version", &env.node_version)
            .header("X-Stainless-Retry-Count", "0")
            .header("X-Stainless-Timeout", "600")
            .json(&body)
            .send()
            .await
            .map_err(|e| AppError::Internal(format!("request failed: {:?}", e)))?;

        if resp.status() != 200 {
            let status = resp.status();
            let text = resp.text().await.unwrap_or_default();
            return Err(AppError::Internal(format!(
                "token test failed: status {} {}",
                status, text
            )));
        }
        Ok(())
    }
}

/// 仅显示 token 末 6 位作为追溯标识，不暴露完整密钥。
/// 用于日志，方便人工对照"同一 refresh 流程的多行日志"是否同 token，但不足以重放。
fn token_hint(token: &str) -> String {
    if token.len() < 10 {
        return "***short***".to_string();
    }
    format!("...{}", &token[token.len() - 6..])
}

/// 使用 refresh token 刷新 OAuth access token。
///
/// 含完整 wire 日志（target=oauth）：请求头/响应头/状态/耗时/上游 body 摘要。
/// 用途：帮助分析真实 platform.claude.com 在 refresh 路径上的行为（比如是否
/// 下发 set-cookie / anthropic-organization-id / Vary，是否有 UA 校验等），
/// 在没有真实 CLI refresh 抓包的前提下用 ccb 自己的真实 wire 流量做基线。
pub async fn refresh_oauth_token(
    refresh_token: &str,
    proxy_url: &str,
) -> Result<RefreshedOAuthTokens, AppError> {
    let started = std::time::Instant::now();
    let in_hint = token_hint(refresh_token);

    let client = make_request_client(proxy_url);
    let body = serde_json::json!({
        "grant_type": "refresh_token",
        "refresh_token": refresh_token,
        "client_id": OAUTH_CLIENT_ID,
        "scope": OAUTH_SCOPES.join(" "),
    });
    let body_size = serde_json::to_vec(&body).map(|v| v.len()).unwrap_or(0);

    info!(
        target: "oauth",
        "refresh_start url={} in_token_hint={} req_headers=[content-type=application/json] body_size={}",
        OAUTH_TOKEN_URL,
        in_hint,
        body_size
    );

    let resp = client
        .post(OAUTH_TOKEN_URL)
        .header("Content-Type", "application/json")
        .json(&body)
        .send()
        .await
        .map_err(|e| {
            warn!(
                target: "oauth",
                "refresh_network_error in_token_hint={} elapsed_ms={} err={}",
                in_hint,
                started.elapsed().as_millis(),
                e
            );
            AppError::Internal(format!("oauth refresh request failed: {}", e))
        })?;

    let status = resp.status();
    // 在 consume body 之前 clone headers，便于无论成败都能记日志
    let resp_headers: Vec<(String, String)> = resp
        .headers()
        .iter()
        .map(|(k, v)| {
            (
                k.as_str().to_string(),
                v.to_str().unwrap_or("<binary>").to_string(),
            )
        })
        .collect();
    let resp_body_bytes = resp.bytes().await.map_err(|e| {
        warn!(
            target: "oauth",
            "refresh_body_read_error in_token_hint={} status={} elapsed_ms={} err={}",
            in_hint,
            status,
            started.elapsed().as_millis(),
            e
        );
        AppError::Internal(format!("oauth refresh body read failed: {}", e))
    })?;
    let elapsed_ms = started.elapsed().as_millis();
    let headers_str = resp_headers
        .iter()
        .map(|(k, v)| format!("{}={}", k, v))
        .collect::<Vec<_>>()
        .join(", ");

    if status != 200 {
        // 失败时上游 body 一般是错误消息，全文 log 帮助诊断（不会含密钥）
        let body_str = String::from_utf8_lossy(&resp_body_bytes).to_string();
        warn!(
            target: "oauth",
            "refresh_failure in_token_hint={} status={} elapsed_ms={} resp_body={} resp_headers=[{}]",
            in_hint,
            status,
            elapsed_ms,
            body_str,
            headers_str
        );
        return Err(AppError::Internal(format!(
            "oauth refresh failed: status {} {}",
            status, body_str
        )));
    }

    info!(
        target: "oauth",
        "refresh_response_ok in_token_hint={} status={} elapsed_ms={} resp_body_size={} resp_headers=[{}]",
        in_hint,
        status,
        elapsed_ms,
        resp_body_bytes.len(),
        headers_str
    );

    let data: OAuthRefreshResponse = serde_json::from_slice(&resp_body_bytes)
        .map_err(|e| AppError::Internal(format!("oauth refresh parse failed: {}", e)))?;

    let expires_in = if data.expires_in > 0 {
        data.expires_in
    } else {
        3600
    };
    let expires_at = Utc::now() + chrono::Duration::seconds(expires_in);
    let new_access_hint = token_hint(&data.access_token);
    let new_refresh_hint = if data.refresh_token.is_empty() {
        format!("(unchanged, same as in_token_hint)")
    } else {
        token_hint(&data.refresh_token)
    };

    info!(
        target: "oauth",
        "refresh_success in_token_hint={} new_access_hint={} new_refresh_hint={} expires_in_sec={} expires_at={}",
        in_hint,
        new_access_hint,
        new_refresh_hint,
        expires_in,
        expires_at.to_rfc3339()
    );

    Ok(RefreshedOAuthTokens {
        access_token: data.access_token,
        refresh_token: if data.refresh_token.is_empty() {
            refresh_token.to_string()
        } else {
            data.refresh_token
        },
        expires_at,
    })
}

/// 从 Anthropic OAuth API 获取账号用量数据。
pub async fn fetch_usage(token: &str, proxy_url: &str) -> Result<Value, AppError> {
    let client = make_request_client(proxy_url);

    let resp = client
        .get("https://api.anthropic.com/api/oauth/usage")
        .header("Authorization", format!("Bearer {}", token))
        .header("Accept", "application/json")
        .header("Content-Type", "application/json")
        .header("anthropic-beta", "oauth-2025-04-20")
        .header(
            "User-Agent",
            format!("claude-code/{}", crate::config::CLAUDE_CODE_VERSION),
        )
        .send()
        .await
        .map_err(|e| AppError::Internal(format!("usage request failed: {}", e)))?;

    let status = resp.status();
    if status != 200 {
        let text = resp.text().await.unwrap_or_default();
        return Err(match status.as_u16() {
            401 | 403 => AppError::BadRequest(format!(
                "usage fetch failed: status {} — token may be expired or invalid: {}",
                status, text
            )),
            429 => AppError::TooManyRequests(format!(
                "usage endpoint rate limited (429), try again later: {}",
                text
            )),
            _ => AppError::Internal(format!("usage fetch failed: status {} {}", status, text)),
        });
    }

    let data: Value = resp
        .json()
        .await
        .map_err(|e| AppError::Internal(format!("usage parse failed: {}", e)))?;
    Ok(data)
}
