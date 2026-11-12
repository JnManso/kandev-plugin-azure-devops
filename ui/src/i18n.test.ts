import { describe, expect, it, vi } from "vitest";
import type { PluginRegistry } from "@kdlbs/plugin-sdk";
import { registerTranslations, translationCatalogs } from "./i18n";
import verbatim from "./locales/verbatim.json";

const english = translationCatalogs.en;
const placeholders = (message: string) => (message.match(/\{\{[^}]+\}\}/g) ?? []).sort();
const declaredVerbatim = verbatim as Record<string, Record<string, string>>;

describe("translation catalogs", () => {
  it("cover every Kandev locale plus pseudo", () => {
    expect(Object.keys(translationCatalogs).sort()).toEqual(
      ["en", "ja", "ko", "pseudo", "pt-pt", "zh-cn", "zh-hk", "zh-tw"].sort(),
    );
  });

  for (const [locale, catalog] of Object.entries(translationCatalogs)) {
    it(`${locale} has exactly the English keys and placeholders`, () => {
      expect(Object.keys(catalog).sort()).toEqual(Object.keys(english).sort());
      for (const [key, message] of Object.entries(english)) {
        expect(placeholders(catalog[key as keyof typeof english]), `${locale}.${key}`).toEqual(placeholders(message));
      }
    });

    it(`${locale} uses no em dash`, () => {
      for (const message of Object.values(catalog)) expect(message).not.toContain("—");
    });
  }

  for (const locale of ["pt-pt", "zh-cn", "zh-hk", "zh-tw", "ja", "ko"]) {
    it(`${locale} translates every non-brand message`, () => {
      for (const [key, message] of Object.entries(english)) {
        if (declaredVerbatim[locale]?.[key]) continue;
        expect(translationCatalogs[locale][key as keyof typeof english], `${locale}.${key}`).not.toBe(message);
      }
    });
  }

  it("registers the catalogs with an English fallback", () => {
    const registry = { registerTranslations: vi.fn() } as unknown as PluginRegistry;
    registerTranslations(registry);
    expect(registry.registerTranslations).toHaveBeenCalledWith(expect.objectContaining({ en: english }));
  });
});
