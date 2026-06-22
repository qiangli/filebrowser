<template>
  <div class="dashboard">
    <header-bar showMenu showLogo />

    <div id="nav">
      <div class="wrapper">
        <ul>
          <router-link to="/settings/profile"
            ><li :class="{ active: $route.path === '/settings/profile' }">
              {{ t("settings.profileSettings") }}
            </li></router-link
          >
          <!-- Global Settings is hidden in the embed: it manages server-wide
               state (users, rules, branding) that a stateless single-user
               File Browser does not persist. -->
        </ul>
      </div>
    </div>

    <div v-if="loading">
      <h2 class="message delayed">
        <div class="spinner">
          <div class="bounce1"></div>
          <div class="bounce2"></div>
          <div class="bounce3"></div>
        </div>
        <span>{{ t("files.loading") }}</span>
      </h2>
    </div>

    <router-view></router-view>
  </div>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/stores/layout";
import HeaderBar from "@/components/header/HeaderBar.vue";
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

const layoutStore = useLayoutStore();

const loading = computed(() => layoutStore.loading);
</script>
