export type Mode = "relay" | "target" | "edge";
export type Role = "edge" | "target";
export type RuntimeState = "idle" | "starting" | "connecting" | "running" | "stopping" | "error";
export type PeerStatus = "waiting" | "paired";
export type MappingState = "listening" | "waiting" | "error";

export const TOKEN_LIFETIMES = ["never", "1d", "7d", "30d", "90d", "365d"] as const;
export type TokenLifetime = (typeof TOKEN_LIFETIMES)[number];

export interface TokenEntry {
  id: string;
  token: string;
  note?: string;
  disabled?: boolean;
  createdAt?: string;
  expiresAt?: string;
  previousToken?: string;
  previousExpiresAt?: string;
}

export interface ServiceEntry {
  id: string;
  name: string;
  address: string;
  groups?: string[];
}

export interface MappingEntry {
  service: string;
  group?: string;
  port: number;
  lan?: boolean;
}

export interface Config {
  mode: Mode;
  listen?: string;
  remote?: string;
  token?: string;
  name?: string;
  tokens?: TokenEntry[];
  services?: ServiceEntry[];
  mappings?: MappingEntry[];
}

export interface CatalogService {
  id: string;
  name: string;
  address: string;
  group?: string;
}

export interface GroupCatalog {
  group: string;
  online: boolean;
  services: CatalogService[];
}

export interface CatalogUpdate {
  online: boolean;
  services: CatalogService[];
  groups?: GroupCatalog[];
}

export interface MappingStatus {
  service: string;
  group?: string;
  serviceName?: string;
  address?: string;
  listen?: string;
  lan?: boolean;
  state: MappingState;
  message?: string;
  connections?: number;
  bytes?: number;
  updatedAt?: string;
}

export interface ServiceStatus {
  id: string;
  name: string;
  address: string;
  groups?: string[];
  streams?: number;
  lastError?: string;
  lastErrorAt?: string;
}

export interface RuntimeStatus {
  state: RuntimeState;
  mode?: Mode;
  listen?: string;
  message?: string;
  startedAt?: string;
  peers?: RelayPeer[];
  catalog?: CatalogUpdate;
  mappings?: MappingStatus[];
  services?: ServiceStatus[];
}

export interface RelayPeer {
  id: string;
  ip: string;
  name?: string;
  role: Role;
  status: PeerStatus;
  tokenId?: string;
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
  catalog?: CatalogUpdate;
  mappings?: MappingStatus[];
  services?: ServiceStatus[];
  transient?: boolean;
}

export interface ValidationResult {
  valid: boolean;
  errors: string[];
}

export interface SessionState {
  authenticated: boolean;
  setupRequired?: boolean;
  csrfToken?: string;
  mode: Mode;
  modeLocked: boolean;
  authRequired: boolean;
}
