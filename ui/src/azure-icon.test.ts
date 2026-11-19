import { describe, expect, it } from "vitest";
import type { PluginHostApi } from "@kdlbs/plugin-sdk";
import { createAzureDevOpsIcon } from "./azure-icon";

type Element = { type: string; props: Record<string, unknown>; children: Element[] };

const host = {
  jsx: (type: string, props: Record<string, unknown>, ...children: Element[]) => ({ type, props, children }),
} as unknown as PluginHostApi;

describe("createAzureDevOpsIcon", () => {
  it("uses the surrounding text color, like the host's navigation icons", () => {
    const icon = createAzureDevOpsIcon(host)({ className: "h-4 w-4" }) as unknown as Element;
    const paths = icon.children.filter((child) => child.type === "path");
    expect(paths.length).toBeGreaterThan(0);
    for (const path of paths) expect(path.props.fill).toBe("currentColor");
    expect(icon.props.className).toBe("h-4 w-4");
  });
});
