import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  ReactNode,
} from "react";
import { Platform } from "react-native";
import * as SecureStore from "expo-secure-store";
import {
  AuthContextValue,
  AuthStatus,
  User,
  TokenPair,
  AuthError,
  RegisterInput} from "./types";
import {
  KratosLoginFlow,
  KratosSuccessfulNativeLogin,
  KratosErrorResponse,
  KratosRegistrationFlow,
  KratosSuccessfulNativeRegistration,
  KratosFormError,
} from "./kratosTypes";

const KRATOS_BASE_URL = "https://auth.cvera.app";
const API_BASE_URL = "https://api.cvera.app";
const REFRESH_TOKEN_KEY = "cvera_refresh_token_v1";

/**
 * Session-expired reason codes (kept local to AuthContext to avoid churn in ./types).
 */
type SessionExpiredReason =
  | "refresh_unauthorized"
  | "api_unauthorized"
  | "api_forbidden";

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

interface Props {
  children: ReactNode;
}

// --- Shared error extractors ----------------------------------------------

// Used by LOGIN flow
const extractLoginErrorMessage = (data: KratosErrorResponse): string => {
  const msgs = data.ui?.messages;
  if (!msgs || msgs.length === 0) {
    return "Unable to sign in. Please check your email and password.";
  }
  return msgs[0]?.text ?? "Unable to sign in. Please try again.";
};

// Used by REGISTRATION flow
const extractRegistrationErrorMessage = (data: KratosErrorResponse): string => {
  const msgs = data.ui?.messages;
  if (!msgs || msgs.length === 0) {
    return "We could not create your account. Please check your details and try again.";
  }
  return msgs[0]?.text ?? "We could not create your account. Please try again.";
};

// --- Provider --------------------------------------------------------------

export const AuthProvider: React.FC<Props> = ({ children }) => {
  // NOTE: We keep your AuthStatus type, but we WILL use "session-expired".
  // If your ./types AuthStatus union does not include it yet, add it there:
  // 'checking' | 'authenticated' | 'unauthenticated' | 'session-expired'
  const [status, setStatus] = useState<AuthStatus>("checking");
  const [accessToken, setAccessToken] = useState<string | null>(null);
  const [user, setUser] = useState<User | null>(null);

  const [sessionExpiredReason, setSessionExpiredReason] =
    useState<SessionExpiredReason | null>(null);

  // [DEV] minimal state logging
  useEffect(() => {
    console.log("[Auth][state] status:", status, "user:", user?.id ?? null);
  }, [status, user]);

  // --- Helpers: refresh token storage --------------------------------------

  const storeRefreshToken = useCallback(async (token: string) => {
    await SecureStore.setItemAsync(REFRESH_TOKEN_KEY, token);
    console.log(
      "[Auth][storeRefreshToken] stored refresh token prefix:",
      token.slice(0, 12)
    );
  }, []);

  const getStoredRefreshToken = useCallback(async (): Promise<string | null> => {
    return SecureStore.getItemAsync(REFRESH_TOKEN_KEY);
  }, []);

  const clearStoredRefreshToken = useCallback(async () => {
    await SecureStore.deleteItemAsync(REFRESH_TOKEN_KEY);
    console.log("[Auth][clearStoredRefreshToken] refresh token cleared");
  }, []);

  // --- Deterministic session-expired transition ----------------------------

  const markSessionExpired = useCallback(
    async (reason: SessionExpiredReason) => {
      // [DEV]
      console.log("[Auth][sessionExpired] reason:", reason);

      // Clear in-memory state first (prevents races with UI + fetch calls)
      setAccessToken(null);
      setUser(null);
      setSessionExpiredReason(reason);

      // Clear stored token (so next bootstrap doesn't spin)
      try {
        await clearStoredRefreshToken();
      } catch {
        // ignore
      }

      setStatus("session-expired" as AuthStatus);
    },
    [clearStoredRefreshToken]
  );

  const beginReauth = useCallback(() => {
    // [DEV]
    console.log("[Auth][beginReauth] -> unauthenticated (Login)");
    setSessionExpiredReason(null);
    setStatus("unauthenticated");
  }, []);

  // --- Token exchange with your API ----------------------------------------

  const exchangeSessionToken = useCallback(
    async (sessionToken: string): Promise<TokenPair> => {
      const res = await fetch(`${API_BASE_URL}/auth/exchange`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ kratos_session_token: sessionToken }),
      });

      if (!res.ok) {
        const text = await res.text();
        console.log("[AuthProvider][token-exchange] failed", res.status, text);
        throw new AuthError(
          "token_exchange_failed",
          "Could not complete sign-in. Please try again.",
          text
        );
      }

      const data = (await res.json()) as TokenPair;
      return data;
    },
    []
  );

  // --- Refresh (wrapped, single-flight, returns *new access token*) ----------

type RefreshResult = { ok: true; accessToken: string } | { ok: false };

const refreshInFlightRef = useRef<Promise<RefreshResult> | null>(null);

const refresh = useCallback(async (): Promise<RefreshResult> => {
  if (refreshInFlightRef.current) return refreshInFlightRef.current;

  refreshInFlightRef.current = (async () => {
    console.log("[Auth][refresh] called");

    const refreshToken = await getStoredRefreshToken();
    console.log("[Auth][refresh] stored refresh token present?", !!refreshToken);

    if (!refreshToken) {
      console.log("[Auth][refresh] no token -> unauthenticated");
      setAccessToken(null);
      setUser(null);
      setSessionExpiredReason(null);
      setStatus("unauthenticated");
      return { ok: false };
    }

    console.log(
      "[Auth][refresh] sending refresh token prefix:",
      refreshToken.slice(0, 12)
    );

    let res: Response;
    try {
      res = await fetch(`${API_BASE_URL}/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });
    } catch (e) {
      console.log("[Auth][refresh] network exception:", String(e));
      await clearStoredRefreshToken();
      setAccessToken(null);
      setUser(null);
      setSessionExpiredReason(null);
      setStatus("unauthenticated");
      return { ok: false };
    }

    console.log("[Auth][refresh] response status:", res.status);

    if (res.status === 401) {
      await markSessionExpired("refresh_unauthorized");
      return { ok: false };
    }

    if (!res.ok) {
      await clearStoredRefreshToken();
      setAccessToken(null);
      setUser(null);
      setSessionExpiredReason(null);
      setStatus("unauthenticated");
      return { ok: false };
    }

    const data = (await res.json()) as TokenPair;

    console.log("[Auth][refresh] OK new access prefix:", data.access_token.slice(0, 12));
    console.log("[Auth][refresh] OK new refresh prefix:", data.refresh_token.slice(0, 12));

    // IMPORTANT: update state, but ALSO return the new access token
    setAccessToken(data.access_token);
    setUser(data.user);
    setSessionExpiredReason(null);
    await storeRefreshToken(data.refresh_token);
    setStatus("authenticated");

    return { ok: true, accessToken: data.access_token };
  })();

  try {
    return await refreshInFlightRef.current;
  } finally {
    refreshInFlightRef.current = null;
  }
}, [
  getStoredRefreshToken,
  clearStoredRefreshToken,
  storeRefreshToken,
  markSessionExpired,
]);


  // --- Global fetch interceptor (API_BASE_URL only) -------------------------

  const originalFetchRef = useRef<typeof fetch | null>(null);

  useEffect(() => {
    if (originalFetchRef.current) return;

    originalFetchRef.current = globalThis.fetch;

    const shouldHandleUrl = (url: string) => url.startsWith(API_BASE_URL);

    const isAuthEndpoint = (url: string) =>
      url.includes("/auth/refresh") ||
      url.includes("/auth/logout") ||
      url.includes("/auth/exchange");

    globalThis.fetch = (async (
      input: RequestInfo | URL,
      init?: RequestInit
    ): Promise<Response> => {
      const url =
        typeof input === "string"
          ? input
          : input instanceof URL
          ? input.toString()
          : (input as any).url ?? "";

      const origFetch = originalFetchRef.current!;
      if (!url || !shouldHandleUrl(url) || isAuthEndpoint(url)) {
        return origFetch(input as any, init);
      }

      const headers = new Headers(init?.headers);

      // Attach access token if present
      if (accessToken && !headers.has("Authorization")) {
        headers.set("Authorization", `Bearer ${accessToken}`);
      }

      // Avoid infinite loop: allow one retry only
      const isRetry = headers.get("x-auth-retry") === "1";

      const res = await origFetch(input as any, { ...init, headers });

      if (res.status !== 401 && res.status !== 403) return res;

      if (isRetry) {
        // Second failure after refresh attempt -> deterministic session expired
        if (res.status === 401) await markSessionExpired("api_unauthorized");
        else await markSessionExpired("api_forbidden");
        return res;
      }

      const result = await refresh();
      if (!result.ok) {
        // refresh() itself will set session-expired on refresh 401
        return res;
      }

      // Retry once with the *new* token returned from refresh()
      const retryHeaders = new Headers(init?.headers);
      retryHeaders.set("x-auth-retry", "1");
      retryHeaders.set("Authorization", `Bearer ${result.accessToken}`);

      return origFetch(input as any, { ...init, headers: retryHeaders });
    });

    return () => {
      if (originalFetchRef.current) {
        globalThis.fetch = originalFetchRef.current;
        originalFetchRef.current = null;
      }
    };
  }, [accessToken, markSessionExpired, refresh]);

  // --- Login via Kratos native flow ----------------------------------------

  const login: AuthContextValue["login"] = useCallback(
    async (email: string, password: string) => {
      setStatus("checking");

      const loginFlowPath =
      Platform.OS === "web"
        ? "/self-service/login/browser"
        : "/self-service/login/api";

      const flowRes = await fetch(`${KRATOS_BASE_URL}${loginFlowPath}`, {
        method: "GET",
        headers: {
          Accept: "application/json",
        },
        credentials: Platform.OS === "web" ? "include" : "omit",
      });

      if (!flowRes.ok) {
        const text = await flowRes.text();
        setStatus("unauthenticated");
        throw new AuthError("create_login_flow_failed", "Unable to start sign-in.", text);
      }

      const flow = (await flowRes.json()) as KratosLoginFlow;
      console.log("[Auth][login] flow.type", flow.type);
      console.log("[Auth][login] flow.ui.action", flow.ui?.action);
      console.log("[Auth][login] flow.ui.method", flow.ui?.method);

      // 2) Submit credentials
      const identifier = (email ?? "").trim().toLowerCase();

      const csrfToken =
        Platform.OS === "web"
          ? flow.ui.nodes?.find(
              (node) => node.attributes?.name === "csrf_token"
            )?.attributes?.value
          : undefined;

      console.log("[Auth][login] csrfToken present?", !!csrfToken);
      console.log("[Auth][login] csrfToken prefix", csrfToken?.slice(0, 12) ?? null);

      if (Platform.OS === "web" && !csrfToken) {
        console.log("[Auth][login] Missing csrfToken", JSON.stringify(flow.ui.nodes ?? []));
        setStatus("unauthenticated");
        throw new AuthError(
          "missing_csrf_token",
          "Unable to complete browser sign-in because the CSRF token was missing."
        );
      }

      const submitBody = {
        method: "password",
        identifier,
        password,
        ...(Platform.OS === "web" ? { csrf_token: csrfToken } : {}),
      };

      console.log("[Auth][login] submitBody keys", Object.keys(submitBody));

      const submitRes = await fetch(flow.ui.action, {
        method: flow.ui.method ?? "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json; charset=utf-8",
          ...(Platform.OS === "web" && csrfToken
            ? { "X-CSRF-Token": csrfToken }
            : {}),
        },
        credentials: Platform.OS === "web" ? "include" : "omit",
        body: JSON.stringify(submitBody),
      });

      // ✅ log response status immediately
      console.log("[Auth][login] submitRes.status", submitRes.status);
      console.log("[Auth][login] submitRes.ok", submitRes.ok);

      const submitJson = await submitRes.json();

      console.log("[Auth][login] submitJson", JSON.stringify(submitJson));

      if (submitRes.status === 400) {
        const msg = extractLoginErrorMessage(submitJson as KratosErrorResponse);
        setStatus("unauthenticated");
        throw new AuthError("invalid_credentials", msg);
      }

      if (!submitRes.ok) {
        setStatus("unauthenticated");
        throw new AuthError(
          "login_failed",
          "Sign-in failed unexpectedly.",
          JSON.stringify(submitJson)
        );
      }

      const loginResult = submitJson as KratosSuccessfulNativeLogin;

      if (!loginResult.session_token) {
        setStatus("unauthenticated");
        throw new AuthError("missing_session_token", "Kratos did not return a session token.");
      }

      console.log(
        "[AuthProvider][login] session_token prefix",
        loginResult.session_token.slice(0, 12)
      );

      // 3) Exchange for YOUR tokens
      const tokens = await exchangeSessionToken(loginResult.session_token);

      console.log(
        "[Auth][login] exchange OK. access_token prefix:",
        tokens.access_token.slice(0, 12)
      );
      console.log(
        "[Auth][login] exchange OK. refresh_token prefix:",
        tokens.refresh_token.slice(0, 12)
      );

      await storeRefreshToken(tokens.refresh_token);
      setAccessToken(tokens.access_token);
      setUser(tokens.user);
      setSessionExpiredReason(null);
      setStatus("authenticated");
    },
    [exchangeSessionToken, storeRefreshToken]
  );

  // --- Register via Kratos native flow -------------------------------------

  const register: AuthContextValue["register"] = useCallback(
    async (input: RegisterInput) => {
      const email = input.email.trim().toLowerCase();
      const password = input.password;

      setStatus("checking");

      try {
        // 1) Create the correct Kratos registration flow for web vs native
        const registrationFlowPath =
          Platform.OS === "web"
            ? "/self-service/registration/browser"
            : "/self-service/registration/api";

        const flowRes = await fetch(`${KRATOS_BASE_URL}${registrationFlowPath}`, {
          method: "GET",
          headers: {
            Accept: "application/json",
          },
          credentials: Platform.OS === "web" ? "include" : "omit",
        });
        
        console.log("[Auth][register] create flow.status", flowRes.status);
        console.log("[Auth][register] create flow.ok", flowRes.ok);

        if (!flowRes.ok) {
          const text = await flowRes.text();
          console.log("[Auth][register] create flow.body", text);
          setStatus("unauthenticated");
          throw new AuthError(
            "create_registration_flow_failed",
            "Unable to start registration. Please try again.",
            text
          );
        }

        const flow = (await flowRes.json()) as KratosRegistrationFlow;
        console.log("[Auth][register] flow.id", flow.id);
        console.log("[Auth][register] flow.ui.action", flow.ui?.action);
        console.log("[Auth][register] flow.ui.method", flow.ui?.method);

        // ✅ TS-safe: your types say KratosUi doesn't have nodes, but runtime DOES.
        // We log it without breaking compilation.
        const uiAny = flow.ui as unknown as { nodes?: unknown };
        console.log("[Auth][register] flow.ui.nodes", JSON.stringify(uiAny.nodes ?? []));

        // 2) Submit credentials
        const identifier = (email ?? "").trim().toLowerCase();

        const csrfToken =
          Platform.OS === "web"
            ? flow.ui.nodes?.find(
                (node) => node.attributes?.name === "csrf_token"
              )?.attributes?.value
            : undefined;

          console.log("[Auth][login] flow.type", flow.type);
          console.log("[Auth][login] flow.ui.action", flow.ui.action);
          console.log("[Auth][login] csrfToken present?", !!csrfToken);
          console.log("[Auth][login] csrfToken prefix", csrfToken?.slice(0, 12) ?? null);

        if (Platform.OS === "web" && !csrfToken) {
          console.log("[Auth][login] Missing csrfToken", JSON.stringify(flow.ui.nodes ?? []));
          setStatus("unauthenticated");
          throw new AuthError(
            "missing_csrf_token",
            "Unable to complete browser sign-in because the CSRF token was missing."
          );
        }

        const submitBody = {
          method: "password",
          identifier,
          password,
          ...(Platform.OS === "web" ? { csrf_token: csrfToken } : {}),
        };

        console.log("[Auth][login] csrfToken present?", !!csrfToken);
        console.log("[Auth][login] submitBody keys", Object.keys(submitBody));

        const submitRes = await fetch(flow.ui.action, {
          method: flow.ui.method ?? "POST",
          headers: {
            Accept: "application/json",
            "Content-Type": "application/json; charset=utf-8",
            ...(Platform.OS === "web" && csrfToken
              ? { "X-CSRF-Token": csrfToken }
              : {}),
          },
          credentials: Platform.OS === "web" ? "include" : "omit",
          body: JSON.stringify(submitBody),
        });

        // ✅ log response status immediately
        console.log("[Auth][login] submitRes.status", submitRes.status);
        console.log("[Auth][login] submitRes.ok", submitRes.ok);

        // ✅ read response body once
        const submitJson = await submitRes.json();

        console.log("[Auth][login] submitJson", JSON.stringify(submitJson));

        // ✅ Read the response body ONCE
        const submitText = await submitRes.text();
        console.log("[Auth][register] submitRes.bodyText", submitText);

        if (submitRes.status === 400) {
          // ✅ Try to parse Kratos JSON error, but don't assume it's JSON
          let errJson: KratosErrorResponse | null = null;
          try {
            errJson = JSON.parse(submitText) as KratosErrorResponse;
          } catch {
            errJson = null;
          }

          if (errJson) {
            const msg = extractRegistrationErrorMessage(errJson);
            setStatus("unauthenticated");
            throw new KratosFormError(msg);
          }

          // Non-JSON 400 (rare, but possible)
          setStatus("unauthenticated");
          throw new KratosFormError(
            "We could not create your account. Please check your details and try again."
          );
        }

        if (!submitRes.ok) {
          setStatus("unauthenticated");
          throw new AuthError(
            "registration_failed",
            "Registration failed unexpectedly. Please try again.",
            submitText
          );
        }

        // ✅ Success path: parse from the same captured body
        const regResult = JSON.parse(submitText) as KratosSuccessfulNativeRegistration;

        if (!regResult.session_token) {
          setStatus("unauthenticated");
          throw new AuthError(
            "missing_session_token",
            "Registration did not return a session token from the identity service."
          );
        }

        console.log(
          "[AuthProvider][registration] session_token prefix",
          regResult.session_token.slice(0, 12)
        );

        // 4) Exchange Kratos session_token for your JWTs
        const tokens = await exchangeSessionToken(regResult.session_token);

        await storeRefreshToken(tokens.refresh_token);
        setAccessToken(tokens.access_token);
        setUser(tokens.user);
        setSessionExpiredReason(null);
        setStatus("authenticated");
        console.log("[AuthProvider] AUTHENTICATED -> should render MainAppNavigator");
      } catch (e) {
        console.log("[AuthProvider][register] error", String(e));
        setStatus("unauthenticated"); // exits LOADING/GATE
        throw e;
      }
    },
    [exchangeSessionToken, storeRefreshToken]
  );

  // --- Logout --------------------------------------------------------------

  const logout: AuthContextValue["logout"] = useCallback(async () => {
    console.log("[Auth][logout] called");

    const refreshToken = await getStoredRefreshToken();
    console.log("[Auth][logout] has refresh token?", !!refreshToken);

    if (refreshToken) {
      console.log("[Auth][logout] sending token prefix:", refreshToken.slice(0, 12));
      try {
        const res = await fetch(`${API_BASE_URL}/auth/logout`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
        console.log("[Auth][logout] server status:", res.status);
      } catch (e) {
        console.log("[Auth][logout] server call failed:", String(e));
      }
    }

    await clearStoredRefreshToken();
    setAccessToken(null);
    setUser(null);
    setSessionExpiredReason(null);
    setStatus("unauthenticated");
    console.log("[Auth][logout] complete -> unauthenticated");
  }, [getStoredRefreshToken, clearStoredRefreshToken]);

  // --- Bootstrap on app startup --------------------------------------------

  useEffect(() => {
    (async () => {
      try {
        await refresh();
      } catch {
        setStatus("unauthenticated");
      }
    })();
  }, [refresh]);

  // --- Context value --------------------------------------------------------

  const value = useMemo((): AuthContextValue => {
    return {
      status,
      accessToken,
      user,
      login,
      logout,
      refresh,
      register,

      // New: session-expired UX hooks
      sessionExpiredReason,
      beginReauth,
    } as AuthContextValue;
  }, [
    status,
    accessToken,
    user,
    login,
    logout,
    refresh,
    register,
    sessionExpiredReason,
    beginReauth,
  ]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export const useAuth = (): AuthContextValue => {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return ctx;
};

export { KratosFormError };
