import { useCallback, useEffect, useRef, useState } from "react";
import {
  ArrowClockwise,
	ArrowRight,
  Check,
  CircleNotch,
  CloudArrowUp,
  Desktop,
  Eye,
  EyeSlash,
  FloppyDisk,
  Key,
  LockKey,
  Moon,
  Play,
  PlugsConnected,
  ShieldCheck,
  SignOut,
  Stop,
  Sun,
  TerminalWindow,
  Translate,
  Warning,
} from "@phosphor-icons/react";
import { api } from "./api";
import {
  copy,
  initialLocale,
  LANGUAGE_STORAGE_KEY,
  localizeRuntimeMessage,
  localizeValidationError,
  secretActionLabel,
} from "./i18n";
import type { Locale } from "./i18n";
import type { Config, Mode, RelayPeer, Role, RuntimeEvent, RuntimeStatus } from "./types";

const emptyConfig: Config = {
  mode: "punch",
  role: "edge",
  secret: "",
  token: "",
  listen: "127.0.0.1:2222",
  remote: "wss://molex.example.com/ws/session",
  tunnel: { local: "127.0.0.1:22", remote: "home-ssh", name: "" },
};

type Theme = "dark" | "light";
export type ThemePreference = Theme | "system";

export const THEME_STORAGE_KEY = "molex:theme";

function initialThemePreference(): ThemePreference {
  const saved = localStorage.getItem(THEME_STORAGE_KEY);
  return saved === "light" || saved === "dark" || saved === "system" ? saved : "system";
}

function nextThemePreference(preference: ThemePreference): ThemePreference {
  if (preference === "system") return "light";
  return preference === "light" ? "dark" : "system";
}

function App() {
  const [config, setConfig] = useState<Config>(emptyConfig);
  const [status, setStatus] = useState<RuntimeStatus>({ state: "idle", message: "Ready" });
  const [events, setEvents] = useState<RuntimeEvent[]>([]);
  const [themePreference, setThemePreference] = useState<ThemePreference>(initialThemePreference);
  const [systemTheme, setSystemTheme] = useState<Theme>(() =>
    window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light",
  );
  const [locale, setLocale] = useState<Locale>(initialLocale);
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [showSecret, setShowSecret] = useState(false);
  const [showToken, setShowToken] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const [notice, setNotice] = useState(false);
	const [now, setNow] = useState(() => Date.now());
	const refreshGeneration = useRef(0);

  const text = copy[locale];
  const resolvedTheme = themePreference === "system" ? systemTheme : themePreference;

  const isRunning = ["starting", "connecting", "running", "stopping"].includes(status.state);
  const isRelay = config.mode === "relay";
  const relayPeers = status.peers ?? [];
  const hasLocalListener = isRelay || config.role === "edge";
  const runtimeListen = hasLocalListener
    ? status.state === "running" ? status.listen || config.listen : text.notListening
    : text.notExposed;

  const refreshRuntime = useCallback(async () => {
		const generation = ++refreshGeneration.current;
    const [nextStatus, nextEvents] = await Promise.all([api.getStatus(), api.getEvents()]);
		if (generation !== refreshGeneration.current) return;
    setStatus(nextStatus);
    setEvents(nextEvents.slice(-20));
  }, []);

  const loadConsole = useCallback(async () => {
    const [loadedConfig, loadedStatus, loadedEvents] = await Promise.all([api.getConfig(), api.getStatus(), api.getEvents()]);
    setConfig({ ...emptyConfig, ...loadedConfig, tunnel: { ...emptyConfig.tunnel, ...loadedConfig.tunnel } });
    setStatus(loadedStatus);
    setEvents(loadedEvents.slice(-20));
  }, []);

  useEffect(() => {
    let mounted = true;
    void api.getSession()
      .then(async (session) => {
        if (!mounted) return;
				if (session.authenticated) {
					await loadConsole();
					if (mounted) setAuthenticated(true);
				} else {
					setAuthenticated(false);
				}
      })
      .catch((error: unknown) => {
        if (!mounted) return;
        setAuthenticated(false);
        setErrors([messageFrom(error)]);
      })
      .finally(() => mounted && setLoading(false));
    return () => {
      mounted = false;
    };
  }, [loadConsole]);

  useEffect(() => {
    if (!authenticated) return;
    let mounted = true;
    const unsubscribe = api.subscribe((event) => {
      if (!mounted) return;
			refreshGeneration.current++;
			setStatus((current) => applyRuntimeEvent(current, event));
			if (!event.transient) {
				setEvents((current) => [...current.slice(-19), event]);
			}
    });
    const interval = window.setInterval(() => void refreshRuntime().catch(() => undefined), 2500);
    return () => {
      mounted = false;
      unsubscribe?.();
      window.clearInterval(interval);
    };
  }, [authenticated, refreshRuntime]);

	useEffect(() => {
		if (!authenticated || !isRelay || relayPeers.length === 0) return;
		setNow(Date.now());
		const interval = window.setInterval(() => setNow(Date.now()), 1000);
		return () => window.clearInterval(interval);
	}, [authenticated, isRelay, relayPeers.length]);

  useEffect(() => {
    const handleUnauthorized = () => {
      setAuthenticated(false);
      setLoading(false);
      setErrors([]);
      setNotice(false);
    };
    window.addEventListener(api.unauthorizedEvent, handleUnauthorized);
    return () => window.removeEventListener(api.unauthorizedEvent, handleUnauthorized);
  }, []);

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const updateSystemTheme = () => setSystemTheme(media.matches ? "dark" : "light");
    updateSystemTheme();
    if (themePreference !== "system") return;
    media.addEventListener("change", updateSystemTheme);
    return () => media.removeEventListener("change", updateSystemTheme);
  }, [themePreference]);

  useEffect(() => {
    document.documentElement.dataset.theme = resolvedTheme;
    document.documentElement.dataset.themePreference = themePreference;
    document.documentElement.style.colorScheme = resolvedTheme;
    document.querySelector('meta[name="theme-color"]')?.setAttribute(
      "content",
      resolvedTheme === "dark" ? "#111216" : "#e9edf3",
    );
    localStorage.setItem(THEME_STORAGE_KEY, themePreference);
  }, [resolvedTheme, themePreference]);

  useEffect(() => {
    document.documentElement.lang = locale;
    localStorage.setItem(LANGUAGE_STORAGE_KEY, locale);
  }, [locale]);

  const setField = <K extends keyof Config>(field: K, value: Config[K]) => {
    setConfig((current) => ({ ...current, [field]: value }));
    setErrors([]);
    setNotice(false);
  };

  const setTunnel = (field: keyof Config["tunnel"], value: string) => {
    setConfig((current) => ({ ...current, tunnel: { ...current.tunnel, [field]: value } }));
    setErrors([]);
    setNotice(false);
  };

  const validate = async () => {
    const result = await api.validateConfig(config);
    setErrors(result.errors);
    return result.valid;
  };

  const save = async () => {
    setBusy(true);
    setNotice(false);
    try {
      if (!(await validate())) return;
      await api.saveConfig(config);
      setNotice(true);
      window.setTimeout(() => setNotice(false), 1800);
    } catch (error) {
      setErrors([messageFrom(error)]);
    } finally {
      setBusy(false);
    }
  };

  const toggleRuntime = async () => {
    setBusy(true);
    setNotice(false);
    try {
      if (isRunning) {
        await api.stop();
      } else {
        if (!(await validate())) return;
        await api.start(config);
      }
      await refreshRuntime();
    } catch (error) {
      setErrors([messageFrom(error)]);
    } finally {
      setBusy(false);
    }
  };

  const generate = async (field: "secret" | "token") => {
    try {
      const value = await api.generateSecret();
      setField(field, value);
      if (field === "secret") setShowSecret(true);
      if (field === "token") setShowToken(true);
    } catch (error) {
      setErrors([messageFrom(error)]);
    }
  };

  const login = async (password: string) => {
    setBusy(true);
    setErrors([]);
    try {
      await api.login(password);
      await loadConsole();
      setAuthenticated(true);
    } catch (error) {
      setErrors([messageFrom(error)]);
    } finally {
      setBusy(false);
    }
  };

  const logout = async () => {
    setBusy(true);
    try {
      await api.logout();
			refreshGeneration.current++;
      setAuthenticated(false);
      setConfig(emptyConfig);
      setStatus({ state: "idle", message: "Ready" });
      setEvents([]);
      setErrors([]);
      setNotice(false);
    } catch (error) {
      setErrors([messageFrom(error)]);
    } finally {
      setBusy(false);
    }
  };

  const routeLabels = isRelay
    ? [text.wssEntry, text.relayHub, text.pairedPeers]
    : config.role === "edge"
      ? [config.listen || text.localPort, text.encryptedHub, config.tunnel.remote || text.channel]
      : [config.tunnel.remote || text.channel, text.encryptedHub, config.tunnel.local || text.targetService];

  if (loading) {
    return (
      <main className="boot-state" aria-live="polite">
        <div className="boot-mark" />
        <div className="boot-line" />
        <span>{text.loading}</span>
      </main>
    );
  }

  if (!authenticated) {
    return (
      <LoginScreen
        locale={locale}
        themePreference={themePreference}
        busy={busy}
        errors={errors}
        onLocaleChange={() => setLocale(locale === "en" ? "zh-CN" : "en")}
        onThemeChange={() => setThemePreference(nextThemePreference(themePreference))}
        onLogin={login}
      />
    );
  }

  return (
    <main className="app-shell">
      <header className="topbar">
        <div className="brand-lockup">
          <img src="./molex-mark.svg" alt="" className="brand-mark" />
          <div>
            <div className="brand-name">MoleX</div>
            <div className="brand-subtitle">{text.brandSubtitle}</div>
          </div>
        </div>
        <div className="topbar-actions">
          <span className="console-label">{text.webConsole}{api.isPreview() ? ` · ${text.previewData}` : ""}</span>
          <StatusTag status={status} label={text.states[status.state]} />
          <LanguageButton locale={locale} onClick={() => setLocale(locale === "en" ? "zh-CN" : "en")} />
          <ThemeButton
            locale={locale}
            preference={themePreference}
            onClick={() => setThemePreference(nextThemePreference(themePreference))}
          />
          <button type="button" className="icon-button" onClick={() => void logout()} disabled={busy} aria-label={text.logout} title={text.logout}>
            <SignOut size={18} />
          </button>
        </div>
      </header>

      <section className={`route-map state-${status.state}`} aria-label={text.currentRouteStatus}>
        <RouteNode label={routeLabels[0]} icon={<TerminalWindow size={19} />} />
        <div className="route-segment" aria-hidden="true"><span /></div>
        <div className="route-core">
          <img src="./molex-mark.svg" alt="" />
          <span>{status.state === "running" ? text.secured : status.state === "connecting" ? text.pairing : text.standby}</span>
        </div>
        <div className="route-segment" aria-hidden="true"><span /></div>
        <RouteNode label={routeLabels[2]} icon={isRelay ? <PlugsConnected size={19} /> : <CloudArrowUp size={19} />} />
      </section>

      <section className={`workspace ${isRelay ? "relay-workspace" : ""}`}>
        <div className="config-panel">
          <div className="panel-heading">
            <div>
              <h1>{text.routeConfiguration}</h1>
              <p>{isRelay ? text.publicRendezvousEndpoint : config.role === "edge" ? text.localAccessEndpoint : text.privateServiceEndpoint}</p>
            </div>
            <div className="segmented-control" aria-label={text.mode}>
              <SegmentButton active={config.mode === "relay"} onClick={() => setField("mode", "relay" as Mode)} disabled={isRunning}>{text.relay}</SegmentButton>
              <SegmentButton active={config.mode === "punch"} onClick={() => setField("mode", "punch" as Mode)} disabled={isRunning}>{text.client}</SegmentButton>
            </div>
          </div>

          {isRelay ? (
            <div className="form-grid relay-fields">
              <Field label={text.listenAddress} htmlFor="listen">
                <input id="listen" value={config.listen} onChange={(event) => setField("listen", event.target.value)} disabled={isRunning} spellCheck={false} />
              </Field>
              <SecretField
                id="token"
                label={text.admissionToken}
                locale={locale}
                value={config.token}
                visible={showToken}
                disabled={isRunning}
                onChange={(value) => setField("token", value)}
                onToggle={() => setShowToken((value) => !value)}
                onGenerate={() => void generate("token")}
              />
            </div>
          ) : (
            <div className="form-grid">
              <div className="field full-width">
                <span className="field-label">{text.clientRole}</span>
                <div className="segmented-control role-control" aria-label={text.clientRole}>
                  <SegmentButton active={config.role === "edge"} onClick={() => setField("role", "edge" as Role)} disabled={isRunning}>{text.edgeListener}</SegmentButton>
                  <SegmentButton active={config.role === "target"} onClick={() => setField("role", "target" as Role)} disabled={isRunning}>{text.targetService}</SegmentButton>
                </div>
              </div>
              <Field label={text.relayEndpoint} htmlFor="remote" wide>
                <input id="remote" value={config.remote} onChange={(event) => setField("remote", event.target.value)} disabled={isRunning} spellCheck={false} />
              </Field>
							<Field label={text.nodeName} htmlFor="node-name">
								<input id="node-name" value={config.tunnel.name} onChange={(event) => setTunnel("name", event.target.value)} disabled={isRunning} spellCheck={false} />
							</Field>
              <Field label={text.channel} htmlFor="channel">
                <input id="channel" value={config.tunnel.remote} onChange={(event) => setTunnel("remote", event.target.value)} disabled={isRunning} spellCheck={false} />
              </Field>
              {config.role === "edge" ? (
                <Field label={text.localListen} htmlFor="listen">
                  <input id="listen" value={config.listen} onChange={(event) => setField("listen", event.target.value)} disabled={isRunning} spellCheck={false} />
                </Field>
              ) : (
                <Field label={text.targetService} htmlFor="local">
                  <input id="local" value={config.tunnel.local} onChange={(event) => setTunnel("local", event.target.value)} disabled={isRunning} spellCheck={false} />
                </Field>
              )}
							<SecretField
								id="token"
								label={text.relayToken}
								locale={locale}
								value={config.token}
								visible={showToken}
								disabled={isRunning}
								onChange={(value) => setField("token", value)}
								onToggle={() => setShowToken((value) => !value)}
								onGenerate={() => void generate("token")}
							/>
              <SecretField
                id="secret"
                label={text.endToEndSecret}
                locale={locale}
                value={config.secret}
                visible={showSecret}
                disabled={isRunning}
                onChange={(value) => setField("secret", value)}
                onToggle={() => setShowSecret((value) => !value)}
                onGenerate={() => void generate("secret")}
                wide
              />
            </div>
          )}

          <div className="form-actions">
            <div className="action-feedback" aria-live="polite">
              {errors.length > 0 && (
                <div className="message-strip error-strip" role="alert">
                  <Warning size={18} weight="fill" />
                  <div>{errors.map((error) => <div key={error}>{localizeValidationError(error, locale)}</div>)}</div>
                </div>
              )}
              {notice && (
                <div className="message-strip success-strip" role="status">
                  <Check size={18} weight="bold" />
                  <span>{text.configurationSaved}</span>
                </div>
              )}
            </div>
            <div className="action-buttons">
              <button type="button" className="button secondary-button" onClick={() => void save()} disabled={busy || isRunning}>
                <FloppyDisk size={18} />
                {text.save}
              </button>
              <button
                type="button"
                className={`button ${isRunning ? "stop-button" : "primary-button"}`}
                onClick={() => void toggleRuntime()}
                disabled={busy || status.state === "stopping"}
              >
                {busy ? <CircleNotch size={18} className="spin" /> : isRunning ? <Stop size={18} weight="fill" /> : <Play size={18} weight="fill" />}
                {isRunning ? text.stop : text.start}
              </button>
            </div>
          </div>
        </div>

        <aside className="runtime-panel">
          <div className="panel-heading runtime-heading">
            <div>
              <h2>{text.runtime}</h2>
              <p className="runtime-message">{status.message ? localizeRuntimeMessage(status.message, locale) : text.ready}</p>
            </div>
            <button type="button" className="icon-button" onClick={() => void refreshRuntime().catch((error) => setErrors([messageFrom(error)]))} aria-label={text.refreshRuntime} title={text.refreshRuntime}>
              <ArrowClockwise size={18} />
            </button>
          </div>

          <dl className="runtime-facts">
            <RuntimeFact label={text.state} value={text.states[status.state]} />
            <RuntimeFact label={text.mode} value={text.modes[status.mode || config.mode]} />
            <RuntimeFact label={text.role} value={text.roles[isRelay ? "hub" : status.role || config.role]} />
            <RuntimeFact
              label={text.listen}
              value={runtimeListen}
              mono={runtimeListen !== text.notListening && runtimeListen !== text.notExposed}
            />
          </dl>

          <div className="security-line">
            <ShieldCheck size={20} />
            <span>{isRelay ? text.ciphertextRelay : text.aesGcmSession}</span>
          </div>

          {isRelay && (
            <section className="peer-section" aria-labelledby="connected-clients-heading">
              <div className="peer-heading">
                <h3 id="connected-clients-heading">{text.connectedClients}</h3>
                <span>{relayPeers.length}</span>
              </div>
              <div className="peer-list" aria-live="polite">
                {relayPeers.length === 0 ? (
                  <div className="empty-peers">
                    <PlugsConnected size={22} />
                    <span>{text.noConnectedClients}</span>
                  </div>
                ) : (
					relayPeers.map((peer) => <RelayPeerRow key={peer.id} peer={peer} locale={locale} now={now} />)
                )}
              </div>
            </section>
          )}

          <div className="activity-heading">
            <h3>{text.activity}</h3>
            <span>{events.length}</span>
          </div>
          <div className="activity-log" aria-live="polite">
            {events.length === 0 ? (
              <div className="empty-activity">
                <TerminalWindow size={24} />
                <span>{text.noActivity}</span>
              </div>
            ) : (
              [...events].reverse().map((event, index) => (
                <div className={`activity-row level-${event.level}`} key={`${event.time}-${event.type}-${index}`}>
                  <time>{formatTime(event.time, locale)}</time>
                  <span>{localizeRuntimeMessage(event.message, locale)}</span>
                </div>
              ))
            )}
          </div>
        </aside>
      </section>
    </main>
  );
}

function RelayPeerRow({ peer, locale, now }: { peer: RelayPeer; locale: Locale; now: number }) {
	const text = copy[locale];
	const clientName = peer.name || text.unnamedClient;
	const counterpart = peer.peerName || (peer.peerId ? `#${peer.peerId}` : text.waitingForPeer);
	const connectedAt = timestampMillis(peer.connectedAt);
	const lastActivityAt = timestampMillis(peer.lastActivityAt);
	const received = peer.bytesReceived ?? 0;
	const sent = peer.bytesSent ?? 0;

	return (
		<article className="peer-row">
			<header className="peer-summary">
				<span className={`peer-indicator state-${peer.status}`} aria-hidden="true" />
				<div className="peer-identity">
					<div className="peer-name-line">
						<strong title={clientName}>{clientName}</strong>
						<span>#{peer.id}</span>
					</div>
					<span className="peer-ip mono" title={peer.ip || text.unknownAddress}>{peer.ip || text.unknownAddress}</span>
				</div>
				<div className="peer-meta">
					<span className="peer-role">{text.roles[peer.role]}</span>
					<span className={`peer-state state-${peer.status}`}>{text.peerStates[peer.status]}</span>
				</div>
			</header>

			<div className="peer-forwarding">
				<div className="peer-forward-node">
					<span>{text.forwardEndpoint}</span>
					<code title={peer.endpoint || text.unknownAddress}>{peer.endpoint || text.unknownAddress}</code>
				</div>
				<div className="peer-forward-link" aria-label={`${text.routeId} ${peer.routeId || "-"}`}>
					<span />
					<ArrowRight size={14} aria-hidden="true" />
					<small className="mono">{peer.routeId || "------------"}</small>
				</div>
				<div className="peer-forward-node peer-forward-peer">
					<span>{text.pairedWith}</span>
					<strong title={counterpart}>{counterpart}</strong>
				</div>
			</div>

			<dl className="peer-details">
				<div className="peer-detail-wide">
					<dt>{text.relayEndpoint}</dt>
					<dd className="mono" title={peer.relayEndpoint || text.unknownAddress}>{peer.relayEndpoint || text.unknownAddress}</dd>
				</div>
				<div>
					<dt>{text.platform}</dt>
					<dd className="mono">{peer.platform || text.unknownAddress}</dd>
				</div>
				<div>
					<dt>{text.connectionSource}</dt>
					<dd>{peer.proxied ? text.trustedProxy : text.directSocket}</dd>
				</div>
				<div>
					<dt>{text.connectedFor}</dt>
					<dd title={peer.connectedAt}>
						{Number.isFinite(connectedAt) ? formatDuration(now - connectedAt, locale) : "--"}
						<span className="peer-detail-time">{formatTime(peer.connectedAt, locale)}</span>
					</dd>
				</div>
				<div>
					<dt>{text.lastActivity}</dt>
					<dd>
						{Number.isFinite(lastActivityAt)
							? `${formatDuration(now - lastActivityAt, locale)}${locale === "zh-CN" ? "" : " "}${text.ago}`
							: text.noTrafficYet}
					</dd>
				</div>
				<div className="peer-detail-wide peer-traffic">
					<dt>{text.traffic}</dt>
					<dd>
						<span><b>{text.received}</b> {formatBytes(received)} · {peer.framesReceived ?? 0} {text.frames}</span>
						<span><b>{text.sent}</b> {formatBytes(sent)} · {peer.framesSent ?? 0} {text.frames}</span>
					</dd>
				</div>
			</dl>
		</article>
	);
}

function LoginScreen({ locale, themePreference, busy, errors, onLocaleChange, onThemeChange, onLogin }: {
  locale: Locale;
  themePreference: ThemePreference;
  busy: boolean;
  errors: string[];
  onLocaleChange: () => void;
  onThemeChange: () => void;
  onLogin: (password: string) => Promise<void>;
}) {
  const [password, setPassword] = useState("");
  const [visible, setVisible] = useState(false);
  const text = copy[locale];

  return (
    <main className="auth-shell">
      <header className="auth-topbar">
        <div className="brand-lockup">
          <img src="./molex-mark.svg" alt="" className="brand-mark" />
          <div>
            <div className="brand-name">MoleX</div>
            <div className="brand-subtitle">{text.brandSubtitle}</div>
          </div>
        </div>
        <div className="topbar-actions">
          <LanguageButton locale={locale} onClick={onLocaleChange} />
          <ThemeButton locale={locale} preference={themePreference} onClick={onThemeChange} />
        </div>
      </header>

      <section className="auth-stage">
        <div className="auth-route" aria-hidden="true">
          <span className="auth-route-node" />
          <span className="auth-route-line" />
          <span className="auth-route-core"><img src="./molex-mark.svg" alt="" /></span>
          <span className="auth-route-line" />
          <span className="auth-route-node" />
        </div>

        <form
          className="login-panel"
          onSubmit={(event) => {
            event.preventDefault();
            void onLogin(password);
          }}
        >
          <div className="login-kicker"><LockKey size={16} />{text.webConsole}</div>
          <div className="login-heading">
            <h1>{text.signInTitle}</h1>
            <p>{text.signInDescription}</p>
          </div>
          <label className="login-field" htmlFor="management-password">
            <span>{text.password}</span>
            <span className="login-password">
              <input
                id="management-password"
                type={visible ? "text" : "password"}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="current-password"
                autoFocus
                disabled={busy}
                required
              />
              <button
                type="button"
                onClick={() => setVisible((current) => !current)}
                disabled={busy}
                aria-label={secretActionLabel(locale, visible ? "hide" : "show", text.password)}
                title={secretActionLabel(locale, visible ? "hide" : "show", text.password)}
              >
                {visible ? <EyeSlash size={18} /> : <Eye size={18} />}
              </button>
            </span>
          </label>
          {errors.length > 0 && (
            <div className="message-strip error-strip login-error" role="alert">
              <Warning size={18} weight="fill" />
              <div>{errors.map((error) => <div key={error}>{localizeValidationError(error, locale)}</div>)}</div>
            </div>
          )}
          <button type="submit" className="button primary-button login-submit" disabled={busy || password.length === 0}>
            {busy ? <CircleNotch size={18} className="spin" /> : <LockKey size={18} />}
            {text.signIn}
          </button>
        </form>
      </section>
    </main>
  );
}

function LanguageButton({ locale, onClick }: { locale: Locale; onClick: () => void }) {
  const text = copy[locale];
  return (
    <button type="button" className="icon-button language-button" onClick={onClick} aria-label={text.switchLanguage} title={text.switchLanguage}>
      <Translate size={17} />
      <span aria-hidden="true">{text.languageShort}</span>
    </button>
  );
}

function ThemeButton({ locale, preference, onClick }: { locale: Locale; preference: ThemePreference; onClick: () => void }) {
  const text = copy[locale];
  const nextPreference = nextThemePreference(preference);
  const nextLabel = nextPreference === "system"
    ? text.useSystemTheme
    : nextPreference === "light"
      ? text.useLightTheme
      : text.useDarkTheme;
  const label = locale === "zh-CN"
    ? `${text.themePreferences[preference]}，${nextLabel}`
    : `${text.themePreferences[preference]}. ${nextLabel}`;
  return (
    <button
      type="button"
      className="icon-button theme-button"
      data-theme-preference={preference}
      onClick={onClick}
      aria-label={label}
      title={label}
    >
      {preference === "system" ? <Desktop size={18} /> : preference === "light" ? <Sun size={18} /> : <Moon size={18} />}
    </button>
  );
}

function SegmentButton({ active, disabled, onClick, children }: { active: boolean; disabled: boolean; onClick: () => void; children: string }) {
  return <button type="button" className={active ? "active" : ""} aria-pressed={active} disabled={disabled} onClick={onClick}>{children}</button>;
}

function Field({ label, htmlFor, wide, children }: { label: string; htmlFor: string; wide?: boolean; children: React.ReactNode }) {
  return (
    <label className={`field ${wide ? "full-width" : ""}`} htmlFor={htmlFor}>
      <span className="field-label">{label}</span>
      {children}
    </label>
  );
}

function SecretField({ id, label, locale, value, visible, disabled, onChange, onToggle, onGenerate, wide }: {
  id: string;
  label: string;
  locale: Locale;
  value: string;
  visible: boolean;
  disabled: boolean;
  onChange: (value: string) => void;
  onToggle: () => void;
  onGenerate: () => void;
  wide?: boolean;
}) {
  return (
    <div className={`field ${wide ? "full-width" : ""}`}>
      <label className="field-label" htmlFor={id}>{label}</label>
      <span className="input-with-actions">
        <input id={id} type={visible ? "text" : "password"} value={value} onChange={(event) => onChange(event.target.value)} disabled={disabled} autoComplete="off" spellCheck={false} />
        <button
          type="button"
          onClick={onToggle}
          disabled={disabled}
          aria-label={secretActionLabel(locale, visible ? "hide" : "show", label)}
          title={secretActionLabel(locale, visible ? "hide" : "show", label)}
        >
          {visible ? <EyeSlash size={17} /> : <Eye size={17} />}
        </button>
        <button type="button" onClick={onGenerate} disabled={disabled} aria-label={secretActionLabel(locale, "generate", label)} title={secretActionLabel(locale, "generate", label)}>
          <Key size={17} />
        </button>
      </span>
    </div>
  );
}

function StatusTag({ status, label }: { status: RuntimeStatus; label: string }) {
  return <span className={`status-tag status-${status.state}`}>{label}</span>;
}

function RouteNode({ label, icon }: { label: string; icon: React.ReactNode }) {
  return (
    <div className="route-node">
      <span className="route-node-icon">{icon}</span>
      <span title={label}>{label}</span>
    </div>
  );
}

function RuntimeFact({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={mono ? "mono" : ""} title={value}>{value}</dd>
    </div>
  );
}

export function applyRuntimeEvent(current: RuntimeStatus, event: RuntimeEvent): RuntimeStatus {
	const next: RuntimeStatus = { ...current };
	if (event.state) {
		next.state = event.state;
		if (["idle", "connecting", "stopping", "error"].includes(event.state)) {
			next.listen = undefined;
		}
	}
	if (event.listen) next.listen = event.listen;
	if (event.message) next.message = event.message;
	if (!event.peerChange) return next;

	const peers = new Map((current.peers ?? []).map((peer) => [peer.id, peer]));
	for (const peer of event.peerChange.peers) {
		switch (event.peerChange.action) {
			case "remove":
				peers.delete(peer.id);
				break;
			case "update":
				if (peers.has(peer.id)) peers.set(peer.id, peer);
				break;
			case "upsert":
				peers.set(peer.id, peer);
				break;
		}
	}
	next.peers = [...peers.values()].sort((left, right) => {
		const timeOrder = new Date(left.connectedAt).getTime() - new Date(right.connectedAt).getTime();
		if (timeOrder !== 0) return timeOrder;
		return left.id.localeCompare(right.id);
	});
	return next;
}

function messageFrom(error: unknown): string {
  if (error instanceof Error) return error.message;
  return String(error);
}

function formatTime(value: string, locale: Locale): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "--:--:--";
  return new Intl.DateTimeFormat(locale === "zh-CN" ? "zh-CN" : "en-US", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

function timestampMillis(value?: string): number {
	const timestamp = value ? new Date(value).getTime() : Number.NaN;
	return Number.isFinite(timestamp) && timestamp > 0 ? timestamp : Number.NaN;
}

function formatDuration(milliseconds: number, locale: Locale): string {
	let seconds = Math.max(0, Math.floor(milliseconds / 1000));
	const days = Math.floor(seconds / 86400);
	seconds %= 86400;
	const hours = Math.floor(seconds / 3600);
	seconds %= 3600;
	const minutes = Math.floor(seconds / 60);
	seconds %= 60;

	if (locale === "zh-CN") {
		if (days > 0) return `${days}天 ${hours}时`;
		if (hours > 0) return `${hours}时 ${minutes}分`;
		if (minutes > 0) return `${minutes}分 ${seconds}秒`;
		return `${seconds}秒`;
	}
	if (days > 0) return `${days}d ${hours}h`;
	if (hours > 0) return `${hours}h ${minutes}m`;
	if (minutes > 0) return `${minutes}m ${seconds}s`;
	return `${seconds}s`;
}

function formatBytes(value: number): string {
	if (!Number.isFinite(value) || value <= 0) return "0 B";
	const units = ["B", "KB", "MB", "GB", "TB"];
	let amount = value;
	let unit = 0;
	while (amount >= 1024 && unit < units.length - 1) {
		amount /= 1024;
		unit++;
	}
	return `${unit === 0 ? Math.floor(amount) : amount.toFixed(amount >= 100 ? 0 : 1)} ${units[unit]}`;
}

export default App;
