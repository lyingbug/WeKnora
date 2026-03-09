import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import router from "./router";
import "./assets/fonts.css";
import TDesign from "tdesign-vue-next";
// 引入组件库的少量全局样式变量
import "tdesign-vue-next/es/style/index.css";
import "@/assets/theme/theme.css";
import i18n from "./i18n";
import { initTheme } from "@/composables/useTheme";

initTheme();

const app = createApp(App);

app.use(TDesign);
app.use(createPinia());
app.use(router);
app.use(i18n);

// Suppress TDesign textarea autosize error when DOM element is unmounted
// before the async height calculation fires (known TDesign issue).
window.addEventListener('unhandledrejection', (event) => {
  if (
    event.reason instanceof TypeError &&
    event.reason.message?.includes('getComputedStyle')
  ) {
    event.preventDefault();
  }
});

app.mount("#app");
