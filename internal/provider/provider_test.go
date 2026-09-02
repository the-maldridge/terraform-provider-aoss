package provider

import (
	"strings"
	"testing"
)

func TestProvider(t *testing.T) {
	if err := New("test")().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		"valid": {
			cfg:     Config{Host: "10.0.0.1", Username: "admin", Password: "secret"},
			wantErr: false,
		},
		"missing host": {
			cfg:     Config{Username: "admin", Password: "secret"},
			wantErr: true,
			errMsg:  "host must be set",
		},
		"missing username": {
			cfg:     Config{Host: "10.0.0.1", Password: "secret"},
			wantErr: true,
			errMsg:  "username must be set",
		},
		"missing password": {
			cfg:     Config{Host: "10.0.0.1", Username: "admin"},
			wantErr: true,
			errMsg:  "password must be set",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}
