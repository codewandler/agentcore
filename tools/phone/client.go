// Package phone provides the phone tool for SIP call origination.
//
// Each dial operation creates an independent SIP user agent and transport
// (via diago/sipgo), so multiple concurrent calls are fully isolated.
package phone

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// Dialer abstracts SIP call origination so the tool can be tested without
// a live SIP stack.
type Dialer interface {
	// Dial places an outbound SIP call. The returned Call stays alive until
	// Hangup is called or the remote side terminates.
	Dial(ctx context.Context, req DialRequest) (DialResult, error)
}

// DialRequest describes one outbound SIP call attempt.
type DialRequest struct {
	Endpoint    string
	Protocol    string
	Number      string
	CallerID    string
	Username    string
	Password    string
	Headers     map[string]string
	Audio       AudioMode
	AudioDevice AudioDevice
	Debug       bool
}

// AudioMode controls media handling after a call is established.
type AudioMode string

const (
	AudioModeNone   AudioMode = "none"
	AudioModeEcho   AudioMode = "echo"
	AudioModeDevice AudioMode = "device"
)

// AudioDevice bridges an established SIP dialog to local audio devices.
type AudioDevice interface {
	Connect(ctx context.Context, dialog *diago.DialogClientSession) error
}

// DialResult contains the established call and optional debug data.
type DialResult struct {
	Call  Call
	Debug DialDebugInfo
}

// DialDebugInfo contains SIP diagnostics collected while dialing.
type DialDebugInfo struct {
	Responses []SIPResponseInfo
}

// SIPResponseInfo is a compact SIP response snapshot for tool output.
type SIPResponseInfo struct {
	StatusCode int
	Reason     string
}

// DialError carries debug information for failed dial attempts.
type DialError struct {
	Err   error
	Debug DialDebugInfo
}

func (e *DialError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *DialError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Call represents an active outbound SIP call.
type Call interface {
	// Hangup terminates the call and releases all resources (UA, transport).
	Hangup()
	// Done returns a channel that closes when the call ends (remote hangup
	// or local hangup).
	Done() <-chan struct{}
}

// ── Live SIP implementation ───────────────────────────────────────────────────

// sipDialer is the production Dialer backed by diago/sipgo.
type sipDialer struct {
	log *slog.Logger
}

func newSIPDialer(log *slog.Logger) *sipDialer {
	if log == nil {
		log = slog.Default()
	}
	return &sipDialer{log: log}
}

func (d *sipDialer) Dial(ctx context.Context, req DialRequest) (DialResult, error) {
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("agentsdk-phone"))
	if err != nil {
		return DialResult{}, fmt.Errorf("create SIP UA: %w", err)
	}

	// Resolve the local outbound IP towards the remote host so the
	// Contact header contains a routable address instead of an empty host.
	remoteHost, _ := parseHostPort(req.Endpoint)
	extHost := resolveOutboundIP(remoteHost)

	dg := diago.NewDiago(ua,
		diago.WithLogger(d.log),
		diago.WithTransport(diago.Transport{
			Transport:      req.Protocol,
			ExternalHost:   extHost,
			RewriteContact: true,
		}),
	)

	// Start the diago server so the transport is active.
	bgCtx, bgCancel := context.WithCancel(context.Background())
	if err := dg.ServeBackground(bgCtx, func(_ *diago.DialogServerSession) {
		d.log.Debug("phone: unexpected inbound SIP call, ignoring")
	}); err != nil {
		bgCancel()
		_ = ua.Close()
		return DialResult{}, fmt.Errorf("start SIP transport: %w", err)
	}

	host, port := parseHostPort(req.Endpoint)
	recipient := sip.Uri{
		User: req.Number,
		Host: host,
		Port: port,
	}

	d.log.Info("phone: SIP INVITE", "to", req.Number, "addr", req.Endpoint, "protocol", req.Protocol)

	debug := DialDebugInfo{}
	inviteOpts := diago.InviteOptions{
		Transport: req.Protocol,
		Username:  req.Username,
		Password:  req.Password,
	}
	if req.Debug {
		inviteOpts.OnResponse = func(res *sip.Response) error {
			if res != nil {
				debug.Responses = append(debug.Responses, SIPResponseInfo{StatusCode: res.StatusCode, Reason: res.Reason})
			}
			return nil
		}
	}
	for name, value := range req.Headers {
		inviteOpts.Headers = append(inviteOpts.Headers, sip.NewHeader(name, value))
	}
	if req.CallerID != "" {
		inviteOpts.Headers = append(inviteOpts.Headers, &sip.FromHeader{
			DisplayName: req.CallerID,
			Address:     sip.Uri{User: req.CallerID, Host: host},
			Params:      sip.NewParams(),
		})
	}

	dialog, err := dg.Invite(ctx, recipient, inviteOpts)
	if err != nil {
		bgCancel()
		_ = ua.Close()
		return DialResult{}, &DialError{Err: fmt.Errorf("SIP INVITE to %s: %w", req.Number, err), Debug: debug}
	}

	d.log.Info("phone: call established", "to", req.Number)

	switch req.Audio {
	case "", AudioModeNone:
		// no media handling
	case AudioModeEcho:
		go func() {
			if err := dialog.Media().Echo(); err != nil {
				d.log.Debug("phone: media echo stopped", "to", req.Number, "error", err)
			}
		}()
	case AudioModeDevice:
		if req.AudioDevice == nil {
			bgCancel()
			_ = ua.Close()
			return DialResult{}, &DialError{Err: fmt.Errorf("audio mode %q requires a configured audio device", req.Audio), Debug: debug}
		}
		go func() {
			if err := req.AudioDevice.Connect(dialog.Context(), dialog); err != nil {
				d.log.Debug("phone: audio device stopped", "to", req.Number, "error", err)
			}
		}()
	default:
		bgCancel()
		_ = ua.Close()
		return DialResult{}, &DialError{Err: fmt.Errorf("unsupported audio mode %q", req.Audio), Debug: debug}
	}

	// When the dialog ends, cancel the background context.
	done := make(chan struct{})
	go func() {
		<-dialog.Context().Done()
		bgCancel()
		close(done)
	}()

	return DialResult{Call: &sipCall{
		dialog:   dialog,
		ua:       ua,
		bgCancel: bgCancel,
		done:     done,
		log:      d.log,
	}, Debug: debug}, nil
}

// sipCall is a live SIP call owning its own UA and transport.
type sipCall struct {
	dialog   *diago.DialogClientSession
	ua       *sipgo.UserAgent
	bgCancel context.CancelFunc
	done     chan struct{}
	log      *slog.Logger

	once sync.Once
}

func (c *sipCall) Hangup() {
	c.once.Do(func() {
		if c.dialog != nil {
			_ = c.dialog.Hangup(context.Background())
		}
		if c.bgCancel != nil {
			c.bgCancel()
		}
		if c.ua != nil {
			_ = c.ua.Close()
		}
	})
}

func (c *sipCall) Done() <-chan struct{} {
	return c.done
}

// resolveOutboundIP determines the local IP address that would be used
// to reach the given remote host. It opens a throwaway UDP "connection"
// (no packets are sent) and reads the local address chosen by the OS.
// Returns the IP string, or empty string on failure.
func resolveOutboundIP(remoteHost string) string {
	conn, err := net.Dial("udp4", net.JoinHostPort(remoteHost, "1"))
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String()
}

// parseHostPort splits "host:port" into host and port number.
func parseHostPort(addr string) (string, int) {
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		p, _ := strconv.Atoi(port)
		return host, p
	}
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) == 2 {
		p, _ := strconv.Atoi(parts[1])
		return parts[0], p
	}
	return addr, 5060
}
