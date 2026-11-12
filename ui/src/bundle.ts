import type { KandevPlugin } from "@kdlbs/plugin-sdk";
import { PLUGIN_ID, createAzureDevOpsPlugin } from "./plugin";

declare global {
  interface Window {
    registerKandevPlugin(id: string, plugin: KandevPlugin): void;
  }
}

window.registerKandevPlugin(PLUGIN_ID, createAzureDevOpsPlugin());
