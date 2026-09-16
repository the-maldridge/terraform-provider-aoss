// Parse functions turn raw "show" command output (captured by the fixture
// collector or fetched live) into typed values, using the TextFSM
// templates embedded alongside this package.
//
// The templates are written for gotextfsm (the sirikothe port of TextFSM),
// which matches only the named capture groups in the RULE lines; the Value
// lines register the field names and stay free of (?P groups so every
// field comes back as a plain string. Fields captured positionally from
// fixed-width columns carry trailing spaces, so every value is trimmed
// with strings.TrimSpace before it is returned.
package client

import (
	"embed"
	"fmt"
	"strings"

	"github.com/sirikothe/gotextfsm"
)

//go:embed templates/*.textfsm
var templateFS embed.FS

// Version holds the parsed "show version" output.
type Version struct {
	BuildDate  string
	Version    string
	BootImage  string
	RomVersion string
}

// System holds the parsed "show system" output.
type System struct {
	SystemName       string
	SoftwareRevision string
	BaseMAC          string
	RomVersion       string
	Serial           string
	Uptime           string
	MemTotal         string
	MemFree          string
	CPUUtil          string
}

// Time holds the parsed "show time" output.
type Time struct {
	DayOfWeek string
	Month     string
	Day       string
	Time      string
	Year      string
}

// ArpEntry is one row of the "show arp" table.
type ArpEntry struct {
	IP   string
	MAC  string
	Type string
	Port string
}

// IPRoute is one row of the "show ip route" table.
type IPRoute struct {
	Destination string
	Gateway     string
	VLAN        string
	Type        string
	SubType     string
	Metric      string
	Distance    string
}

// VLAN is one row of the "show vlan" table.
type VLAN struct {
	ID     string
	Name   string
	Status string
	Voice  string
	Jumbo  string
}

// InterfaceCounters is one row of the "show interfaces" summary table.
type InterfaceCounters struct {
	Port        string
	TotalBytes  string
	TotalFrames string
	ErrorsRx    string
	DropsTx     string
	FlowCtrl    string
}

// InterfaceDetail holds the parsed "show interface <port>" output.
type InterfaceDetail struct {
	Port                string
	Name                string
	MACAddress          string
	LinkStatus          string
	PortEnabled         string
	BytesRx             string
	BytesTx             string
	UnicastRx           string
	UnicastTx           string
	BcastMcastRx        string
	BcastMcastTx        string
	FCSRx               string
	DropsTx             string
	AlignmentRx         string
	CollisionsTx        string
	RuntsRx             string
	LateCollisionsTx    string
	GiantsRx            string
	ExcessiveCollisions string
	TotalRxErrors       string
	DeferredTx          string
	DiscardRx           string
	OutQueueLen         string
	UnknownProtos       string
	RateRxBps           string
	RateTxBps           string
	RateUnicastRxPps    string
	RateUnicastTxPps    string
	RateBmcastRxPps     string
	RateBmcastTxPps     string
	UtilizationRx       string
	UtilizationTx       string
}

// LLDPNeighbor is one row of the "show lldp neighbors" table.
type LLDPNeighbor struct {
	LocalPort string
	ChassisID string
	PortID    string
	PortDescr string
	SysName   string
}

// LLDPInfo holds the parsed "show lldp local" device information block.
type LLDPInfo struct {
	ChassisID           string
	SystemName          string
	SystemDescription   string
	SystemCapsSupported string
	SystemCapsEnabled   string
	MgmtType            string
	MgmtAddress         string
}

// RunningConfig holds the minimal parsed "show running-config" output.
type RunningConfig struct {
	Release  string
	Hostname string
}

// runTemplate loads the named embedded TextFSM template, runs the output
// through it, and returns one trimmed string map per parsed record.
func runTemplate(name, output string) ([]map[string]string, error) {
	tpl, err := templateFS.ReadFile("templates/" + name)
	if err != nil {
		return nil, fmt.Errorf("aoss: loading template %s: %w", name, err)
	}
	fsm := gotextfsm.TextFSM{}
	if err := fsm.ParseString(string(tpl)); err != nil {
		return nil, fmt.Errorf("aoss: parsing template %s: %w", name, err)
	}
	p := gotextfsm.ParserOutput{}
	if err := p.ParseTextString(output, fsm, true); err != nil {
		return nil, fmt.Errorf("aoss: running template %s: %w", name, err)
	}
	rows := make([]map[string]string, 0, len(p.Dict))
	for _, rec := range p.Dict {
		row := make(map[string]string, len(rec))
		for k, v := range rec {
			row[k] = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// requireSingle returns the only record for single-record templates or an
// error if the parser produced any other number of records.
func requireSingle(rows []map[string]string, template string) (map[string]string, error) {
	if len(rows) != 1 {
		return nil, fmt.Errorf("aoss: %s: got %d records, want 1", template, len(rows))
	}
	return rows[0], nil
}

// ParseVersion parses the output of "show version".
func ParseVersion(output string) (Version, error) {
	rows, err := runTemplate("show_version.textfsm", output)
	if err != nil {
		return Version{}, err
	}
	row, err := requireSingle(rows, "show_version")
	if err != nil {
		return Version{}, err
	}
	return Version{
		BuildDate:  row["build_date"],
		Version:    row["version"],
		BootImage:  row["boot_image"],
		RomVersion: row["rom_version"],
	}, nil
}

// ParseSystem parses the output of "show system".
func ParseSystem(output string) (System, error) {
	rows, err := runTemplate("show_system.textfsm", output)
	if err != nil {
		return System{}, err
	}
	row, err := requireSingle(rows, "show_system")
	if err != nil {
		return System{}, err
	}
	return System{
		SystemName:       row["system_name"],
		SoftwareRevision: row["software_revision"],
		BaseMAC:          row["base_mac"],
		RomVersion:       row["rom_version"],
		Serial:           row["serial"],
		Uptime:           row["uptime"],
		MemTotal:         row["mem_total"],
		MemFree:          row["mem_free"],
		CPUUtil:          row["cpu_util"],
	}, nil
}

// ParseTime parses the output of "show time".
func ParseTime(output string) (Time, error) {
	rows, err := runTemplate("show_time.textfsm", output)
	if err != nil {
		return Time{}, err
	}
	row, err := requireSingle(rows, "show_time")
	if err != nil {
		return Time{}, err
	}
	return Time{
		DayOfWeek: row["day_of_week"],
		Month:     row["month"],
		Day:       row["day"],
		Time:      row["time"],
		Year:      row["year"],
	}, nil
}

// ParseArp parses the "show arp" table.
func ParseArp(output string) ([]ArpEntry, error) {
	rows, err := runTemplate("show_arp.textfsm", output)
	if err != nil {
		return nil, err
	}
	entries := make([]ArpEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, ArpEntry{
			IP:   row["ip"],
			MAC:  row["mac"],
			Type: row["type"],
			Port: row["port"],
		})
	}
	return entries, nil
}

// ParseIPRoute parses the "show ip route" table.
func ParseIPRoute(output string) ([]IPRoute, error) {
	rows, err := runTemplate("show_ip_route.textfsm", output)
	if err != nil {
		return nil, err
	}
	routes := make([]IPRoute, 0, len(rows))
	for _, row := range rows {
		routes = append(routes, IPRoute{
			Destination: row["destination"],
			Gateway:     row["gateway"],
			VLAN:        row["vlan"],
			Type:        row["type"],
			SubType:     row["sub_type"],
			Metric:      row["metric"],
			Distance:    row["distance"],
		})
	}
	return routes, nil
}

// ParseVLAN parses the "show vlan" table.
func ParseVLAN(output string) ([]VLAN, error) {
	rows, err := runTemplate("show_vlan.textfsm", output)
	if err != nil {
		return nil, err
	}
	vlans := make([]VLAN, 0, len(rows))
	for _, row := range rows {
		vlans = append(vlans, VLAN{
			ID:     row["id"],
			Name:   row["name"],
			Status: row["status"],
			Voice:  row["voice"],
			Jumbo:  row["jumbo"],
		})
	}
	return vlans, nil
}

// ParseInterfaces parses the "show interfaces" summary table.
func ParseInterfaces(output string) ([]InterfaceCounters, error) {
	rows, err := runTemplate("show_interfaces.textfsm", output)
	if err != nil {
		return nil, err
	}
	ifaces := make([]InterfaceCounters, 0, len(rows))
	for _, row := range rows {
		ifaces = append(ifaces, InterfaceCounters{
			Port:        row["port"],
			TotalBytes:  row["total_bytes"],
			TotalFrames: row["total_frames"],
			ErrorsRx:    row["errors_rx"],
			DropsTx:     row["drops_tx"],
			FlowCtrl:    row["flow_ctrl"],
		})
	}
	return ifaces, nil
}

// ParseInterfaceDetail parses the "show interface <port>" output.
func ParseInterfaceDetail(output string) (InterfaceDetail, error) {
	rows, err := runTemplate("show_interface.textfsm", output)
	if err != nil {
		return InterfaceDetail{}, err
	}
	row, err := requireSingle(rows, "show_interface")
	if err != nil {
		return InterfaceDetail{}, err
	}
	return InterfaceDetail{
		Port:                row["port"],
		Name:                row["name"],
		MACAddress:          row["mac_address"],
		LinkStatus:          row["link_status"],
		PortEnabled:         row["port_enabled"],
		BytesRx:             row["bytes_rx"],
		BytesTx:             row["bytes_tx"],
		UnicastRx:           row["unicast_rx"],
		UnicastTx:           row["unicast_tx"],
		BcastMcastRx:        row["bcast_mcast_rx"],
		BcastMcastTx:        row["bcast_mcast_tx"],
		FCSRx:               row["fcs_rx"],
		DropsTx:             row["drops_tx"],
		AlignmentRx:         row["alignment_rx"],
		CollisionsTx:        row["collisions_tx"],
		RuntsRx:             row["runts_rx"],
		LateCollisionsTx:    row["late_collisions_tx"],
		GiantsRx:            row["giants_rx"],
		ExcessiveCollisions: row["excessive_collisions"],
		TotalRxErrors:       row["total_rx_errors"],
		DeferredTx:          row["deferred_tx"],
		DiscardRx:           row["discard_rx"],
		OutQueueLen:         row["out_queue_len"],
		UnknownProtos:       row["unknown_protos"],
		RateRxBps:           row["rate_rx_bps"],
		RateTxBps:           row["rate_tx_bps"],
		RateUnicastRxPps:    row["rate_unicast_rx_pps"],
		RateUnicastTxPps:    row["rate_unicast_tx_pps"],
		RateBmcastRxPps:     row["rate_bmcast_rx_pps"],
		RateBmcastTxPps:     row["rate_bmcast_tx_pps"],
		UtilizationRx:       row["utilization_rx"],
		UtilizationTx:       row["utilization_tx"],
	}, nil
}

// ParseLLDPNeighbors parses the "show lldp neighbors" table.
func ParseLLDPNeighbors(output string) ([]LLDPNeighbor, error) {
	rows, err := runTemplate("show_lldp_remote.textfsm", output)
	if err != nil {
		return nil, err
	}
	neighbors := make([]LLDPNeighbor, 0, len(rows))
	for _, row := range rows {
		neighbors = append(neighbors, LLDPNeighbor{
			LocalPort: row["local_port"],
			ChassisID: row["chassis_id"],
			PortID:    row["port_id"],
			PortDescr: row["port_descr"],
			SysName:   row["sys_name"],
		})
	}
	return neighbors, nil
}

// ParseLLDPInfo parses the "show lldp local" device information block.
func ParseLLDPInfo(output string) (LLDPInfo, error) {
	rows, err := runTemplate("show_lldp_local.textfsm", output)
	if err != nil {
		return LLDPInfo{}, err
	}
	row, err := requireSingle(rows, "show_lldp_local")
	if err != nil {
		return LLDPInfo{}, err
	}
	return LLDPInfo{
		ChassisID:           row["chassis_id"],
		SystemName:          row["system_name"],
		SystemDescription:   row["system_description"],
		SystemCapsSupported: row["system_caps_supported"],
		SystemCapsEnabled:   row["system_caps_enabled"],
		MgmtType:            row["mgmt_type"],
		MgmtAddress:         row["mgmt_address"],
	}, nil
}

// ParseRunningConfig parses the minimal "show running-config" fields.
func ParseRunningConfig(output string) (RunningConfig, error) {
	rows, err := runTemplate("show_running_config.textfsm", output)
	if err != nil {
		return RunningConfig{}, err
	}
	row, err := requireSingle(rows, "show_running_config")
	if err != nil {
		return RunningConfig{}, err
	}
	return RunningConfig{
		Release:  row["release"],
		Hostname: row["hostname"],
	}, nil
}
