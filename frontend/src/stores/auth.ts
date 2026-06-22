import { defineStore } from "pinia";
import { detectLocale, setLocale } from "@/i18n";
import { cloneDeep } from "lodash-es";
import { loadPrefs, savePref, type PrefKey } from "@/utils/prefs";

export const useAuthStore = defineStore("auth", {
  // convert to a function
  state: (): {
    user: IUser | null;
    jwt: string;
    logoutTimer: number | null;
  } => ({
    user: null,
    jwt: "",
    logoutTimer: null,
  }),
  getters: {
    // user and jwt getter removed, no longer needed
    isLoggedIn: (state) => state.user !== null,
  },
  actions: {
    // no context as first argument, use `this` instead
    setUser(user: IUser) {
      if (user === null) {
        this.user = null;
        return;
      }

      // Overlay per-device preferences (localStorage) onto the server-provided
      // user. The single-user embed has no per-user DB record, so the server
      // only supplies code-default prefs; the browser's saved choices win.
      const merged = { ...user, ...loadPrefs() } as IUser;
      setLocale(merged.locale || detectLocale());
      this.user = merged;
    },
    // setPref updates one UI preference: in memory (so the app reacts) and in
    // localStorage (so it survives a reload, per-device). Replaces the old
    // PUT /api/users persistence — the embed keeps no server-side user state.
    setPref(key: PrefKey, value: unknown) {
      savePref(key, value);
      this.user = { ...this.user, [key]: cloneDeep(value) } as IUser;
      if (key === "locale") {
        setLocale(value as string);
      }
    },
    updateUser(user: Partial<IUser>) {
      if (user.locale) {
        setLocale(user.locale);
      }

      this.user = { ...this.user, ...cloneDeep(user) } as IUser;
    },
    // easily reset state using `$reset`
    clearUser() {
      this.$reset();
    },
    setLogoutTimer(logoutTimer: number | null) {
      this.logoutTimer = logoutTimer;
    },
  },
});
