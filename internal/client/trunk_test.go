package client

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseRunningConfigTrunks(t *testing.T) {
	input := `
hostname "idf02"
trunk 9-10 trk1 lacp
trunk 24 trk2 trunk
trunk 1-3,25 trk3 lacp
interface 24
   name "OPTIMUX-TIE"
   exit
`
	got, err := ParseRunningConfigTrunks(input)
	if err != nil {
		t.Fatalf("ParseRunningConfigTrunks: %v", err)
	}
	want := map[string]TrunkConfig{
		"trk1": {Name: "trk1", Ports: []string{"9", "10"}, Mode: "lacp"},
		"trk2": {Name: "trk2", Ports: []string{"24"}, Mode: "trunk"},
		"trk3": {Name: "trk3", Ports: []string{"1", "2", "3", "25"}, Mode: "lacp"},
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d trunks, want %d: %v", len(got), len(want), got)
	}
	for name, w := range want {
		t.Run(name, func(t *testing.T) {
			g, ok := got[name]
			if !ok {
				t.Fatalf("trunk %s not parsed", name)
			}
			if !reflect.DeepEqual(g, w) {
				t.Errorf("trunk %s = %+v, want %+v", name, g, w)
			}
		})
	}
}

func TestParseRunningConfigTrunksAbsent(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/running_config.cfg")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseRunningConfigTrunks(string(data))
	if err != nil {
		t.Fatalf("ParseRunningConfigTrunks: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("parsed %d trunks from fixture, want 0: %v", len(got), got)
	}
}

func TestParseRunningConfigTrunksErrors(t *testing.T) {
	cases := map[string]struct {
		in       string
		contains string
	}{
		"port too large": {
			in:       "trunk 999 trk1 lacp\n",
			contains: "out of range",
		},
		"trunk as member": {
			in:       "trunk trk2 trk1 lacp\n",
			contains: "invalid member",
		},
		"non-numeric member": {
			in:       "trunk abc trk1 lacp\n",
			contains: "invalid member",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRunningConfigTrunks(tc.in)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error %q does not contain %q", err, tc.contains)
			}
		})
	}
}

func TestBuildTrunkConfig(t *testing.T) {
	cases := map[string]struct {
		current *TrunkConfig
		desired *TrunkConfig
		want    string
	}{
		"create": {
			desired: &TrunkConfig{Name: "trk1", Ports: []string{"9", "10"}, Mode: "lacp"},
			want:    "trunk 9-10 trk1 lacp",
		},
		"create single port": {
			desired: &TrunkConfig{Name: "trk2", Ports: []string{"24"}, Mode: "trunk"},
			want:    "trunk 24 trk2 trunk",
		},
		"create non-contiguous": {
			desired: &TrunkConfig{Name: "trk3", Ports: []string{"1", "2", "3", "25"}, Mode: "lacp"},
			want:    "trunk 1-3,25 trk3 lacp",
		},
		"change mode": {
			current: &TrunkConfig{Name: "trk1", Ports: []string{"9", "10"}, Mode: "lacp"},
			desired: &TrunkConfig{Name: "trk1", Ports: []string{"9", "10"}, Mode: "trunk"},
			want:    "trunk 9-10 trk1 trunk",
		},
		"change ports": {
			current: &TrunkConfig{Name: "trk1", Ports: []string{"9", "10"}, Mode: "lacp"},
			desired: &TrunkConfig{Name: "trk1", Ports: []string{"10", "11"}, Mode: "lacp"},
			want:    "trunk 10-11 trk1 lacp",
		},
		"unchanged": {
			current: &TrunkConfig{Name: "trk1", Ports: []string{"9", "10"}, Mode: "lacp"},
			desired: &TrunkConfig{Name: "trk1", Ports: []string{"9", "10"}, Mode: "lacp"},
			want:    "",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := BuildTrunkConfig(tc.current, tc.desired); got != tc.want {
				t.Errorf("BuildTrunkConfig() = %q, want %q", got, tc.want)
			}
		})
	}
}
