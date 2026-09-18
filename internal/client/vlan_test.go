package client

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParseRunningConfigVLANs(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/running_config.cfg")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseRunningConfigVLANs(string(data))
	if err != nil {
		t.Fatalf("ParseRunningConfigVLANs: %v", err)
	}
	want := map[int]VLANConfig{
		1: {
			ID:       1,
			Name:     "DEFAULT_VLAN",
			Tagged:   []string{},
			Untagged: portRefs(24, 28),
			DHCP:     true,
		},
		10: {
			ID:       10,
			Name:     "LAN",
			Tagged:   portRefs(24, 28),
			Untagged: portRefs(1, 23),
			DHCP:     false,
		},
		15: {
			ID:       15,
			Name:     "TRUST",
			Tagged:   portRefs(24, 28),
			Untagged: []string{},
			DHCP:     false,
		},
		20: {
			ID:       20,
			Name:     "WAN",
			Tagged:   portRefs(24, 28),
			Untagged: []string{},
			DHCP:     false,
		},
		25: {
			ID:       25,
			Name:     "MEDIA",
			Tagged:   portRefs(24, 28),
			Untagged: []string{},
			DHCP:     false,
		},
		30: {
			ID:       30,
			Name:     "MGMT",
			Tagged:   portRefs(24, 28),
			Untagged: []string{},
			DHCP:     true,
		},
		500: {
			ID:       500,
			Name:     "VLAN500",
			Tagged:   portRefs(24, 28),
			Untagged: []string{},
			DHCP:     false,
		},
		501: {
			ID:       501,
			Name:     "VLAN501",
			Tagged:   portRefs(24, 28),
			Untagged: []string{},
			DHCP:     false,
		},
		502: {
			ID:       502,
			Name:     "VLAN502",
			Tagged:   portRefs(24, 28),
			Untagged: []string{},
			DHCP:     false,
		},
		503: {
			ID:       503,
			Name:     "VLAN503",
			Tagged:   portRefs(24, 28),
			Untagged: []string{},
			DHCP:     false,
		},
		504: {
			ID:       504,
			Name:     "VLAN504",
			Tagged:   portRefs(24, 28),
			Untagged: []string{},
			DHCP:     false,
		},
	}
	if len(got) != len(want) {
		t.Fatalf("parsed %d vlans, want %d: %v", len(got), len(want), vlanIDs(got))
	}
	for id, w := range want {
		t.Run(nameOf(id), func(t *testing.T) {
			g, ok := got[id]
			if !ok {
				t.Fatalf("vlan %d not parsed", id)
			}
			if !reflect.DeepEqual(g, w) {
				t.Errorf("vlan %d = %+v, want %+v", id, g, w)
			}
		})
	}
}

func TestParseRunningConfigVLANsMemberOrder(t *testing.T) {
	input := `
vlan 10
   name "MIXED"
   tagged 1-10
   no tagged 5-8
   tagged 20
   tagged Trk1-Trk2
   untagged 30
   no untagged 30
   untagged 40
   ip address dhcp
   exit
vlan 20
   name "PLAIN"
   no tagged 1-10
   exit
`
	got, err := ParseRunningConfigVLANs(input)
	if err != nil {
		t.Fatalf("ParseRunningConfigVLANs: %v", err)
	}
	if w, ok := got[10]; !ok {
		t.Fatal("vlan 10 not parsed")
	} else if !reflect.DeepEqual(w, VLANConfig{
		ID:       10,
		Name:     "MIXED",
		Tagged:   []string{"1", "2", "3", "4", "9", "10", "20", "Trk1", "Trk2"},
		Untagged: []string{"40"},
		DHCP:     true,
	}) {
		t.Errorf("vlan 10 = %+v", w)
	}
	if w, ok := got[20]; !ok {
		t.Fatal("vlan 20 not parsed")
	} else if !reflect.DeepEqual(w, VLANConfig{
		ID:       20,
		Name:     "PLAIN",
		Tagged:   []string{},
		Untagged: []string{},
		DHCP:     false,
	}) {
		t.Errorf("vlan 20 = %+v", w)
	}
}

func TestParseRunningConfigVLANsErrors(t *testing.T) {
	cases := map[string]struct {
		in       string
		contains string
	}{
		"bad member range": {
			in:       "vlan 10\n   tagged 999\n   exit\n",
			contains: "out of range",
		},
		"trunk range inverted": {
			in:       "vlan 10\n   tagged trk2-1\n   exit\n",
			contains: "invalid trunk range",
		},
		"trunk too large": {
			in:       "vlan 10\n   tagged trk256\n   exit\n",
			contains: "out of range",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRunningConfigVLANs(tc.in)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error %q does not contain %q", err, tc.contains)
			}
		})
	}
}

func TestBuildVLANConfig(t *testing.T) {
	cases := map[string]struct {
		current *VLANConfig
		desired *VLANConfig
		want    string
	}{
		"create": {
			desired: &VLANConfig{
				ID:       10,
				Name:     "LAN",
				Tagged:   portRefs(24, 28),
				Untagged: portRefs(1, 23),
			},
			want: "vlan 10\nname \"LAN\"\ntagged 24-28\nuntagged 1-23\nexit",
		},
		"create with dhcp": {
			desired: &VLANConfig{
				ID:   30,
				Name: "MGMT",
				DHCP: true,
			},
			want: "vlan 30\nname \"MGMT\"\nip address dhcp\nexit",
		},
		"no change": {
			current: &VLANConfig{
				ID:       10,
				Name:     "LAN",
				Tagged:   portRefs(24, 28),
				Untagged: portRefs(1, 23),
			},
			desired: &VLANConfig{
				ID:       10,
				Name:     "LAN",
				Tagged:   portRefs(24, 28),
				Untagged: portRefs(1, 23),
			},
			want: "",
		},
		"name change only": {
			current: &VLANConfig{ID: 10, Name: "LAN"},
			desired: &VLANConfig{ID: 10, Name: "LAN2"},
			want:    "vlan 10\nname \"LAN2\"\nexit",
		},
		"member move and dhcp": {
			current: &VLANConfig{
				ID:       10,
				Name:     "LAN",
				Tagged:   portRefs(24, 28),
				Untagged: portRefs(1, 23),
			},
			desired: &VLANConfig{
				ID:       10,
				Name:     "LAN",
				Tagged:   []string{"28"},
				Untagged: portRefs(24, 27),
				DHCP:     true,
			},
			want: "vlan 10\nno tagged 24-27\nno untagged 1-23\nuntagged 24-27\nip address dhcp\nexit",
		},
		"dhcp off": {
			current: &VLANConfig{ID: 1, Name: "DEFAULT_VLAN", DHCP: true},
			desired: &VLANConfig{ID: 1, Name: "DEFAULT_VLAN"},
			want:    "vlan 1\nno ip address\nexit",
		},
		"clear members": {
			current: &VLANConfig{ID: 15, Name: "TRUST", Tagged: portRefs(24, 28)},
			desired: &VLANConfig{ID: 15, Name: "TRUST"},
			want:    "vlan 15\nno tagged 24-28\nexit",
		},
		"trunk create": {
			desired: &VLANConfig{
				ID:     20,
				Name:   "WAN",
				Tagged: []string{"Trk1"},
			},
			want: "vlan 20\nname \"WAN\"\ntagged Trk1\nexit",
		},
		"trunk replace": {
			current: &VLANConfig{ID: 20, Name: "WAN", Tagged: []string{"Trk1", "24", "25"}},
			desired: &VLANConfig{ID: 20, Name: "WAN", Tagged: []string{"Trk2", "24"}},
			want:    "vlan 20\nno tagged 25,Trk1\ntagged Trk2\nexit",
		},
		"trunk to untagged move": {
			current: &VLANConfig{ID: 30, Name: "MGMT", Tagged: []string{"Trk1"}},
			desired: &VLANConfig{ID: 30, Name: "MGMT", Untagged: []string{"Trk1"}},
			want:    "vlan 30\nno tagged Trk1\nuntagged Trk1\nexit",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := BuildVLANConfig(tc.current, tc.desired)
			if got != tc.want {
				t.Errorf("BuildVLANConfig() =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func vlanIDs(m map[int]VLANConfig) []int {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	return ids
}

func nameOf(id int) string {
	switch id {
	case 1:
		return "default"
	case 10:
		return "lan"
	case 15:
		return "trust"
	case 20:
		return "wan"
	case 25:
		return "media"
	case 30:
		return "mgmt"
	default:
		return strconv.Itoa(id)
	}
}
