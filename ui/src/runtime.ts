import type { PluginHostApi } from "@kdlbs/plugin-sdk";

let activeHost: PluginHostApi | undefined;

/** Records the host the plugin was initialized with. */
export function setHost(host: PluginHostApi): void {
  activeHost = host;
}

/** The host for render-time helpers; throws before initialize. */
export function currentHost(): PluginHostApi {
  if (!activeHost) throw new Error("Azure DevOps plugin used before initialize");
  return activeHost;
}

type HostReact = PluginHostApi["React"];

export const useState: HostReact["useState"] = (initial) => currentHost().React.useState(initial);
export const useEffect: HostReact["useEffect"] = (effect, deps) => currentHost().React.useEffect(effect, deps);
export const useMemo: HostReact["useMemo"] = (factory, deps) => currentHost().React.useMemo(factory, deps);
export const useCallback: HostReact["useCallback"] = (callback, deps) => currentHost().React.useCallback(callback, deps);
export const useLayoutEffect: HostReact["useLayoutEffect"] = (effect, deps) => currentHost().React.useLayoutEffect(effect, deps);
export const useRef: HostReact["useRef"] = ((initial: unknown) => currentHost().React.useRef(initial)) as HostReact["useRef"];

/** Host UI components; resolved at render time. */
export function ui(): PluginHostApi["ui"] {
  return currentHost().ui;
}

/** Renderable content, as React would accept it. */
export type ReactNode = unknown;

export type SetStateAction<T> = T | ((previous: T) => T);
export type Dispatch<A> = (value: A) => void;

export function useResponsiveBreakpoint(): ReturnType<PluginHostApi["useResponsiveBreakpoint"]> {
  return currentHost().useResponsiveBreakpoint();
}
