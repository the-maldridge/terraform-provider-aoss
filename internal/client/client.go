// Package client wraps scrapligo around the AOS-S platform definition.
//
// It exists so the provider (and the fixture collector) have one place to
// open, drive, and close sessions against switches running HP AOS-S, and
// so the captured fixtures can be replayed through a test transport in
// unit tests without touching a real switch.
package client

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	scraplicli "github.com/scrapli/scrapligo/v2/cli"
	scraplioptions "github.com/scrapli/scrapligo/v2/options"
)

// definitionContent holds the AOS-S platform definition embedded in this
// package so the provider works from any working directory (a repo-relative
// path would break once the binary is installed and run from elsewhere).
//
//go:embed hp_aoss.yaml
var definitionContent []byte

// DefinitionPlatform is the platform name assigned to the embedded
// definition.
const DefinitionPlatform = "hp_aoss"

// DefinitionContent returns the embedded AOS-S platform definition YAML.
// The collector uses this to drive a live switch with exactly the same
// definition the provider replays against fixtures.
func DefinitionContent() []byte {
	return definitionContent
}

// ErrBadTransport is returned when an unknown transport is requested.
func errBadTransport(name string) error {
	return fmt.Errorf("aoss: unknown transport %q (expected %q or %q)", name, TransportSSH2, TransportBin)
}

// Client is an open (or openable) session to an AOS-S switch.
type Client struct {
	c *scraplicli.Cli
}

// New builds an unopened client for a live switch.
func New(cfg ClientConfig) (*Client, error) {
	opts, err := cfg.DefaultOptions()
	if err != nil {
		return nil, err
	}
	opts, err = cfg.appendTransport(opts)
	if err != nil {
		return nil, err
	}
	c, err := scraplicli.NewCli(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("aoss: building cli client: %w", err)
	}
	return &Client{c: c}, nil
}

// NewFromFixture builds a client that replays the recorded terminal byte
// stream at fixturePath (a session captured with WithSessionRecorderPath)
// through scrapligo's test transport instead of connecting to a switch.
func NewFromFixture(fixturePath string) (*Client, error) {
	cfg := ClientConfig{
		Host:     "fixture",
		Username: "admin",
		Password: "admin",
	}
	opts, err := cfg.DefaultOptions()
	if err != nil {
		return nil, err
	}
	opts = append(opts,
		scraplioptions.WithTransportTest(),
		scraplioptions.WithTestTransportF(fixturePath),
		scraplioptions.WithReadSize(1),
		scraplioptions.WithOperationTimeout(30*time.Second),
		scraplioptions.WithBypassInSessionAuth(),
	)
	c, err := scraplicli.NewCli(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("aoss: building fixture-replay client: %w", err)
	}
	return &Client{c: c}, nil
}

// Open opens the session. For a fixture client this replays the recorded
// stream up to and including the first prompt (the post-login banner and
// the on-open "no page" instruction are consumed as part of the replay).
func (cl *Client) Open(ctx context.Context) error {
	_, err := cl.c.Open(ctx)
	if err != nil {
		return fmt.Errorf("aoss: opening session: %w", err)
	}
	return nil
}

// Close closes the session.
func (cl *Client) Close(ctx context.Context) error {
	_, err := cl.c.Close(ctx)
	if err != nil {
		return fmt.Errorf("aoss: closing session: %w", err)
	}
	return nil
}

// EnterMode transitions the session to the named mode using the
// definition's mode graph.
func (cl *Client) EnterMode(ctx context.Context, mode string) (*scraplicli.Result, error) {
	res, err := cl.c.EnterMode(ctx, mode)
	if err != nil {
		return nil, fmt.Errorf("aoss: entering mode %q: %w", mode, err)
	}
	return res, nil
}

// GetPrompt returns the current prompt.
func (cl *Client) GetPrompt(ctx context.Context) (*scraplicli.Result, error) {
	return cl.c.GetPrompt(ctx)
}

// SendInput sends a single input in the current mode and returns the
// device's response. A device-reported failure indicator (see
// failure_indicators in the definition) makes the returned Result report
// Failed() == true; callers should check that before acting.
//
// opts are forwarded to the underlying scrapligo client. When operating in a
// mode other than the definition's default (configuration), callers MUST pass
// scraplicli.WithRequestedMode, otherwise scrapligo auto-transitions back to
// the default mode before sending the input and the session desynchronizes.
func (cl *Client) SendInput(ctx context.Context, input string, opts ...scraplicli.Option) (*scraplicli.Result, error) {
	res, err := cl.c.SendInput(ctx, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("aoss: sending input %q: %w", input, err)
	}
	if res.Failed() {
		return res, fmt.Errorf("aoss: device rejected input %q: %s", input, strings.TrimSpace(res.Result()))
	}
	return res, nil
}

// aossConfigPromptPattern is a non-anchored variant of the configuration
// mode prompt pattern from the platform definition, for use as a
// substring ("contains") match against ReadWithCallbacks output windows.
// The definition's pattern is anchored ^...$ for full-buffer prompt
// detection and would not match inside a multi-line window.
const aossConfigPromptPattern = `[\w.\-@/: ]{1,63}\([\w.\-@/:]{1,63}\)[>#$]`

// SendConfig transitions to configuration mode, sends input, and returns
// to privileged exec. It is the unit of every Terraform resource
// mutation.
//
// input may span multiple lines: each line is sent as its own input and
// read back to the prompt it elicits. AOS-S processes configuration
// commands one at a time and prints a new prompt after each, so sending a
// whole block in a single SendInput strands the read on a prompt that
// never arrives and the operation hangs until its context expires.
func (cl *Client) SendConfig(ctx context.Context, input string) (*scraplicli.Result, error) {
	if _, err := cl.EnterMode(ctx, "configuration"); err != nil {
		return nil, fmt.Errorf("aoss: entering configuration mode: %w", err)
	}
	// Returning to privileged exec is best-effort: the config has already
	// been accepted, so a failure here must not mask a successful mutation.
	defer func() {
		_, _ = cl.EnterMode(ctx, "privileged_exec")
	}()
	lines := make([]string, 0, strings.Count(input, "\n")+1)
	for _, line := range strings.Split(input, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("aoss: sending config input: empty input")
	}
	var res *scraplicli.Result
	for _, line := range lines {
		var err error
		res, err = cl.SendInput(ctx, line, scraplicli.WithRequestedMode("configuration"))
		if err != nil {
			return res, fmt.Errorf("aoss: sending config input %q: %w", line, err)
		}
	}
	return res, nil
}

// RemoveVLAN deletes a VLAN in configuration mode.
//
// "no vlan <id>" is conditionally interactive: when deleting the VLAN
// would move at least one port to the default VLAN (a port's only
// untagged home is removed), AOS-S prints
// "The following ports will be moved to the default VLAN: ..." and waits
// at a "[y/n]" prompt. A plain SendInput cannot see that prompt, so the
// read runs to the operation timeout and the VLAN is never removed.
// confirm reports whether the confirmation may appear (see
// NeedsVLANDeleteConfirm); when it may, the command is driven with
// ReadWithCallbacks: one callback answers "Y" the moment "[y/n]"
// appears (once, and only if it appears, so a device that skips the
// confirmation is never answered), and a completing callback ends the
// read the moment the configuration prompt returns - either the
// confirmation was accepted, or no confirmation was needed at all.
// When confirm is false, a plain SendInput is used.
//
// This cannot use scrapligo's SendPromptedInput: libscrapli
// 0.0.1-rc.33 (pinned by scrapligo v2.0.0-rc.18) panics with "attempt to
// use null value" in sendPromptedInput whenever a non-empty prompt
// pattern is supplied, because it reuses the confirmation read's check
// arguments (pattern left null) for the trailing prompt read.
//
// A device-reported failure indicator in the response (for example a
// rejected answer in a race) is not surfaced as an error; the caller
// verifies the running config afterwards and is the authority.
func (cl *Client) RemoveVLAN(ctx context.Context, id int, confirm bool) error {
	if _, err := cl.EnterMode(ctx, "configuration"); err != nil {
		return fmt.Errorf("aoss: entering configuration mode: %w", err)
	}
	// Returning to privileged exec is best-effort: the delete has already
	// been accepted, so a failure here must not mask it.
	defer func() {
		_, _ = cl.EnterMode(ctx, "privileged_exec")
	}()
	cmd := fmt.Sprintf("no vlan %d", id)
	if !confirm {
		_, err := cl.SendInput(ctx, cmd, scraplicli.WithRequestedMode("configuration"))
		return err
	}
	// searchDepth lets each callback rescan the most recent output even
	// after the other fired: ReadWithCallbacks only advances the search
	// position past the end of the buffer at the moment a callback runs,
	// so without depth the completing callback would never see the
	// configuration prompt when "[y/n]" and the prompt arrive in the
	// same read chunk.
	_, err := cl.c.ReadWithCallbacks(ctx, cmd,
		scraplicli.NewReadCallback("confirm",
			func(ctx context.Context, c *scraplicli.Cli, _, _ string) error {
				return c.WriteAndReturn("Y")
			},
			scraplicli.WithContainsPattern(`(?i)\[y/n\]`),
			scraplicli.WithOnce(),
			scraplicli.WithSearchDepth(512),
		),
		scraplicli.NewReadCallback("done",
			func(ctx context.Context, _ *scraplicli.Cli, _, _ string) error { return nil },
			scraplicli.WithContainsPattern(aossConfigPromptPattern),
			scraplicli.WithCompletes(),
			scraplicli.WithSearchDepth(512),
		),
	)
	if err != nil {
		return fmt.Errorf("aoss: removing vlan %d: %w", id, err)
	}
	return nil
}

// Show runs a "show" command in privileged exec and returns the raw
// output.
func (cl *Client) Show(ctx context.Context, command string) (string, error) {
	res, err := cl.SendInput(ctx, command)
	if err != nil {
		return "", err
	}
	return res.Result(), nil
}

// Write writes raw input to the session without a return; used for
// pager interrupts and the like.
func (cl *Client) Write(input string) error {
	if err := cl.c.Write(input); err != nil {
		return fmt.Errorf("aoss: write: %w", err)
	}
	return nil
}
