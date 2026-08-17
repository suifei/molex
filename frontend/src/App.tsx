import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowClockwise,
  ArrowRight,
  Check,
  CircleNotch,
  CloudArrowUp,
  Copy,
  Desktop,
  DownloadSimple,
  Eye,
  EyeSlash,
  FloppyDisk,
  HardDrives,
  LockKey,
  Moon,
  Play,
  Plugs,
  PlugsConnected,
  Plus,
  ShieldCheck,
  SignOut,
  Stop,
  Sun,
  TerminalWindow,
  Translate,
  Trash,
  Warning,
  WifiSlash,
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
import type {
  Config,
  MappingEntry,
  Mode,
  RelayPeer,
  RuntimeEvent,
  RuntimeStatus,
  ServiceEntry,
  SessionState,
  TokenEntry,
} from "./types";

type Theme = "dark" | "light";
export type ThemePreference = Theme | "system";

export const THEME_STORAGE_KEY = "molex:theme";

const emptyStatus: RuntimeStatus = { state: "idle", message: "Ready" };

function emptyConfigFor(mode: Mode): Config {
  if (mode === "relay") return { mode, listen: "127.0.0.1:8080", tokens: [] };
  return { mode, remote: "wss://molex.example.com/ws/session", token: "", name: "" };
}

function clientGroupList(config: Config): TokenEntry[] {
  if ((config.tokens ?? []).length > 0) return config.tokens ?? [];
  return [{ id: "", token: config.token ?? "" }];
}

function groupsToConfig(groups: TokenEntry[]): Pick<Config, "token" | "tokens"> {
  const cleaned = groups.map((group) => ({ id: group.id.trim(), token: group.token }));
  if (cleaned.length <= 1 && !cleaned[0]?.id) {
    return { token: cleaned[0]?.token ?? "", tokens: undefined };
  }
  return { token: undefined, tokens: cleaned };
}

function mappingKey(group: string | undefined, service: string): string {
  return `${group ?? ""}\0${service}`;
}

function initialThemePreference(): ThemePreference {
  const saved = localStorage.getItem(THEME_STORAGE_KEY);
  return saved === "light" || saved === "dark" || saved === "system" ? saved : "system";
}

function nextThemePreference(preference: ThemePreference): ThemePreference {
  if (preference === "system") return "light";
  return preference === "light" ? "dark" : "system";
}

function App() {
  const [session, setSession] = useState<SessionState | null>(null);
  const [config, setConfig] = useState<Config>(emptyConfigFor("edge"));
  const [status, setStatus] = useState<RuntimeStatus>(emptyStatus);
  const [events, setEvents] = useState<RuntimeEvent[]>([]);
  const [themePreference, setThemePreference] = useState<ThemePreference>(initialThemePreference);
  const [systemTheme, setSystemTheme] = useState<Theme>(() =>
    window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light",
  );
  const [locale, setLocale] = useState<Locale>(initialLocale);
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [setupRequired, setSetupRequired] = useState(false);
  const [loading, setLoading] = useState(true);
  const [consoleLoading, setConsoleLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const [notice, setNotice] = useState(false);
  const [liveDown, setLiveDown] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const refreshGeneration = useRef(0);

  const text = copy[locale];
  const resolvedTheme = themePreference === "system" ? systemTheme : themePreference;
  const mode: Mode = session?.mode ?? "edge";
  const modeReady = Boolean(session && (session.modeLocked || !session.authRequired));
  const isRunning = ["starting", "connecting", "running", "stopping"].includes(status.state);

  const refreshRuntime = useCallback(async () => {
    const generation = ++refreshGeneration.current;
    const [nextStatus, nextEvents] = await Promise.all([api.getStatus(), api.getEvents()]);
    if (generation !== refreshGeneration.current) return;
    setStatus(nextStatus);
    setEvents(nextEvents.slice(-20));
  }, []);

  const loadConsole = useCallback(async (consoleMode: Mode) => {
    setConsoleLoading(true);
    try {
      const [loadedConfig, loadedStatus, loadedEvents] = await Promise.all([
        api.getConfig(),
        api.getStatus(),
        api.getEvents(),
      ]);
      setConfig({ ...emptyConfigFor(consoleMode), ...loadedConfig, mode: consoleMode });
      setStatus(loadedStatus);
      setEvents(loadedEvents.slice(-20));
    } finally {
      setConsoleLoading(false);
    }
  }, []);

  useEffect(() => {
    let mounted = true;
    void api.getSession()
      .then((loaded) => {
        if (!mounted) return;
        setSession(loaded);
        if (loaded.authenticated) {
          setAuthenticated(true);
          if (loaded.modeLocked || !loaded.authRequired) {
            // Render the console immediately with skeletons while the
            // configuration and runtime state stream in.
            void loadConsole(loaded.mode).catch((error: unknown) => setErrors([messageFrom(error)]));
          }
        } else {
          setSetupRequired(Boolean(loaded.setupRequired));
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
    if (!authenticated || !session?.modeLocked) return;
    let mounted = true;
    const unsubscribe = api.subscribe({
      onEvent: (event) => {
        if (!mounted) return;
        setLiveDown(false);
        refreshGeneration.current++;
        setStatus((current) => applyRuntimeEvent(current, event));
        if (!event.transient) {
          setEvents((current) => [...current.slice(-19), event]);
        }
      },
      onOpen: () => mounted && setLiveDown(false),
      onError: () => mounted && setLiveDown(true),
    });
    const interval = window.setInterval(() => void refreshRuntime().catch(() => setLiveDown(true)), 2500);
    return () => {
      mounted = false;
      unsubscribe?.();
      window.clearInterval(interval);
    };
  }, [authenticated, session?.modeLocked, refreshRuntime]);

  useEffect(() => {
    if (!authenticated) return;
    setNow(Date.now());
    const interval = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [authenticated]);

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

  const save = async () => {
    setBusy(true);
    setNotice(false);
    try {
      const validation = await api.validateConfig(config);
      if (!validation.valid) {
        setErrors(validation.errors);
        return;
      }
      await api.saveConfig(config);
      setErrors([]);
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
        const validation = await api.validateConfig(config);
        if (!validation.valid) {
          setErrors(validation.errors);
          return;
        }
        setErrors([]);
        await api.start(config);
      }
      await refreshRuntime();
    } catch (error) {
      setErrors([messageFrom(error)]);
    } finally {
      setBusy(false);
    }
  };

  const login = async (password: string) => {
    setBusy(true);
    setErrors([]);
    try {
      await api.login(password);
      const refreshed = await api.getSession();
      setSession(refreshed);
      setAuthenticated(true);
      void loadConsole(refreshed.mode).catch((error: unknown) => setErrors([messageFrom(error)]));
    } catch (error) {
      setErrors([messageFrom(error)]);
    } finally {
      setBusy(false);
    }
  };

  const setup = async (password: string) => {
    setBusy(true);
    setErrors([]);
    try {
      await api.setup(password);
      const refreshed = await api.getSession();
      setSession(refreshed);
      setSetupRequired(false);
      setAuthenticated(true);
      void loadConsole(refreshed.mode).catch((error: unknown) => setErrors([messageFrom(error)]));
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
      setConfig(emptyConfigFor(mode));
      setStatus(emptyStatus);
      setEvents([]);
      setErrors([]);
      setNotice(false);
    } catch (error) {
      setErrors([messageFrom(error)]);
    } finally {
      setBusy(false);
    }
  };

  const chooseRole = async (choice: Mode) => {
    setBusy(true);
    setErrors([]);
    try {
      const refreshed = await api.bootstrap(choice);
      setSession(refreshed);
      setConfig(emptyConfigFor(refreshed.mode));
      void loadConsole(refreshed.mode).catch((error: unknown) => setErrors([messageFrom(error)]));
    } catch (error) {
      setErrors([messageFrom(error)]);
    } finally {
      setBusy(false);
    }
  };

  if (loading) {
    return (
      <main className="boot-state" aria-live="polite">
        <div className="boot-mark" />
        <div className="boot-line" />
        <span>{text.loading}</span>
      </main>
    );
  }

  if (session?.authRequired && !authenticated) {
    if (setupRequired) {
      return (
        <SetupScreen
          locale={locale}
          themePreference={themePreference}
          busy={busy}
          errors={errors}
          onLocaleChange={() => setLocale(locale === "en" ? "zh-CN" : "en")}
          onThemeChange={() => setThemePreference(nextThemePreference(themePreference))}
          onSetup={setup}
        />
      );
    }
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

  if (session && !session.modeLocked) {
    return (
      <RolePicker
        locale={locale}
        themePreference={themePreference}
        busy={busy}
        errors={errors}
        onLocaleChange={() => setLocale(locale === "en" ? "zh-CN" : "en")}
        onThemeChange={() => setThemePreference(nextThemePreference(themePreference))}
        onChoose={chooseRole}
      />
    );
  }

  if (!session || !modeReady) {
    return (
      <main className="boot-state" aria-live="polite">
        <div className="boot-mark" />
        <div className="boot-line" />
        <span>{text.loading}</span>
      </main>
    );
  }

  const routeLabels: [string, string] = mode === "relay"
    ? [text.wssEntry, text.pairedPeers]
    : mode === "target"
      ? [text.publishedServicesNode, text.wssEntry]
      : [text.localMappings, text.publishedServicesNode];

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
          <span className="console-label">{text.consoleLabels[mode]}{api.isPreview() ? ` · ${text.previewData}` : ""}</span>
          <StatusTag status={status} label={text.states[status.state]} />
          <LanguageButton locale={locale} onClick={() => setLocale(locale === "en" ? "zh-CN" : "en")} />
          <ThemeButton
            locale={locale}
            preference={themePreference}
            onClick={() => setThemePreference(nextThemePreference(themePreference))}
          />
          {session.authRequired && (
            <button type="button" className="icon-button" onClick={() => void logout()} disabled={busy} aria-label={text.logout} title={text.logout}>
              <SignOut size={18} />
            </button>
          )}
        </div>
      </header>

      {liveDown && isRunning && (
        <div className="live-banner" role="status">
          <WifiSlash size={16} />
          <span>{text.liveUpdatesLost}</span>
        </div>
      )}

      <section className={`route-map state-${status.state}`} aria-label={text.currentRouteStatus}>
        <RouteNode label={routeLabels[0]} icon={<TerminalWindow size={19} />} />
        <div className="route-segment" aria-hidden="true"><span /></div>
        <div className="route-core">
          <img src="./molex-mark.svg" alt="" />
          <span>{status.state === "running" ? text.secured : status.state === "connecting" ? text.pairing : text.standby}</span>
        </div>
        <div className="route-segment" aria-hidden="true"><span /></div>
        <RouteNode label={routeLabels[1]} icon={mode === "relay" ? <PlugsConnected size={19} /> : <CloudArrowUp size={19} />} />
      </section>

      <section className={`workspace ${mode === "relay" ? "relay-workspace" : ""}`}>
        {mode === "relay" && (
          <RelayConsole
            locale={locale}
            config={config}
            status={status}
            busy={busy}
            errors={errors}
            notice={notice}
            isRunning={isRunning}
            loading={consoleLoading}
            onField={setField}
            onSave={() => void save()}
            onToggleRuntime={() => void toggleRuntime()}
            onError={(message) => setErrors([message])}
          />
        )}
        {mode === "target" && (
          <TargetConsole
            locale={locale}
            config={config}
            status={status}
            busy={busy}
            errors={errors}
            notice={notice}
            isRunning={isRunning}
            loading={consoleLoading}
            onField={setField}
            onSave={() => void save()}
            onToggleRuntime={() => void toggleRuntime()}
            onServicesSaved={(services) => setConfig((current) => ({ ...current, services }))}
            onError={(message) => setErrors([message])}
          />
        )}
        {mode === "edge" && (
          <EdgeConsole
            locale={locale}
            config={config}
            status={status}
            busy={busy}
            errors={errors}
            notice={notice}
            isRunning={isRunning}
            loading={consoleLoading}
            onField={setField}
            onSave={() => void save()}
            onToggleRuntime={() => void toggleRuntime()}
            onMappingsSaved={(mappings) => setConfig((current) => ({ ...current, mappings }))}
            onError={(message) => setErrors([message])}
          />
        )}

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
            <RuntimeFact label={text.mode} value={text.modes[mode]} />
            {mode === "relay" && (
              <RuntimeFact
                label={text.listen}
                value={status.state === "running" ? status.listen || config.listen || "" : text.notListening}
                mono={status.state === "running"}
              />
            )}
          </dl>

          <div className="security-line">
            <ShieldCheck size={20} />
            <span>{mode === "relay" ? text.ciphertextRelay : text.aesGcmSession}</span>
          </div>

          {mode === "relay" && (
            <RelayPeersSection locale={locale} status={status} now={now} onError={(message) => setErrors([message])} />
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

interface ConsoleProps {
  locale: Locale;
  config: Config;
  status: RuntimeStatus;
  busy: boolean;
  errors: string[];
  notice: boolean;
  isRunning: boolean;
  loading: boolean;
  onField: <K extends keyof Config>(field: K, value: Config[K]) => void;
  onSave: () => void;
  onToggleRuntime: () => void;
  onError: (message: string) => void;
}

function RelayConsole(props: ConsoleProps) {
  const { locale, config, status, busy, errors, notice, isRunning, loading, onField, onSave, onToggleRuntime, onError } = props;
  const text = copy[locale];
  const [relayDomain, setRelayDomain] = useState("molex.example.com");
  const [adminDomain, setAdminDomain] = useState("admin.molex.example.com");

  const downloadCaddyfile = () => {
    const relayHost = relayDomain.trim();
    const adminHost = adminDomain.trim();
    if (!relayHost || !adminHost) return;
    const content = `${relayHost} {\n    @molex_session {\n        path /ws/session\n        header Connection *Upgrade*\n        header Upgrade websocket\n    }\n    handle @molex_session {\n        reverse_proxy ${config.listen || "127.0.0.1:8080"}\n    }\n    handle {\n        respond "Hello, world." 200\n    }\n}\n\n${adminHost} {\n    reverse_proxy 127.0.0.1:9090\n}\n`;
    const url = URL.createObjectURL(new Blob([content], { type: "text/plain;charset=utf-8" }));
    const link = document.createElement("a");
    link.href = url;
    link.download = "Caddyfile";
    link.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="config-panel">
      <div className="panel-heading">
        <div>
          <h1>{text.relayConfiguration}</h1>
          <p>{text.relayConfigurationDescription}</p>
        </div>
      </div>

      {loading ? (
        <SkeletonRows count={4} label={text.loadingData} />
      ) : (
        <>
          <div className="form-grid relay-fields">
            <Field label={text.listenAddress} htmlFor="listen">
              <input id="listen" value={config.listen ?? ""} onChange={(event) => onField("listen", event.target.value)} disabled={isRunning} spellCheck={false} />
            </Field>
            <div className="field full-width caddy-helper">
              <div className="rule-manager-heading"><div><span className="field-label">{text.caddySetup}</span><p>{text.caddySetupDescription}</p></div><a className="text-link" href="https://caddyserver.com/docs/install" target="_blank" rel="noreferrer">{text.caddyOfficialGuide}</a></div>
              <div className="caddy-fields">
                <label><span>{text.relayDomain}</span><input value={relayDomain} onChange={(event) => setRelayDomain(event.target.value)} spellCheck={false} /></label>
                <label><span>{text.adminDomain}</span><input value={adminDomain} onChange={(event) => setAdminDomain(event.target.value)} spellCheck={false} /></label>
                <button type="button" className="button secondary-button" onClick={downloadCaddyfile} disabled={!relayDomain.trim() || !adminDomain.trim()}><DownloadSimple size={17} />{text.downloadCaddyfile}</button>
              </div>
            </div>
          </div>

          <TokensManager locale={locale} status={status} onError={onError} />
        </>
      )}

      <FormActions
        locale={locale}
        errors={errors}
        notice={notice}
        busy={busy}
        isRunning={isRunning}
        stopping={status.state === "stopping"}
        onSave={onSave}
        onToggleRuntime={onToggleRuntime}
      />
    </div>
  );
}

function TokensManager({ locale, status, onError }: { locale: Locale; status: RuntimeStatus; onError: (message: string) => void }) {
  const text = copy[locale];
  const [tokens, setTokens] = useState<TokenEntry[] | null>(null);
  const [note, setNote] = useState("");
  const [creating, setCreating] = useState(false);
  const [pendingID, setPendingID] = useState("");
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const [copiedID, setCopiedID] = useState("");
  const [graceDays, setGraceDays] = useState(3);

  useEffect(() => {
    let mounted = true;
    void api.getTokens()
      .then((loaded) => mounted && setTokens(loaded))
      .catch((error: unknown) => {
        if (!mounted) return;
        setTokens([]);
        onError(messageFrom(error));
      });
    return () => {
      mounted = false;
    };
  }, [onError]);

  const groups = useMemo(() => {
    const byToken = new Map<string, { targetOnline: boolean; edges: number }>();
    for (const peer of status.peers ?? []) {
      if (!peer.tokenId) continue;
      const entry = byToken.get(peer.tokenId) ?? { targetOnline: false, edges: 0 };
      if (peer.role === "target") entry.targetOnline = true;
      if (peer.role === "edge") entry.edges++;
      byToken.set(peer.tokenId, entry);
    }
    return byToken;
  }, [status.peers]);

  const create = async () => {
    setCreating(true);
    try {
      const entry = await api.createToken(note.trim());
      setTokens((current) => [...(current ?? []), entry]);
      setRevealed((current) => ({ ...current, [entry.id]: true }));
      setNote("");
    } catch (error) {
      onError(messageFrom(error));
    } finally {
      setCreating(false);
    }
  };

  const toggleDisabled = async (token: TokenEntry) => {
    setPendingID(token.id);
    try {
      const updated = await api.updateToken(token.id, { disabled: !token.disabled });
      setTokens((current) => (current ?? []).map((entry) => (entry.id === token.id ? updated : entry)));
    } catch (error) {
      onError(messageFrom(error));
    } finally {
      setPendingID("");
    }
  };

  const rotate = async (token: TokenEntry) => {
    setPendingID(token.id);
    try {
      const updated = await api.rotateToken(token.id, graceDays);
      setTokens((current) => (current ?? []).map((entry) => (entry.id === token.id ? updated : entry)));
      setRevealed((current) => ({ ...current, [token.id]: true }));
    } catch (error) {
      onError(messageFrom(error));
    } finally {
      setPendingID("");
    }
  };

  const remove = async (token: TokenEntry) => {
    setPendingID(token.id);
    try {
      await api.deleteToken(token.id);
      setTokens((current) => (current ?? []).filter((entry) => entry.id !== token.id));
    } catch (error) {
      onError(messageFrom(error));
    } finally {
      setPendingID("");
    }
  };

  const copyValue = async (token: TokenEntry) => {
    try {
      await navigator.clipboard.writeText(token.token);
      setCopiedID(token.id);
      window.setTimeout(() => setCopiedID((current) => (current === token.id ? "" : current)), 1500);
    } catch (error) {
      onError(messageFrom(error));
    }
  };

  return (
    <div className="field full-width token-manager">
      <div className="rule-manager-heading">
        <div>
          <span className="field-label">{text.accessTokens}</span>
          <p>{text.accessTokensDescription}</p>
        </div>
      </div>
      <div className="token-create">
        <input
          value={note}
          onChange={(event) => setNote(event.target.value)}
          placeholder={text.tokenNotePlaceholder}
          aria-label={text.tokenNotePlaceholder}
          spellCheck={false}
        />
        <button type="button" className="button secondary-button compact-command" onClick={() => void create()} disabled={creating}>
          {creating ? <CircleNotch size={17} className="spin" /> : <Plus size={17} />}
          {text.createToken}
        </button>
        <label className="token-grace">
          <span>{text.graceDays}</span>
          <input
            type="number"
            min={1}
            max={30}
            value={graceDays}
            onChange={(event) => setGraceDays(Math.min(30, Math.max(1, Number(event.target.value) || 3)))}
            aria-label={text.graceDays}
          />
        </label>
      </div>

      {tokens === null ? (
        <SkeletonRows count={2} label={text.loadingData} />
      ) : tokens.length === 0 ? (
        <div className="rule-empty">{text.noTokensYet}</div>
      ) : (
        <div className="token-list">
          {tokens.map((token) => {
            const group = groups.get(token.id);
            const visible = Boolean(revealed[token.id]);
            const pending = pendingID === token.id;
            return (
              <div className={`token-row ${token.disabled ? "token-disabled" : ""}`} key={token.id}>
                <div className="token-row-top">
                  <div className="token-identity">
                    <strong>{token.note || token.id}</strong>
                    <span className="mono">{token.id}</span>
                  </div>
                  <div className="token-chips">
                    {token.disabled ? (
                      <span className="token-chip chip-disabled">{text.tokenDisabledTag}</span>
                    ) : (
                      <>
                        <span className={`token-chip ${group?.targetOnline ? "chip-online" : "chip-offline"}`}>
                          {group?.targetOnline ? text.tokenTargetOnline : text.tokenTargetOffline}
                        </span>
                        <span className="token-chip chip-neutral">{group?.edges ?? 0} {text.tokenEdgeCount}</span>
                      </>
                    )}
                  </div>
                </div>
                <div className="token-value">
                  <code className="mono" title={visible ? token.token : undefined}>
                    {visible ? token.token : "•".repeat(24)}
                  </code>
                  <button type="button" className="icon-button" onClick={() => setRevealed((current) => ({ ...current, [token.id]: !visible }))} aria-label={visible ? text.hideToken : text.revealToken} title={visible ? text.hideToken : text.revealToken}>
                    {visible ? <EyeSlash size={16} /> : <Eye size={16} />}
                  </button>
                  <button type="button" className="icon-button" onClick={() => void copyValue(token)} aria-label={text.copyToken} title={text.copyToken}>
                    {copiedID === token.id ? <Check size={16} /> : <Copy size={16} />}
                  </button>
                </div>
                <div className="token-row-actions">
                  {token.createdAt && <span className="token-created">{text.createdAt} {formatTime(token.createdAt, locale)}</span>}
                  {token.previousExpiresAt && (
                    <span className="token-created">{text.tokenGraceUntil} {formatTime(token.previousExpiresAt, locale)}</span>
                  )}
                  <button type="button" className="button secondary-button compact-command" onClick={() => void rotate(token)} disabled={pending || token.disabled}>
                    {pending ? <CircleNotch size={16} className="spin" /> : <ArrowClockwise size={16} />}
                    {text.rotateToken}
                  </button>
                  <button type="button" className="button secondary-button compact-command" onClick={() => void toggleDisabled(token)} disabled={pending}>
                    {pending ? <CircleNotch size={16} className="spin" /> : token.disabled ? <Plugs size={16} /> : <Stop size={16} />}
                    {token.disabled ? text.enableToken : text.disableToken}
                  </button>
                  <button type="button" className="icon-button rule-delete" onClick={() => void remove(token)} disabled={pending} aria-label={text.deleteToken} title={text.deleteToken}>
                    <Trash size={16} />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function RelayPeersSection({ locale, status, now, onError }: { locale: Locale; status: RuntimeStatus; now: number; onError: (message: string) => void }) {
  const text = copy[locale];
  const peers = status.peers ?? [];
  const [pendingID, setPendingID] = useState("");

  const kick = async (peer: RelayPeer) => {
    setPendingID(peer.id);
    try {
      await api.disconnectPeer(peer.id);
    } catch (error) {
      onError(messageFrom(error));
    } finally {
      setPendingID("");
    }
  };

  return (
    <section className="peer-section" aria-labelledby="connected-clients-heading">
      <div className="peer-heading">
        <h3 id="connected-clients-heading">{text.connectedClients}</h3>
        <span>{peers.length}</span>
      </div>
      <div className="peer-list" aria-live="polite">
        {peers.length === 0 ? (
          <div className="empty-peers">
            <PlugsConnected size={22} />
            <span>{text.noConnectedClients}</span>
          </div>
        ) : (
          peers.map((peer) => (
            <RelayPeerRow
              key={peer.id}
              peer={peer}
              locale={locale}
              now={now}
              pending={pendingID === peer.id}
              onKick={() => void kick(peer)}
            />
          ))
        )}
      </div>
    </section>
  );
}

function TargetConsole(props: ConsoleProps & { onServicesSaved: (services: ServiceEntry[]) => void }) {
  const { locale, config, status, busy, errors, notice, isRunning, loading, onField, onSave, onToggleRuntime, onServicesSaved, onError } = props;
  const text = copy[locale];

  return (
    <div className="config-panel">
      <div className="panel-heading">
        <div>
          <h1>{text.connection}</h1>
          <p>{text.connectionDescriptionTarget}</p>
        </div>
      </div>

      {loading ? (
        <SkeletonRows count={5} label={text.loadingData} />
      ) : (
        <>
          <div className="form-grid">
            <Field label={text.wssEndpoint} htmlFor="remote" wide>
              <input id="remote" value={config.remote ?? ""} onChange={(event) => onField("remote", event.target.value)} disabled={isRunning} spellCheck={false} />
            </Field>
            <ClientGroupsEditor
              locale={locale}
              config={config}
              disabled={isRunning}
              onChange={(next) => {
                onField("token", next.token);
                onField("tokens", next.tokens);
              }}
            />
            <Field label={text.nodeName} htmlFor="node-name">
              <input id="node-name" value={config.name ?? ""} onChange={(event) => onField("name", event.target.value)} disabled={isRunning} spellCheck={false} />
            </Field>
          </div>

          <ServicesEditor
            locale={locale}
            services={config.services ?? []}
            groupNames={clientGroupList(config).map((group) => group.id).filter(Boolean)}
            statuses={status.services ?? []}
            onSaved={onServicesSaved}
            onError={onError}
          />
        </>
      )}

      <FormActions
        locale={locale}
        errors={errors}
        notice={notice}
        busy={busy}
        isRunning={isRunning}
        stopping={status.state === "stopping"}
        onSave={onSave}
        onToggleRuntime={onToggleRuntime}
      />
    </div>
  );
}

function ServicesEditor({ locale, services, groupNames, statuses, onSaved, onError }: {
  locale: Locale;
  services: ServiceEntry[];
  groupNames: string[];
  statuses: { id: string; name: string; address: string; streams?: number; lastError?: string }[];
  onSaved: (services: ServiceEntry[]) => void;
  onError: (message: string) => void;
}) {
  const text = copy[locale];
  const [draft, setDraft] = useState<ServiceEntry[]>(services);
  const [saving, setSaving] = useState(false);
  const [savedFlash, setSavedFlash] = useState(false);
  const loadedRef = useRef(false);

  useEffect(() => {
    if (loadedRef.current) return;
    loadedRef.current = true;
    setDraft(services);
  }, [services]);

  const statusByID = useMemo(() => new Map(statuses.map((entry) => [entry.id, entry])), [statuses]);
  const dirty = JSON.stringify(draft) !== JSON.stringify(services);

  const update = (index: number, field: "name" | "address", value: string) => {
    setDraft((current) => current.map((entry, entryIndex) => (entryIndex === index ? { ...entry, [field]: value } : entry)));
  };
  const toggleGroup = (index: number, group: string) => {
    setDraft((current) => current.map((entry, entryIndex) => {
      if (entryIndex !== index) return entry;
      const selected = new Set(entry.groups ?? groupNames);
      if (selected.has(group)) selected.delete(group);
      else selected.add(group);
      const next = groupNames.filter((name) => selected.has(name));
      return { ...entry, groups: next.length === 0 || next.length === groupNames.length ? undefined : next };
    }));
  };
  const add = () => setDraft((current) => [...current, { id: "", name: "", address: "" }]);
  const remove = (index: number) => setDraft((current) => current.filter((_, entryIndex) => entryIndex !== index));

  const saveServices = async () => {
    setSaving(true);
    try {
      const saved = await api.saveServices(draft);
      setDraft(saved);
      onSaved(saved);
      setSavedFlash(true);
      window.setTimeout(() => setSavedFlash(false), 1500);
    } catch (error) {
      onError(messageFrom(error));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="field full-width rule-manager service-manager">
      <div className="rule-manager-heading">
        <div>
          <span className="field-label">{text.publishedServices}</span>
          <p>{text.publishedServicesDescription}</p>
        </div>
        <button type="button" className="button secondary-button compact-command" onClick={add}>
          <Plus size={17} />
          {text.addService}
        </button>
      </div>
      {draft.length === 0 ? (
        <div className="rule-empty">{text.noServicesYet}</div>
      ) : (
        <div className="rule-list">
          {draft.map((service, index) => {
            const live = service.id ? statusByID.get(service.id) : undefined;
            return (
              <div className="rule-row service-row" key={service.id || `new-${index}`}>
                <div className="rule-index">{index + 1}</div>
                <label><span>{text.serviceName}</span><input value={service.name} onChange={(event) => update(index, "name", event.target.value)} spellCheck={false} /></label>
                <label><span>{text.serviceAddress}</span><input value={service.address} onChange={(event) => update(index, "address", event.target.value)} spellCheck={false} placeholder="10.188.200.16:30927" /></label>
                {groupNames.length > 1 && (
                  <fieldset className="service-groups">
                    <legend>{text.visibleGroups}</legend>
                    {groupNames.map((group) => {
                      const checked = !service.groups?.length || service.groups.includes(group);
                      return (
                        <label key={group}>
                          <input type="checkbox" checked={checked} onChange={() => toggleGroup(index, group)} aria-label={group} />
                          <span>{group}</span>
                        </label>
                      );
                    })}
                  </fieldset>
                )}
                <div className="service-live">
                  {live && <span className="token-chip chip-neutral">{live.streams ?? 0} {text.serviceStreams}</span>}
                  {live?.lastError && <span className="service-error" title={live.lastError}>{text.serviceLastError}: {live.lastError}</span>}
                </div>
                <button type="button" className="icon-button rule-delete" onClick={() => remove(index)} aria-label={text.deleteService} title={text.deleteService}>
                  <Trash size={17} />
                </button>
              </div>
            );
          })}
        </div>
      )}
      <div className="manager-actions">
        {savedFlash && <span className="manager-saved"><Check size={15} /> {text.configurationSaved}</span>}
        <button type="button" className="button secondary-button" onClick={() => void saveServices()} disabled={saving || !dirty}>
          {saving ? <CircleNotch size={17} className="spin" /> : <FloppyDisk size={17} />}
          {text.saveServices}
        </button>
      </div>
    </div>
  );
}

function EdgeConsole(props: ConsoleProps & { onMappingsSaved: (mappings: MappingEntry[]) => void }) {
  const { locale, config, status, busy, errors, notice, isRunning, loading, onField, onSave, onToggleRuntime, onMappingsSaved, onError } = props;
  const text = copy[locale];

  return (
    <div className="config-panel">
      <div className="panel-heading">
        <div>
          <h1>{text.connection}</h1>
          <p>{text.connectionDescriptionEdge}</p>
        </div>
      </div>

      {loading ? (
        <SkeletonRows count={5} label={text.loadingData} />
      ) : (
        <>
          <div className="form-grid">
            <Field label={text.wssEndpoint} htmlFor="remote" wide>
              <input id="remote" value={config.remote ?? ""} onChange={(event) => onField("remote", event.target.value)} disabled={isRunning} spellCheck={false} />
            </Field>
            <ClientGroupsEditor
              locale={locale}
              config={config}
              disabled={isRunning}
              onChange={(next) => {
                onField("token", next.token);
                onField("tokens", next.tokens);
              }}
            />
            <Field label={text.nodeName} htmlFor="node-name">
              <input id="node-name" value={config.name ?? ""} onChange={(event) => onField("name", event.target.value)} disabled={isRunning} spellCheck={false} />
            </Field>
          </div>

          <CatalogMapper
            locale={locale}
            config={config}
            status={status}
            isRunning={isRunning}
            onSaved={onMappingsSaved}
            onError={onError}
          />
        </>
      )}

      <FormActions
        locale={locale}
        errors={errors}
        notice={notice}
        busy={busy}
        isRunning={isRunning}
        stopping={status.state === "stopping"}
        onSave={onSave}
        onToggleRuntime={onToggleRuntime}
      />
    </div>
  );
}

interface MappingDraft {
  checked: boolean;
  port: string;
  lan: boolean;
}

function CatalogMapper({ locale, config, status, isRunning, onSaved, onError }: {
  locale: Locale;
  config: Config;
  status: RuntimeStatus;
  isRunning: boolean;
  onSaved: (mappings: MappingEntry[]) => void;
  onError: (message: string) => void;
}) {
  const text = copy[locale];
  const [drafts, setDrafts] = useState<Map<string, MappingDraft>>(() => {
    const initial = new Map<string, MappingDraft>();
    for (const mapping of config.mappings ?? []) {
      initial.set(mappingKey(mapping.group, mapping.service), { checked: true, port: String(mapping.port), lan: Boolean(mapping.lan) });
    }
    return initial;
  });
  const [applying, setApplying] = useState(false);
  const [savedFlash, setSavedFlash] = useState(false);

  const catalog = status.catalog;
  const online = Boolean(catalog?.online);
  const grouped = (catalog?.groups ?? []).length > 0;
  const statusByService = useMemo(
    () => new Map((status.mappings ?? []).map((entry) => [mappingKey(entry.group, entry.service), entry])),
    [status.mappings],
  );

  // Rows = published services plus any configured mapping whose service is
  // currently unpublished (kept so it can be unchecked or resumed later).
  const rows = useMemo(() => {
    const seen = new Set<string>();
    const list: { id: string; group?: string; name?: string; address?: string; published: boolean; groupOnline?: boolean }[] = [];
    const published = grouped
      ? (catalog?.groups ?? []).flatMap((entry) => entry.services.map((service) => ({ ...service, group: entry.group, groupOnline: entry.online })))
      : (catalog?.services ?? []).map((service) => ({ ...service, groupOnline: online }));
    for (const service of published) {
      const key = mappingKey(service.group, service.id);
      seen.add(key);
      list.push({ id: service.id, group: service.group, name: service.name, address: service.address, published: true, groupOnline: service.groupOnline });
    }
    for (const [key, draft] of drafts) {
      if (seen.has(key) || !draft.checked) continue;
      const runtime = statusByService.get(key);
      const [group, serviceID] = key.split("\0");
      list.push({ id: serviceID, group: group || undefined, name: runtime?.serviceName, address: runtime?.address, published: false, groupOnline: false });
    }
    return list;
  }, [catalog?.groups, catalog?.services, drafts, grouped, online, statusByService]);

  const savedMappings = config.mappings ?? [];
  const draftMappings = useMemo(() => {
    const list: MappingEntry[] = [];
    for (const [key, draft] of drafts) {
      if (!draft.checked) continue;
      const [group, serviceID] = key.split("\0");
      list.push({ service: serviceID, group: group || undefined, port: Number(draft.port) || 0, lan: draft.lan || undefined });
    }
    return list.sort((left, right) => mappingKey(left.group, left.service).localeCompare(mappingKey(right.group, right.service)));
  }, [drafts]);
  const dirty = JSON.stringify(draftMappings) !== JSON.stringify(
    [...savedMappings].sort((left, right) => mappingKey(left.group, left.service).localeCompare(mappingKey(right.group, right.service)))
      .map((mapping) => ({ service: mapping.service, group: mapping.group || undefined, port: mapping.port, lan: mapping.lan || undefined })),
  );

  const toggleService = async (service: { id: string; group?: string }) => {
    const key = mappingKey(service.group, service.id);
    const current = drafts.get(key);
    if (current?.checked) {
      setDrafts((previous) => {
        const next = new Map(previous);
        next.set(key, { ...current, checked: false });
        return next;
      });
      return;
    }
    let port = current?.port ?? "";
    if (!port) {
      try {
        port = String(await api.suggestPort(false));
      } catch {
        port = String(20000 + Math.floor(Math.random() * 20000));
      }
    }
    setDrafts((previous) => {
      const next = new Map(previous);
      next.set(key, { checked: true, port, lan: current?.lan ?? false });
      return next;
    });
  };

  const updateDraft = (service: { id: string; group?: string }, changes: Partial<MappingDraft>) => {
    const key = mappingKey(service.group, service.id);
    setDrafts((previous) => {
      const next = new Map(previous);
      const current = next.get(key) ?? { checked: false, port: "", lan: false };
      next.set(key, { ...current, ...changes });
      return next;
    });
  };

  const apply = async () => {
    for (const mapping of draftMappings) {
      if (!Number.isInteger(mapping.port) || mapping.port < 1 || mapping.port > 65535) {
        onError(`mappings[${draftMappings.indexOf(mapping)}].port must be between 1 and 65535`);
        return;
      }
    }
    setApplying(true);
    try {
      const saved = await api.saveMappings(draftMappings);
      onSaved(saved);
      setSavedFlash(true);
      window.setTimeout(() => setSavedFlash(false), 1500);
    } catch (error) {
      onError(messageFrom(error));
    } finally {
      setApplying(false);
    }
  };

  return (
    <div className="field full-width rule-manager catalog-manager">
      <div className="rule-manager-heading">
        <div>
          <span className="field-label">{text.serviceCatalog}</span>
          <p>{text.serviceCatalogDescription}</p>
        </div>
      </div>

      {!isRunning && rows.length === 0 ? (
        <div className="rule-empty">{text.catalogStartHint}</div>
      ) : isRunning && !online && rows.length === 0 ? (
        <div className="catalog-loading" role="status">
          <CircleNotch size={18} className="spin" />
          <span>{text.catalogWaiting}</span>
        </div>
      ) : isRunning && !online && rows.length > 0 ? (
        <div className="catalog-offline" role="status">
          <WifiSlash size={16} />
          <span>{text.catalogOffline}</span>
        </div>
      ) : null}

      {isRunning && online && rows.length === 0 && (
        <div className="rule-empty">{text.noCatalogServices}</div>
      )}

      {rows.length > 0 && (
        <div className="catalog-list">
          {rows.map((service) => {
            const key = mappingKey(service.group, service.id);
            const draft = drafts.get(key) ?? { checked: false, port: "", lan: false };
            const runtime = statusByService.get(key);
            const label = service.group ? `${service.group} / ${service.name ?? service.id}` : (service.name ?? service.id);
            return (
              <div className={`catalog-row ${draft.checked ? "catalog-checked" : ""}`} key={key}>
                <label className="catalog-main">
                  <input
                    type="checkbox"
                    checked={draft.checked}
                    onChange={() => void toggleService(service)}
                    aria-label={`${text.serviceCatalog}: ${label}`}
                  />
                  <span className="catalog-identity">
                    <strong>{service.name ?? service.id}</strong>
                    {service.group && <span className="token-chip chip-neutral">{text.catalogGroup} {service.group}</span>}
                    <span className="mono catalog-address" title={service.address ?? ""}>{service.address ?? "—"}</span>
                  </span>
                </label>
                {!service.published && <span className="token-chip chip-offline">{text.mappingUnpublished}</span>}
                {service.published && service.groupOnline === false && <span className="token-chip chip-offline">{text.catalogGroupOffline}</span>}
                {draft.checked && (
                  <div className="catalog-mapping">
                    <label className="catalog-port">
                      <span>{text.localPort}</span>
                      <input
                        type="number"
                        min={1}
                        max={65535}
                        value={draft.port}
                        onChange={(event) => updateDraft(service, { port: event.target.value })}
                        aria-label={`${text.localPort} ${label}`}
                      />
                    </label>
                    <label className="catalog-lan" title={text.lanVisibleHint}>
                      <input
                        type="checkbox"
                        checked={draft.lan}
                        onChange={(event) => updateDraft(service, { lan: event.target.checked })}
                      />
                      <span>{text.lanVisible}</span>
                    </label>
                    {runtime && (
                      <span className={`mapping-state mapping-${runtime.state}`} title={runtime.message ? localizeRuntimeMessage(runtime.message, locale) : undefined}>
                        {text.mappingStates[runtime.state]}
                        {runtime.state === "listening" && runtime.listen ? <code className="mono">{runtime.listen}</code> : null}
                      </span>
                    )}
                    {runtime && (runtime.connections ?? 0) > 0 && (
                      <span className="mapping-stats">{runtime.connections} {text.connectionsCount} · {formatBytes(runtime.bytes ?? 0)} {text.transferred}</span>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      <div className="manager-actions">
        {savedFlash && <span className="manager-saved"><Check size={15} /> {text.configurationSaved}</span>}
        <button type="button" className="button secondary-button" onClick={() => void apply()} disabled={applying || !dirty}>
          {applying ? <CircleNotch size={17} className="spin" /> : <FloppyDisk size={17} />}
          {text.applyMappings}
        </button>
      </div>
    </div>
  );
}

function RolePicker({ locale, themePreference, busy, errors, onLocaleChange, onThemeChange, onChoose }: {
  locale: Locale;
  themePreference: ThemePreference;
  busy: boolean;
  errors: string[];
  onLocaleChange: () => void;
  onThemeChange: () => void;
  onChoose: (mode: Mode) => Promise<void>;
}) {
  const text = copy[locale];
  const command = "molex config init --mode relay";
  const [copied, setCopied] = useState(false);

  return (
    <main className="auth-shell">
      <header className="auth-topbar">
        <div className="brand-lockup">
          <img src="./molex-mark.svg" alt="" className="brand-mark" />
          <div><div className="brand-name">MoleX</div><div className="brand-subtitle">{text.brandSubtitle}</div></div>
        </div>
        <div className="topbar-actions">
          <LanguageButton locale={locale} onClick={onLocaleChange} />
          <ThemeButton locale={locale} preference={themePreference} onClick={onThemeChange} />
        </div>
      </header>
      <section className="auth-stage">
        <div className="login-panel role-picker">
          <div className="login-heading">
            <h1>{text.chooseRoleTitle}</h1>
            <p>{text.chooseRoleDescription}</p>
          </div>
          <div className="role-options">
            <button type="button" className="role-option" onClick={() => void onChoose("edge")} disabled={busy}>
              <Plugs size={26} />
              <strong>{text.roleEdgeTitle}</strong>
              <span>{text.roleEdgeDescription}</span>
            </button>
            <button type="button" className="role-option" onClick={() => void onChoose("target")} disabled={busy}>
              <HardDrives size={26} />
              <strong>{text.roleTargetTitle}</strong>
              <span>{text.roleTargetDescription}</span>
            </button>
          </div>
          <div className="role-relay-hint">
            <strong>{text.roleRelayTitle}</strong>
            <p>{text.roleRelayDescription}</p>
            <div className="role-command">
              <code className="mono">{command}</code>
              <button
                type="button"
                className="icon-button"
                onClick={() => {
                  void navigator.clipboard.writeText(command).then(() => {
                    setCopied(true);
                    window.setTimeout(() => setCopied(false), 1500);
                  }).catch(() => undefined);
                }}
                aria-label={text.copyCommand}
                title={text.copyCommand}
              >
                {copied ? <Check size={16} /> : <Copy size={16} />}
              </button>
            </div>
          </div>
          {errors.length > 0 && (
            <div className="message-strip error-strip" role="alert">
              <Warning size={18} weight="fill" />
              <div>{errors.map((error) => <div key={error}>{localizeValidationError(error, locale)}</div>)}</div>
            </div>
          )}
        </div>
      </section>
    </main>
  );
}

function FormActions({ locale, errors, notice, busy, isRunning, stopping, onSave, onToggleRuntime }: {
  locale: Locale;
  errors: string[];
  notice: boolean;
  busy: boolean;
  isRunning: boolean;
  stopping: boolean;
  onSave: () => void;
  onToggleRuntime: () => void;
}) {
  const text = copy[locale];
  return (
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
        <button type="button" className="button secondary-button" onClick={onSave} disabled={busy || isRunning}>
          <FloppyDisk size={18} />
          {text.save}
        </button>
        <button
          type="button"
          className={`button ${isRunning ? "stop-button" : "primary-button"}`}
          onClick={onToggleRuntime}
          disabled={busy || stopping}
        >
          {busy ? <CircleNotch size={18} className="spin" /> : isRunning ? <Stop size={18} weight="fill" /> : <Play size={18} weight="fill" />}
          {isRunning ? text.stop : text.start}
        </button>
      </div>
    </div>
  );
}

function ClientGroupsEditor({ locale, config, disabled, onChange }: {
  locale: Locale;
  config: Config;
  disabled: boolean;
  onChange: (next: Pick<Config, "token" | "tokens">) => void;
}) {
  const text = copy[locale];
  const groups = clientGroupList(config);
  const multi = groups.length > 1;

  const update = (index: number, changes: Partial<TokenEntry>) => {
    onChange(groupsToConfig(groups.map((group, groupIndex) => (groupIndex === index ? { ...group, ...changes } : group))));
  };
  const add = () => {
    const next = groups.map((group, index) => ({
      ...group,
      id: group.id || (index === 0 ? "default" : group.id),
    }));
    next.push({ id: "", token: "" });
    onChange(groupsToConfig(next));
  };
  const remove = (index: number) => {
    onChange(groupsToConfig(groups.filter((_, groupIndex) => groupIndex !== index)));
  };

  return (
    <div className="field full-width client-groups">
      <div className="rule-manager-heading">
        <div>
          <span className="field-label">{multi ? text.tokenGroups : text.accessToken}</span>
          <p>{text.tokenGroupsDescription}</p>
        </div>
        <button type="button" className="button secondary-button compact-command" onClick={add} disabled={disabled}>
          <Plus size={17} />
          {text.addGroup}
        </button>
      </div>
      <div className="client-group-list">
        {groups.map((group, index) => (
          <div className="client-group-row" key={`${group.id}-${index}`}>
            {multi && (
              <label>
                <span>{text.groupName}</span>
                <input
                  value={group.id}
                  onChange={(event) => update(index, { id: event.target.value })}
                  disabled={disabled}
                  placeholder={text.groupNamePlaceholder}
                  spellCheck={false}
                />
              </label>
            )}
            <label className={multi ? undefined : "full-width"}>
              <span>{text.accessToken}</span>
              <TokenField
                id={index === 0 ? "access-token" : `access-token-${index}`}
                locale={locale}
                value={group.token}
                disabled={disabled}
                onChange={(value) => update(index, { token: value })}
              />
            </label>
            {multi && (
              <button type="button" className="icon-button rule-delete" onClick={() => remove(index)} disabled={disabled} aria-label={text.deleteGroup} title={text.deleteGroup}>
                <Trash size={16} />
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

function TokenField({ id, locale, value, disabled, onChange }: {
  id: string;
  locale: Locale;
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  const text = copy[locale];
  const [visible, setVisible] = useState(false);
  return (
    <span className="input-with-actions">
      <input
        id={id}
        type={visible ? "text" : "password"}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        autoComplete="off"
        spellCheck={false}
      />
      <button
        type="button"
        onClick={() => setVisible((current) => !current)}
        disabled={disabled}
        aria-label={secretActionLabel(locale, visible ? "hide" : "show", text.accessToken)}
        title={secretActionLabel(locale, visible ? "hide" : "show", text.accessToken)}
      >
        {visible ? <EyeSlash size={17} /> : <Eye size={17} />}
      </button>
    </span>
  );
}

function SkeletonRows({ count, label }: { count: number; label: string }) {
  return (
    <div className="skeleton-block" role="status" aria-label={label}>
      {Array.from({ length: count }, (_, index) => (
        <div className="skeleton-row" key={index}>
          <span className="skeleton-line skeleton-short" />
          <span className="skeleton-line" />
        </div>
      ))}
    </div>
  );
}

function SetupScreen({ locale, themePreference, busy, errors, onLocaleChange, onThemeChange, onSetup }: {
  locale: Locale;
  themePreference: ThemePreference;
  busy: boolean;
  errors: string[];
  onLocaleChange: () => void;
  onThemeChange: () => void;
  onSetup: (password: string) => Promise<void>;
}) {
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [visible, setVisible] = useState(false);
  const text = copy[locale];
  const mismatch = confirmation.length > 0 && password !== confirmation;

  return (
    <main className="auth-shell">
      <header className="auth-topbar">
        <div className="brand-lockup">
          <img src="./molex-mark.svg" alt="" className="brand-mark" />
          <div><div className="brand-name">MoleX</div><div className="brand-subtitle">{text.brandSubtitle}</div></div>
        </div>
        <div className="topbar-actions">
          <LanguageButton locale={locale} onClick={onLocaleChange} />
          <ThemeButton locale={locale} preference={themePreference} onClick={onThemeChange} />
        </div>
      </header>
      <section className="auth-stage">
        <div className="auth-route" aria-hidden="true">
          <span className="auth-route-node" /><span className="auth-route-line" /><span className="auth-route-core"><img src="./molex-mark.svg" alt="" /></span><span className="auth-route-line" /><span className="auth-route-node" />
        </div>
        <form className="login-panel setup-panel" onSubmit={(event) => { event.preventDefault(); void onSetup(password); }}>
          <div className="login-kicker"><LockKey size={16} />{text.firstRunSetup}</div>
          <div className="login-heading"><h1>{text.createPassword}</h1><p>{text.createPasswordDescription}</p></div>
          <label className="login-field" htmlFor="setup-password">
            <span>{text.newPassword}</span>
            <span className="login-password">
              <input id="setup-password" type={visible ? "text" : "password"} value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="new-password" autoFocus disabled={busy} minLength={12} required />
              <button type="button" onClick={() => setVisible((current) => !current)} disabled={busy} aria-label={secretActionLabel(locale, visible ? "hide" : "show", text.newPassword)} title={secretActionLabel(locale, visible ? "hide" : "show", text.newPassword)}>{visible ? <EyeSlash size={18} /> : <Eye size={18} />}</button>
            </span>
          </label>
          <label className="login-field setup-confirm" htmlFor="setup-confirmation"><span>{text.confirmPassword}</span><input id="setup-confirmation" type={visible ? "text" : "password"} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="new-password" disabled={busy} required /></label>
          <p className={`setup-requirement ${mismatch ? "setup-mismatch" : ""}`}>{mismatch ? text.passwordMismatch : text.passwordRequirement}</p>
          {errors.length > 0 && <div className="message-strip error-strip login-error" role="alert"><Warning size={18} weight="fill" /><div>{errors.map((error) => <div key={error}>{localizeValidationError(error, locale)}</div>)}</div></div>}
          <button type="submit" className="button primary-button login-submit" disabled={busy || password.length < 12 || password !== confirmation}>{busy ? <CircleNotch size={18} className="spin" /> : <LockKey size={18} />}{text.finishSetup}</button>
        </form>
      </section>
    </main>
  );
}

function RelayPeerRow({ peer, locale, now, pending, onKick }: { peer: RelayPeer; locale: Locale; now: number; pending: boolean; onKick: () => void }) {
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
          <span>{text.tokenColumn}</span>
          <code title={peer.tokenId || "-"}>{peer.tokenId || "-"}</code>
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
          <dt>{text.forwardEndpoint}</dt>
          <dd className="mono" title={peer.endpoint || text.unknownAddress}>{peer.endpoint || text.unknownAddress}</dd>
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
      <div className="peer-actions">
        <button type="button" className="button secondary-button compact-command" onClick={onKick} disabled={pending}>
          {pending ? <CircleNotch size={15} className="spin" /> : <Plugs size={15} />}
          {text.disconnectPeer}
        </button>
      </div>
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

function Field({ label, htmlFor, wide, children }: { label: string; htmlFor: string; wide?: boolean; children: React.ReactNode }) {
  return (
    <label className={`field ${wide ? "full-width" : ""}`} htmlFor={htmlFor}>
      <span className="field-label">{label}</span>
      {children}
    </label>
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
  if (event.catalog) next.catalog = event.catalog;
  if (event.mappings) next.mappings = event.mappings;
  if (event.services) next.services = event.services;
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
