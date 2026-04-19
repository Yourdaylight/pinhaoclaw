import { http } from "./request";
import type { Lobster } from "./lobster";

export interface Node {
  id: string;
  type?: "ssh" | "local";
  name: string;
  host: string;
  ssh_port: number;
  ssh_user: string;
  ssh_auth_type?: "password" | "key_path" | "private_key";
  ssh_key_path?: string;
  ssh_private_key?: string;
  ssh_certificate_path?: string;
  ssh_key_passphrase?: string;
  ssh_password?: string;
  region: string;
  status: string;
  current_count: number;
  max_lobsters: number;
  remote_home?: string;
  picoclaw_path?: string;
  created_at: string;
}

export interface Invite {
  code: string;
  created_by: string;
  created_at: string;
  max_uses: number;
  used_count: number;
  used_by: string[];
}

export interface Settings {
  default_monthly_token_limit: number;
  default_monthly_space_limit_mb: number;
  default_max_lobsters_per_user: number;
}

export interface Overview {
  total_users: number;
  total_lobsters: number;
  running_lobsters: number;
  total_nodes: number;
}

export interface PicoclawPackageInfo {
  configured_path: string;
  resolved_path: string;
  version: string;
  managed_path: string;
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

export interface SkillUploadPayload {
  slug?: string;
  display_name?: string;
  summary?: string;
  category?: string;
  author?: string;
  version?: string;
  icon?: string;
  tags?: string;
  requires_bins?: string;
  requires_env?: string;
  is_verified?: boolean;
}

async function uploadAdminSkillZip(file: File, payload: SkillUploadPayload) {
  const formData = new FormData();
  formData.append("file", file);
  Object.entries(payload).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") return;
    formData.append(key, typeof value === "boolean" ? String(value) : value);
  });

  const res = await fetch("/api/admin/skills/upload", {
    method: "POST",
    headers: {
      "X-Admin-Token": uni.getStorageSync("pc_admin_token") || "",
    },
    body: formData,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data?.message || data?.error || "上传 Skill 失败");
  }
  return data as { ok: boolean; skill: SkillRegistryEntry };
}

export const adminApi = {
  login: (password: string) =>
    http.post<{ ok: boolean; token: string; message?: string }>("/api/admin/login", { password }),

  overview: () => http.get<Overview>("/api/admin/overview", true),

  lobsters: () => http.get<Lobster[]>("/api/admin/lobsters", true),

  nodes: () => http.get<Node[]>("/api/admin/nodes", true),

  addNode: (node: Partial<Node>) =>
    http.post<{ ok: boolean; node: Node }>("/api/admin/nodes", node, true),

  updateNode: (id: string, node: Partial<Node>) =>
    http.put<{ ok: boolean; node: Node }>(`/api/admin/nodes/${id}`, node, true),

  deleteNode: (id: string) => http.del(`/api/admin/nodes/${id}`, true),

  testNode: (id: string) =>
    http.post<{ ok: boolean; message: string }>(`/api/admin/nodes/${id}/test`, {}, true),

  invites: () => http.get<Record<string, Invite>>("/api/admin/invites", true),

  createInvite: () =>
    http.post<{ ok: boolean; code: string; url: string }>("/api/admin/invites", {}, true),

  deleteInvite: (code: string) => http.del(`/api/admin/invites/${code}`, true),

  settings: () => http.get<Settings>("/api/admin/settings", true),

  updateSettings: (s: Partial<Settings>) =>
    http.put<Settings>("/api/admin/settings", s, true),

  picoclawPackage: () => http.get<PicoclawPackageInfo>("/api/admin/picoclaw/package", true),

  setPicoclawPackage: (path: string) =>
    http.put<{ ok: boolean; package: PicoclawPackageInfo }>("/api/admin/picoclaw/package", { path }, true),

  fetchLatestPicoclawPackage: () =>
    http.post<{ ok: boolean; package: PicoclawPackageInfo }>("/api/admin/picoclaw/package/fetch-latest", {}, true),

  skills: () => http.get<{ skills: SkillRegistryEntry[] }>("/api/admin/skills", true),

  deleteSkill: (slug: string) => http.del<{ ok: boolean }>(`/api/admin/skills/${slug}`, true),

  uploadSkillZip: (file: File, payload: SkillUploadPayload) => uploadAdminSkillZip(file, payload),
};
