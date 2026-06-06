import { createApp } from "vue";
import { createRouter, createWebHistory, RouteRecordRaw } from "vue-router";
import App from "./App.vue";
import "./main.tailwind.css";

import HomeView from "./views/HomeView.vue";
import SessionView from "./views/SessionView.vue";
import BrowseView from "./views/BrowseView.vue";
import AuthView from "./views/AuthView.vue";

const routes: RouteRecordRaw[] = [
  { path: "/", component: HomeView },
  {
    name: "session",
    path: "/session/:id",
    component: SessionView,
    props: true,
  },
  {
    name: "browse",
    path: "/browse",
    component: BrowseView,
  },
  {
    name: "auth",
    path: "/auth",
    component: AuthView,
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

const app = createApp(App);
app.use(router);
app.mount("#app");
