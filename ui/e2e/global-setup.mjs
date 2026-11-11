import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const pluginRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

/** Installs the packaged plugin into the disposable host named by KANDEV_PLUGIN_E2E_URL. */
export default async function installPackagedPlugin() {
  const baseURL = process.env.KANDEV_PLUGIN_E2E_URL?.replace(/\/$/, "");
  if (!baseURL) {
    throw new Error("KANDEV_PLUGIN_E2E_URL is required; run npm run e2e, which starts a disposable host.");
  }
  const manifest = await readFile(path.join(pluginRoot, "manifest.yaml"), "utf8");
  const id = manifest.match(/^id: "([^"]+)"$/m)?.[1];
  const version = manifest.match(/^version: "([^"]+)"$/m)?.[1];
  if (!id || !version) throw new Error("manifest.yaml must declare id and version");
  const packagePath = process.env.KANDEV_PLUGIN_E2E_PACKAGE ?? path.join(pluginRoot, `${id}-${version}.tar.gz`);
  const archive = await readFile(packagePath);
  const form = new FormData();
  form.set("package", new Blob([archive], { type: "application/gzip" }), path.basename(packagePath));
  const response = await fetch(`${baseURL}/api/plugins/install`, { method: "POST", body: form, signal: AbortSignal.timeout(60_000) });
  if (!response.ok) throw new Error(`Install failed (${response.status}): ${await response.text()}`);
  const installed = await response.json();
  if (installed?.plugin?.id !== id || installed?.plugin?.status !== "active") {
    throw new Error(`Packaged plugin did not become active: ${JSON.stringify(installed)}`);
  }
}
