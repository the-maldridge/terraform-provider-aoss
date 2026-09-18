package client

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseRunningConfigInterfaces(t *testing.T) {
	input := `
hostname "idf02"
interface 24
   name "OPTIMUX-TIE"
   exit
interface 9
   exit
interface 3
   disable
   exit
vlan 1
   name "DEFAULT_VLAN"
   no untagged 1-23
   exit
`
	got, err := ParseRunningConfigInterfaces(input)
	if err != nil {
		t.Fatalf("ParseRunningConfigInterfaces: %v", err)
	}
	want := map[int]InterfaceConfig{
		24: {Port: 24, Name: "OPTIMUX-TIE"},
		3:  {Port: 3, Shutdown: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseRunningConfigInterfaces() = %v, want %v", got, want)
	}
}

func TestParseRunningConfigInterfacesBackToBack(t *testing.T) {
	input := `interface 2
   name "b"
   exit
interface 3
   name "c"
   disable
   exit
`
	got, err := ParseRunningConfigInterfaces(input)
	if err != nil {
		t.Fatalf("ParseRunningConfigInterfaces: %v", err)
	}
	want := map[int]InterfaceConfig{
		2: {Port: 2, Name: "b"},
		3: {Port: 3, Name: "c", Shutdown: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseRunningConfigInterfaces() = %v, want %v", got, want)
	}
}

func TestParseRunningConfigInterfacesFixture(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/running_config.cfg")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseRunningConfigInterfaces(string(data))
	if err != nil {
		t.Fatalf("ParseRunningConfigInterfaces: %v", err)
	}
	want := map[int]InterfaceConfig{
		24: {Port: 24, Name: "OPTIMUX-TIE"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseRunningConfigInterfaces() = %v, want %v", got, want)
	}
}

func TestParseRunningConfigInterfacesErrors(t *testing.T) {
	cases := map[string]struct {
		in       string
		contains string
	}{
		"port too large": {
			in:       "interface 999\n   name \"x\"\n   exit\n",
			contains: "out of range",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRunningConfigInterfaces(tc.in)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error %q does not contain %q", err, tc.contains)
			}
		})
	}
}

func TestBuildInterfaceConfig(t *testing.T) {
	cases := map[string]struct {
		current *InterfaceConfig
		desired *InterfaceConfig
		want    string
	}{
		"create": {
			desired: &InterfaceConfig{Port: 24, Name: "OPTIMUX-TIE"},
			want:    "interface 24\nname \"OPTIMUX-TIE\"\nexit",
		},
		"create shut down": {
			desired: &InterfaceConfig{Port: 5, Shutdown: true},
			want:    "interface 5\ndisable\nexit",
		},
		"create name and shutdown": {
			desired: &InterfaceConfig{Port: 5, Name: "UP", Shutdown: true},
			want:    "interface 5\nname \"UP\"\ndisable\nexit",
		},
		"no change": {
			current: &InterfaceConfig{Port: 24, Name: "OPTIMUX-TIE"},
			desired: &InterfaceConfig{Port: 24, Name: "OPTIMUX-TIE"},
			want:    "",
		},
		"name change": {
			current: &InterfaceConfig{Port: 24, Name: "OLD"},
			desired: &InterfaceConfig{Port: 24, Name: "NEW"},
			want:    "interface 24\nname \"NEW\"\nexit",
		},
		"shutdown on": {
			current: &InterfaceConfig{Port: 5},
			desired: &InterfaceConfig{Port: 5, Shutdown: true},
			want:    "interface 5\ndisable\nexit",
		},
		"shutdown off": {
			current: &InterfaceConfig{Port: 5, Shutdown: true},
			desired: &InterfaceConfig{Port: 5},
			want:    "interface 5\nenable\nexit",
		},
		"name and shutdown change": {
			current: &InterfaceConfig{Port: 5, Name: "UP", Shutdown: true},
			desired: &InterfaceConfig{Port: 5, Name: "DOWN"},
			want:    "interface 5\nname \"DOWN\"\nenable\nexit",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := BuildInterfaceConfig(tc.current, tc.desired)
			if got != tc.want {
				t.Errorf("BuildInterfaceConfig() =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}
