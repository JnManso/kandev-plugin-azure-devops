package plugin

import (
	"strings"
	"testing"
)

func TestDecodeBodyAcceptsTheLargestDeclaredActionBody(t *testing.T) {
	body := []byte(`{"views":"` + strings.Repeat("x", 500_000) + `"}`)
	var target map[string]any
	if err := decodeBody(body, &target); err != nil {
		t.Fatalf("decodeBody(500 KB) = %v", err)
	}
	if err := decodeBody(make([]byte, maxActionBodyBytes+1), &target); err == nil {
		t.Fatal("decodeBody accepted a body above the largest declared limit")
	}
}
