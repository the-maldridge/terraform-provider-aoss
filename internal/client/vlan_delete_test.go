package client

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// deleteFixturePrefix extracts the recorded stream up to the point where
// the session's first real command ("show version", the first operation
// in TestFixtureReplay) begins. The prefix ends exactly at the post-Open
// stream position: the banner, the on-open "no page" instruction and its
// resulting prompt are all inside the prefix and are consumed by Open the
// same way they are in TestFixtureReplay.
func deleteFixturePrefix(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	idx := bytes.Index(data, []byte("show version"))
	if idx < 0 {
		t.Fatalf("fixture %s: \"show version\" not found", fixturePath)
	}
	return data[:idx]
}

// writeDeleteFixture writes the login prefix plus a synthetic delete
// sequence to a temp file and returns its path for NewFromFixture.
func writeDeleteFixture(t *testing.T, seq string) string {
	t.Helper()
	data := append(append([]byte{}, deleteFixturePrefix(t)...), []byte(seq)...)
	path := filepath.Join(t.TempDir(), "delete_session.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing synthetic fixture: %v", err)
	}
	return path
}

func TestRemoveVLANReplayPrompted(t *testing.T) {
	// A port's only untagged home is the deleted VLAN, so the device
	// confirms before moving it to the default VLAN. The stream is the
	// login prefix plus:
	//   config terminal        (EnterMode configuration)
	//   no vlan 3  -> prompt + [y/n] (confirm callback answers Y)
	//   end            (deferred EnterMode privileged_exec)
	seq := "config terminalidf02(config)#" +
		"no vlan 3The following ports will be moved to the default VLAN:\r\n6-10\r\nDo you want to continue?\r\n[y/n] Yidf02(config)#" +
		"endidf02#idf02#"
	cl, err := NewFromFixture(writeDeleteFixture(t, seq))
	if err != nil {
		t.Fatalf("NewFromFixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := cl.Open(ctx); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	if err := cl.RemoveVLAN(ctx, 3, true); err != nil {
		t.Fatalf("RemoveVLAN(3, true): %v", err)
	}
	// The deferred "end" must have completed and returned the session to
	// privileged exec: the stream is still aligned.
	res, err := cl.GetPrompt(ctx)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if got := strings.TrimSpace(res.Result()); got != "idf02#" {
		t.Fatalf("GetPrompt = %q, want %q", got, "idf02#")
	}
}

func TestRemoveVLANReplayNoPrompt(t *testing.T) {
	// No port is orphaned, so the device does not confirm: no "[y/n]"
	// appears, the confirm callback never fires (no spurious "Y" is
	// typed), and the completing callback ends the read on the
	// configuration prompt.
	seq := "config terminalidf02(config)#" +
		"no vlan 3idf02(config)#" +
		"endidf02#idf02#"
	cl, err := NewFromFixture(writeDeleteFixture(t, seq))
	if err != nil {
		t.Fatalf("NewFromFixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := cl.Open(ctx); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	if err := cl.RemoveVLAN(ctx, 3, true); err != nil {
		t.Fatalf("RemoveVLAN(3, true): %v", err)
	}
	res, err := cl.GetPrompt(ctx)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if got := strings.TrimSpace(res.Result()); got != "idf02#" {
		t.Fatalf("GetPrompt = %q, want %q", got, "idf02#")
	}
}

func TestRemoveVLANReplayPlain(t *testing.T) {
	// confirm=false: the plain SendInput path. A failure indicator in the
	// response is a real error (nothing was being confirmed).
	seq := "config terminalidf02(config)#" +
		"no vlan 3Invalid input: vlan 3 does not exist\r\nidf02(config)#" +
		"endidf02#"
	cl, err := NewFromFixture(writeDeleteFixture(t, seq))
	if err != nil {
		t.Fatalf("NewFromFixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := cl.Open(ctx); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		_ = cl.Close(ctx)
	}()

	if err := cl.RemoveVLAN(ctx, 3, false); err == nil {
		t.Fatal("RemoveVLAN(3, false): expected a device-reported failure, got none")
	}
}

func TestNeedsVLANDeleteConfirm(t *testing.T) {
	cases := map[string]struct {
		v    VLANConfig
		want bool
	}{
		"untagged ports":             {VLANConfig{ID: 3, Untagged: []string{"6", "7"}}, true},
		"tagged trunk only":          {VLANConfig{ID: 10, Tagged: []string{"Trk1"}}, false},
		"tagged ports only":          {VLANConfig{ID: 15, Tagged: []string{"24", "25"}}, false},
		"no members":                 {VLANConfig{ID: 1}, false},
		"untagged with tagged mixed": {VLANConfig{ID: 20, Tagged: []string{"Trk2"}, Untagged: []string{"12"}}, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := NeedsVLANDeleteConfirm(tc.v); got != tc.want {
				t.Fatalf("NeedsVLANDeleteConfirm(%+v) = %v, want %v", tc.v, got, tc.want)
			}
		})
	}
}
