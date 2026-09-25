package mcpkit_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/haakco/mcp-kit/mcpkit"
)

// TestProtocolVersionIsOfferedBySDK guards the kit's modern-only contract: the
// revision mcp-kit targets must still be one the SDK can negotiate.
func TestProtocolVersionIsOfferedBySDK(t *testing.T) {
	if !slices.Contains(mcp.SupportedProtocolVersions(), mcpkit.ProtocolVersion) {
		t.Fatalf("SDK does not support %s; supported: %v", mcpkit.ProtocolVersion, mcp.SupportedProtocolVersions())
	}
}

func TestPrivateCacheMarksResultsPrivate(t *testing.T) {
	setCacheable := mcpkit.PrivateCache(90 * time.Second)

	cacheable := &mcp.Cacheable{CacheScope: "public", TTLMs: 1}
	setCacheable(context.Background(), nil, cacheable)

	if cacheable.CacheScope != "private" {
		t.Fatalf("cacheScope = %q, want private", cacheable.CacheScope)
	}
	if cacheable.TTLMs != 90_000 {
		t.Fatalf("ttlMs = %d, want 90000", cacheable.TTLMs)
	}
}
