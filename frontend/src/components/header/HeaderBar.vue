<template>
  <header>
    <img v-if="showLogo" :src="logoURL" />
    <Action
      v-if="showMenu"
      class="menu-button"
      icon="menu"
      :label="t('buttons.toggleSidebar')"
      @action="layoutStore.showHover('sidebar')"
    />

    <slot />

    <div
      id="dropdown"
      :class="{ active: layoutStore.currentPromptName === 'more' }"
    >
      <slot name="actions" />
    </div>

    <Action
      v-if="ifActionsSlot"
      id="more"
      icon="more_vert"
      :label="t('buttons.more')"
      @action="layoutStore.showHover('more')"
    />

    <div
      class="overlay"
      v-show="layoutStore.currentPromptName == 'more'"
      @click="layoutStore.closeHovers"
    />

    <a
      v-if="showApps"
      class="action apps-button"
      :href="appsHref"
      :aria-label="t('buttons.apps')"
      :title="t('buttons.apps')"
    >
      <!-- The console's own all-apps mark, four rounded squares. Copied in
           shape (not imported) because this SPA is MOUNTED rather than served
           through the console's chrome injection, so it never receives
           #all-apps-btn and has to carry the same mark itself. -->
      <svg
        class="apps-mark"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.9"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <rect x="3" y="3" width="7" height="7" rx="1.5" />
        <rect x="14" y="3" width="7" height="7" rx="1.5" />
        <rect x="3" y="14" width="7" height="7" rx="1.5" />
        <rect x="14" y="14" width="7" height="7" rx="1.5" />
      </svg>
    </a>
  </header>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/stores/layout";

import { baseURL, logoURL } from "@/utils/constants";

import Action from "@/components/header/Action.vue";
import { computed, useSlots } from "vue";
import { useI18n } from "vue-i18n";

defineProps<{
  showLogo?: boolean;
  showMenu?: boolean;
  showApps?: boolean;
}>();

const layoutStore = useLayoutStore();
const slots = useSlots();

// The launcher is the PARENT of this SPA's mount, not the parent of whatever
// route happens to be open. A bare parent-relative link, resolved against a
// route such as /files/files/<path>, lands back on the files app itself --
// which is exactly what it did. baseURL is the mount prefix the console gave
// us, so strip its last segment instead of guessing from the URL.
const appsHref = computed(() => {
  const base = (baseURL || "/").replace(/\/+$/, "");
  const parent = base.slice(0, base.lastIndexOf("/") + 1);
  return parent || "/";
});

const { t } = useI18n();

const ifActionsSlot = computed(() => (slots.actions ? true : false));
</script>

<style></style>
