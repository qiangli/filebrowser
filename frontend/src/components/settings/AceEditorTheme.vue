<template>
  <select
    name="selectAceEditorTheme"
    v-on:change="change"
    :value="aceEditorTheme"
  >
    <option v-for="theme in themes" :value="theme.theme" :key="theme.theme">
      {{ theme.name }}
    </option>
  </select>
</template>

<script setup lang="ts">
import { type SelectHTMLAttributes } from "vue";
// ext-themelist is an ace extension that references the global `ace`, so the
// ace-builds core (which defines it) MUST be imported first. Without this the
// extension can evaluate before core depending on chunk order and throw
// "ace is not defined", breaking app bootstrap.
import "ace-builds";
import { themes } from "ace-builds/src-noconflict/ext-themelist";

defineProps<{
  aceEditorTheme: string;
}>();

const emit = defineEmits<{
  (e: "update:aceEditorTheme", val: string | null): void;
}>();

const change = (event: Event) => {
  emit("update:aceEditorTheme", (event.target as SelectHTMLAttributes)?.value);
};
</script>
