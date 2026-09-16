package client

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/show_version.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseVersion(string(data))
	if err != nil {
		t.Fatalf("ParseVersion: %v", err)
	}
	want := Version{
		BuildDate:  "Jun 17 2025 02:34:30",
		Version:    "YA.16.11.0026",
		BootImage:  "Primary",
		RomVersion: "YA.15.20",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseVersion() = %+v, want %+v", got, want)
	}
}

func TestParseSystem(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/shows/show_system.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseSystem(string(data))
	if err != nil {
		t.Fatalf("ParseSystem: %v", err)
	}
	want := System{
		SystemName:       "idf02",
		SoftwareRevision: "YA.16.11.0026",
		BaseMAC:          "883a30-1a2020",
		RomVersion:       "YA.15.20",
		Serial:           "CN01FP47X1",
		Uptime:           "19 hours",
		MemTotal:         "67,108,864",
		MemFree:          "37,836,892",
		CPUUtil:          "12",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseSystem() = %+v, want %+v", got, want)
	}
}

func TestParseTime(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/shows/show_time.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseTime(string(data))
	if err != nil {
		t.Fatalf("ParseTime: %v", err)
	}
	want := Time{
		DayOfWeek: "Wed",
		Month:     "Sep",
		Day:       "2",
		Time:      "22:29:02",
		Year:      "2026",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseTime() = %+v, want %+v", got, want)
	}
}

func TestParseArp(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/shows/show_arp.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseArp(string(data))
	if err != nil {
		t.Fatalf("ParseArp: %v", err)
	}
	want := []ArpEntry{
		{IP: "172.16.34.1", MAC: "f41e57-36fd7c", Type: "dynamic", Port: "24"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseArp() = %+v, want %+v", got, want)
	}
}

func TestParseIPRoute(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/shows/show_ip_route.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseIPRoute(string(data))
	if err != nil {
		t.Fatalf("ParseIPRoute: %v", err)
	}
	cases := []struct {
		name string
		want IPRoute
	}{
		{
			name: "default static",
			want: IPRoute{
				Destination: "0.0.0.0/0",
				Gateway:     "172.16.34.1",
				VLAN:        "30",
				Type:        "static",
				SubType:     "",
				Metric:      "1",
				Distance:    "1",
			},
		},
		{
			name: "loopback reject",
			want: IPRoute{
				Destination: "127.0.0.0/8",
				Gateway:     "reject",
				VLAN:        "",
				Type:        "static",
				SubType:     "",
				Metric:      "0",
				Distance:    "0",
			},
		},
		{
			name: "loopback connected",
			want: IPRoute{
				Destination: "127.0.0.1/32",
				Gateway:     "",
				VLAN:        "",
				Type:        "connected",
				SubType:     "",
				Metric:      "1",
				Distance:    "0",
			},
		},
		{
			name: "management connected",
			want: IPRoute{
				Destination: "172.16.34.0/24",
				Gateway:     "MGMT",
				VLAN:        "30",
				Type:        "connected",
				SubType:     "",
				Metric:      "1",
				Distance:    "0",
			},
		},
	}
	if len(got) != len(cases) {
		t.Fatalf("ParseIPRoute() returned %d rows, want %d: %+v", len(got), len(cases), got)
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(got[i], tc.want) {
				t.Errorf("ParseIPRoute()[%d] = %+v, want %+v", i, got[i], tc.want)
			}
		})
	}
}

func TestParseVLAN(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/shows/show_vlan.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseVLAN(string(data))
	if err != nil {
		t.Fatalf("ParseVLAN: %v", err)
	}
	names := map[string]string{
		"1":   "DEFAULT_VLAN",
		"10":  "LAN",
		"15":  "TRUST",
		"20":  "WAN",
		"25":  "MEDIA",
		"30":  "MGMT",
		"500": "VLAN500",
		"501": "VLAN501",
		"502": "VLAN502",
		"503": "VLAN503",
		"504": "VLAN504",
	}
	if len(got) != len(names) {
		t.Fatalf("ParseVLAN() returned %d rows, want %d: %+v", len(got), len(names), got)
	}
	for _, row := range got {
		want := VLAN{
			ID:     row.ID,
			Name:   names[row.ID],
			Status: "Port-based",
			Voice:  "No",
			Jumbo:  "No",
		}
		if !reflect.DeepEqual(row, want) {
			t.Errorf("ParseVLAN() row = %+v, want %+v", row, want)
		}
	}
}

func TestParseInterfaces(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/shows/show_interfaces.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseInterfaces(string(data))
	if err != nil {
		t.Fatalf("ParseInterfaces: %v", err)
	}
	want := make([]InterfaceCounters, 0, 28)
	for port := 1; port <= 28; port++ {
		ic := InterfaceCounters{
			Port:        strconv.Itoa(port),
			TotalBytes:  "0",
			TotalFrames: "0",
			ErrorsRx:    "0",
			DropsTx:     "0",
			FlowCtrl:    "off",
		}
		if port == 24 {
			ic.TotalBytes = "81,140,314"
			ic.TotalFrames = "827,733"
		}
		want = append(want, ic)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseInterfaces() = %+v, want %+v", got, want)
	}
}

func TestParseInterfaceDetail(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/shows/show_interface_detail.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseInterfaceDetail(string(data))
	if err != nil {
		t.Fatalf("ParseInterfaceDetail: %v", err)
	}
	zero := make(map[string]string)
	for _, field := range []string{
		"bytes_rx", "bytes_tx", "unicast_rx", "unicast_tx", "bcast_mcast_rx",
		"bcast_mcast_tx", "fcs_rx", "drops_tx", "alignment_rx", "collisions_tx",
		"runts_rx", "late_collisions_tx", "giants_rx", "excessive_collisions",
		"total_rx_errors", "deferred_tx", "discard_rx", "out_queue_len",
		"unknown_protos", "rate_rx_bps", "rate_tx_bps", "rate_unicast_rx_pps",
		"rate_unicast_tx_pps", "rate_bmcast_rx_pps", "rate_bmcast_tx_pps",
		"utilization_rx", "utilization_tx",
	} {
		zero[field] = "0"
	}
	want := InterfaceDetail{
		Port:                "1",
		Name:                "",
		MACAddress:          "883a30-1a203f",
		LinkStatus:          "Down",
		PortEnabled:         "Yes",
		BytesRx:             zero["bytes_rx"],
		BytesTx:             zero["bytes_tx"],
		UnicastRx:           zero["unicast_rx"],
		UnicastTx:           zero["unicast_tx"],
		BcastMcastRx:        zero["bcast_mcast_rx"],
		BcastMcastTx:        zero["bcast_mcast_tx"],
		FCSRx:               zero["fcs_rx"],
		DropsTx:             zero["drops_tx"],
		AlignmentRx:         zero["alignment_rx"],
		CollisionsTx:        zero["collisions_tx"],
		RuntsRx:             zero["runts_rx"],
		LateCollisionsTx:    zero["late_collisions_tx"],
		GiantsRx:            zero["giants_rx"],
		ExcessiveCollisions: zero["excessive_collisions"],
		TotalRxErrors:       zero["total_rx_errors"],
		DeferredTx:          zero["deferred_tx"],
		DiscardRx:           zero["discard_rx"],
		OutQueueLen:         zero["out_queue_len"],
		UnknownProtos:       zero["unknown_protos"],
		RateRxBps:           zero["rate_rx_bps"],
		RateTxBps:           zero["rate_tx_bps"],
		RateUnicastRxPps:    zero["rate_unicast_rx_pps"],
		RateUnicastTxPps:    zero["rate_unicast_tx_pps"],
		RateBmcastRxPps:     zero["rate_bmcast_rx_pps"],
		RateBmcastTxPps:     zero["rate_bmcast_tx_pps"],
		UtilizationRx:       zero["utilization_rx"],
		UtilizationTx:       zero["utilization_tx"],
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseInterfaceDetail() = %+v, want %+v", got, want)
	}
}

func TestParseLLDPNeighbors(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/shows/show_lldp_remote.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseLLDPNeighbors(string(data))
	if err != nil {
		t.Fatalf("ParseLLDPNeighbors: %v", err)
	}
	want := []LLDPNeighbor{
		{LocalPort: "24", ChassisID: "38 32 7a 16 fe 18", PortID: "ether1", PortDescr: "br0/et...", SysName: "optimux1"},
		{LocalPort: "24", ChassisID: "edge0", PortID: "br0/sfp-sfpplus3", PortDescr: "", SysName: "MikroTik"},
		{LocalPort: "24", ChassisID: "edge0", PortID: "mgmt0", PortDescr: "", SysName: "MikroTik"},
		{LocalPort: "24", ChassisID: "edge0", PortID: "lan0", PortDescr: "", SysName: "MikroTik"},
		{LocalPort: "24", ChassisID: "edge0", PortID: "wan0", PortDescr: "", SysName: "MikroTik"},
		{LocalPort: "24", ChassisID: "edge0", PortID: "trust0", PortDescr: "", SysName: "MikroTik"},
		{LocalPort: "24", ChassisID: "edge0", PortID: "media0", PortDescr: "", SysName: "MikroTik"},
		{LocalPort: "24", ChassisID: "optimux1", PortID: "br0/ether1", PortDescr: "", SysName: "MikroTik"},
		{LocalPort: "24", ChassisID: "optimux1", PortID: "mgmt0", PortDescr: "", SysName: "MikroTik"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseLLDPNeighbors() = %+v, want %+v", got, want)
	}
}

func TestParseLLDPInfo(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/shows/show_lldp_local.txt")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseLLDPInfo(string(data))
	if err != nil {
		t.Fatalf("ParseLLDPInfo: %v", err)
	}
	want := LLDPInfo{
		ChassisID:           "88 3a 30 1a 20 20",
		SystemName:          "idf02",
		SystemDescription:   "HP J9773A 2530-24G-PoEP Switch, revision YA.16.11.00...",
		SystemCapsSupported: "bridge",
		SystemCapsEnabled:   "bridge",
		MgmtType:            "ipv4",
		MgmtAddress:         "172.16.34.76",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseLLDPInfo() = %+v, want %+v", got, want)
	}
}

func TestParseRunningConfig(t *testing.T) {
	data, err := os.ReadFile("../../fixtures/aoss/running_config.cfg")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	got, err := ParseRunningConfig(string(data))
	if err != nil {
		t.Fatalf("ParseRunningConfig: %v", err)
	}
	want := RunningConfig{
		Release:  "YA.16.11.0026",
		Hostname: "idf02",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseRunningConfig() = %+v, want %+v", got, want)
	}
}

func TestParseRequiresSingleRecord(t *testing.T) {
	cases := map[string]struct {
		parse func(string) (any, error)
	}{
		"version":        {func(s string) (any, error) { return ParseVersion(s) }},
		"system":         {func(s string) (any, error) { return ParseSystem(s) }},
		"time":           {func(s string) (any, error) { return ParseTime(s) }},
		"interface":      {func(s string) (any, error) { return ParseInterfaceDetail(s) }},
		"lldp_local":     {func(s string) (any, error) { return ParseLLDPInfo(s) }},
		"running_config": {func(s string) (any, error) { return ParseRunningConfig(s) }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tc.parse("")
			if err == nil || !strings.Contains(err.Error(), "got 0 records, want 1") {
				t.Errorf("parse(\"\") error = %v, want ... got 0 records, want 1", err)
			}
		})
	}
}
