package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestValidateTrunkName(t *testing.T) {
	tests := map[string]struct {
		value   string
		wantErr bool
		errMsg  string
	}{
		"single digit": {
			value:   "trk1",
			wantErr: false,
		},
		"multiple digits": {
			value:   "trk12",
			wantErr: false,
		},
		"no digits": {
			value:   "trk",
			wantErr: true,
			errMsg:  "of the form trk<N>",
		},
		"wrong prefix": {
			value:   "trk1x",
			wantErr: true,
			errMsg:  "of the form trk<N>",
		},
		"empty": {
			value:   "",
			wantErr: true,
			errMsg:  "of the form trk<N>",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, errs := validateTrunkName(tc.value, "name")
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

func TestValidateTrunkMode(t *testing.T) {
	tests := map[string]struct {
		value   string
		wantErr bool
		errMsg  string
	}{
		"trunk": {
			value:   "trunk",
			wantErr: false,
		},
		"lacp": {
			value:   "lacp",
			wantErr: false,
		},
		"wrong mode": {
			value:   "802.3ad",
			wantErr: true,
			errMsg:  `must be "trunk" or "lacp"`,
		},
		"uppercase": {
			value:   "LACP",
			wantErr: true,
			errMsg:  `must be "trunk" or "lacp"`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, errs := validateTrunkMode(tc.value, "mode")
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

func TestTrunkResourceSchema(t *testing.T) {
	// Read converts the parser's []string output to ints and sets it
	// into the TypeInt set; verify the round trip.
	nums, err := trunkReadPorts([]string{"9", "10"})
	if err != nil {
		t.Fatalf("trunkReadPorts: %v", err)
	}
	d := schema.TestResourceDataRaw(t, aossTrunkResource().Schema, nil)
	if err := d.Set("ports", nums); err != nil {
		t.Fatalf("Set ports: %v", err)
	}
	got := d.Get("ports").(*schema.Set).List()
	if len(got) != 2 {
		t.Fatalf("ports = %v", got)
	}
	for _, v := range got {
		if _, ok := v.(int); !ok {
			t.Fatalf("port %v is %T, want int", v, v)
		}
	}
}

func TestValidateTrunkPort(t *testing.T) {
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
			value:   24,
			wantErr: false,
		},
		"max": {
			value:   255,
			wantErr: false,
		},
		"below min": {
			value:   0,
			wantErr: true,
			errMsg:  "between 1 and 255",
		},
		"above max": {
			value:   256,
			wantErr: true,
			errMsg:  "between 1 and 255",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, errs := validateTrunkPort(tc.value, "ports")
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
