package provider

import (
	"strings"
	"testing"
)

func TestValidateHostname(t *testing.T) {
	tests := map[string]struct {
		value   string
		wantErr bool
		errMsg  string
	}{
		"simple": {
			value:   "idf02",
			wantErr: false,
		},
		"with dot": {
			value:   "core-rtr-01.internal",
			wantErr: false,
		},
		"with underscore": {
			value:   "switch_1",
			wantErr: false,
		},
		"with hyphens": {
			value:   "core-rtr-01",
			wantErr: false,
		},
		"single character": {
			value:   "a",
			wantErr: false,
		},
		"max length": {
			value:   strings.Repeat("a", 63),
			wantErr: false,
		},
		"empty": {
			value:   "",
			wantErr: true,
			errMsg:  "must not be empty",
		},
		"too long": {
			value:   strings.Repeat("a", 64),
			wantErr: true,
			errMsg:  "63 characters or fewer",
		},
		"leading hyphen": {
			value:   "-idf02",
			wantErr: true,
			errMsg:  "start and end with an alphanumeric",
		},
		"leading dot": {
			value:   ".idf02",
			wantErr: true,
			errMsg:  "start and end with an alphanumeric",
		},
		"trailing hyphen": {
			value:   "idf02-",
			wantErr: true,
			errMsg:  "start and end with an alphanumeric",
		},
		"space": {
			value:   "idf 02",
			wantErr: true,
			errMsg:  "start and end with an alphanumeric",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, errs := validateHostname(tc.value, "hostname")
			if tc.wantErr {
				if len(errs) == 0 {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(errs[0].Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsg, errs[0].Error())
				}
				return
			}
			if len(errs) != 0 {
				t.Fatalf("unexpected error: %s", errs[0].Error())
			}
		})
	}
}
