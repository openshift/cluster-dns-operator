package responsevalidation

import (
	"fmt"

	"github.com/coredns/caddy"
	_ "github.com/coredns/coredns/plugin/template"
)

// ValidateRenderedStanzas runs CoreDNS's template setup against only the
// response-rule stanzas, before they can replace the live Corefile.
func ValidateRenderedStanzas(stanzas string) error {
	if stanzas == "" {
		return nil
	}
	input := caddy.CaddyfileInput{
		Filepath:       "response-rules.Corefile",
		Contents:       []byte(".:5353 {\n" + stanzas + "\n}\n"),
		ServerTypeName: "dns",
	}
	if err := caddy.ValidateAndExecuteDirectives(input, nil, true); err != nil {
		return fmt.Errorf("CoreDNS template validation failed: %w", err)
	}
	return nil
}
