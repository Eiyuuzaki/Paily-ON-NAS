package clash

import (
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/internal/format"
)

// A malformed stored config must be an error, not a silently empty Config.
// Returning a zero value would drop the template and emit a bare document
// without its proxy-groups and rules, and the subscription itself
// would still look healthy to the operator.
func TestParseConfigRejectsMalformedJSON(t *testing.T) {
	if _, err := parseConfig(format.FormatConfig(`{"template":`)); err == nil {
		t.Fatal("malformed config must return an error")
	}
}

func TestParseConfigAcceptsEmptyBlob(t *testing.T) {
	c, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("nil config must be accepted: %v", err)
	}
	if c.Template != "" {
		t.Fatalf("nil config must have no template, got %q", c.Template)
	}
}

// The generator must propagate the error rather than emit a bare document.
func TestGenerateRejectsMalformedConfig(t *testing.T) {
	var g Generator
	if _, err := g.Generate(format.FormatConfig(`{"template":`)); err == nil {
		t.Fatal("Generate must fail on a malformed config")
	}
}

func TestParseConfigDecodesTemplate(t *testing.T) {
	c, err := parseConfig(format.FormatConfig(`{"template":"fixture"}`))
	if err != nil {
		t.Fatalf("valid config must be accepted: %v", err)
	}
	if c.Template != "fixture" {
		t.Fatalf("template not decoded: %q", c.Template)
	}
}
