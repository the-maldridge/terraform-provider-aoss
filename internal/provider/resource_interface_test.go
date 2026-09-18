package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/the-maldridge/terraform-provider-aoss/internal/client"
)

func TestValidateInterfaceNumber(t *testing.T) {
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
			_, errs := validateInterfaceNumber(tc.value, "interface")
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

func TestInterfaceResourceSchema(t *testing.T) {
	d := schema.TestResourceDataRaw(t, aossInterfaceResource().Schema, map[string]any{
		"interface": 24,
		"name":      "OPTIMUX-TIE",
		"shutdown":  true,
	})
	if got := d.Get("interface").(int); got != 24 {
		t.Fatalf("interface = %d, want 24", got)
	}
	if got := d.Get("name").(string); got != "OPTIMUX-TIE" {
		t.Fatalf("name = %q, want %q", got, "OPTIMUX-TIE")
	}
	if got := d.Get("shutdown").(bool); got != true {
		t.Fatalf("shutdown = %v, want true", got)
	}
}

func TestBuildInterfaceDeleteBlock(t *testing.T) {
	tests := map[string]struct {
		port    int
		current *client.InterfaceConfig
		want    string
	}{
		"name and shutdown": {
			port:    5,
			current: &client.InterfaceConfig{Port: 5, Name: "UP", Shutdown: true},
			want:    "interface 5\nno name\nenable\nexit",
		},
		"name only": {
			port:    24,
			current: &client.InterfaceConfig{Port: 24, Name: "OPTIMUX-TIE"},
			want:    "interface 24\nno name\nexit",
		},
		"shutdown only": {
			port:    5,
			current: &client.InterfaceConfig{Port: 5, Shutdown: true},
			want:    "interface 5\nenable\nexit",
		},
		"nothing to clear": {
			port:    5,
			current: &client.InterfaceConfig{Port: 5},
			want:    "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := buildInterfaceDeleteBlock(tc.port, tc.current)
			if got != tc.want {
				t.Fatalf("buildInterfaceDeleteBlock = %q, want %q", got, tc.want)
			}
		})
	}
}
