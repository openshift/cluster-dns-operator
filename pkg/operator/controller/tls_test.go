package controller

import (
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func TestTLSGroupToCurveID(t *testing.T) {
	tests := []struct {
		group  configv1.TLSGroup
		wantID tls.CurveID
		wantOK bool
	}{
		{configv1.TLSGroupX25519, tls.X25519, true},
		{configv1.TLSGroupSecP256r1, tls.CurveP256, true},
		{configv1.TLSGroupSecP384r1, tls.CurveP384, true},
		{configv1.TLSGroupSecP521r1, tls.CurveP521, true},
		{configv1.TLSGroupX25519MLKEM768, tls.X25519MLKEM768, true},
		{"UnknownGroup", 0, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.group), func(t *testing.T) {
			id, ok := TLSGroupToCurveID(tt.group)
			if ok != tt.wantOK {
				t.Fatalf("TLSGroupToCurveID(%q): got ok=%v, want %v", tt.group, ok, tt.wantOK)
			}
			if id != tt.wantID {
				t.Errorf("TLSGroupToCurveID(%q): got id=%v, want %v", tt.group, id, tt.wantID)
			}
		})
	}
}

func TestTLSConfigFromProfile(t *testing.T) {
	t.Run("nil spec returns secure defaults", func(t *testing.T) {
		cfg, err := TLSConfigFromProfile(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
	})

	t.Run("intermediate profile", func(t *testing.T) {
		spec := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		cfg, err := TLSConfigFromProfile(spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.MinVersion != tls.VersionTLS12 {
			t.Errorf("MinVersion: got %d, want %d", cfg.MinVersion, tls.VersionTLS12)
		}
		if len(cfg.CipherSuites) == 0 {
			t.Error("expected non-empty CipherSuites")
		}
	})

	t.Run("custom profile with groups", func(t *testing.T) {
		spec := &configv1.TLSProfileSpec{
			Ciphers:       []string{"ECDHE-RSA-AES256-GCM-SHA384"},
			MinTLSVersion: configv1.VersionTLS12,
			Groups: []configv1.TLSGroup{
				configv1.TLSGroupX25519,
				configv1.TLSGroupSecP256r1,
			},
		}
		cfg, err := TLSConfigFromProfile(spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.CurvePreferences) != 2 {
			t.Errorf("CurvePreferences: got %d entries, want 2", len(cfg.CurvePreferences))
		}
	})

	t.Run("unsupported groups are skipped", func(t *testing.T) {
		spec := &configv1.TLSProfileSpec{
			Ciphers:       []string{"ECDHE-RSA-AES256-GCM-SHA384"},
			MinTLSVersion: configv1.VersionTLS12,
			Groups:        []configv1.TLSGroup{"UnknownGroup"},
		}
		cfg, err := TLSConfigFromProfile(spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.CurvePreferences) != 0 {
			t.Errorf("CurvePreferences: got %d entries, want 0", len(cfg.CurvePreferences))
		}
	})

	t.Run("invalid TLS version returns error", func(t *testing.T) {
		spec := &configv1.TLSProfileSpec{
			MinTLSVersion: "InvalidVersion",
		}
		_, err := TLSConfigFromProfile(spec)
		if err == nil {
			t.Fatal("expected error for invalid TLS version")
		}
	})

	t.Run("empty groups leaves CurvePreferences nil", func(t *testing.T) {
		spec := &configv1.TLSProfileSpec{
			Ciphers:       []string{"ECDHE-RSA-AES256-GCM-SHA384"},
			MinTLSVersion: configv1.VersionTLS12,
		}
		cfg, err := TLSConfigFromProfile(spec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.CurvePreferences != nil {
			t.Errorf("expected nil CurvePreferences, got %v", cfg.CurvePreferences)
		}
	})
}

func TestTLSProfileSpecForSecurityProfile(t *testing.T) {
	t.Run("nil profile defaults to Intermediate", func(t *testing.T) {
		spec := TLSProfileSpecForSecurityProfile(nil)
		intermediate := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		if spec.MinTLSVersion != intermediate.MinTLSVersion {
			t.Errorf("MinTLSVersion: got %s, want %s", spec.MinTLSVersion, intermediate.MinTLSVersion)
		}
	})

	t.Run("custom profile with nil Custom defaults to Intermediate", func(t *testing.T) {
		profile := &configv1.TLSSecurityProfile{
			Type:   configv1.TLSProfileCustomType,
			Custom: nil,
		}
		spec := TLSProfileSpecForSecurityProfile(profile)
		intermediate := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		if spec.MinTLSVersion != intermediate.MinTLSVersion {
			t.Errorf("MinTLSVersion: got %s, want %s", spec.MinTLSVersion, intermediate.MinTLSVersion)
		}
	})

	t.Run("custom profile preserves groups", func(t *testing.T) {
		profile := &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileCustomType,
			Custom: &configv1.CustomTLSProfile{
				TLSProfileSpec: configv1.TLSProfileSpec{
					Ciphers:       []string{"ECDHE-RSA-AES256-GCM-SHA384"},
					MinTLSVersion: configv1.VersionTLS12,
					Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519},
				},
			},
		}
		spec := TLSProfileSpecForSecurityProfile(profile)
		if len(spec.Groups) != 1 || spec.Groups[0] != configv1.TLSGroupX25519 {
			t.Errorf("Groups: got %v, want [X25519]", spec.Groups)
		}
	})

	t.Run("old profile", func(t *testing.T) {
		profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType}
		spec := TLSProfileSpecForSecurityProfile(profile)
		old := configv1.TLSProfiles[configv1.TLSProfileOldType]
		if spec.MinTLSVersion != old.MinTLSVersion {
			t.Errorf("MinTLSVersion: got %s, want %s", spec.MinTLSVersion, old.MinTLSVersion)
		}
	})

	t.Run("modern profile", func(t *testing.T) {
		profile := &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType}
		spec := TLSProfileSpecForSecurityProfile(profile)
		modern := configv1.TLSProfiles[configv1.TLSProfileModernType]
		if spec.MinTLSVersion != modern.MinTLSVersion {
			t.Errorf("MinTLSVersion: got %s, want %s", spec.MinTLSVersion, modern.MinTLSVersion)
		}
	})
}

func TestCopyTLSSpec(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		if got := copyTLSSpec(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("copies ciphers and groups", func(t *testing.T) {
		in := &configv1.TLSProfileSpec{
			Ciphers:       []string{"A", "B"},
			MinTLSVersion: configv1.VersionTLS12,
			Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519},
		}
		out := copyTLSSpec(in)
		if &out.Ciphers[0] == &in.Ciphers[0] {
			t.Error("Ciphers slice was not copied")
		}
		if &out.Groups[0] == &in.Groups[0] {
			t.Error("Groups slice was not copied")
		}
		out.Ciphers[0] = "CHANGED"
		if in.Ciphers[0] == "CHANGED" {
			t.Error("modifying copy changed original")
		}
	})
}
