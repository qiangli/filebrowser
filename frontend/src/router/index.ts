import type { RouteLocation } from "vue-router";
import { createRouter, createWebHistory } from "vue-router";
import Layout from "@/views/Layout.vue";
import Files from "@/views/Files.vue";
import Settings from "@/views/Settings.vue";
import ProfileSettings from "@/views/settings/Profile.vue";
import Errors from "@/views/Errors.vue";
import { baseURL, name } from "@/utils/constants";
import i18n from "@/i18n";
import { recaptcha } from "@/utils/constants";
import { login } from "@/utils/auth";

const titles = {
  Files: "files.files",
  Settings: "sidebar.settings",
  ProfileSettings: "settings.profileSettings",
  Forbidden: "errors.forbidden",
  NotFound: "errors.notFound",
  InternalServerError: "errors.internal",
};

const routes = [
  {
    path: "/files",
    component: Layout,
    children: [
      {
        path: ":path*",
        name: "Files",
        component: Files,
      },
    ],
  },
  {
    path: "/settings",
    component: Layout,
    children: [
      {
        path: "",
        name: "Settings",
        component: Settings,
        redirect: {
          path: "/settings/profile",
        },
        children: [
          {
            path: "profile",
            name: "ProfileSettings",
            component: ProfileSettings,
          },
          {
            // Global Settings is unsupported in the stateless single-user
            // embed — it manages server-wide state (users, rules, branding)
            // the embed doesn't keep, and the embed can't distinguish owner
            // from a shared viewer. Redirect any direct/bookmarked link to
            // Profile instead of rendering a broken page.
            path: "global",
            redirect: { path: "/settings/profile" },
          },
        ],
      },
    ],
  },
  {
    path: "/403",
    name: "Forbidden",
    component: Errors,
    props: {
      errorCode: 403,
      showHeader: true,
    },
  },
  {
    path: "/404",
    name: "NotFound",
    component: Errors,
    props: {
      errorCode: 404,
      showHeader: true,
    },
  },
  {
    path: "/500",
    name: "InternalServerError",
    component: Errors,
    props: {
      errorCode: 500,
      showHeader: true,
    },
  },
  {
    path: "/:catchAll(.*)*",
    redirect: (to: RouteLocation) => {
      const catchAll = to.params.catchAll;
      if (!catchAll) return "/files/";
      return `/files/${Array.isArray(catchAll) ? catchAll.join("/") : catchAll}`;
    },
  },
];

async function initAuth() {
  // Single-user local app: there is no login. Authenticate the lone user.
  await login("", "", "");

  if (recaptcha) {
    await new Promise<void>((resolve) => {
      const check = () => {
        if (typeof window.grecaptcha === "undefined") {
          setTimeout(check, 100);
        } else {
          resolve();
        }
      };

      check();
    });
  }
}

const router = createRouter({
  history: createWebHistory(baseURL),
  routes,
});

router.beforeResolve(async (to, from, next) => {
  const title = i18n.global.t(titles[to.name as keyof typeof titles]);
  document.title = title + " - " + name;

  // this will only be null on first route
  if (from.name == null) {
    try {
      await initAuth();
    } catch (error) {
      console.error(error);
    }
  }

  next();
});

export { router, router as default };
