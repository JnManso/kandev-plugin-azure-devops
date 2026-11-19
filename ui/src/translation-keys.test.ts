import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import english from "./locales/en.json";

function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((name) => {
    const path = join(directory, name);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name) ? [path] : [];
  });
}

const KEY_PATTERN = /\bt\(\s*["']([A-Za-z0-9_.]+)["']/g;

describe("translation keys", () => {
  it("every literal t() key used in the UI exists in the English catalog", () => {
    const missing: string[] = [];
    for (const file of sources(join(__dirname))) {
      for (const match of readFileSync(file, "utf8").matchAll(KEY_PATTERN)) {
        const key = match[1];
        const plural = `${key}_one` in english && `${key}_other` in english;
        if (!(key in english) && !plural) missing.push(`${file.replace(__dirname, "")}: ${key}`);
      }
    }
    expect(missing).toEqual([]);
  });
});
