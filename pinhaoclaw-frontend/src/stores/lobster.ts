import { defineStore } from "pinia";
import { ref } from "vue";
import { lobsterApi, type Lobster } from "../api/lobster";

export const useLobsterStore = defineStore("lobster", () => {
  const lobsters = ref<Lobster[]>([]);
  const loading = ref(false);

  async function fetchAll() {
    loading.value = true;
    try {
      lobsters.value = await lobsterApi.list();
    } finally {
      loading.value = false;
    }
  }

  async function create(name: string, region?: string) {
    const res = await lobsterApi.create({ name, region });
    if (!res.ok) throw new Error(res.message || "创建失败");
    await fetchAll();
    return res.lobster;
  }

  async function stop(id: string) {
    const res = await lobsterApi.stop(id);
    await fetchAll();
    return res.message;
  }

  async function start(id: string) {
    const res = await lobsterApi.start(id);
    await fetchAll();
    return res.message;
  }

  async function remove(id: string) {
    const res = await lobsterApi.remove(id);
    await fetchAll();
    return res.message;
  }

  return { lobsters, loading, fetchAll, create, stop, start, remove };
});
