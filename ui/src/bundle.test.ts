import { afterEach, describe, expect, it, vi } from "vitest";
import { PLUGIN_ID } from "./plugin";

describe("bundle entry", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it("registers the plugin under the manifest id", async () => {
    const registerKandevPlugin = vi.fn();
    vi.stubGlobal("window", { registerKandevPlugin });

    await import("./bundle");

    expect(registerKandevPlugin).toHaveBeenCalledTimes(1);
    expect(registerKandevPlugin).toHaveBeenCalledWith(
      "kandev-plugin-azure-devops",
      expect.objectContaining({ initialize: expect.any(Function) }),
    );
    expect(PLUGIN_ID).toBe("kandev-plugin-azure-devops");
  });
});
