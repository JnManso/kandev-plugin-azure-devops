import type { Component, PluginHostApi, PluginIconProps } from "@kdlbs/plugin-sdk";

/** Path of the official Azure DevOps service icon (assets/icon.svg), 18x18.
 * The UI draws it in the surrounding text color, like the host navigation
 * icons; the full-color mark stays the marketplace icon. */
export const AZURE_DEVOPS_ICON_PATH =
  "M17,4v9.74l-4,3.28-6.2-2.26V17L3.29,12.41l10.23.8V4.44Zm-3.41.49L7.85,1V3.29L2.58,4.84,1,6.87v4.61l2.26,1V6.57Z";

export function createAzureDevOpsIcon(host: PluginHostApi): Component<PluginIconProps> {
  return function AzureDevOpsIcon(props: PluginIconProps = {}) {
    return host.jsx(
      "svg",
      {
        viewBox: "0 0 18 18",
        "aria-hidden": "true",
        focusable: "false",
        className: props.className,
        "data-testid": "azure-devops-icon",
      },
      host.jsx("path", { d: AZURE_DEVOPS_ICON_PATH, fill: "currentColor" }),
    );
  };
}
