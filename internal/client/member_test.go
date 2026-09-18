package client

import (
	"reflect"
	"strconv"
	"testing"
)

func TestParseMemberRange(t *testing.T) {
	cases := map[string]struct {
		in   string
		want []string
	}{
		"single":                {in: "24", want: []string{"24"}},
		"range":                 {in: "1-23", want: portRefs(1, 23)},
		"mixed":                 {in: "1-3,5,24-28", want: []string{"1", "2", "3", "5", "24", "25", "26", "27", "28"}},
		"singletons":            {in: "1,2,3", want: []string{"1", "2", "3"}},
		"spaces":                {in: " 1-3 , 5 ", want: []string{"1", "2", "3", "5"}},
		"duplicates":            {in: "1-2,2-3", want: []string{"1", "2", "3"}},
		"unsorted":              {in: "28,1-3,5", want: []string{"1", "2", "3", "5", "28"}},
		"trunk":                 {in: "trk1", want: []string{"Trk1"}},
		"trunk caps":            {in: "TRK2", want: []string{"Trk2"}},
		"trunk mid":             {in: "Trk3", want: []string{"Trk3"}},
		"ports and trunk":       {in: "1-3,trk1,5", want: []string{"1", "2", "3", "5", "Trk1"}},
		"trunk dedupe":          {in: "trk1,Trk1", want: []string{"Trk1"}},
		"trunk order":           {in: "trk10,trk2", want: []string{"Trk2", "Trk10"}},
		"empty":                 {in: "", want: nil},
		"blank":                 {in: "   ", want: nil},
		"zero":                  {in: "0", want: nil},
		"too large":             {in: "256", want: nil},
		"trunk zero":            {in: "trk0", want: nil},
		"trunk too large":       {in: "trk256", want: nil},
		"trunk range":           {in: "trk1-2", want: []string{"Trk1", "Trk2"}},
		"trunk range caps":      {in: "Trk1-Trk3", want: []string{"Trk1", "Trk2", "Trk3"}},
		"trunk range mixed":     {in: "Trk1-3", want: []string{"Trk1", "Trk2", "Trk3"}},
		"trunk range dedupe":    {in: "trk1,Trk1-Trk2", want: []string{"Trk1", "Trk2"}},
		"trunk range inverted":  {in: "trk2-1", want: nil},
		"trunk range too large": {in: "trk255-trk256", want: nil},
		"range with trunk":      {in: "1-trk1", want: nil},
		"inverted":              {in: "23-1", want: nil},
		"non numeric":           {in: "1-abc", want: nil},
		"garbage":               {in: "1-2-3", want: nil},
		"ethernet":              {in: "ethernet 1", want: nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseMemberRange(tc.in)
			if tc.want == nil {
				if err == nil {
					t.Fatalf("ParseMemberRange(%q) = %v, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMemberRange(%q): %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseMemberRange(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseMember(t *testing.T) {
	cases := map[string]struct {
		in   string
		want string
	}{
		"port":        {in: "24", want: "24"},
		"trunk lower": {in: "trk1", want: "Trk1"},
		"trunk upper": {in: "TRK1", want: "Trk1"},
		"trunk mixed": {in: "Trk1", want: "Trk1"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseMember(tc.in)
			if err != nil {
				t.Fatalf("ParseMember(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseMember(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	errCases := []string{"", "0", "256", "trk0", "trk256", "trk", "trkx", "abc"}
	for _, in := range errCases {
		t.Run("error "+in, func(t *testing.T) {
			if _, err := ParseMember(in); err == nil {
				t.Fatalf("ParseMember(%q) succeeded, want error", in)
			}
		})
	}
}

func TestFormatMemberRange(t *testing.T) {
	cases := map[string]struct {
		in   []string
		want string
	}{
		"empty":                  {in: nil, want: ""},
		"single":                 {in: []string{"24"}, want: "24"},
		"contiguous":             {in: []string{"1", "2", "3"}, want: "1-3"},
		"contiguous wide":        {in: portRefs(24, 28), want: "24-28"},
		"mixed":                  {in: []string{"1", "2", "3", "24"}, want: "1-3,24"},
		"unsorted input":         {in: []string{"28", "5", "1", "2", "3"}, want: "1-3,5,28"},
		"duplicates":             {in: []string{"5", "5", "6"}, want: "5-6"},
		"two ranges":             {in: []string{"1", "2", "3", "5", "6"}, want: "1-3,5-6"},
		"trunk only":             {in: []string{"Trk1"}, want: "Trk1"},
		"trunk range":            {in: []string{"Trk1", "Trk2"}, want: "Trk1-Trk2"},
		"trunk non-consecutive":  {in: []string{"Trk2", "Trk4"}, want: "Trk2,Trk4"},
		"trunk range with ports": {in: []string{"1", "Trk2", "2", "Trk3"}, want: "1-2,Trk2-Trk3"},
		"ports and trunks":       {in: []string{"1", "2", "3", "Trk1", "5"}, want: "1-3,5,Trk1"},
		"trunks sorted":          {in: []string{"Trk10", "Trk2"}, want: "Trk2,Trk10"},
		"all mixed":              {in: []string{"Trk3", "1", "Trk1", "28", "24", "25"}, want: "1,24-25,28,Trk1,Trk3"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := FormatMemberRange(tc.in)
			if got != tc.want {
				t.Errorf("FormatMemberRange(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func portRefs(start, end int) []string {
	out := make([]string, 0, end-start+1)
	for p := start; p <= end; p++ {
		out = append(out, strconv.Itoa(p))
	}
	return out
}

func TestTrunkNumber(t *testing.T) {
	if n, ok := TrunkNumber("Trk12"); !ok || n != 12 {
		t.Errorf("TrunkNumber(Trk12) = %d, %v, want 12, true", n, ok)
	}
	if n, ok := TrunkNumber("24"); ok {
		t.Errorf("TrunkNumber(24) = %d, true, want false", n)
	}
}

func TestMemberDiff(t *testing.T) {
	cases := map[string]struct {
		current []string
		desired []string
		add     []string
		remove  []string
	}{
		"no change": {
			current: []string{"1", "2", "3"},
			desired: []string{"1", "2", "3"},
		},
		"add only": {
			current: []string{"1", "2"},
			desired: []string{"1", "2", "3"},
			add:     []string{"3"},
		},
		"remove only": {
			current: []string{"1", "2", "3"},
			desired: []string{"1", "2"},
			remove:  []string{"3"},
		},
		"replace": {
			current: []string{"1", "2"},
			desired: []string{"3", "4"},
			add:     []string{"3", "4"},
			remove:  []string{"1", "2"},
		},
		"empty current": {
			current: nil,
			desired: []string{"24", "25"},
			add:     []string{"24", "25"},
		},
		"empty desired": {
			current: []string{"24", "25"},
			desired: nil,
			remove:  []string{"24", "25"},
		},
		"both empty": {},
		"trunk move": {
			current: []string{"1", "Trk1"},
			desired: []string{"1", "Trk2"},
			add:     []string{"Trk2"},
			remove:  []string{"Trk1"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			add, remove := MemberDiff(tc.current, tc.desired)
			if !reflect.DeepEqual(add, tc.add) {
				t.Errorf("add = %v, want %v", add, tc.add)
			}
			if !reflect.DeepEqual(remove, tc.remove) {
				t.Errorf("remove = %v, want %v", remove, tc.remove)
			}
		})
	}
}
