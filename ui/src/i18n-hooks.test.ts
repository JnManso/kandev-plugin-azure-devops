import { describe, expect, it } from "vitest";
import type { PluginHostApi } from "@kdlbs/plugin-sdk";
import { usePluginTranslation, useT } from "./i18n";
import { setHost } from "./runtime";

type Slot = { deps: readonly unknown[]; value: unknown };

/** Memo hooks with React's identity rules, replayed across renders. */
function hookHarness(translate: (key: string) => string) {
  const slots: Slot[] = [];
  let index = 0;
  const memo = <T>(factory: () => T, deps: readonly unknown[]): T => {
    const slot = slots[index];
    if (slot && slot.deps.length === deps.length && slot.deps.every((dep, at) => Object.is(dep, deps[at]))) {
      index += 1;
      return slot.value as T;
    }
    const value = factory();
    slots[index] = { deps, value };
    index += 1;
    return value;
  };
  let current = translate;
  const host = {
    React: {
      useMemo: memo,
      useCallback: <T>(callback: T, deps: readonly unknown[]) => memo(() => callback, deps),
    },
    // The host returns a new object on every render, with a stable `t`.
    i18n: { useTranslation: () => ({ locale: "en", t: current }) },
  } as unknown as PluginHostApi;
  return {
    host,
    render<T>(hook: () => T): T {
      index = 0;
      return hook();
    },
    changeLocale(next: (key: string) => string) {
      current = next;
    },
  };
}

describe("translation hooks", () => {
  for (const [name, hook] of [
    ["useT", () => () => useT()],
    ["usePluginTranslation", (host: PluginHostApi) => () => usePluginTranslation(host)],
  ] as const) {
    it(`${name} keeps its identity until the locale changes`, () => {
      const harness = hookHarness((key) => `en:${key}`);
      setHost(harness.host);
      const first = harness.render(hook(harness.host));
      expect(harness.render(hook(harness.host))).toBe(first);
      expect(first("save")).toBe("en:save");

      harness.changeLocale((key) => `pt:${key}`);
      const next = harness.render(hook(harness.host));
      expect(next).not.toBe(first);
      expect(next("save")).toBe("pt:save");
    });
  }
});
