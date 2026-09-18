package client

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TrunkConfig is the effective configuration of one trunk circuit, as
// recorded in a root-level `trunk <ports> <name> <mode>` line of the
// running configuration. Name is the lowercase trunk name ("trk1"),
// Ports are canonical port references (plain numbers, e.g. "24") sorted
// in numeric order, and Mode is "trunk" or "lacp".
type TrunkConfig struct {
	Name  string
	Ports []string
	Mode  string
}

var trunkLineRe = regexp.MustCompile(`(?i)^trunk (\S+) (trk\d+) (trunk|lacp)$`)

// ParseRunningConfigTrunks parses every root-level `trunk <ports>
// <name> <mode>` line of a "show running-config" output into its
// effective per-trunk state, keyed by (lowercase) trunk name. Port lists
// may be comma-separated ranges ("9-10,24"); they are expanded to
// individual port numbers. A switch can carry several trunks, so this is
// a line scanner rather than part of the single-record running-config
// template.
func ParseRunningConfigTrunks(output string) (map[string]TrunkConfig, error) {
	trunks := make(map[string]TrunkConfig)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, " ") {
			continue
		}
		m := trunkLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		name := strings.ToLower(m[2])
		ports, err := parseTrunkPorts(m[1])
		if err != nil {
			return nil, fmt.Errorf("aoss: trunk %s: %w", name, err)
		}
		trunks[name] = TrunkConfig{
			Name:  name,
			Ports: ports,
			Mode:  strings.ToLower(m[3]),
		}
	}
	return trunks, nil
}

// parseTrunkPorts expands a trunk port list such as "9-10,24" into a
// sorted, de-duplicated list of canonical port references. Every entry
// must be a port number or a range of port numbers; trunks are not valid
// members of a trunk.
func parseTrunkPorts(s string) ([]string, error) {
	seen := make(map[int]bool)
	var ports []string
	for _, part := range strings.Split(s, ",") {
		start, end, err := parseOneRange(part)
		if err != nil {
			return nil, err
		}
		for p := start; p <= end; p++ {
			if !seen[p] {
				seen[p] = true
				ports = append(ports, strconv.Itoa(p))
			}
		}
	}
	sortMembers(ports)
	return ports, nil
}

// BuildTrunkConfig returns the configuration line that moves the trunk
// from current (nil on create) to desired, e.g.
//
//	trunk 9-10 trk1 lacp
//
// Ports are collapsed to the canonical range form. The name is never
// changed in place (renaming a trunk is a replace). It returns "" when
// the trunk is unchanged.
func BuildTrunkConfig(current, desired *TrunkConfig) string {
	if current != nil && current.Mode == desired.Mode && portsEqual(current.Ports, desired.Ports) {
		return ""
	}
	return fmt.Sprintf("trunk %s %s %s", FormatMemberRange(desired.Ports), desired.Name, desired.Mode)
}

func portsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
