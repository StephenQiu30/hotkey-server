const configuredBase = process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:8000";
const trimmedBase = configuredBase === "/" ? "" : configuredBase.endsWith("/") ? configuredBase.slice(0, -1) : configuredBase;
export const API_BASE_URL = trimmedBase === "" ? "" : trimmedBase;

export type Keyword = {
  id: number;
  keyword: string;
  query_template: string | null;
  enabled: boolean;
  priority: number;
  created_at: string;
  updated_at: string;
};

export type AuthUser = {
  id: number;
  github_id: number;
  github_login: string;
  github_name: string | null;
  email: string | null;
  avatar_url: string | null;
  is_active: boolean;
  last_login_at: string | null;
  created_at: string;
  updated_at: string;
};

const AUTH_TOKEN_KEY = "ai_hotspot_radar_session_token";

export function getAuthToken(): string | null {
  if (typeof window === "undefined") {
    return null;
  }
  return window.localStorage.getItem(AUTH_TOKEN_KEY);
}

export function setAuthToken(token: string): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.setItem(AUTH_TOKEN_KEY, token);
}

export function clearAuthToken(): void {
  if (typeof window === "undefined") {
    return;
  }
  window.localStorage.removeItem(AUTH_TOKEN_KEY);
}

export type Source = {
  id: number;
  name: string;
  source_type: string;
  enabled: boolean;
  config: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type AiAnalysis = {
  id: number;
  hotspot_id: number;
  is_real: boolean | null;
  relevance_score: string;
  relevance_reason: string | null;
  keyword_mentioned: boolean;
  importance: string;
  summary: string | null;
  model_name: string | null;
  raw_response: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type Hotspot = {
  id: number;
  title: string;
  url: string;
  source_id: number;
  keyword_id: number | null;
  author: string | null;
  snippet: string | null;
  published_at: string | null;
  fetched_at: string;
  status: string;
  raw_payload: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  source?: Source | null;
  keyword?: Keyword | null;
  ai_analysis?: AiAnalysis | null;
};

export type CheckRun = {
  id: number;
  trigger_type: string;
  started_at: string;
  finished_at: string | null;
  status: string;
  success_count: number;
  failure_count: number;
  error_summary: string | null;
  created_at: string;
  updated_at: string;
};

export type Notification = {
  id: number;
  hotspot_id: number | null;
  report_id: number | null;
  channel: string;
  recipient: string | null;
  status: string;
  error_message: string | null;
  sent_at: string | null;
  created_at: string;
  updated_at: string;
};

export type SearchResult = {
  title: string;
  url: string;
  source_id: number;
  source_name: string;
  source_type: string;
  author: string | null;
  published_at: string | null;
  snippet: string | null;
  relevance_score: number;
  relevance_reason: string;
  keyword_mentioned: boolean;
  importance: string;
  summary: string;
  status: string;
  raw_payload: Record<string, unknown>;
};

export type SearchResponse = {
  query: string;
  items: SearchResult[];
  errors: string[];
};

export type Report = {
  id: number;
  report_type: "daily" | "weekly" | string;
  period_start: string;
  period_end: string;
  status: string;
  subject: string;
  summary: string | null;
  content: string;
  hotspot_count: number;
  sent_at: string | null;
  created_at: string;
  updated_at: string;
};

export type AnalyticsTrendPoint = {
  date: string;
  total_count: number;
  active_count: number;
  filtered_count: number;
};

export type AnalyticsTrendResponse = {
  period_days: number;
  points: AnalyticsTrendPoint[];
};

export type AnalyticsSourceStat = {
  source_id: number;
  source_name: string;
  hotspot_count: number;
  active_count: number;
  filtered_count: number;
};

export type AnalyticsSourceResponse = {
  period_days: number;
  limit: number;
  items: AnalyticsSourceStat[];
};

export type AnalyticsSentimentPoint = {
  importance: string;
  count: number;
};

export type AnalyticsSentimentResponse = {
  period_days: number;
  total: number;
  by_importance: AnalyticsSentimentPoint[];
};

export type Setting = {
  key: string;
  value: Record<string, unknown>;
  description: string | null;
  created_at: string;
  updated_at: string;
};

export type Page<T> = {
  items: T[];
  limit: number;
  offset: number;
};

function withAuthHeaders(init?: RequestInit): HeadersInit {
  const token = getAuthToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  if (init?.headers) {
    const custom = init.headers as Record<string, string>;
    return { ...headers, ...custom };
  }
  return headers;
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers: withAuthHeaders(init),
    cache: "no-store",
  });
  if (!response.ok) {
    if (response.status === 401) {
      clearAuthToken();
    }
    const body = await response.text();
    throw new Error(body || response.statusText);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export async function apiWithoutAuth<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers || {}),
    },
    cache: "no-store",
  });
  if (!response.ok) {
    const body = await response.text();
    throw new Error(body || response.statusText);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export async function getAuthLoginUrl(): Promise<{ authorization_url: string }> {
  return apiWithoutAuth<{ authorization_url: string }>("/api/auth/github/login");
}

export async function getCurrentUser(): Promise<AuthUser> {
  return api<AuthUser>("/api/auth/me");
}

export function formatDate(value: string | null | undefined): string {
  if (!value) return "-";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export function statusTone(status: string): "success" | "warning" | "muted" | "destructive" | "default" {
  if (["active", "completed", "sent", "generated"].includes(status)) return "success";
  if (["running", "skipped", "filtered"].includes(status)) return "warning";
  if (["failed", "error"].includes(status)) return "destructive";
  if (["pending", "new"].includes(status)) return "muted";
  return "default";
}
