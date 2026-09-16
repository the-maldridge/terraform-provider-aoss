// Command collect captures prompt, error, and show-output fixtures from a
// live AOS-S switch so that a custom scrapligo platform definition and
// TextFSM templates can be written and tested.
//
// It uses the same environment variables as the provider:
//
//	AOSS_HOST, AOSS_USERNAME, AOSS_PASSWORD
//
// Optional overrides:
//
//	AOSS_PORT       SSH port (default 22)
//	AOSS_TRANSPORT  scrapligo transport: "bin" (default, shells out to the
//	                local ssh client) or "ssh2" (built-in libssh2; requires
//	                AOSS_KNOWN_HOSTS because libssh2 does not do password
//	                auth without host key verification)
//	AOSS_KNOWN_HOSTS  path to a known_hosts file for the ssh2 transport
//	AOSS_USERNAME_PATTERN / AOSS_PASSWORD_PATTERN
//	                regex patterns for the in-session auth prompts (bin
//	                transport only). Needed to work around the "Press any
//	                key to continue" login banner, see README.
//	AOSS_DEF_FILE   path to a custom scrapligo platform definition YAML
//	AOSS_DEF_NAME   scrapligo platform name (default: the embedded AOS-S definition)
//	AOSS_INTERFACE  interface name to exercise (default 1)
//	AOSS_VLAN       VLAN ID to exercise (default 1)
//	AOSS_OUT        fixture output directory (default fixtures/aoss)
//	AOSS_LOG_LEVEL  scrapligo/libscrapli log level: trace, debug, info,
//	                warn, critical, disabled (default trace). Trace logs go
//	                to <AOSS_OUT>/scrapli.log; the raw terminal stream
//	                (login banner included) is always recorded to
//	                <AOSS_OUT>/raw_session.bin.
//
// The switch should be left in the same state it was found in; every change
// the collector makes is undone or saved before it exits. See
// cmd/collect/README.md for the expected starting configuration.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	scraplicli "github.com/scrapli/scrapligo/v2/cli"
	scraplilogging "github.com/scrapli/scrapligo/v2/logging"
	scraplioptions "github.com/scrapli/scrapligo/v2/options"

	"github.com/the-maldridge/terraform-provider-aoss/internal/client"
)

type collector struct {
	c     *scraplicli.Cli
	out   string
	mode  string // current mode name
	iface string
	vlan  string
}

func main() {
	var defFile, defName string
	flag.StringVar(&defFile, "def-file", envOr("AOSS_DEF_FILE", ""), "path to a scrapligo platform definition YAML")
	flag.StringVar(&defName, "def-name", envOr("AOSS_DEF_NAME", ""), "scrapligo platform name (e.g. hp_comware)")
	flag.Parse()

	host := envOr("AOSS_HOST", "")
	user := envOr("AOSS_USERNAME", "")
	pass := envOr("AOSS_PASSWORD", "")
	for _, v := range []struct {
		name, val string
	}{
		{"AOSS_HOST", host},
		{"AOSS_USERNAME", user},
		{"AOSS_PASSWORD", pass},
	} {
		if v.val == "" {
			fatal(v.name+" must be set", fmt.Errorf("missing environment variable"))
		}
	}

	port := 22
	if p := envOr("AOSS_PORT", ""); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	transport := envOr("AOSS_TRANSPORT", "bin")
	knownHosts := envOr("AOSS_KNOWN_HOSTS", "")

	out := envOr("AOSS_OUT", "fixtures/aoss")
	if err := os.MkdirAll(filepath.Join(out, "shows"), 0o755); err != nil {
		fatal("creating output directory", err)
	}
	iface := envOr("AOSS_INTERFACE", "1")
	vlan := envOr("AOSS_VLAN", "1")

	var opts []scraplioptions.Option
	opts = append(opts,
		scraplioptions.WithPort(uint16(port)),
		scraplioptions.WithUsername(user),
		scraplioptions.WithPassword(pass),
		scraplioptions.WithOperationTimeout(120*time.Second),
		scraplioptions.WithTermWidth(200),
		scraplioptions.WithTermHeight(50),
		scraplioptions.WithSessionRecorderPath(filepath.Join(out, "raw_session.bin")),
		scraplioptions.WithLogger(traceLogger(out)),
		scraplioptions.WithLoggerLevel(logLevel()),
	)
	switch transport {
	case "ssh2":
		opts = append(opts, scraplioptions.WithTransportSSH2())
		if knownHosts == "" {
			knownHosts = filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
		}
		opts = append(opts, scraplioptions.WithSSH2KnownHostsPath(knownHosts))
	case "bin":
		opts = append(opts, scraplioptions.WithTransportBin())
	default:
		fatal("invalid AOSS_TRANSPORT "+transport, fmt.Errorf("expected bin or ssh2"))
	}
	// AOS-S prints a restricted-rights legend after the password is
	// accepted, ending in "Press any key to continue", and waits for a
	// keypress before showing the first prompt. The bin transport's
	// in-session auth loop never sends input unless it sees a username or
	// password pattern, so by default it deadlocks on the legend. Pointing
	// the username pattern at the legend text makes the loop send the
	// username (a keypress) at the right moment; see the README
	// troubleshooting section for the exact value to use.
	if up := envOr("AOSS_USERNAME_PATTERN", ""); up != "" {
		opts = append(opts, scraplioptions.WithUsernamePattern(up))
	}
	if pp := envOr("AOSS_PASSWORD_PATTERN", ""); pp != "" {
		opts = append(opts, scraplioptions.WithPasswordPattern(pp))
	}
	switch {
	case defFile != "":
		opts = append(opts, scraplioptions.WithDefinitionFileOrName(defFile))
	case defName != "":
		opts = append(opts, scraplioptions.WithDefinitionFileOrName(defName))
	default:
		opts = append(opts, scraplioptions.WithDefinitionContent(client.DefinitionPlatform, client.DefinitionContent()))
	}

	c, err := scraplicli.NewCli(host, opts...)
	if err != nil {
		fatal("building CLI client", err)
	}
	if _, err := c.Open(context.Background()); err != nil {
		dumpRawSessionTail(out)
		fmt.Fprintf(os.Stderr, "hint: the exact point of failure is the last bytes of %s\n", filepath.Join(out, "raw_session.bin"))
		fatal("opening SSH session to "+host, err)
	}
	defer func() { _, _ = c.Close(context.Background()) }()

	col := &collector{c: c, out: out, iface: iface, vlan: vlan}
	col.logf("=== collection started ===")
	col.logf("host=%s port=%d user=%s transport=%s", host, port, user, transport)
	if defFile != "" {
		col.logf("definition=file:%s", defFile)
	} else if defName != "" {
		col.logf("definition=name:%s", defName)
	} else {
		col.logf("definition=hp_aoss")
	}

	sections := []struct {
		name string
		fn   func()
	}{
		{"version", col.version},
		{"modes", col.modes},
		{"config_sessions", col.configSessions},
		{"error_strings", col.errorStrings},
		{"running_config", col.runningConfig},
		{"shows", col.shows},
		{"config_restored", col.restore},
	}
	var failed bool
	for _, s := range sections {
		col.logf("---- section: %s ----", s.name)
		if fn := recoverSection(s.fn, col); fn != nil {
			failed = true
		}
	}
	col.logf("=== collection finished ===")

	summary := "collector run summary\n" +
		"=====================\n"
	if failed {
		summary += "status: some sections FAILED, review session.log\n"
	} else {
		summary += "status: ok\n"
	}
	summary += "\nfiles captured:\n"
	for _, root := range []string{col.out, filepath.Join(col.out, "shows")} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				summary += "  " + filepath.Join(root, e.Name()) + "\n"
			}
		}
	}
	summary += "\nnote: prompts.log holds the raw prompt strings; session.log\n" +
		"holds the full transcript including mode transitions.\n"
	if err := os.WriteFile(filepath.Join(out, "summary.txt"), []byte(summary), 0o644); err != nil {
		fatal("writing summary", err)
	}
	if failed {
		fmt.Fprintf(os.Stderr, "warning: some sections failed; check %s/session.log\n", out)
	}
	fmt.Printf("fixtures written to %s\n", out)
}

// recoverSection runs fn and reports panics so one bad section never
// aborts the whole collection.
func recoverSection(fn func(), col *collector) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			col.logf("ERROR in section: %v", r)
		}
	}()
	fn()
	return
}

// send runs a single command, appends the transcript to the session log,
// and returns the output. It fails hard if the device reported a failure
// indicator, because continuing from an unknown state would corrupt the
// fixtures.
func (col *collector) send(input, mode string) (string, error) {
	if mode != "" {
		col.mode = mode
	}
	opts := []scraplicli.Option{}
	if mode != "" && mode != "privileged_exec" {
		opts = append(opts, scraplicli.WithRequestedMode(mode), scraplicli.WithStopOnIndicatedFailure())
	}
	res, err := col.c.SendInput(context.Background(), input, opts...)
	if err != nil {
		col.resync(input, err)
		return "", err
	}
	out := res.Result()
	if res.Failed() {
		return "", fmt.Errorf("device reported failure for %q: %s", input, strings.TrimSpace(out))
	}
	col.logf("%s%s\n%s\n", col.promptGuess(mode), input, out)
	return out, nil
}

// resync aborts a stuck pager (or any wedged state) after a failed
// operation. If the device is waiting at a "-- MORE --" prompt, the next
// command would be fed to the pager instead of the CLI and everything
// after that would be garbage; Ctrl-C is the documented pager quit key.
func (col *collector) resync(input string, err error) {
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "Timeout") {
		return
	}
	if werr := col.c.Write("\x03"); werr != nil {
		col.logf("[resync: ctrl-c write failed: %v]\n", werr)
		return
	}
	col.logf("[resync: operation timed out on %q, sent ctrl-c to abort possible pager]\n", input)
}

// sendSoft is like send but does not treat a device failure indicator as an
// error; it is used for the deliberate-error capture section.
func (col *collector) sendSoft(input, mode string) string {
	opts := []scraplicli.Option{}
	if mode != "" && mode != "privileged_exec" {
		opts = append(opts, scraplicli.WithRequestedMode(mode))
	}
	res, err := col.c.SendInput(context.Background(), input, opts...)
	if err != nil {
		col.logf("%s%s\n[collector error: %v]\n", col.promptGuess(mode), input, err)
		return ""
	}
	out := res.Result()
	failed := ""
	if res.Failed() {
		failed = " [failure-indicator-seen]"
	}
	col.logf("%s%s\n%s%s\n", col.promptGuess(mode), input, out, failed)
	return out
}

// promptGuess renders an approximate prompt for the transcript so it reads
// like a terminal session; the real prompt is recorded in prompts.log.
func (col *collector) promptGuess(mode string) string {
	if mode == "configuration" {
		return "#(config) "
	}
	return "> "
}

// enterMode transitions to a named mode using the definition's mode graph
// and records the raw prompt actually observed.
func (col *collector) enterMode(mode string) {
	if mode == col.mode {
		return
	}
	res, err := col.c.EnterMode(context.Background(), mode)
	if err != nil {
		col.logf("[collector error entering mode %s: %v]\n", mode, err)
		return
	}
	col.mode = mode
	col.logf("[entered mode %s]\n%s", mode, res.Result())
}

// logf appends to session.log immediately (unbuffered) so partial runs are
// still useful.
func (col *collector) logf(format string, args ...any) {
	data := fmt.Sprintf(format+"\n", args...)
	f, err := os.OpenFile(filepath.Join(col.out, "session.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprint(f, data)
}

// save persists the running config; the collector must never leave a switch
// with unsaved changes behind.
func (col *collector) save() error {
	_, err := col.send("write memory", "")
	return err
}

func (col *collector) version() {
	if err := col.writeFile("show_version.txt", "show version"); err != nil {
		col.logf("ERROR: %v", err)
	}
}

func (col *collector) writeFile(name, command string) error {
	out, err := col.send(command, "")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(col.out, name), []byte(out), 0o644); err != nil {
		return err
	}
	return nil
}

func (col *collector) modes() {
	prompts := map[string]string{}
	for _, mode := range []string{"privileged_exec", "configuration"} {
		if mode != col.mode {
			col.enterMode(mode)
		}
		if p, err := col.c.GetPrompt(context.Background()); err == nil {
			prompts[mode] = p.Result()
		}
	}
	if col.mode != "privileged_exec" {
		col.enterMode("privileged_exec")
	}

	var b strings.Builder
	for _, mode := range []string{"privileged_exec", "configuration"} {
		fmt.Fprintf(&b, "%s: %q\n", mode, prompts[mode])
	}
	if err := os.WriteFile(filepath.Join(col.out, "prompts.log"), []byte(b.String()), 0o644); err != nil {
		col.logf("ERROR writing prompts.log: %v", err)
	}
}

func (col *collector) configSessions() {
	// Nested mode entry and exit, to learn end/quit/exit semantics and
	// which prompt each sub-mode shows.
	col.enterMode("configuration")
	for _, input := range []string{
		"vlan " + col.vlan,
		"exit",
		"interface " + col.iface,
		"exit",
	} {
		if _, err := col.send(input, "configuration"); err != nil {
			col.logf("ERROR: %v", err)
		}
	}
	col.enterMode("privileged_exec")
}

func (col *collector) errorStrings() {
	var b strings.Builder
	write := func(label, input string) {
		b.WriteString("=== " + label + " ===\n")
		b.WriteString(col.sendSoft(input, ""))
		b.WriteString("\n")
	}
	// Privileged mode errors.
	write("unknown_command", "show badcommand123")
	write("incomplete_command", "show")
	write("bad_interface", "show interfaces bogus0/0")
	// Config mode errors.
	col.enterMode("configuration")
	cfgWrite := func(label, input string) {
		b.WriteString("=== " + label + " ===\n")
		b.WriteString(col.sendSoft(input, "configuration"))
		b.WriteString("\n")
	}
	cfgWrite("config_unknown_command", "badcommand123")
	cfgWrite("config_bad_parameter", "vlan "+col.vlan+" name "+strings.Repeat("x", 64))
	cfgWrite("config_unknown_interface", "interface bogus0/0")
	col.enterMode("privileged_exec")

	if err := os.WriteFile(filepath.Join(col.out, "errors.log"), []byte(b.String()), 0o644); err != nil {
		col.logf("ERROR writing errors.log: %v", err)
	}
}

func (col *collector) runningConfig() {
	if err := col.writeFile("running_config.cfg", "show running-config"); err != nil {
		col.logf("ERROR: %v", err)
	}
}

func (col *collector) shows() {
	commands := []struct {
		name, cmd string
	}{
		{"show_vlan", "show vlan"},
		{"show_vlan_detail", "show vlan " + col.vlan},
		{"show_interfaces", "show interfaces"},
		{"show_interface_detail", "show interface " + col.iface},
		{"show_ip_route", "show ip route"},
		{"show_arp", "show arp"},
		{"show_lldp_remote", "show lldp info remote-device"},
		{"show_lldp_local", "show lldp info local-device"},
		{"show_system", "show system"},
		{"show_time", "show time"},
		{"show_running_config_full", "show running-config"},
	}
	for _, c := range commands {
		out, err := col.send(c.cmd, "")
		if err != nil {
			col.logf("ERROR: %v", err)
			continue
		}
		if err := os.WriteFile(filepath.Join(col.out, "shows", c.name+".txt"), []byte(out), 0o644); err != nil {
			col.logf("ERROR: %v", err)
		}
	}
}

func (col *collector) restore() {
	// The config section only visited existing vlan/interface objects
	// without altering them; re-save so the running config is definitely
	// stored either way.
	if err := col.save(); err != nil {
		col.logf("ERROR saving config: %v", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// traceLogger appends scrapligo/libscrapli log lines (trace level by
// default) to <out>/scrapli.log. The libscrapli trace includes every
// auth-stage event (username prompt seen, password sent, prompt
// matched) which is the fastest way to tell where login is stalling.
func traceLogger(out string) func(scraplilogging.LogLevel, string) {
	return func(level scraplilogging.LogLevel, message string) {
		data := fmt.Sprintf("[%s] [%s] %s\n", time.Now().Format(time.RFC3339), level, message)
		f, err := os.OpenFile(filepath.Join(out, "scrapli.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprint(f, data)
	}
}

func logLevel() scraplilogging.LogLevel {
	switch strings.ToLower(envOr("AOSS_LOG_LEVEL", "trace")) {
	case "debug":
		return scraplilogging.Debug
	case "info":
		return scraplilogging.Info
	case "warn":
		return scraplilogging.Warn
	case "critical":
		return scraplilogging.Critical
	case "disabled":
		return scraplilogging.Disabled
	default:
		return scraplilogging.Trace
	}
}

// dumpRawSessionTail prints the last 2KB of the raw terminal stream so a
// failed login shows exactly what the switch sent before the collector
// gave up (e.g. whether the password prompt was ever printed, whether a
// "press any key" banner appeared).
func dumpRawSessionTail(out string) {
	path := filepath.Join(out, "raw_session.bin")
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}
	const tail = 2048
	start := len(data) - tail
	if start < 0 {
		start = 0
	}
	fmt.Fprintf(os.Stderr, "--- last %d bytes of raw terminal stream (%s) ---\n", len(data)-start, path)
	fmt.Fprint(os.Stderr, string(data[start:]))
	fmt.Fprintln(os.Stderr, "--- end of raw terminal stream ---")
}

func fatal(step string, err error) {
	fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", step, err)
	os.Exit(1)
}
