export type Mode = "relay" | "punch";
export type Role = "edge" | "target";
export type RuntimeState = "idle" | "starting" | "connecting" | "running" | "stopping" | "error";
export type PeerStatus = "waiting" | "paired";

export interface TunnelConfig {
  local: string;
  remote: string;
  name: string;
  pool?: number;
  rules?: TunnelRule[];
}

export interface TunnelRule {
  name: string;
  listen: string;
  local: string;
  remote: string;
  pool?: number;
}

export interface Config {
  mode: Mode;
  role: Role;
  secret: string;
  token: string;
  listen: string;
  remote: string;
  tunnel: TunnelConfig;
}

export interface RuntimeStatus {
  state: RuntimeState;
  mode?: Mode;
  role?: Role;
  listen?: string;
  message?: string;
  startedAt?: string;
  peers?: RelayPeer[];
}

export interface RelayPeer {
  id: string;
  ip: string;
  name?: string;
  role: Role;
  status: PeerStatus;
  endpoint?: string;
  relayEndpoint?: string;
  platform?: string;
  routeId?: string;
  peerId?: string;
  peerName?: string;
  proxied?: boolean;
  connectedAt: string;
  lastActivityAt?: string;
  bytesReceived?: number;
  bytesSent?: number;
  framesReceived?: number;
  framesSent?: number;
}

export interface PeerChange {
  action: "upsert" | "update" | "remove";
  peers: RelayPeer[];
}

export interface RuntimeEvent {
  type: string;
  level: "info" | "warning" | "error";
  state?: RuntimeState;
  message: string;
  listen?: string;
  time: string;
  peerChange?: PeerChange;
  transient?: boolean;
}

export interface ValidationResult {
  valid: boolean;
  errors: string[];
}
