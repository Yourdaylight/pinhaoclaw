import { http } from "./request";

export interface Lobster {
  id: string;
  user_id: string;
  name: string;
  node_id: string;
  node_name: string;
  region: string;
  port: number;
  status: "created" | "running" | "stopped" | "binding" | "error";
  weixin_bound: boolean;
  bound_at: string;
  created_at: string;
  monthly_token_limit: number;
  monthly_token_used: number;
  monthly_space_limit_mb: number;
  monthly_space_used_mb: number;
  quota_reset_month: string;
}

export interface CreateLobsterReq {
  name?: string;
  region?: string;
}

export interface SkillRequires {
  bins?: string[];
  env?: string[];
}

export interface SkillSource {
  type: string;
  repo?: string;
  clawhub?: string;
  local_dir?: string;
}

export interface SkillRegistryEntry {
  slug: string;
  display_name: string;
  summary: string;
  category: string;
  author: string;
  version: string;
  icon?: string;
  tags?: string[];
  requires?: SkillRequires;
  source: SkillSource;
  is_verified: boolean;
  created_at: string;
  updated_at: string;
}

export interface LobsterSkillInfo {
  slug: string;
  display_name: string;
  summary: string;
  icon?: string;
  version: string;
  installed_at?: string;
}

export const lobsterApi = {
  list: () => http.get<Lobster[]>("/api/lobsters"),

  get: (id: string) => http.get<Lobster>(`/api/lobsters/${id}`),

  create: (req: CreateLobsterReq) => http.post<{ ok: boolean; lobster: Lobster; message?: string }>("/api/lobsters", req),

  stop: (id: string) => http.post<{ ok: boolean; message: string }>(`/api/lobsters/${id}/stop`),

  start: (id: string) => http.post<{ ok: boolean; message: string }>(`/api/lobsters/${id}/start`),

  remove: (id: string) => http.del<{ ok: boolean; message: string }>(`/api/lobsters/${id}`),

  skillLibrary: () => http.get<{ skills: SkillRegistryEntry[] }>("/api/skills"),

  listSkills: (id: string) => http.get<{ skills: LobsterSkillInfo[] }>(`/api/lobsters/${id}/skills`),

  installSkill: (id: string, slug: string) =>
    http.post<{ ok: boolean; message: string }>(`/api/lobsters/${id}/skills`, { slug }),

  uninstallSkill: (id: string, slug: string) =>
    http.del<{ ok: boolean; message: string }>(`/api/lobsters/${id}/skills/${slug}`),

  // QR 码图片 URL（H5 端直接用 img src）
  qrcodeUrl: (url: string) => `/api/qrcode?url=${encodeURIComponent(url)}`,
};
