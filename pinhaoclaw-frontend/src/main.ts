import { createSSRApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";

// #ifdef H5
import ElementPlus from "element-plus";
import "element-plus/dist/index.css";
// #endif

export function createApp() {
  const app = createSSRApp(App);
  const pinia = createPinia();
  app.use(pinia);

  // #ifdef H5
  app.use(ElementPlus);
  // #endif

  return { app, Pinia: pinia };
}
