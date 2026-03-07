package policy

import (
	"testing"

	"webterm/internal/config"
)

func TestAllowDisabled(t *testing.T) {
	engine, err := New(config.PolicyConfig{WhitelistEnabled: false})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	allowed, _ := engine.Allow("rm -rf /")
	if !allowed {
		t.Fatalf("expected command allowed when policy disabled")
	}
}

func TestAllowExactPrefixRegex(t *testing.T) {
	engine, err := New(config.PolicyConfig{
		WhitelistEnabled: true,
		Rules: []config.PolicyRule{
			{Type: "exact", Pattern: "ls"},
			{Type: "prefix", Pattern: "git "},
			{Type: "regex", Pattern: `^kubectl(\s+.*)?$`},
		},
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if allowed, _ := engine.Allow("ls"); !allowed {
		t.Fatalf("expected exact match allowed")
	}
	if allowed, _ := engine.Allow("git status"); !allowed {
		t.Fatalf("expected prefix match allowed")
	}
	if allowed, _ := engine.Allow("kubectl get pods"); !allowed {
		t.Fatalf("expected regex match allowed")
	}
	if allowed, _ := engine.Allow("docker ps"); allowed {
		t.Fatalf("expected non-matching command denied")
	}
}

func TestAllowEmptyCommand(t *testing.T) {
	engine, err := New(config.PolicyConfig{WhitelistEnabled: true})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	allowed, _ := engine.Allow("   ")
	if !allowed {
		t.Fatalf("expected empty command allowed")
	}
}

func TestPolicyRejectsEmptyPattern(t *testing.T) {
	_, err := New(config.PolicyConfig{WhitelistEnabled: true, Rules: []config.PolicyRule{{Type: "exact", Pattern: ""}}})
	if err == nil {
		t.Fatalf("expected empty pattern error")
	}
}

func TestPolicyRejectsInvalidRegex(t *testing.T) {
	_, err := New(config.PolicyConfig{WhitelistEnabled: true, Rules: []config.PolicyRule{{Type: "regex", Pattern: "["}}})
	if err == nil {
		t.Fatalf("expected invalid regex error")
	}
}

func TestPolicyRejectsUnknownType(t *testing.T) {
	_, err := New(config.PolicyConfig{WhitelistEnabled: true, Rules: []config.PolicyRule{{Type: "unknown", Pattern: "ls"}}})
	if err == nil {
		t.Fatalf("expected unknown type error")
	}
}

func TestEnabledFlag(t *testing.T) {
	engine, err := New(config.PolicyConfig{WhitelistEnabled: true})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if !engine.Enabled() {
		t.Fatalf("expected enabled")
	}
}
