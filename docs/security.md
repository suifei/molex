# Security model

## Goals

MoleX v2 is designed to provide:

- TLS 1.3 protection from each client to Caddy;
- mutual peer authentication between Edge and Target from a high-entropy access token;
- forward secrecy through ephemeral X25519 keys;
- confidentiality and integrity of tunneled payloads against everyone who does not hold the group token;
- token-based relay admission with per-token grouping, one-Target enforcement, disable, and kick;
- unique AES-GCM nonces and separate directional keys;
- a Target-side allowlist: an Edge can only reach addresses the Target itself published;
- no literal product marker, token, or application payload in protocol data frames.

## The v2 trust model, stated plainly

In v2 the access token is the only credential an Edge or Target needs, and the Relay operator creates and stores those tokens. The end-to-end pre-shared secret is derived from the token with HKDF, therefore:

- **The Relay operator is trusted.** Whoever controls the Relay configuration holds every group's token and could, with a modified build, derive session keys and read tunneled payloads.
- **The shipped Relay never does this.** The implementation forwards opaque binary frames, never derives payload keys, and its traffic counters are based on ciphertext sizes only.
- **Everyone else stays outside.** Without the token, an observer between the clients and the Relay (or another tenant on the same Relay with a different token) cannot authenticate hellos, derive keys, or read payloads. Cross-token groups are cryptographically and administratively isolated.

If you require payload secrecy from the Relay operator themselves, v2's single-token model is not that tool; run your own relay, which is the intended deployment.

## Non-goals

MoleX does not provide:

- anonymity between clients and the public endpoint;
- resistance to timing, volume, or WebSocket fingerprint analysis;
- payload secrecy from a malicious relay *operator* (see the trust model above);
- protection after an edge or target host is compromised;
- certificate pinning or a private PKI;
- browser-origin access control for arbitrary web applications;
- UDP forwarding.

Standard WSS makes the external connection look like a WebSocket connection because that is what it is. MoleX does not claim that the connection is indistinguishable from unrelated web traffic.

## Credentials

### Web management password (Relay console only)

The Relay's `molex web` requires a password of at least 12 characters from `MOLEX_WEB_PASSWORD` or `--password-file`. This password protects management access to the Relay node; it is never sent to clients and is not used for tunnel encryption.

After login, the server issues an opaque, `HttpOnly`, `SameSite=Strict` session cookie and a separate CSRF token. State-changing API requests require both, and cross-origin requests are rejected. Login failures are rate-limited per source address. Sessions expire after 12 hours by default and are invalidated on logout.

Target and Edge consoles are login-free by design, but they only accept loopback TCP peers, local host names (anti DNS-rebinding), same-origin browser contexts, and a per-boot CSRF token for mutations. Remote access to any console requires an SSH tunnel or a trusted HTTPS reverse proxy.

### Access token

A token is one line of trust: it admits clients to the Relay, groups them (one Target plus any number of Edges), and seeds the end-to-end key schedule via HKDF with domain separation. Tokens are generated in the Relay console from 32 bytes of operating-system randomness with the `mx2_` prefix.

- The Relay stores token values in its configuration file and shows them, masked by default, to the authenticated operator so they can be distributed to Target and Edge machines.
- Caddy can read the bearer token in the upgrade request on the loopback hop; it runs on the same trusted host as the Relay.
- A token may carry an optional `expiresAt`. The console presets are 1 / 7 / 30 / 90 days, 1 year, or never (the default). Changing the lifetime on an existing token recomputes expiry from now; clearing it makes the token unlimited again.
- Disabling, deleting, or letting a token expire immediately disconnects the whole group and blocks reconnection with an actionable error.
- Rotating a token issues a new value and keeps the previous one valid for a configured grace window (1–30 days, 3 by default) so Edges and the Target can migrate without downtime. After expiry the old value is rejected and remaining legacy sessions are disconnected.
- Create, rotate, disable, enable, and delete actions are appended to a JSONL audit file beside the configuration. The log records the action and token id, never the token value.
- Token values must never appear in logs, telemetry events, or client-side status output.

Treat a token like an SSH private key for the services behind it: anyone holding it can map and reach every service the Target publishes to that group. A Target that joins several groups can restrict each service to a subset of those groups.

## Cryptographic construction

- PSK: `HKDF-SHA256(token, "molex/v2/e2e-psk")`, 32 bytes, never sent on the wire.
- Route: `HMAC-SHA256(PSK, "molex/rendezvous/v1" || "molex/v2")`; the Relay pre-computes the expected route per token and rejects mismatched hellos.
- ECDH: X25519 with a fresh ephemeral key on every WebSocket session.
- Peer proof: HMAC-SHA256 keyed by the PSK.
- KDF: HKDF-SHA256 over ECDH output, a PSK-keyed transcript salt, and a versioned context string.
- Records: AES-256-GCM with independent transmit and receive keys.
- Nonce: four-byte random session prefix plus an eight-byte monotonic counter.
- Confirmation: encrypted HMAC over the transcript and sender role.

All comparisons of authentication values use constant-time comparison. The relay hashes presented bearer tokens with SHA-256 before looking them up, so token length does not affect the comparison path.

## Service catalog and allowlist

The Target publishes its service catalog (ids, names, addresses) to each paired Edge over an end-to-end encrypted yamux control stream; the Relay never parses it. Every Edge data stream begins with a service-id preamble. The Target resolves the id against its **current** local service list and refuses anything else, so a malicious or stale Edge cannot use the Target as an arbitrary dialer. Refusals are reported on the Target console.

## Metadata

The relay sees a deterministic 32-byte route key per token and can tell when the same group reconnects, plus frame timing and sizes. Caddy sees source IP addresses and the HTTP upgrade request.

Clients also send Relay-visible operational metadata (node name, mapping/service counts, relay URL without query parameters, platform, and the Target's random instance id) in fixed 125-byte AES-GCM-protected WebSocket ping payloads before the hello and again when the catalog or mappings change. Each refresh uses a unique GCM nonce. The Relay decrypts these operator-facing fields by design; do not put secrets in node names. The instance id lets the Relay enforce the one-Target-per-token rule.

Traffic counters are based on encrypted WebSocket payload lengths and frame counts. They do not reveal application plaintext or individual yamux stream boundaries. The displayed route ID is a short SHA-256-derived label, not the route key itself.

MoleX does not add traffic padding, cover traffic, timing jitter, or multiplexing between unrelated tokens.

## Local exposure

Edge mapping listeners bind `127.0.0.1` by default. Marking a mapping "LAN visible" binds `0.0.0.0` and makes the tunneled service reachable from the edge host's network; protect it with the host firewall and application authentication.

The Web management listener is stricter: MoleX rejects non-loopback addresses even when explicitly configured. Reverse proxy it through HTTPS or forward it through SSH instead of binding it to a LAN or public interface.

The Target can dial any address in its own published service list. Treat write access to the Target's `molex.json` (or its loopback console) as equivalent to control of the Target connector.

MoleX requests restrictive POSIX permissions when it writes configuration files. Windows permissions follow the containing directory ACL, so store the file under a user-private directory.

## TLS

Remote clients reject plain `ws://` endpoints except loopback addresses. WSS uses TLS 1.3 or fails. Certificate validation uses the operating system trust store and Go's standard hostname checks.

Caddy-to-relay traffic uses a loopback WebSocket without TLS. The peer hello is opaque and tunneled records remain encrypted end to end, but the bearer token is plaintext on that loopback hop. Keep the relay bound to loopback and protect the host.

When Caddy proxies the Web console from a loopback connection, it forwards the original HTTPS scheme and MoleX marks the session cookie as `Secure`. Do not expose the management console through plain HTTP on a remote network and do not add wildcard CORS headers.

## Operational recommendations

1. Generate tokens only in the Relay console or `molex config init`; never invent memorable ones.
2. Use one token per trust group; do not share a token across unrelated Targets.
3. Bind Caddy publicly and the relay only to loopback; expose only `443/tcp`.
4. Run MoleX under an unprivileged service account and restrict configuration file access.
5. Keep Caddy, Go, browser, and frontend dependencies updated.
6. Publish only the services Edges actually need; remove catalog entries when they are retired.
7. Prefer loopback mappings; enable "LAN visible" per mapping only when other devices genuinely need it.
8. Monitor repeated unauthorized upgrades, duplicate-Target rejections, and login failures at the reverse proxy.
9. Disable or delete a token immediately if it reaches logs, chats, or support bundles, then issue a new one.
10. Rotate tokens when a member machine is decommissioned or compromised; use the console rotate action so the previous value remains valid through the grace window, then update every Target and Edge before it expires.

## Reporting vulnerabilities

Do not include live tokens, private hostnames, or packet captures containing sensitive metadata in a public issue. Provide a minimal reproduction with generated credentials.
