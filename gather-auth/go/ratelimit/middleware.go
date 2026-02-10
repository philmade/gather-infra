package ratelimit

import (
	"net"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// CheckIP checks the PublicRead limiter for the given IP.
// Returns a 429 huma error if over limit, nil otherwise.
func CheckIP(ip string) error {
	if !PublicRead.Allow(ip) {
		return huma.Error429TooManyRequests("Rate limit exceeded. Try again shortly.")
	}
	return nil
}

// CheckAgent checks the appropriate write limiter based on verified status.
// verified=true uses the higher-limit tier.
func CheckAgent(agentID string, verified bool) error {
	if verified {
		if !AuthWriteVerified.Allow(agentID) {
			return huma.Error429TooManyRequests("Rate limit exceeded. Try again shortly.")
		}
	} else {
		if !AuthWrite.Allow(agentID) {
			return huma.Error429TooManyRequests("Rate limit exceeded. Try again shortly.")
		}
	}
	return nil
}

// CheckDesignUpload checks the design upload limiter based on verified status.
func CheckDesignUpload(agentID string, verified bool) error {
	if verified {
		if !DesignUploadVerified.Allow(agentID) {
			return huma.Error429TooManyRequests("Design upload rate limit exceeded. Try again shortly.")
		}
	} else {
		if !DesignUpload.Allow(agentID) {
			return huma.Error429TooManyRequests("Design upload rate limit exceeded. Try again shortly.")
		}
	}
	return nil
}

// IPRateLimitMiddleware returns a Huma middleware that rate-limits all requests by client IP.
func IPRateLimitMiddleware(ctx huma.Context, next func(huma.Context)) {
	ip := clientIP(ctx)
	if !PublicRead.Allow(ip) {
		ctx.SetStatus(429)
		ctx.BodyWriter().Write([]byte(`{"title":"Too Many Requests","status":429,"detail":"Rate limit exceeded. Try again shortly."}`))
		return
	}
	next(ctx)
}

// clientIP extracts the client IP from X-Real-IP (set by nginx to $remote_addr, not spoofable).
// Falls back to RemoteAddr if X-Real-IP is absent (direct access without proxy).
func clientIP(ctx huma.Context) string {
	if realIP := ctx.Header("X-Real-IP"); realIP != "" {
		return strings.TrimSpace(realIP)
	}
	// Fall back to RemoteAddr, strip port
	addr := ctx.RemoteAddr()
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
