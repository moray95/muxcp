package gateway

import (
	"sort"
	"testing"
)

func TestBuildEnv_Nil(t *testing.T) {
	t.Parallel()

	result := buildEnv(nil)
	if result != nil {
		t.Errorf("buildEnv(nil) = %v, want nil", result)
	}
}

func TestBuildEnv_Empty(t *testing.T) {
	t.Parallel()

	result := buildEnv(map[string]string{})
	if result != nil {
		t.Errorf("buildEnv(empty) = %v, want nil", result)
	}
}

func TestBuildEnv_WithVars(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"FOO": "bar",
		"BAZ": "qux",
	}
	result := buildEnv(env)

	if len(result) != 2 {
		t.Fatalf("buildEnv returned %d items, want 2", len(result))
	}

	// Sort for deterministic comparison
	sort.Strings(result)
	if result[0] != "BAZ=qux" {
		t.Errorf("result[0] = %q, want \"BAZ=qux\"", result[0])
	}
	if result[1] != "FOO=bar" {
		t.Errorf("result[1] = %q, want \"FOO=bar\"", result[1])
	}
}

func TestBuildEnv_SpecialChars(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"URL": "https://example.com?a=1&b=2",
	}
	result := buildEnv(env)

	if len(result) != 1 {
		t.Fatalf("buildEnv returned %d items, want 1", len(result))
	}
	if result[0] != "URL=https://example.com?a=1&b=2" {
		t.Errorf("result[0] = %q, want URL with special chars preserved", result[0])
	}
}

func TestBuildSSEOpts_Nil(t *testing.T) {
	t.Parallel()

	opts := buildSSEOpts(nil)
	if len(opts) != 0 {
		t.Errorf("buildSSEOpts(nil) returned %d opts, want 0", len(opts))
	}
}

func TestBuildSSEOpts_Empty(t *testing.T) {
	t.Parallel()

	opts := buildSSEOpts(map[string]string{})
	if len(opts) != 0 {
		t.Errorf("buildSSEOpts(empty) returned %d opts, want 0", len(opts))
	}
}

func TestBuildSSEOpts_WithHeaders(t *testing.T) {
	t.Parallel()

	opts := buildSSEOpts(map[string]string{
		"Authorization": "Bearer token",
	})
	if len(opts) != 1 {
		t.Errorf("buildSSEOpts returned %d opts, want 1", len(opts))
	}
}

func TestBuildStreamableHTTPOpts_Nil(t *testing.T) {
	t.Parallel()

	opts := buildStreamableHTTPOpts(nil)
	if len(opts) != 0 {
		t.Errorf("buildStreamableHTTPOpts(nil) returned %d opts, want 0", len(opts))
	}
}

func TestBuildStreamableHTTPOpts_Empty(t *testing.T) {
	t.Parallel()

	opts := buildStreamableHTTPOpts(map[string]string{})
	if len(opts) != 0 {
		t.Errorf("buildStreamableHTTPOpts(empty) returned %d opts, want 0", len(opts))
	}
}

func TestBuildStreamableHTTPOpts_WithHeaders(t *testing.T) {
	t.Parallel()

	opts := buildStreamableHTTPOpts(map[string]string{
		"Authorization": "Bearer token",
		"X-Tenant":      "org1",
	})
	if len(opts) != 1 {
		t.Errorf("buildStreamableHTTPOpts returned %d opts, want 1", len(opts))
	}
}

func TestBackendClose_NilClient(t *testing.T) {
	t.Parallel()

	b := &Backend{Name: "test", Client: nil}
	err := b.Close()
	if err != nil {
		t.Errorf("Close() with nil client should return nil, got %v", err)
	}
}
