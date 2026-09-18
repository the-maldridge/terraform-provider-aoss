package provider

import (
	"strings"
	"testing"
)

func TestValidateVLANID(t *testing.T) {
	tests := map[string]struct {
		value   int
		wantErr bool
		errMsg  string
	}{
		"min": {
			value:   1,
			wantErr: false,
		},
		"typical": {
			value:   30,
			wantErr: false,
		},
		"max": {
			value:   4094,
			wantErr: false,
		},
		"below min": {
			value:   0,
			wantErr: true,
			errMsg:  "between 1 and 4094",
		},
		"negative": {
			value:   -1,
			wantErr: true,
			errMsg:  "between 1 and 4094",
		},
		"above max": {
			value:   4095,
			wantErr: true,
			errMsg:  "between 1 and 4094",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, errs := validateVLANID(tc.value, "vlan_id")
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
