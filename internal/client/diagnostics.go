package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/suifei/molex/internal/relay"
)

var errSessionClosed = errors.New("encrypted session closed by relay or peer")

type relayHTTPError struct {
	statusCode int
	status     string
	err        error
}

func (e *relayHTTPError) Error() string {
	return fmt.Sprintf("relay returned HTTP %s: %v", e.status, e.err)
}

func (e *relayHTTPError) Unwrap() error {
	return e.err
}

type localListenError struct {
	address string
	err     error
}

func (e *localListenError) Error() string {
	return fmt.Sprintf("listen on %s: %v", e.address, e.err)
}

func (e *localListenError) Unwrap() error {
	return e.err
}

// permanentRejection highlights errors that keep failing until an operator
// acts, so the console shows one clear error besides the retry countdown.
func permanentRejection(err error) string {
	var httpErr *relayHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.statusCode {
		case 401:
			return "The relay rejected this token (HTTP 401). Copy a valid token from the relay console and paste the exact value here."
		case 403:
			return "The relay reports this token is disabled or expired (HTTP 403). Ask the relay administrator to enable it, extend its lifetime, or issue a new one."
		}
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		switch closeErr.Code {
		case relay.CloseDuplicateTarget:
			return "Another Target is already connected with this token. Each token accepts exactly one Target; stop the other Target or use a different token."
		case relay.CloseTokenDisabled:
			if strings.Contains(strings.ToLower(closeErr.Text), "expired") {
				return "This token has expired. Ask the relay administrator to extend its lifetime or issue a new token before reconnecting."
			}
			return "The relay administrator disabled this token. Ask for a new or re-enabled token before reconnecting."
		case relay.CloseKicked:
			return "The relay administrator disconnected this client. It reconnects automatically; contact the administrator if access stays blocked."
		}
	}
	return ""
}

func guidanceForClientError(err error) string {
	if err == nil {
		return "The encrypted route ended unexpectedly. Check the relay and peer; MoleX will keep retrying."
	}

	if rejection := permanentRejection(err); rejection != "" {
		return rejection
	}

	var listenErr *localListenError
	if errors.As(err, &listenErr) {
		return fmt.Sprintf("The local listener could not start on %s. Stop the process using that address or choose a free listen address; MoleX will keep retrying.", listenErr.address)
	}

	var httpErr *relayHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.statusCode {
		case 404:
			return "The relay WebSocket route was not found (HTTP 404). Check that the URL ends with /ws/session and Caddy forwards that path."
		case 429:
			return "The relay is limiting connection attempts (HTTP 429). Wait before retrying and check the relay logs."
		case 502, 503, 504:
			return fmt.Sprintf("The relay gateway is unavailable (HTTP %d). Start MoleX Relay and verify Caddy's upstream address.", httpErr.statusCode)
		default:
			return fmt.Sprintf("The relay returned HTTP %d instead of opening a WebSocket. Check the relay URL and Caddy routing.", httpErr.statusCode)
		}
	}

	detail := strings.ToLower(err.Error())
	if strings.Contains(detail, "pair timeout") {
		return "No Target answered before the pairing timeout. Start the Target for this token and check that both sides run MoleX v2 with the same token."
	}
	if strings.Contains(detail, "session unavailable") {
		return "The relay could not accept this session. Verify the relay route and token, then retry."
	}
	if strings.Contains(detail, "route does not match") {
		return "The relay rejected the session route for this token. Make sure Edge, Target, and Relay all run MoleX v2 and use the exact same token."
	}
	if strings.Contains(detail, "peer authentication failed") ||
		strings.Contains(detail, "session key confirmation failed") ||
		strings.Contains(detail, "peer joined a different channel") ||
		strings.Contains(detail, "peer role does not complement") {
		return "The encrypted handshake failed. Verify that Edge and Target use the exact same token and run compatible MoleX v2 versions."
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) || strings.Contains(detail, "no such host") {
		return "The relay hostname could not be resolved. Check the hostname and this machine's DNS settings."
	}
	if errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(detail, "connection refused") || strings.Contains(detail, "actively refused") {
		return "The relay connection was refused. Start Caddy or MoleX Relay, verify the configured port, and check the firewall."
	}
	var certificateErr *tls.CertificateVerificationError
	if errors.As(err, &certificateErr) || strings.Contains(detail, "x509:") || strings.Contains(detail, "tls:") || strings.Contains(detail, "certificate") {
		return "TLS verification failed. Check the certificate hostname and chain, and verify this machine's system time."
	}
	var networkErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkErr) && networkErr.Timeout()) || strings.Contains(detail, "i/o timeout") {
		return "The relay connection timed out. Check network reachability, Caddy, and firewall rules."
	}
	if errors.Is(err, errSessionClosed) {
		return "The relay or peer closed the encrypted route. Retry the local connection after the route is ready."
	}

	return "Check relay reachability and verify that Edge and Target use the same token. Details: " + compactError(err)
}

func targetServiceUnavailableMessage(name, address string, err error) string {
	return fmt.Sprintf("Service %q at %s is unavailable. Start the backend service or correct its address in the service list, then retry from the Edge. Details: %s", name, address, compactError(err))
}

func compactError(err error) string {
	if err == nil {
		return "unknown error"
	}
	detail := strings.Join(strings.Fields(err.Error()), " ")
	const maximumLength = 240
	if len(detail) > maximumLength {
		detail = detail[:maximumLength-3] + "..."
	}
	return detail
}
