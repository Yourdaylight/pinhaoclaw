import { beforeEach, vi } from "vitest";

type StorageMap = Map<string, string>;

const storage: StorageMap = new Map();

const uniMock = {
  getStorageSync: vi.fn((key: string) => storage.get(key) ?? ""),
  setStorageSync: vi.fn((key: string, value: unknown) => {
    storage.set(key, String(value));
  }),
  removeStorageSync: vi.fn((key: string) => {
    storage.delete(key);
  }),
  reLaunch: vi.fn(),
  showLoading: vi.fn(),
  hideLoading: vi.fn(),
  request: vi.fn(),
};

Object.defineProperty(globalThis, "uni", {
  value: uniMock,
  writable: true,
});

beforeEach(() => {
  storage.clear();
  window.localStorage.clear();
  uniMock.getStorageSync.mockClear();
  uniMock.setStorageSync.mockClear();
  uniMock.removeStorageSync.mockClear();
  uniMock.reLaunch.mockClear();
  uniMock.showLoading.mockClear();
  uniMock.hideLoading.mockClear();
  uniMock.request.mockClear();
});