package client

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// MaxPort is the highest member number accepted; AOS-S line-card port
// numbering is 1-255 and trunk numbers are accepted in the same range.
const MaxPort = 255

// A VLAN member reference is either a port number, formatted as a plain
// number (e.g. "24"), or a trunk circuit, formatted as "Trk" followed by
// its number (e.g. "Trk1"). Parsing accepts the "trk" prefix in any case;
// formatting always emits the canonical "Trk" spelling.

// ParseMember parses a single member reference such as "24" or "trk1".
func ParseMember(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("aoss: empty member reference")
	}
	if n, ok := strings.CutPrefix(strings.ToLower(s), "trk"); ok {
		return trunkRef(n)
	}
	return portRef(s)
}

func portRef(s string) (string, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return "", fmt.Errorf("aoss: invalid member %q", s)
	}
	if n < 1 || n > MaxPort {
		return "", fmt.Errorf("aoss: port %d out of range 1-%d", n, MaxPort)
	}
	return strconv.Itoa(n), nil
}

func trunkRef(s string) (string, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return "", fmt.Errorf("aoss: invalid trunk %q", "trk"+s)
	}
	if n < 1 || n > MaxPort {
		return "", fmt.Errorf("aoss: trunk %d out of range 1-%d", n, MaxPort)
	}
	return "Trk" + strconv.Itoa(n), nil
}

// ParseMemberRange parses a CLI member list such as "24", "1-23", or
// "1-3,trk1" into a sorted, de-duplicated list of canonical member
// references. Port ranges expand; a trunk may appear as "Trk1" or as a
// trunk range ("Trk1-Trk3") that expands like a port range.
func ParseMemberRange(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("aoss: empty member range")
	}
	seen := make(map[string]bool)
	var refs []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if isTrunkRef(part) {
			members, err := trunkMembers(part)
			if err != nil {
				return nil, err
			}
			for _, ref := range members {
				if !seen[ref] {
					seen[ref] = true
					refs = append(refs, ref)
				}
			}
			continue
		}
		start, end, err := parseOneRange(part)
		if err != nil {
			return nil, err
		}
		for p := start; p <= end; p++ {
			ref := strconv.Itoa(p)
			if !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	sortMembers(refs)
	return refs, nil
}

func parseOneRange(part string) (int, int, error) {
	lo, hi, ok := strings.Cut(part, "-")
	if !ok {
		n, err := strconv.Atoi(strings.TrimSpace(lo))
		if err != nil {
			return 0, 0, fmt.Errorf("aoss: invalid member %q", part)
		}
		if n < 1 || n > MaxPort {
			return 0, 0, fmt.Errorf("aoss: port %d out of range 1-%d", n, MaxPort)
		}
		return n, n, nil
	}
	start, err := strconv.Atoi(strings.TrimSpace(lo))
	if err != nil {
		return 0, 0, fmt.Errorf("aoss: invalid member %q", part)
	}
	end, err := strconv.Atoi(strings.TrimSpace(hi))
	if err != nil {
		return 0, 0, fmt.Errorf("aoss: invalid member %q", part)
	}
	if start < 1 || end > MaxPort || start > end {
		return 0, 0, fmt.Errorf("aoss: invalid member range %q", part)
	}
	return start, end, nil
}

// trunkMembers parses a single trunk reference ("Trk1") or a trunk range
// ("Trk1-Trk3") into its canonical trunk references.
func trunkMembers(part string) ([]string, error) {
	if !strings.Contains(part, "-") {
		ref, err := ParseMember(part)
		if err != nil {
			return nil, err
		}
		return []string{ref}, nil
	}
	lo, hi, _ := strings.Cut(part, "-")
	lo = strings.TrimSpace(lo)
	hi = strings.TrimSpace(hi)
	lo, _ = strings.CutPrefix(strings.ToLower(lo), "trk")
	hi, _ = strings.CutPrefix(strings.ToLower(hi), "trk")
	start, err := strconv.Atoi(lo)
	if err != nil {
		return nil, fmt.Errorf("aoss: invalid trunk %q", "trk"+lo)
	}
	end, err := strconv.Atoi(hi)
	if err != nil {
		return nil, fmt.Errorf("aoss: invalid trunk %q", "trk"+hi)
	}
	if start < 1 || end > MaxPort || start > end {
		return nil, fmt.Errorf("aoss: invalid trunk range %q", part)
	}
	out := make([]string, 0, end-start+1)
	for n := start; n <= end; n++ {
		out = append(out, "Trk"+strconv.Itoa(n))
	}
	return out, nil
}

func isTrunkRef(part string) bool {
	_, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(part)), "trk")
	return ok
}

// TrunkNumber returns the number of a trunk reference (2 for "Trk2") or
// false if the reference is not a trunk.
func TrunkNumber(ref string) (int, bool) {
	n, ok := strings.CutPrefix(ref, "Trk")
	if !ok {
		return 0, false
	}
	num, err := strconv.Atoi(n)
	if err != nil {
		return 0, false
	}
	return num, true
}

// FormatMemberRange formats a list of member references as a canonical CLI
// member string: port ranges collapsed (e.g. "1-3"), trunks in their own
// trailing section in number order with consecutive trunks collapsed into a
// range (e.g. "1-3,Trk1-Trk2"), ports before trunks. Duplicate references
// are collapsed.
func FormatMemberRange(refs []string) string {
	portSet := make(map[int]bool, len(refs))
	trunkSet := make(map[int]bool, len(refs))
	for _, ref := range refs {
		if n, ok := TrunkNumber(ref); ok {
			trunkSet[n] = true
		} else if n, err := strconv.Atoi(ref); err == nil {
			portSet[n] = true
		}
	}
	ports := make([]int, 0, len(portSet))
	for n := range portSet {
		ports = append(ports, n)
	}
	trunks := make([]int, 0, len(trunkSet))
	for n := range trunkSet {
		trunks = append(trunks, n)
	}
	sort.Ints(ports)
	sort.Ints(trunks)

	var parts []string
	var start, prev int
	flush := func() {
		if start == prev {
			parts = append(parts, strconv.Itoa(start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
		}
	}
	for i, p := range ports {
		if i > 0 && p == prev+1 {
			prev = p
			continue
		}
		if i > 0 {
			flush()
		}
		start, prev = p, p
	}
	if len(ports) > 0 {
		flush()
	}
	var tstart, tprev int
	flushTrunk := func() {
		if tstart == tprev {
			parts = append(parts, "Trk"+strconv.Itoa(tstart))
		} else {
			parts = append(parts, fmt.Sprintf("Trk%d-Trk%d", tstart, tprev))
		}
	}
	for i, n := range trunks {
		if i > 0 && n == tprev+1 {
			tprev = n
			continue
		}
		if i > 0 {
			flushTrunk()
		}
		tstart, tprev = n, n
	}
	if len(trunks) > 0 {
		flushTrunk()
	}
	return strings.Join(parts, ",")
}

// sortMembers orders references with ports (numeric order) first, then
// trunks (numeric order).
func sortMembers(refs []string) {
	sort.Slice(refs, func(i, j int) bool {
		_, okt := TrunkNumber(refs[i])
		_, oku := TrunkNumber(refs[j])
		if okt != oku {
			return !okt // ports before trunks
		}
		if okt {
			ni, _ := TrunkNumber(refs[i])
			nj, _ := TrunkNumber(refs[j])
			return ni < nj
		}
		pi, _ := strconv.Atoi(refs[i])
		pj, _ := strconv.Atoi(refs[j])
		return pi < pj
	})
}

// MemberDiff returns the members to add to and remove from a VLAN member
// list to move it from current to desired; both results are sorted.
func MemberDiff(current, desired []string) (add, remove []string) {
	want := make(map[string]bool, len(desired))
	for _, r := range desired {
		want[r] = true
	}
	have := make(map[string]bool, len(current))
	for _, r := range current {
		have[r] = true
	}
	for r := range want {
		if !have[r] {
			add = append(add, r)
		}
	}
	for r := range have {
		if !want[r] {
			remove = append(remove, r)
		}
	}
	sortMembers(add)
	sortMembers(remove)
	return add, remove
}
