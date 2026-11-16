package responsevalidation

import "testing"

func TestValidateRenderedStanzas(t *testing.T) {
	valid := "    template IN A . {\n        match \"^app\\.example\\.com\\.$\"\n        answer \"{{ .Name }} 60 IN A 203.0.113.10\"\n        fallthrough\n    }"
	if err := ValidateRenderedStanzas(valid); err != nil {
		t.Fatalf("valid template stanza was rejected: %v", err)
	}
	invalid := "    template IN A . {\n        match \"[\"\n        answer \"{{ .Name }} 60 IN A 203.0.113.10\"\n    }"
	if err := ValidateRenderedStanzas(invalid); err == nil {
		t.Fatal("malformed match regex was accepted")
	}
}
