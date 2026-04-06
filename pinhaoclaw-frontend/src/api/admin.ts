import { http } from "./request";
import type { Lobster } from "./lobster";

export interface Node {
  id: string;
  name: string;
  host: string;
  ssh_port: number;
  ssh_user: string;
  ssh_password?: string;
  region: string;
  status: string;
  current_count: number;
  max_lobsters: number;
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

export const adminApi = {
  login: (password: string) =>
    http.post<{ ok: boolean; token: string; message?: string }>("/api/admin/login", { password }),

  overview: () => http.get<Overview>("/api/admin/overview", true),

  lobsters: () => http.get<Lobster[]>("/api/admin/lobsters", true),

  nodes: () => http.get<Node[]>("/api/admin/nodes", true),

  addNode: (node: Partial<Node>) =>
    http.post<{ ok: boolean; node: Node }>("/api/admin/nodes", node, true),

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
};
