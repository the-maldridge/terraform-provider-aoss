package client

import (
	"context"
	"strings"
	"testing"
	"time"

	scraplicli "github.com/scrapli/scrapligo/v2/cli"
)

const fixturePath = "../../fixtures/aoss/raw_session.bin"

// TestFixtureOpen replays the first prompt of the recorded session to
// prove the embedded definition parses and the test transport can reach
// the post-login prompt without real credentials.
func TestFixtureOpen(t *testing.T) {
	cl, err := NewFromFixture(fixturePath)
	if err != nil {
		t.Fatalf("NewFromFixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cl.Open(ctx); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	res, err := cl.GetPrompt(ctx)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if got := res.Result(); got != "idf02#" {
		t.Fatalf("GetPrompt = %q, want %q", got, "idf02#")
	}
}

// TestFixtureReplay replays the entire recorded session through the same
// operation sequence the collector (cmd/collect) used to capture it.
//
// The test transport streams fixtures/aoss/raw_session.bin in order, so
// every SendInput/EnterMode/GetPrompt here must line up with the bytes the
// collector consumed at that step; any divergence desynchronizes the stream
// and the offending operation times out. Keeping the operation order exactly
// in sync with the collector is what makes the replay deterministic.
func TestFixtureReplay(t *testing.T) {
	cl, err := NewFromFixture(fixturePath)
	if err != nil {
		t.Fatalf("NewFromFixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := cl.Open(ctx); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if err := cl.Close(ctx); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	prompt := func(want string) {
		t.Helper()
		res, err := cl.GetPrompt(ctx)
		if err != nil {
			t.Fatalf("GetPrompt: %v", err)
		}
		got := strings.TrimSpace(res.Result())
		if got != want {
			t.Fatalf("GetPrompt = %q, want %q", got, want)
		}
	}

	// sendOK runs a command expected to succeed and returns the output.
	sendOK := func(cmd string) string {
		t.Helper()
		out, err := cl.Show(ctx, cmd)
		if err != nil {
			t.Fatalf("SendInput(%q): %v", cmd, err)
		}
		return out
	}

	// sendFail runs a command expected to trip a failure indicator and
	// returns the raw result for substring assertions. It mirrors the
	// collector's sendSoft, which tolerates device-reported failures.
	// opts are forwarded verbatim; config-mode sends must pass
	// scraplicli.WithRequestedMode so scrapligo does not auto-transition
	// back to the default mode before sending.
	sendFail := func(cmd, wantSub string, opts ...scraplicli.Option) {
		t.Helper()
		res, err := cl.SendInput(ctx, cmd, opts...)
		if err == nil {
			t.Fatalf("SendInput(%q): expected a device-reported failure, got none", cmd)
		}
		if res == nil {
			t.Fatalf("SendInput(%q): expected a result, got nil", cmd)
		}
		if got := res.Result(); !strings.Contains(got, wantSub) {
			t.Fatalf("SendInput(%q) = %q, want it to contain %q", cmd, got, wantSub)
		}
	}

	// ---- version ----
	if out := sendOK("show version"); !strings.Contains(out, "YA.16.11.0026") || !strings.Contains(out, "Image stamp:") {
		t.Fatalf("show version output missing expected fields: %q", out)
	}

	// ---- modes ----
	if _, err := cl.EnterMode(ctx, "privileged_exec"); err != nil {
		t.Fatalf("EnterMode(privileged_exec): %v", err)
	}
	prompt("idf02#")
	if _, err := cl.EnterMode(ctx, "configuration"); err != nil {
		t.Fatalf("EnterMode(configuration): %v", err)
	}
	prompt("idf02(config)#")
	if _, err := cl.EnterMode(ctx, "privileged_exec"); err != nil {
		t.Fatalf("EnterMode(privileged_exec): %v", err)
	}

	// ---- config_sessions ----
	if _, err := cl.EnterMode(ctx, "configuration"); err != nil {
		t.Fatalf("EnterMode(configuration): %v", err)
	}
	// Config-mode sends must carry WithRequestedMode: scrapligo resolves
	// an empty requested mode to the definition's default mode and
	// auto-transitions when the two differ, which would consume the
	// "end" echo bytes and desynchronize the replay.
	for _, cmd := range []string{"vlan 1", "exit", "interface 1", "exit"} {
		if _, err := cl.SendInput(ctx, cmd, scraplicli.WithRequestedMode("configuration")); err != nil {
			t.Fatalf("SendInput(%q): %v", cmd, err)
		}
	}
	if _, err := cl.EnterMode(ctx, "privileged_exec"); err != nil {
		t.Fatalf("EnterMode(privileged_exec): %v", err)
	}

	// ---- error_strings ----
	sendFail("show badcommand123", "Invalid input:")
	sendFail("show", "Incomplete input:")
	sendFail("show interfaces bogus0/0", "Module not present for port or invalid port")
	if _, err := cl.EnterMode(ctx, "configuration"); err != nil {
		t.Fatalf("EnterMode(configuration): %v", err)
	}
	sendFail("badcommand123", "Invalid input:", scraplicli.WithRequestedMode("configuration"))
	sendFail("vlan 1 name "+strings.Repeat("x", 64), "too long. Allowed length is", scraplicli.WithRequestedMode("configuration"))
	sendFail("interface bogus0/0", "Module not present for port or invalid port", scraplicli.WithRequestedMode("configuration"))
	if _, err := cl.EnterMode(ctx, "privileged_exec"); err != nil {
		t.Fatalf("EnterMode(privileged_exec): %v", err)
	}

	// ---- running_config ----
	if out := sendOK("show running-config"); !strings.Contains(out, "hostname") {
		t.Fatalf("show running-config output missing hostname: %q", out)
	}

	// ---- shows ----
	shows := []struct {
		cmd string
		sub string
	}{
		{"show vlan", "VLAN Information"},
		{"show vlan 1", "VLAN Information - VLAN 1"},
		{"show interfaces", "Port Counters"},
		{"show interface 1", "Port Counters for port 1"},
		{"show ip route", "IP Route Entries"},
		{"show arp", "IP ARP table"},
		{"show lldp info remote-device", "LLDP Remote Devices Information"},
		{"show lldp info local-device", "LLDP Local Device Information"},
		{"show system", "General System Information"},
		{"show time", "2026"},
		{"show running-config", "hostname"},
	}
	for _, s := range shows {
		if out := sendOK(s.cmd); !strings.Contains(out, s.sub) {
			t.Fatalf("%s output missing %q: %q", s.cmd, s.sub, out)
		}
	}
	// ---- restore ----
	if _, err := cl.SendInput(ctx, "write memory"); err != nil {
		t.Fatalf("SendInput(write memory): %v", err)
	}
}
