import { http } from "./request";

export interface LoginResponse {
  ok: boolean;
  token: string;
  message?: string;
  user: {
    id: string;
    name: string;
    max_lobsters: number;
  };
}

export interface MeResponse {
  user: {
    id: string;
    name: string;
    max_lobsters: number;
    created_at: string;
    lobster_count: number;
    auth_source?: string;
    organization?: string;
    email?: string;
  };
}

export interface AuthConfigResponse {
  mode: "invite" | "sidecar";
  sidecar_enabled: boolean;
  login_url?: string;
  casdoor_logout_url?: string;
  organization?: string;
}

export const authApi = {
  config: () => http.get<AuthConfigResponse>("/api/auth/config"),

  login: (inviteCode: string, name?: string) =>
    http.post<LoginResponse>("/api/auth/login", { invite_code: inviteCode, name }),

  me: () => http.get<MeResponse>("/api/auth/me"),

  logout: () => http.post<{ ok: boolean }>("/api/auth/sidecar/logout", {}),

  regions: () => http.get<{ regions: string[] }>("/api/regions"),
};
