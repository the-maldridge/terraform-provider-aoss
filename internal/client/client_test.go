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

// TestSendConfigMultiLine drives the fixed multi-line config path. It
// replays the fixture's prefix exactly as TestFixtureReplay does (so the
// stream is aligned at the same point), then issues the whole
// config_sessions block through a single SendConfig call instead of four
// separate SendInputs. SendConfig splits the block and sends each line as
// its own input read back to the prompt it elicits, so the per-line stream
// consumption is identical to the sequence TestFixtureReplay already
// proves. If SendConfig regresses to a single multi-line SendInput, the
// read strands on a prompt that never arrives and the operation times out.
func TestSendConfigMultiLine(t *testing.T) {
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
		if got := strings.TrimSpace(res.Result()); got != want {
			t.Fatalf("GetPrompt = %q, want %q", got, want)
		}
	}

	// Mirror TestFixtureReplay's version + modes sections verbatim so the
	// replayed stream is at the identical position when the config block
	// is sent.
	if out, err := cl.Show(ctx, "show version"); err != nil || !strings.Contains(out, "YA.16.11.0026") {
		t.Fatalf("show version = %q, err %v; expected YA.16.11.0026", out, err)
	}
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

	// The config_sessions block through a single multi-line SendConfig
	// call: it enters configuration mode, sends each line to its own
	// prompt, and returns to privileged exec.
	if _, err := cl.SendConfig(ctx, "vlan 1\nexit\ninterface 1\nexit"); err != nil {
		t.Fatalf("SendConfig(multi-line): %v", err)
	}

	// The session must be back in privileged exec and aligned with the
	// stream: the next recorded command is the error_strings probe.
	res, err := cl.SendInput(ctx, "show badcommand123")
	if err == nil {
		t.Fatalf("SendInput(show badcommand123): expected a device-reported failure, got none")
	}
	if res == nil || !strings.Contains(res.Result(), "Invalid input: badcommand123") {
		t.Fatalf("SendInput(show badcommand123) = %v, want it to contain %q", res, "Invalid input: badcommand123")
	}
}
