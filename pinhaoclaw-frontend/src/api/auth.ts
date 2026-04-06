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
  };
}

export const authApi = {
  login: (inviteCode: string, name?: string) =>
    http.post<LoginResponse>("/api/auth/login", { invite_code: inviteCode, name }),

  me: () => http.get<MeResponse>("/api/auth/me"),

  regions: () => http.get<{ regions: string[] }>("/api/regions"),
};
