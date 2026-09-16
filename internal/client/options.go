package client

import (
	"github.com/scrapli/scrapligo/v2/options"
)

// Transport names accepted by [Client.Open].
const (
	// TransportSSH2 uses the built-in libssh2 transport. It authenticates at
	// the SSH protocol layer, so the in-session prompt-matching auth loop
	// never runs; the post-login banner is dismissed by the library's
	// normal "send a return before reading the prompt" step. The switch's
	// host key must already be in KnownHostsPath.
	TransportSSH2 = "ssh2"
	// TransportBin shells out to the local ssh client. AOS-S prints a
	// restricted-rights legend ("Press any key to continue") after
	// password acceptance, which the in-session auth loop cannot
	// recognize; Open passes BypassInSessionAuth for this transport so
	// the library does not wait for prompts the banner flow never shows
	// in the recorded byte stream.
	TransportBin = "bin"
)

// ClientConfig holds everything needed to open a session to an AOS-S
// switch.
type ClientConfig struct {
	// Host is the switch address.
	Host string
	// Port defaults to 22 when zero.
	Port uint16
	// Username and Password are sent to the switch.
	Username string
	Password string
	// Transport selects the underlying transport; defaults to
	// [TransportSSH2].
	Transport string
	// KnownHostsPath is the known_hosts file for the ssh2 transport.
	// Empty means the default location (~/.ssh/known_hosts).
	KnownHostsPath string
	// UsernamePattern/PasswordPattern override the in-session auth
	// prompt patterns. Leave empty for the library defaults; set
	// UsernamePattern to the post-login banner text
	// ("press any key to continue") when driving the bin transport
	// against a live switch.
	UsernamePattern string
	PasswordPattern string
	// TermWidth/TermHeight are the terminal dimensions advertised to the
	// device; default 200x50 (matching the fixture collection).
	TermWidth  uint16
	TermHeight uint16
}

// DefaultOptions returns the scrapligo options common to every AOS-S
// session: the captured platform definition, terminal size, and
// credentials.
func (c *ClientConfig) DefaultOptions() ([]options.Option, error) {
	if c.Port == 0 {
		c.Port = 22
	}
	if c.TermWidth == 0 {
		c.TermWidth = 200
	}
	if c.TermHeight == 0 {
		c.TermHeight = 50
	}
	opts := []options.Option{
		options.WithPort(c.Port),
		options.WithTermWidth(c.TermWidth),
		options.WithTermHeight(c.TermHeight),
		options.WithUsername(c.Username),
		options.WithPassword(c.Password),
		options.WithDefinitionContent(DefinitionPlatform, definitionContent),
	}
	if c.UsernamePattern != "" {
		opts = append(opts, options.WithUsernamePattern(c.UsernamePattern))
	}
	if c.PasswordPattern != "" {
		opts = append(opts, options.WithPasswordPattern(c.PasswordPattern))
	}
	return opts, nil
}

// appendTransport adds the transport-specific options for the requested
// [ClientConfig.Transport].
func (c *ClientConfig) appendTransport(opts []options.Option) ([]options.Option, error) {
	switch c.Transport {
	case TransportSSH2, "":
		opts = append(opts, options.WithTransportSSH2())
		if c.KnownHostsPath != "" {
			opts = append(opts, options.WithSSH2KnownHostsPath(c.KnownHostsPath))
		}
		return opts, nil
	case TransportBin:
		opts = append(opts,
			options.WithTransportBin(),
			options.WithBypassInSessionAuth(),
		)
		if c.KnownHostsPath != "" {
			opts = append(opts, options.WithSSH2KnownHostsPath(c.KnownHostsPath))
		}
		return opts, nil
	default:
		return nil, errBadTransport(c.Transport)
	}
}
