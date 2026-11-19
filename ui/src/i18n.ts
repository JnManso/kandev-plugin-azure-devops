import type {
  PluginHostApi,
  PluginRegistry,
  PluginTranslationOptions,
} from "@kdlbs/plugin-sdk";
import english from "./locales/en.json";
import japanese from "./locales/ja.json";
import korean from "./locales/ko.json";
import portuguese from "./locales/pt-pt.json";
import simplifiedChinese from "./locales/zh-cn.json";
import traditionalChineseHongKong from "./locales/zh-hk.json";
import traditionalChineseTaiwan from "./locales/zh-tw.json";
import { currentHost } from "./runtime";

export type TranslationKey = keyof typeof english;
type Catalog = Record<TranslationKey, string>;

const pseudoGlyphs: Readonly<Record<string, string>> = {
  a: "á", b: "ƀ", c: "ç", d: "ď", e: "é", f: "ƒ", g: "ĝ", h: "ĥ", i: "í",
  j: "ĵ", k: "ķ", l: "ĺ", m: "ɱ", n: "ñ", o: "ó", p: "þ", q: "ǫ", r: "ŕ",
  s: "š", t: "ţ", u: "ú", v: "ṽ", w: "ŵ", x: "ẋ", y: "ý", z: "ž",
};

export function pseudoMessage(message: string): string {
  const transformed = message
    .split(/(\{\{[^}]+\}\})/g)
    .map((part) =>
      part.startsWith("{{")
        ? part
        : part.replace(/[A-Za-z]/g, (letter) => {
            const glyph = pseudoGlyphs[letter.toLowerCase()] ?? letter;
            return letter === letter.toUpperCase() ? glyph.toUpperCase() : glyph;
          }),
    )
    .join("");
  return `[${transformed}${"~".repeat(Math.ceil(message.length * 0.3))}]`;
}

const pseudo = Object.fromEntries(
  Object.entries(english).map(([key, message]) => [key, pseudoMessage(message)]),
) as Catalog;

export const translationCatalogs: Readonly<Record<string, Catalog>> = {
  en: english,
  "pt-pt": portuguese,
  "zh-cn": simplifiedChinese,
  "zh-hk": traditionalChineseHongKong,
  "zh-tw": traditionalChineseTaiwan,
  ja: japanese,
  ko: korean,
  pseudo,
};

export type Translate = (key: TranslationKey, options?: PluginTranslationOptions) => string;

export function registerTranslations(registry: PluginRegistry): void {
  registry.registerTranslations(translationCatalogs);
}

/**
 * Translation for render paths; re-renders when the locale changes. The
 * function keeps its identity until then, so effects may depend on it.
 */
export function usePluginTranslation(host: PluginHostApi): Translate {
  const translate = host.i18n.useTranslation().t;
  return host.React.useCallback<Translate>((key, options) => translate(key, options), [translate]);
}

/** Translation outside render, resolved at call time. */
export function pluginTranslate(host: PluginHostApi): Translate {
  return (key, options) => host.i18n.t(key, options);
}

/**
 * i18next-style translation for ported components: `t(key, { name, count })`.
 * Values other than `count` and `defaultValue` become interpolation values.
 * Keys are checked against the English catalog by a unit test.
 */
export type TFunction = (key: string, options?: Record<string, unknown>) => string;

export function toTranslationOptions(options?: Record<string, unknown>): PluginTranslationOptions | undefined {
  if (!options) return undefined;
  const { count, defaultValue, ...values } = options;
  const result: { count?: number; defaultValue?: string; values?: Record<string, string | number> } = {};
  if (typeof count === "number") result.count = count;
  if (typeof defaultValue === "string") result.defaultValue = defaultValue;
  const interpolation: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(values)) {
    if (typeof value === "string" || typeof value === "number") interpolation[key] = value;
    else if (value !== undefined && value !== null) interpolation[key] = String(value);
  }
  if (typeof count === "number") interpolation.count = count;
  if (Object.keys(interpolation).length > 0) result.values = interpolation;
  return result;
}

/**
 * Translation hook for ported components; uses the initialized host. Stable
 * until the locale changes, like the host's own translation function.
 */
export function useT(): TFunction {
  const host = currentHost();
  const translate = host.i18n.useTranslation().t;
  return host.React.useCallback<TFunction>(
    (key, options) => translate(key, toTranslationOptions(options)),
    [translate],
  );
}
