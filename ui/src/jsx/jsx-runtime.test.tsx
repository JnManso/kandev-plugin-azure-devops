import { describe, expect, it, vi } from "vitest";
import type { PluginHostApi } from "@kdlbs/plugin-sdk";
import { setHost } from "../runtime";

describe("plugin JSX runtime", () => {
  it("renders elements through host.jsx with children and keys", () => {
    const jsx = vi.fn((type: unknown, props: unknown, ...children: unknown[]) => ({ type, props, children }));
    setHost({ jsx, React: { Fragment: "fragment" } } as unknown as PluginHostApi);
    const Item = (props: { label: string }) => props.label;
    const tree = (
      <div className="list">
        <Item key="a" label="A" />
        text
      </div>
    ) as { type: unknown; props: Record<string, unknown>; children: unknown[] };
    expect(tree.type).toBe("div");
    expect(tree.props).toEqual({ className: "list" });
    expect(tree.children).toHaveLength(2);
    expect(jsx).toHaveBeenCalledWith(Item, { label: "A", key: "a" });
  });
});
