package policy

import (
	"fmt"
	"regexp"
	"strings"

	"webterm/internal/config"
)

type RuleType string

const (
	RuleExact  RuleType = "exact"
	RulePrefix RuleType = "prefix"
	RuleRegex  RuleType = "regex"
)

type Rule struct {
	Type    RuleType
	Pattern string
	regex   *regexp.Regexp
}

type Engine struct {
	enabled bool
	rules   []Rule
}

func New(cfg config.PolicyConfig) (*Engine, error) {
	engine := &Engine{enabled: cfg.WhitelistEnabled}
	for _, input := range cfg.Rules {
		r := Rule{Type: RuleType(strings.ToLower(strings.TrimSpace(input.Type))), Pattern: input.Pattern}
		switch r.Type {
		case RuleExact, RulePrefix:
			if strings.TrimSpace(r.Pattern) == "" {
				return nil, fmt.Errorf("policy rule pattern cannot be empty")
			}
		case RuleRegex:
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid regex rule %q: %w", r.Pattern, err)
			}
			r.regex = re
		default:
			return nil, fmt.Errorf("unknown rule type %q", input.Type)
		}
		engine.rules = append(engine.rules, r)
	}

	return engine, nil
}

func (e *Engine) Enabled() bool {
	return e.enabled
}

func (e *Engine) Allow(command string) (bool, string) {
	if !e.enabled {
		return true, ""
	}

	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return true, ""
	}

	for _, rule := range e.rules {
		switch rule.Type {
		case RuleExact:
			if trimmed == rule.Pattern {
				return true, string(rule.Type) + ":" + rule.Pattern
			}
		case RulePrefix:
			if strings.HasPrefix(trimmed, rule.Pattern) {
				return true, string(rule.Type) + ":" + rule.Pattern
			}
		case RuleRegex:
			if rule.regex != nil && rule.regex.MatchString(trimmed) {
				return true, string(rule.Type) + ":" + rule.Pattern
			}
		}
	}

	return false, ""
}
