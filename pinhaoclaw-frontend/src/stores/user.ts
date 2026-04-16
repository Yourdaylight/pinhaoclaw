import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { authApi } from "../api/auth";

export const useUserStore = defineStore("user", () => {
  const token = ref<string>(
    uni.getStorageSync("casdoor_auth_token") ||
    uni.getStorageSync("pc_user_token") || ""
  );
  const userId = ref<string>(uni.getStorageSync("pc_user_id") || "");
  const userName = ref<string>(uni.getStorageSync("pc_user_name") || "");
  const maxLobsters = ref<number>(Number(uni.getStorageSync("pc_max_lobsters")) || 3);
  const lobsterCount = ref<number>(0);

  const isLoggedIn = computed(() => !!token.value);

  async function login(inviteCode: string, name?: string) {
    const res = await authApi.login(inviteCode, name);
    if (!res.ok) throw new Error(res.message || "登录失败");

    token.value = res.token;
    userId.value = res.user.id;
    userName.value = res.user.name;
    maxLobsters.value = res.user.max_lobsters;

    uni.setStorageSync("pc_user_token", res.token);
    uni.setStorageSync("pc_user_id", res.user.id);
    uni.setStorageSync("pc_user_name", res.user.name);
    uni.setStorageSync("pc_max_lobsters", res.user.max_lobsters);
  }

  async function fetchMe() {
    const res = await authApi.me();
    userName.value = res.user.name;
    maxLobsters.value = res.user.max_lobsters;
    lobsterCount.value = res.user.lobster_count;
    uni.setStorageSync("pc_user_name", res.user.name);
  }

  async function logout() {
    // Grab token before clearing
    const currentToken = token.value;

    const hadSidecarToken = !!uni.getStorageSync("casdoor_auth_token");

    if (currentToken && hadSidecarToken) {
      try {
        await authApi.logout();
      } catch {
        // Force re-login locally even if sidecar cleanup fails.
      }
    }

    // Clear local state
    token.value = "";
    userId.value = "";
    userName.value = "";
    maxLobsters.value = 3;
    uni.removeStorageSync("pc_user_token");
    uni.removeStorageSync("pc_user_id");
    uni.removeStorageSync("pc_user_name");
    uni.removeStorageSync("pc_max_lobsters");
    uni.removeStorageSync("casdoor_auth_token");
    // Mark that user explicitly logged out (next login should force re-auth)
    uni.setStorageSync("force_relogin", "1");

    uni.reLaunch({ url: "/pages/login/index" });
  }

  return {
    token,
    userId,
    userName,
    maxLobsters,
    lobsterCount,
    isLoggedIn,
    login,
    fetchMe,
    logout,
  };
});
