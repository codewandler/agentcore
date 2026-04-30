package phone

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/codewandler/agentsdk/tool"
)

// ── Configuration ─────────────────────────────────────────────────────────────

// Config configures the phone tool. SIPAddr is the default outbound SIP endpoint.
// It may be left empty when callers provide a sip_endpoint on dial operations.
type Config struct {
	// SIPAddr is the default outbound SIP endpoint in "host:port" form (e.g. "asterisk.dev.internal:5062").
	SIPAddr string

	// Transport is the default SIP protocol/transport ("tcp", "udp", "ws", or v4/v6 variants like "tcp4"). Default: "tcp".
	Transport string

	// Log is the logger for SIP operations. Default: slog.Default().
	Log *slog.Logger

	// AudioDevice connects established calls to local microphone/speaker IO when
	// dial.audio is "device". If nil, dial.audio="device" is rejected.
	AudioDevice AudioDevice
	// Dialer overrides the default SIP dialer (for testing).
	Dialer Dialer
}

func (c Config) transport() string {
	if c.Transport != "" {
		return c.Transport
	}
	return "tcp"
}

func (c Config) dialer() Dialer {
	if c.Dialer != nil {
		return c.Dialer
	}
	return newSIPDialer(c.Log)
}

// ── Parameter types (oneOf operations) ────────────────────────────────────────

// PhoneParams defines the parameters for the phone tool.
type PhoneParams struct {
	Operations []PhoneOperation `json:"operations" jsonschema:"description=Phone operations to perform.,required"`
}

// PhoneOperation is a discriminated union — exactly one field must be set.
type PhoneOperation struct {
	Dial   *DialOp   `json:"dial,omitempty" jsonschema:"description=Place an outbound SIP call."`
	Hangup *HangupOp `json:"hangup,omitempty" jsonschema:"description=Hang up an active call."`
	Status *StatusOp `json:"status,omitempty" jsonschema:"description=List active calls and their state."`
}

// DialOp places an outbound call.
type DialOp struct {
	Number      string            `json:"number" jsonschema:"description=Phone number or SIP address to dial.,required"`
	SIPEndpoint string            `json:"sip_endpoint,omitempty" jsonschema:"description=Outbound SIP endpoint in host:port form. Optional when the tool has a default endpoint configured."`
	CallerID    string            `json:"caller_id,omitempty" jsonschema:"description=Optional SIP caller ID / From user. Empty by default."`
	Protocol    string            `json:"protocol,omitempty" jsonschema:"description=SIP protocol/transport to use for this call: tcp, udp, ws, or v4/v6 variants like tcp4. Defaults to the tool transport, which defaults to tcp."`
	Headers     map[string]string `json:"headers,omitempty" jsonschema:"description=Additional SIP headers to include in the INVITE. Header names should be valid SIP header names; From is managed by caller_id."`
	Credentials string            `json:"credentials,omitempty" jsonschema:"description=Optional SIP digest credentials in username:password form. Prefer app/env configuration for secrets."`
	Audio       string            `json:"audio,omitempty" jsonschema:"description=Audio mode: none, echo, or device. Empty defaults to none. echo echoes RTP media; device connects microphone/speaker when supported."`
	Debug       bool              `json:"debug,omitempty" jsonschema:"description=Include SIP response diagnostics in the tool result."`
	Timeout     int               `json:"timeout,omitempty" jsonschema:"description=Dial timeout in seconds (default 30)."`
}

// HangupOp terminates an active call.
type HangupOp struct {
	CallID string `json:"call_id" jsonschema:"description=Call ID to hang up (from dial result).,required"`
}

// StatusOp lists active calls. No parameters.
type StatusOp struct{}

// ── Call registry ─────────────────────────────────────────────────────────────

type activeCall struct {
	ID        string
	Number    string
	StartedAt time.Time
	State     string // "ringing", "active", "ended"
	call      Call
}

type callRegistry struct {
	mu    sync.Mutex
	calls map[string]*activeCall
	seq   int
}

func newRegistry() *callRegistry {
	return &callRegistry{calls: make(map[string]*activeCall)}
}

func (r *callRegistry) add(number string, call Call) *activeCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	id := fmt.Sprintf("call-%d", r.seq)
	ac := &activeCall{
		ID:        id,
		Number:    number,
		StartedAt: time.Now(),
		State:     "active",
		call:      call,
	}
	r.calls[id] = ac

	// Watch for remote hangup.
	go func() {
		<-call.Done()
		r.mu.Lock()
		ac.State = "ended"
		r.mu.Unlock()
	}()

	return ac
}

func (r *callRegistry) get(id string) (*activeCall, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ac, ok := r.calls[id]
	return ac, ok
}

func (r *callRegistry) remove(id string) (*activeCall, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ac, ok := r.calls[id]
	if ok {
		delete(r.calls, id)
	}
	return ac, ok
}

func (r *callRegistry) active() []*activeCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*activeCall
	for _, ac := range r.calls {
		out = append(out, ac)
	}
	return out
}

// ── Tool construction ─────────────────────────────────────────────────────────

const defaultDialTimeout = 30

// Tools returns the phone tool configured with the given SIP settings.
// If cfg.SIPAddr is empty, dial operations must provide sip_endpoint.
func Tools(cfg Config) []tool.Tool {
	registry := newRegistry()
	dialer := cfg.dialer()
	sipAddr := cfg.SIPAddr
	transport := cfg.transport()

	return []tool.Tool{
		tool.New("phone",
			"Place and manage SIP phone calls. Supports dialing numbers, hanging up active calls, and listing call status.",
			func(ctx tool.Ctx, p PhoneParams) (tool.Result, error) {
				if len(p.Operations) == 0 {
					return nil, fmt.Errorf("at least one operation is required")
				}
				return executeOps(ctx, p.Operations, registry, dialer, sipAddr, transport, cfg.AudioDevice)
			},
			phoneIntent(sipAddr),
		),
	}
}

// ── Operation dispatch ────────────────────────────────────────────────────────

func executeOps(
	ctx tool.Ctx,
	ops []PhoneOperation,
	registry *callRegistry,
	dialer Dialer,
	sipAddr, transport string,
	audioDevice AudioDevice,
) (tool.Result, error) {
	var parts []string
	anyError := false

	for _, op := range ops {
		var text string
		var err error

		switch {
		case op.Dial != nil:
			text, err = executeDial(ctx, op.Dial, registry, dialer, sipAddr, transport, audioDevice)
		case op.Hangup != nil:
			text, err = executeHangup(op.Hangup, registry)
		case op.Status != nil:
			text, err = executeStatus(registry)
		default:
			err = fmt.Errorf("operation must have exactly one of: dial, hangup, status")
		}

		if err != nil {
			parts = append(parts, fmt.Sprintf("error: %v", err))
			anyError = true
		} else {
			parts = append(parts, text)
		}
	}

	b := tool.NewResult()
	if anyError {
		b.WithError()
	}
	b.Text(strings.Join(parts, "\n\n"))
	return b.Build(), nil
}

func executeDial(
	ctx tool.Ctx,
	op *DialOp,
	registry *callRegistry,
	dialer Dialer,
	sipAddr, transport string,
	audioDevice AudioDevice,
) (string, error) {
	if op.Number == "" {
		return "", fmt.Errorf("dial: number is required")
	}
	endpoint := strings.TrimSpace(sipAddr)
	if endpoint == "" {
		endpoint = strings.TrimSpace(op.SIPEndpoint)
	}
	if endpoint == "" {
		return "", fmt.Errorf("dial: sip_endpoint is required when the phone tool has no default SIP endpoint configured")
	}

	protocol := strings.TrimSpace(op.Protocol)
	if protocol == "" {
		protocol = transport
	}

	audioMode, err := parseAudioMode(op.Audio)
	if err != nil {
		return "", err
	}
	if audioMode == AudioModeDevice && audioDevice == nil {
		return "", fmt.Errorf("dial: audio mode %q requires a configured audio device", audioMode)
	}
	username, password, err := parseCredentials(op.Credentials)
	if err != nil {
		return "", err
	}
	headers, err := cleanSIPHeaders(op.Headers)
	if err != nil {
		return "", err
	}

	timeout := op.Timeout
	if timeout < 1 {
		timeout = defaultDialTimeout
	}

	dialCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	dialResult, err := dialer.Dial(dialCtx, DialRequest{
		Endpoint:    endpoint,
		Protocol:    protocol,
		Number:      op.Number,
		CallerID:    strings.TrimSpace(op.CallerID),
		Username:    username,
		Password:    password,
		Headers:     headers,
		Audio:       audioMode,
		AudioDevice: audioDevice,
		Debug:       op.Debug,
	})
	if err != nil {
		debug := DialDebugInfo{}
		var dialErr *DialError
		if errors.As(err, &dialErr) {
			debug = dialErr.Debug
		}
		msg := fmt.Sprintf("dial %s: %v", op.Number, err)
		if op.Debug {
			msg = appendDialDebug(msg, debug)
		}
		return "", errors.New(msg)
	}

	ac := registry.add(op.Number, dialResult.Call)
	text := fmt.Sprintf("Call started: %s\nNumber: %s\nState: %s", ac.ID, ac.Number, ac.State)
	if op.Debug {
		text = appendDialDebug(text, dialResult.Debug)
	}
	return text, nil
}

func parseAudioMode(raw string) (AudioMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(AudioModeNone):
		return AudioModeNone, nil
	case string(AudioModeEcho):
		return AudioModeEcho, nil
	case string(AudioModeDevice):
		return AudioModeDevice, nil
	default:
		return "", fmt.Errorf("dial: unsupported audio mode %q (supported: none, echo, device)", raw)
	}
}

func parseCredentials(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	username, password, ok := strings.Cut(raw, ":")
	username = strings.TrimSpace(username)
	if !ok || username == "" {
		return "", "", fmt.Errorf("dial: credentials must be in username:password form")
	}
	return username, password, nil
}

func cleanSIPHeaders(headers map[string]string) (map[string]string, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(headers))
	for name, value := range headers {
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}
		if forbiddenCustomSIPHeaders[strings.ToLower(name)] {
			return nil, fmt.Errorf("dial: custom SIP header %q is managed by the phone tool and cannot be overridden", name)
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

var forbiddenCustomSIPHeaders = map[string]bool{
	"from":           true,
	"to":             true,
	"via":            true,
	"contact":        true,
	"call-id":        true,
	"cseq":           true,
	"content-length": true,
	"content-type":   true,
}

func appendDialDebug(text string, debug DialDebugInfo) string {
	if len(debug.Responses) == 0 {
		return text + "\nSIP responses: none"
	}
	var sb strings.Builder
	sb.WriteString(text)
	sb.WriteString("\nSIP responses:")
	for _, res := range debug.Responses {
		fmt.Fprintf(&sb, "\n  %d %s", res.StatusCode, res.Reason)
	}
	return sb.String()
}

func executeHangup(op *HangupOp, registry *callRegistry) (string, error) {
	if op.CallID == "" {
		return "", fmt.Errorf("hangup: call_id is required")
	}

	ac, ok := registry.remove(op.CallID)
	if !ok {
		return "", fmt.Errorf("hangup: unknown call %q", op.CallID)
	}

	duration := time.Since(ac.StartedAt).Truncate(time.Second)
	ac.call.Hangup()
	return fmt.Sprintf("Call %s ended (duration: %s)", ac.ID, duration), nil
}

func executeStatus(registry *callRegistry) (string, error) {
	calls := registry.active()
	if len(calls) == 0 {
		return "No active calls.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Active calls: %d\n", len(calls))
	for _, ac := range calls {
		dur := time.Since(ac.StartedAt).Truncate(time.Second)
		fmt.Fprintf(&sb, "  %-10s %-20s %-8s %s\n", ac.ID, ac.Number, ac.State, dur)
	}
	return sb.String(), nil
}
