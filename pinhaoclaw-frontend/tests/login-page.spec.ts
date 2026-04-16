import { flushPromises, shallowMount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { configMock, loginMock, meMock, userStoreMock } = vi.hoisted(() => {
  const config = vi.fn();
  const login = vi.fn();
  const me = vi.fn();

  return {
    configMock: config,
    loginMock: login,
    meMock: me,
    userStoreMock: {
      isLoggedIn: false,
      login,
      fetchMe: me,
    },
  };
});

vi.mock("../src/api/auth", () => ({
  authApi: {
    config: configMock,
    login: vi.fn(),
    me: vi.fn(),
    logout: vi.fn(),
    regions: vi.fn(),
  },
}));

vi.mock("../src/stores/user", () => ({
  useUserStore: () => userStoreMock,
}));

import LoginPage from "../src/pages/login/index.vue";

function mountPage() {
  return shallowMount(LoginPage, {
    global: {
      stubs: {
        "el-card": { template: '<div><slot /><slot name="header" /></div>' },
        "el-alert": { template: '<div>{{ title }}<slot /></div>', props: ["title"] },
        "el-button": { template: '<button @click="$emit(\'click\')"><slot /></button>' },
        "el-form": { template: '<form @submit.prevent="$emit(\'submit\')"><slot /></form>' },
        "el-form-item": { template: '<div><slot /></div>' },
        "el-input": {
          template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keyup.enter="$emit(\'keyup.enter\')" />',
          props: ["modelValue", "size"],
        },
      },
    },
  });
}

describe("login page", () => {
  beforeEach(() => {
    userStoreMock.isLoggedIn = false;
    configMock.mockReset();
    loginMock.mockReset();
    meMock.mockReset();
    vi.restoreAllMocks();
  });

  it("redirects to panel when sidecar token exists", async () => {
    configMock.mockResolvedValue({
      mode: "sidecar",
      sidecar_enabled: true,
      login_url: "/api/auth/sidecar/login",
    });
    window.localStorage.setItem("casdoor_auth_token", "sidecar-token");
    meMock.mockResolvedValue(undefined);

    mountPage();
    await flushPromises();

    expect(meMock).toHaveBeenCalledTimes(1);
    expect((globalThis as any).uni.reLaunch).toHaveBeenCalledWith({
      url: "/pages/panel/index",
    });
  });

  it("appends prompt=login for forced re-login in sidecar mode", async () => {
    configMock.mockResolvedValue({
      mode: "sidecar",
      sidecar_enabled: true,
      login_url: "https://example.test/api/auth/sidecar/login",
    });
    (globalThis as any).uni.getStorageSync.mockImplementation((key: string) => {
      if (key === "force_relogin") {
        return "1";
      }
      return "";
    });

    const locationSpy = vi.spyOn(window, "location", "get");
    const locationState = { href: "http://localhost/" };
    locationSpy.mockReturnValue(locationState as Location);

    const wrapper = mountPage();
    await flushPromises();

    await wrapper.find("button").trigger("click");

    expect(locationState.href).toBe(
      "https://example.test/api/auth/sidecar/login?prompt=login"
    );
    expect((globalThis as any).uni.removeStorageSync).toHaveBeenCalledWith("force_relogin");
  });

  it("shows validation error when invite code is empty", async () => {
    configMock.mockResolvedValue({
      mode: "invite",
      sidecar_enabled: false,
    });

    const wrapper = mountPage();
    await flushPromises();

    await wrapper.find("button").trigger("click");

    expect(wrapper.text()).toContain("请输入邀请码");
    expect(loginMock).not.toHaveBeenCalled();
  });
});