package client

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// A physical port is identified by its port number (1-255) and may carry
// an optional configured name and shutdown state recorded in an
// `interface <n>` block of the running configuration; a trunk is a logical
// interface with no port number.

var interfaceBlockRe = regexp.MustCompile(`^interface (\d+)$`)
var interfaceNameRe = regexp.MustCompile(`^\s+name "([^"]*)"`)
var interfaceDisableRe = regexp.MustCompile(`^\s+disable\s*$`)

// InterfaceConfig is the effective configuration of one physical port as
// recorded in its `interface <n>` block of the running configuration. Port
// is the port number, Name is the configured port name (empty when none),
// and Shutdown is true when the block carries a `disable` line (the
// default `enable` is not shown by the device).
type InterfaceConfig struct {
	Port     int
	Name     string
	Shutdown bool
}

// ParseRunningConfigInterfaces parses every root-level `interface <n>`
// block of a "show running-config" output into a map of port number to its
// effective configuration. A port is present in the map only when its
// block carries a `name` or `disable` line; a caller that needs the
// physical presence of a port must check "show interfaces" separately. A
// switch can carry several interface blocks, so this is a line scanner
// rather than part of the single-record running-config template.
func ParseRunningConfigInterfaces(output string) (map[int]InterfaceConfig, error) {
	ifaces := make(map[int]InterfaceConfig)
	var cur *InterfaceConfig
	inBlock := false
	var number int
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if inBlock && !strings.HasPrefix(line, " ") {
			// A root-level line closes the current block; it may
			// itself open a new one.
			inBlock = false
			if cur != nil {
				ifaces[number] = *cur
			}
			cur = nil
		}
		if inBlock {
			if m := interfaceNameRe.FindStringSubmatch(line); m != nil {
				if cur == nil {
					cur = &InterfaceConfig{Port: number}
				}
				cur.Name = m[1]
			} else if interfaceDisableRe.MatchString(line) {
				if cur == nil {
					cur = &InterfaceConfig{Port: number}
				}
				cur.Shutdown = true
			}
			continue
		}
		m := interfaceBlockRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("aoss: invalid interface number %q", m[1])
		}
		if n < 1 || n > MaxPort {
			return nil, fmt.Errorf("aoss: interface %d out of range 1-%d", n, MaxPort)
		}
		inBlock = true
		number = n
	}
	if inBlock && cur != nil {
		ifaces[number] = *cur
	}
	return ifaces, nil
}

// BuildInterfaceConfig returns the configuration block that moves the
// port from current (nil on create) to desired, e.g.
//
//	interface 24
//	name "OPTIMUX-TIE"
//	disable
//	exit
//
// It returns "" when the existing configuration would not change at all.
func BuildInterfaceConfig(current, desired *InterfaceConfig) string {
	lines := []string{fmt.Sprintf("interface %d", desired.Port)}
	if current != nil {
		if desired.Name != current.Name {
			lines = append(lines, fmt.Sprintf("name %q", desired.Name))
		}
		if desired.Shutdown != current.Shutdown {
			if desired.Shutdown {
				lines = append(lines, "disable")
			} else {
				lines = append(lines, "enable")
			}
		}
		if len(lines) == 1 {
			return ""
		}
	} else {
		if desired.Name != "" {
			lines = append(lines, fmt.Sprintf("name %q", desired.Name))
		}
		if desired.Shutdown {
			lines = append(lines, "disable")
		}
	}
	lines = append(lines, "exit")
	return strings.Join(lines, "\n")
}
