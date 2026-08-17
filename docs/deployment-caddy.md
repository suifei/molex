# Caddy deployment

Run both MoleX listeners on loopback and let Caddy own the only public TCP port:

```text
Internet :443 -> molex.example.com       -> relay data 127.0.0.1:8080
              -> admin.molex.example.com -> Web console 127.0.0.1:9090
```

The two hostnames share Caddy's `443/tcp` listener. The relay data plane and management plane remain separate on the loopback interface.

## Relay configuration

```json
{
  "mode": "relay",
  "token": "mx1_replace-with-a-long-random-token",
  "listen": "127.0.0.1:8080",
  "tunnel": {}
}
```

On the first run, you can omit `--password-file`: MoleX opens the browser setup screen and writes the password you create to the default private state directory. For unattended services, create a protected file containing a Web password of at least 12 characters, then start the console and relay together:

```bash
molex web \
  --config /var/lib/molex/molex.json \
  --password-file /etc/molex/web-password \
  --listen 127.0.0.1:9090 \
  --open-browser=false \
  --autostart
```

`--listen` controls the management listener. Interactive use prefers `127.0.0.1:9090` and advances to a free loopback port when it is occupied. Services and reverse proxies must explicitly pin the address as above; `--open-browser=false` prevents attempts to launch a browser on a headless host. The `listen` field in `molex.json` controls the relay data listener. MoleX rejects a non-loopback management address.

On Windows and macOS, launching `molex` without arguments uses the per-user `MoleX` configuration directory, starts the loopback Web console, and opens the default browser. The browser-based setup is the supported management experience; there is no desktop shell.

## Caddyfile

```caddyfile
molex.example.com {
    tls operator@example.com

    @molex_session {
        path /ws/session
        header Connection *Upgrade*
        header Upgrade websocket
    }

    handle @molex_session {
        reverse_proxy 127.0.0.1:8080
    }

    handle {
        respond "Hello, world." 200
    }

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options nosniff
        Referrer-Policy no-referrer
        -Server
    }
}

admin.molex.example.com {
    tls operator@example.com
    reverse_proxy 127.0.0.1:9090

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options nosniff
        Referrer-Policy no-referrer
        -Server
    }
}
```

Caddy handles WebSocket upgrades and forwards the original host and HTTPS scheme automatically. MoleX uses that information to mark the management session cookie as `Secure`. Do not force `Upgrade` or `Connection` upstream headers.

Caddy also sets the standard forwarded client address for the Relay upstream. The Relay Web console uses it for the connection table only when Caddy reaches MoleX from loopback; forwarded headers on direct non-loopback requests are ignored. The same table shows client-provided names, forwarding endpoints, platform, pairing, uptime, and encrypted traffic counters. No custom `X-Real-IP` directive is required.

Do not add wildcard CORS headers. MoleX clients are native WebSocket clients, and the management API intentionally accepts only same-origin browser requests.

The optional relay token is sent in the data-plane `Authorization` header and is visible to Caddy after TLS termination. It controls relay admission only and is not used for payload encryption. The Web management password is a separate credential.

## Relay keep-alive

Prefer systemd on Linux. The unit below keeps the Web console and data plane together. A data-plane-only unit lives at `deploy/molex-relay.service` (`Restart=always`). On hosts without systemd, run `deploy/molex-keepalive.sh` as a POSIX watchdog around `molex serve`.

## systemd unit

```ini
[Unit]
Description=MoleX relay and Web console
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=molex
Group=molex
UMask=0077
ExecStart=/usr/local/bin/molex web --config /var/lib/molex/molex.json --password-file /etc/molex/web-password --listen 127.0.0.1:9090 --open-browser=false --autostart
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/molex
ReadOnlyPaths=/etc/molex/web-password

[Install]
WantedBy=multi-user.target
```

The Web console saves configuration changes, so its state directory must be writable by the service account. Keep the login password outside that directory:

```bash
sudo install -d -m 0700 -o molex -g molex /var/lib/molex
sudo install -m 0600 -o molex -g molex relay.json /var/lib/molex/molex.json
sudo install -d -m 0750 -o root -g molex /etc/molex
sudo install -m 0640 -o root -g molex web-password /etc/molex/web-password
```

## Client nodes

Run the same `molex web` command with an edge or target configuration. For occasional remote administration, keep the console private and use an SSH tunnel:

```bash
ssh -N -L 9090:127.0.0.1:9090 user@molex-host
```

Then open `http://127.0.0.1:9090`. Use a trusted HTTPS reverse proxy if the console needs continuous browser access without an SSH tunnel.

## Firewall

- Allow inbound `443/tcp` to Caddy.
- Keep `8080/tcp` and `9090/tcp` bound to loopback and blocked externally.
- Do not expose the edge listener unless you intentionally bind it beyond loopback.

The Caddy-to-relay WebSocket is not TLS, but all MoleX data frames after peer setup contain end-to-end authenticated ciphertext. The Caddy-to-console hop is loopback-only; browser-to-Caddy management traffic remains HTTPS.

## Health checks

Both listeners expose a local health endpoint:

```bash
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:9090/healthz
```

The data-plane Caddy hostname does not publish `/healthz`. Restrict the management hostname at the network or reverse-proxy layer if its health endpoint should not be public.

## Guided troubleshooting

The Edge and Target consoles keep retrying and show a specific next action. The most common messages map to these checks:

| Console result | Check |
| --- | --- |
| HTTP `401` or `403` | Make `token` identical on Relay, Edge, and Target. |
| HTTP `404` | Use the exact `/ws/session` URL and confirm the Caddy matcher forwards it. |
| HTTP `502`, `503`, or `504` | Start MoleX Relay and verify Caddy's `reverse_proxy` upstream address. |
| DNS failure or connection refused | Check the hostname, DNS, listener port, service state, and firewall. |
| TLS verification failure | Check the certificate hostname and chain, plus the client machine's system time. |
| Pairing timeout | Start the other client and match channel, secret, token, and complementary roles. |
| Edge address in use | Stop the process occupying the address or select a free local listen address. |
| Target service unavailable | Start the private service or correct `tunnel.local`, then retry the Edge connection. |

During recovery, `Not listening` is intentional: Edge does not accept a local TCP connection until an authenticated route is ready.
