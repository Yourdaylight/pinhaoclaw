// API 基础配置和 uni.request 封装

const BASE_URL = (() => {
  // #ifdef H5
  return "";  // H5 通过 vite proxy 转发，使用相对路径
  // #endif
  // #ifdef MP-WEIXIN
  return "https://your-server.com";  // 小程序需要配置实际域名
  // #endif
})();

export function getBaseUrl() {
  return BASE_URL;
}

export interface ApiResponse<T = any> {
  ok?: boolean;
  data?: T;
  message?: string;
  error?: string;
}

function getToken(): string {
  return uni.getStorageSync("pc_user_token") || "";
}

function getAdminToken(): string {
  return uni.getStorageSync("pc_admin_token") || "";
}

function request<T = any>(
  url: string,
  method: "GET" | "POST" | "PUT" | "DELETE",
  data?: any,
  useAdminToken = false
): Promise<T> {
  const token = useAdminToken ? getAdminToken() : getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    if (useAdminToken) {
      headers["X-Admin-Token"] = token;
    } else {
      headers["X-User-Token"] = token;
    }
  }

  return new Promise((resolve, reject) => {
    uni.request({
      url: BASE_URL + url,
      method,
      data,
      header: headers,
      success: (res) => {
        if (res.statusCode === 401) {
          uni.removeStorageSync("pc_user_token");
          uni.reLaunch({ url: "/pages/login/index" });
          reject(new Error("登录已过期"));
          return;
        }
        if (res.statusCode >= 400) {
          const errData = res.data as any;
          reject(new Error(errData?.message || errData?.error || "请求失败"));
          return;
        }
        resolve(res.data as T);
      },
      fail: (err) => {
        console.error("Request failed:", err);
        reject(new Error("网络请求失败"));
      },
    });
  });
}

export const http = {
  get: <T = any>(url: string, useAdmin = false) =>
    request<T>(url, "GET", undefined, useAdmin),
  post: <T = any>(url: string, data?: any, useAdmin = false) =>
    request<T>(url, "POST", data, useAdmin),
  put: <T = any>(url: string, data?: any, useAdmin = false) =>
    request<T>(url, "PUT", data, useAdmin),
  del: <T = any>(url: string, useAdmin = false) =>
    request<T>(url, "DELETE", undefined, useAdmin),
};
