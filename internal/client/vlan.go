package client

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// VLANConfig is the effective configuration of one `vlan <id>` block in
// the running config. Tagged and Untagged are sorted, de-duplicated
// member lists that reflect every member statement in the block, applied
// in order (a `no tagged` line removes the members it names). Members are
// canonical references: port numbers as plain strings ("24") and trunk
// circuits as "Trk<n>" ("Trk1").
type VLANConfig struct {
	ID       int
	Name     string
	Tagged   []string
	Untagged []string
	DHCP     bool
}

// needsVLANDeleteConfirm reports whether deleting this VLAN can make the
// device print its "The following ports will be moved to the default
// VLAN" confirmation and wait at a "[y/n]" prompt.
//
// AOS-S asks for confirmation when the delete would orphan a port: a
// port's only untagged home (its PVID) is the VLAN being deleted, so it
// must be moved to the default VLAN 1. Only untagged port members can be
// a port's PVID, so an untagged member is the trigger; tagged members and
// trunk circuits (Trk<N> is always tagged) never can. When the VLAN has
// no untagged members, "no vlan <id>" completes without prompting.
func NeedsVLANDeleteConfirm(v VLANConfig) bool {
	return len(v.Untagged) > 0
}

var (
	vlanBlockRe  = regexp.MustCompile(`^vlan (\d+)$`)
	vlanNameRe   = regexp.MustCompile(`^name "(.*)"$`)
	vlanMemberRe = regexp.MustCompile(`^(no )?(tagged|untagged)\s+(.+)$`)
)

// ParseRunningConfigVLANs parses every `vlan <id>` block of a
// "show running-config" output into its effective per-VLAN state. Member
// lines are applied in the order they appear, so `no untagged 1-23`
// followed by `untagged 24-28` yields untagged {"24".."28"}. A line
// starting `ip address dhcp` (including the device's `ip address
// dhcp-bootp` spelling) sets DHCP; `no ip address` (or no such line)
// clears it.
func ParseRunningConfigVLANs(output string) (map[int]VLANConfig, error) {
	vlans := make(map[int]VLANConfig)
	var cur VLANConfig
	inBlock := false

	flush := func() {
		if inBlock {
			vlans[cur.ID] = cur
			inBlock = false
		}
	}

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			if m := vlanBlockRe.FindStringSubmatch(line); m != nil {
				flush()
				id, err := strconv.Atoi(m[1])
				if err != nil {
					return nil, fmt.Errorf("aoss: vlan id %q: %w", m[1], err)
				}
				cur = vlans[id]
				if cur.ID == 0 {
					cur = VLANConfig{ID: id, Tagged: []string{}, Untagged: []string{}}
				}
				inBlock = true
			} else {
				flush()
			}
			continue
		}
		if !inBlock {
			// Indented line of some other block (interface, snmp-server,
			// ...); only vlan blocks are of interest.
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "exit":
			flush()
		case vlanNameRe.MatchString(trimmed):
			m := vlanNameRe.FindStringSubmatch(trimmed)
			cur.Name = m[1]
		case vlanMemberRe.MatchString(trimmed):
			m := vlanMemberRe.FindStringSubmatch(trimmed)
			no, kind, rangeStr := m[1] == "no ", m[2], m[3]
			members, err := ParseMemberRange(rangeStr)
			if err != nil {
				return nil, fmt.Errorf("aoss: vlan %d: %w", cur.ID, err)
			}
			if kind == "tagged" {
				if no {
					cur.Tagged = subtractMembers(cur.Tagged, members)
				} else {
					cur.Tagged = unionMembers(cur.Tagged, members)
				}
			} else {
				if no {
					cur.Untagged = subtractMembers(cur.Untagged, members)
				} else {
					cur.Untagged = unionMembers(cur.Untagged, members)
				}
			}
		case strings.HasPrefix(trimmed, "ip address dhcp"):
			cur.DHCP = true
		case trimmed == "no ip address":
			cur.DHCP = false
		}
	}
	flush()
	return vlans, nil
}

func unionMembers(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, m := range a {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	for _, m := range b {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sortMembers(out)
	return out
}

func subtractMembers(a, b []string) []string {
	drop := make(map[string]bool, len(b))
	for _, m := range b {
		drop[m] = true
	}
	out := make([]string, 0, len(a))
	for _, m := range a {
		if !drop[m] {
			out = append(out, m)
		}
	}
	return out
}

// BuildVLANConfig returns the configuration block that moves the VLAN
// from current (nil on create) to desired, e.g.
//
//	vlan 10
//	name "LAN"
//	no tagged Trk1
//	tagged 24-28
//	ip address dhcp
//	exit
//
// It returns "" when an existing VLAN would not change at all.
func BuildVLANConfig(current, desired *VLANConfig) string {
	lines := []string{fmt.Sprintf("vlan %d", desired.ID)}

	name := desired.Name
	if name != "" {
		currentName := ""
		if current != nil {
			currentName = current.Name
		}
		if name != currentName {
			lines = append(lines, fmt.Sprintf("name %q", name))
		}
	}

	var curTagged, curUntagged []string
	if current != nil {
		curTagged = current.Tagged
		curUntagged = current.Untagged
	}
	tagAdd, tagRemove := MemberDiff(curTagged, desired.Tagged)
	untagAdd, untagRemove := MemberDiff(curUntagged, desired.Untagged)
	// All removals precede all additions so a member can move between
	// tagged and untagged within this block.
	if len(tagRemove) > 0 {
		lines = append(lines, "no tagged "+FormatMemberRange(tagRemove))
	}
	if len(untagRemove) > 0 {
		lines = append(lines, "no untagged "+FormatMemberRange(untagRemove))
	}
	if len(tagAdd) > 0 {
		lines = append(lines, "tagged "+FormatMemberRange(tagAdd))
	}
	if len(untagAdd) > 0 {
		lines = append(lines, "untagged "+FormatMemberRange(untagAdd))
	}

	if current != nil {
		if desired.DHCP && !current.DHCP {
			lines = append(lines, "ip address dhcp")
		} else if !desired.DHCP && current.DHCP {
			lines = append(lines, "no ip address")
		}
	} else if desired.DHCP {
		lines = append(lines, "ip address dhcp")
	}

	if len(lines) == 1 && current != nil {
		return ""
	}
	lines = append(lines, "exit")
	return strings.Join(lines, "\n")
}
