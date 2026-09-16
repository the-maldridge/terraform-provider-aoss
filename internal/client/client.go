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

// SendConfig transitions to configuration mode, sends input, and returns
// to privileged exec. It is the unit of every Terraform resource
// mutation.
func (cl *Client) SendConfig(ctx context.Context, input string) (*scraplicli.Result, error) {
	if _, err := cl.c.EnterMode(ctx, "configuration"); err != nil {
		return nil, fmt.Errorf("aoss: entering configuration mode: %w", err)
	}
	defer func() {
		_, _ = cl.c.EnterMode(ctx, "privileged_exec")
	}()
	res, err := cl.c.SendInput(ctx, input, scraplicli.WithRequestedMode("configuration"))
	if err != nil {
		return nil, fmt.Errorf("aoss: sending config input %q: %w", input, err)
	}
	if res.Failed() {
		return res, fmt.Errorf("aoss: device rejected config input %q: %s", input, strings.TrimSpace(res.Result()))
	}
	return res, nil
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
