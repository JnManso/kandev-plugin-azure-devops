/** Host settings helpers the ported settings sections expect. */
import type { SettingsSaveContributor } from "@kdlbs/plugin-sdk";
import { currentHost } from "../runtime";
import {
  IconBug,
  IconChecks,
  IconCode,
  IconEye,
  IconMessage,
  IconSearch,
  IconSparkles,
  IconTool,
  type Icon,
} from "../icons";

export function useSettingsSaveContributor(contributor: SettingsSaveContributor): void {
  currentHost().useSettingsSaveContributor(contributor);
}

type ToastInput = { description?: string; title?: string; variant?: "success" | "error" | "default" };

export function useToast() {
  return {
    toast: ({ description, title, variant }: ToastInput) => {
      const message = description ?? title ?? "";
      if (variant === "error") currentHost().toast.error(message);
      else if (variant === "success") currentHost().toast.success(message);
      else currentHost().toast(message);
    },
  };
}

/** Kandev settings controls use host sizing; the plugin keeps the classes. */
export function settingsControlClassName(className = ""): string {
  return className;
}

export function settingsActionClassName(className = ""): string {
  return className;
}

/** A prompt placeholder offered by the editor. */
export type ScriptPlaceholder = { key: string; description: string; example?: string; executor_types?: string[] };

export function controlSizingClassName(_size: string, className = ""): string {
  return className;
}

/** Removes i18next <Trans> component markers such as <4>…</4>. */
export function stripTransMarkers(message: string): string {
  return message.replace(/<\/?\d+>/g, "");
}

export const ACTION_PRESET_ICON_CHOICES: Array<{ key: string; icon: Icon; labelKey: string }> = [
  { key: "eye", icon: IconEye, labelKey: "presetIconEye" },
  { key: "message", icon: IconMessage, labelKey: "presetIconMessage" },
  { key: "tool", icon: IconTool, labelKey: "presetIconTool" },
  { key: "code", icon: IconCode, labelKey: "presetIconCode" },
  { key: "search", icon: IconSearch, labelKey: "presetIconSearch" },
  { key: "bug", icon: IconBug, labelKey: "presetIconBug" },
  { key: "sparkle", icon: IconSparkles, labelKey: "presetIconSparkle" },
  { key: "check", icon: IconChecks, labelKey: "presetIconCheck" },
];

const ICONS = new Map(ACTION_PRESET_ICON_CHOICES.map((choice) => [choice.key, choice.icon]));

export function iconForActionPreset(key: string | undefined): Icon {
  return (key && ICONS.get(key)) || IconSparkles;
}
