# Security model

## Goals

MoleX is designed to provide:

- TLS 1.3 protection from each client to Caddy;
- mutual peer authentication from a high-entropy pre-shared secret;
- forward secrecy through ephemeral X25519 keys;
- end-to-end confidentiality and integrity across Caddy and the relay;
- independent relay admission control through an optional bearer token;
- unique AES-GCM nonces and separate directional keys;
- no literal product marker, channel name, secret, or application payload in protocol data frames.

## Non-goals

MoleX does not provide:

- anonymity between clients and the public endpoint;
- resistance to timing, volume, or WebSocket fingerprint analysis;
- protection after an edge or target host is compromised;
- concealment of the target service from its local target process;
- certificate pinning or a private PKI;
- browser-origin access control for arbitrary web applications;
- UDP forwarding.

Standard WSS makes the external connection look like a WebSocket connection because that is what it is. MoleX does not claim that the connection is indistinguishable from unrelated web traffic.

## Credentials

MoleX deliberately separates data-plane credentials from the Web management credential.

### Web management password

`molex web` requires a password of at least 12 characters from `MOLEX_WEB_PASSWORD` or `--password-file`. This password protects management access to one node; it is not sent to the relay and is not used for tunnel encryption.

After login, the server issues an opaque, `HttpOnly`, `SameSite=Strict` session cookie and a separate CSRF token. State-changing API requests require both the session and CSRF token, and cross-origin requests are rejected. Login failures are rate-limited per source address. Sessions expire after 12 hours by default and are invalidated on logout.

The management server only accepts loopback listen addresses. For remote access, use an SSH tunnel or a trusted HTTPS reverse proxy. HTTPS is required outside the local host so credentials, session cookies, configuration, and runtime controls are not exposed in transit.

The authenticated Relay console displays operational connection metadata: normalized client IP, client-provided node name, Edge/Target role, forwarding and relay endpoints, platform, waiting/paired state, counterpart, a one-way pseudonymous route ID, connection duration, last traffic time, and encrypted byte/frame counters. It does not display route keys, literal channel names, payload secrets, tokens, cookies, or CSRF values. The Relay trusts `X-Forwarded-For` or `X-Real-IP` only when the direct upstream address is loopback, preventing a public client from spoofing the displayed IP.

### Payload secret

`secret` is shared only by the edge and target. It creates the opaque route key, authenticates peer hello messages, and contributes to the session key schedule. The relay configuration does not need it.

Generate it with `molex config init` or the key button in the Web console. The generator uses 32 bytes from the operating system cryptographic random source and URL-safe Base64 encoding.

Do not use a memorable password. A captured route key can be used to test guesses when likely channel names are known.

### Relay token

`token` is optional and shared with the relay. Clients send it as a bearer token in the WSS upgrade request. It prevents unauthenticated users from consuming pairing slots and connection resources.

Caddy terminates TLS and can read this token. The relay also reads it. It is not an end-to-end encryption key.

Use distinct values for `secret` and `token`.

## Cryptographic construction

- ECDH: X25519 with a fresh ephemeral key on every WebSocket session.
- Peer proof: HMAC-SHA256 keyed by the payload secret.
- KDF: HKDF-SHA256 over ECDH output, a PSK-keyed transcript salt, and a versioned context string.
- Records: AES-256-GCM with independent transmit and receive keys.
- Nonce: four-byte random session prefix plus an eight-byte monotonic counter.
- Confirmation: encrypted HMAC over the transcript and sender role.

All comparisons of authentication values and relay tokens use constant-time comparison. The relay hashes candidate and configured bearer tokens before comparing them so token length does not affect the comparison loop.

## Metadata

The relay sees a deterministic 32-byte route key for each secret/channel pair. It can tell when the same pair reconnects and can measure frame timing and size. Caddy sees source IP addresses and the HTTP upgrade request.

Clients also send Relay-visible operational metadata in fixed 125-byte AES-GCM-protected WebSocket ping payloads before the unchanged peer hello. The encryption prevents plaintext fingerprints and accidental logging but is not an end-to-end privacy boundary: the Relay is intentionally able to decrypt these fields, and Caddy can derive the metadata key because it sees the relay token on the loopback hop. Do not put credentials, sensitive query strings, or secrets in the node name or endpoint labels. MoleX strips URL query parameters before reporting the relay endpoint.

Traffic counters are based on encrypted WebSocket payload lengths and frame counts. They do not reveal application plaintext or individual yamux stream boundaries. The displayed route ID is a short SHA-256-derived label, not the route key itself.

MoleX does not add traffic padding, cover traffic, timing jitter, or multiplexing between unrelated route keys. Operators who require those properties need a different threat model and protocol.

## Local exposure

The default relay and edge listeners bind to `127.0.0.1`. Changing an edge listener to `0.0.0.0` makes the tunneled service reachable from the edge host's network. Protect that listener with the host firewall and application authentication.

The Web management listener is stricter: MoleX rejects non-loopback addresses even when explicitly configured. Reverse proxy it through HTTPS or forward it through SSH instead of binding it to a LAN or public interface.

The target can connect to any TCP address allowed by its configuration and operating system. Treat write access to `molex.json` as equivalent to control of the target connector.

MoleX requests restrictive POSIX permissions when it writes configuration files. Windows permissions follow the containing directory ACL, so store the file under a user-private directory.

## TLS

Remote clients reject plain `ws://` endpoints except loopback addresses. WSS uses TLS 1.3 or fails. Certificate validation uses the operating system trust store and Go's standard hostname checks.

Caddy-to-relay traffic uses a loopback WebSocket without TLS. The peer hello is opaque and tunneled records remain end-to-end encrypted, but HTTP headers such as the relay token are plaintext on that loopback hop. Keep the relay bound to loopback and protect the host.

When Caddy proxies the Web console from a loopback connection, it forwards the original HTTPS scheme and MoleX marks the session cookie as `Secure`. Do not expose the management console through plain HTTP on a remote network and do not add wildcard CORS headers.

## Operational recommendations

1. Generate independent 32-byte payload and relay secrets.
2. Bind Caddy publicly and the relay only to loopback.
3. Expose only `443/tcp` in the public firewall.
4. Run MoleX under an unprivileged service account.
5. Restrict configuration file access.
6. Keep Caddy, Go, browser, and frontend dependencies updated.
7. Use a distinct Web password on each node and store service passwords in protected files.
8. Monitor repeated unauthorized upgrades, login failures, and pairing timeouts at the reverse proxy.
9. Rotate the relay token if it reaches logs or support bundles.
10. Rotate the payload secret if either endpoint is compromised.

## Reporting vulnerabilities

Do not include live secrets, tokens, private hostnames, or packet captures containing sensitive metadata in a public issue. Provide a minimal reproduction with generated credentials.
