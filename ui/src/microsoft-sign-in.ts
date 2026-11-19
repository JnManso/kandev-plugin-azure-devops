import type { PluginHostApi } from "@kdlbs/plugin-sdk";
import {
  type ConnectionApi,
  type ConnectionForm,
  type ConnectionView,
  errorMessage,
  parseOAuthRedirect,
} from "./connection-model";
import type { Translate } from "./i18n";

/** OAuth error Microsoft returns when the user declines. */
const ACCESS_DENIED = "access_denied";
export const POPUP_WATCH_INTERVAL_MS = 250;

export type RedirectResult = { code: string; state: string; error: string };

/**
 * Reads the authorization result once the popup is back on Kandev's origin.
 * While the popup shows Microsoft's pages, reading its location throws.
 */
export function readSameOriginRedirect(popup: Window, origin: string): RedirectResult | null {
  try {
    if (popup.location.origin !== origin) return null;
    const params = new URLSearchParams(popup.location.search);
    const state = params.get("state");
    if (!state) return null;
    return { code: params.get("code") ?? "", state, error: params.get("error") ?? "" };
  } catch {
    return null;
  }
}

export type MicrosoftSignIn = {
  connecting: boolean;
  completing: boolean;
  pasteOpen: boolean;
  pasteUrl: string;
  setPasteUrl(value: string): void;
  connect(): void;
  completeFromPaste(): void;
  cancel(): void;
};

type Options = {
  host: PluginHostApi;
  api: ConnectionApi;
  t: Translate;
  form: ConnectionForm;
  connection: ConnectionView | null;
  onConnected(connection: ConnectionView): void;
};

/**
 * Microsoft sign-in in a popup. The redirect URL is this page's origin; the
 * popup watch completes sign-in when it returns there, and the paste panel
 * covers a popup that cannot return.
 */
export function useMicrosoftSignIn({ host, api, t, form, connection, onConnected }: Options): MicrosoftSignIn {
  const React = host.React;
  const [connecting, setConnecting] = React.useState(false);
  const [completing, setCompleting] = React.useState(false);
  const [pasteOpen, setPasteOpen] = React.useState(false);
  const [pasteUrl, setPasteUrl] = React.useState("");
  const popupRef = React.useRef<Window | null>(null);
  const pendingStateRef = React.useRef<string | null>(null);
  const completingRef = React.useRef(false);
  const latest = React.useRef({ form, connection, onConnected });
  latest.current = { form, connection, onConnected };

  const reset = React.useCallback(() => {
    popupRef.current?.close();
    popupRef.current = null;
    pendingStateRef.current = null;
    setPasteOpen(false);
    setPasteUrl("");
  }, []);

  const fail = React.useCallback(
    (error: string) => host.toast.error(t("oauthFailed", { values: { error } })),
    [t],
  );

  const complete = React.useCallback(
    async (code: string, state: string) => {
      if (completingRef.current) return;
      completingRef.current = true;
      pendingStateRef.current = null;
      setCompleting(true);
      try {
        const connected = await api.completeOAuth(code, state);
        reset();
        host.toast.success(t("oauthConnected"));
        latest.current.onConnected(connected);
      } catch (error) {
        fail(errorMessage(error));
      } finally {
        completingRef.current = false;
        setCompleting(false);
      }
    },
    [api, fail, reset, t],
  );

  const handleResult = React.useCallback(
    (result: RedirectResult) => {
      pendingStateRef.current = null;
      if (!result.error) {
        void complete(result.code, result.state);
        return;
      }
      reset();
      if (result.error === ACCESS_DENIED) host.toast.error(t("oauthCancelled"));
      else fail(result.error);
    },
    [complete, fail, reset, t],
  );

  React.useEffect(() => {
    if (!pasteOpen) return;
    const interval = window.setInterval(() => {
      const popup = popupRef.current;
      if (!popup || popup.closed) {
        window.clearInterval(interval);
        return;
      }
      const result = readSameOriginRedirect(popup, window.location.origin);
      if (result && result.state === pendingStateRef.current) handleResult(result);
    }, POPUP_WATCH_INTERVAL_MS);
    return () => window.clearInterval(interval);
  }, [pasteOpen, handleResult]);

  React.useEffect(() => () => popupRef.current?.close(), []);

  const connect = React.useCallback(async () => {
    if (completingRef.current) return;
    // Open synchronously in the click handler so pop-up blockers allow it.
    const popup = window.open("", "_blank");
    if (!popup) {
      host.toast.error(t("oauthPopupBlocked"));
      return;
    }
    popupRef.current = popup;
    setConnecting(true);
    try {
      const { form: currentForm, connection: currentConnection } = latest.current;
      const { authUrl, state } = await api.startOAuth(currentForm, currentConnection, window.location.origin);
      pendingStateRef.current = state;
      popup.location.assign(authUrl);
      setPasteOpen(true);
    } catch (error) {
      reset();
      fail(errorMessage(error));
    } finally {
      setConnecting(false);
    }
  }, [api, fail, reset, t]);

  const completeFromPaste = React.useCallback(() => {
    const redirect = parseOAuthRedirect(pasteUrl);
    if (!redirect) {
      host.toast.error(t("oauthPasteInvalid"));
      return;
    }
    void complete(redirect.code, redirect.state);
  }, [complete, pasteUrl, t]);

  return {
    connecting,
    completing,
    pasteOpen,
    pasteUrl,
    setPasteUrl,
    connect: () => void connect(),
    completeFromPaste,
    cancel: reset,
  };
}
