import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  ReactNode,
} from 'react';
import * as SecureStore from 'expo-secure-store';
import {
  AuthContextValue,
  AuthStatus,
  User,
  TokenPair,
  AuthError,
  RegisterInput,
} from './types';
import {
  KratosLoginFlow,
  KratosSuccessfulNativeLogin,
  KratosErrorResponse,
  KratosRegistrationFlow,
  KratosSuccessfulNativeRegistration,
  KratosFormError,
} from './kratosTypes';

const KRATOS_BASE_URL = 'https://auth.cvera.app';
const API_BASE_URL = 'https://api.cvera.app';
const REFRESH_TOKEN_KEY = 'cvera_refresh_token_v1';

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

interface Props {
  children: ReactNode;
}

// --- Shared error extractors ----------------------------------------------

// Used by LOGIN flow
const extractLoginErrorMessage = (data: KratosErrorResponse): string => {
  const msgs = data.ui?.messages;
  if (!msgs || msgs.length === 0) {
    return 'Unable to sign in. Please check your email and password.';
  }
  // For HR users: show the first message text
  return msgs[0]?.text ?? 'Unable to sign in. Please try again.';
};

// Used by REGISTRATION flow
const extractRegistrationErrorMessage = (data: KratosErrorResponse): string => {
  const msgs = data.ui?.messages;
  if (!msgs || msgs.length === 0) {
    return 'We could not create your account. Please check your details and try again.';
  }
  return (
    msgs[0]?.text ??
    'We could not create your account. Please try again.'
  );
};

// --- Provider --------------------------------------------------------------

export const AuthProvider: React.FC<Props> = ({ children }) => {
  const [status, setStatus] = useState<AuthStatus>('checking');
  const [accessToken, setAccessToken] = useState<string | null>(null);
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
  console.log("[Auth][state] status:", status, "user:", user?.id ?? null);
}, [status, user]);

  // --- Helpers: refresh token storage --------------------------------------

  const storeRefreshToken = useCallback(async (token: string) => {
    await SecureStore.setItemAsync(REFRESH_TOKEN_KEY, token);
    console.log("[Auth][storeRefreshToken] stored refresh token prefix:", token.slice(0, 12));
  }, []);

  const getStoredRefreshToken = useCallback(async (): Promise<string | null> => {
    return SecureStore.getItemAsync(REFRESH_TOKEN_KEY);
  }, []);

  const clearStoredRefreshToken = useCallback(async () => {
    await SecureStore.deleteItemAsync(REFRESH_TOKEN_KEY);
    console.log("[Auth][clearStoredRefreshToken] refresh token cleared");
  }, []);

  // --- Token exchange with your API ----------------------------------------

  const exchangeSessionToken = useCallback(
    async (sessionToken: string): Promise<TokenPair> => {
      const res = await fetch(`${API_BASE_URL}/auth/exchange`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kratos_session_token: sessionToken }),
      });

      if (!res.ok) {
        const text = await res.text();
        console.log("[AuthProvider][token-exchange] failed", res.status, text);
        throw new AuthError(
          'token_exchange_failed',
          'Could not complete sign-in. Please try again.',
          text,
        );
      }

      const data = (await res.json()) as TokenPair;
      return data;
    },
    [],
  );

  // --- Refresh -------------------------------------------------------------

  const refresh: AuthContextValue['refresh'] = useCallback(async () => {
    console.log("[Auth][refresh] called");
    const refreshToken = await getStoredRefreshToken();
    console.log("[Auth][refresh] stored refresh token present?", !!refreshToken);
    if (!refreshToken) {
    console.log("[Auth][refresh] no token -> unauthenticated");
      setAccessToken(null);
      setUser(null);
      setStatus('unauthenticated');
      return;
    }

    console.log("[Auth][refresh] sending refresh token prefix:", refreshToken.slice(0, 12));

    const res = await fetch(`${API_BASE_URL}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });

    console.log("[Auth][refresh] response status:", res.status);

    if (!res.ok) {
      await clearStoredRefreshToken();
      setAccessToken(null);
      setUser(null);
      setStatus('unauthenticated');
      return;
    }

    const data = (await res.json()) as TokenPair;
    console.log("[Auth][refresh] OK new access prefix:", data.access_token.slice(0, 12));
    console.log("[Auth][refresh] OK new refresh prefix:", data.refresh_token.slice(0, 12));

    setAccessToken(data.access_token);
    setUser(data.user);
    await storeRefreshToken(data.refresh_token);
    setStatus('authenticated');
  }, [getStoredRefreshToken, clearStoredRefreshToken, storeRefreshToken]);

  // --- Login via Kratos native flow ----------------------------------------

    const login: AuthContextValue["login"] = useCallback(
    async (email: string, password: string) => {
        setStatus("checking");

        // 1) Create flow
        const flowRes = await fetch(`${KRATOS_BASE_URL}/self-service/login/api`, {
        method: "GET",
        headers: { Accept: "application/json" },
        });

        if (!flowRes.ok) {
        const text = await flowRes.text();
        setStatus("unauthenticated");
        throw new AuthError("create_login_flow_failed", "Unable to start sign-in.", text);
        }

        const flow = (await flowRes.json()) as KratosLoginFlow;

        // 2) Submit credentials
        const identifier = (email ?? "").trim().toLowerCase();
        const submitUrl = `${KRATOS_BASE_URL}/self-service/login?flow=${flow.id}`;

        const submitRes = await fetch(submitUrl, {
        method: "POST",
        headers: {
            Accept: "application/json",
            "Content-Type": "application/json; charset=utf-8",
        },
        body: JSON.stringify({
            method: "password",
            identifier,
            password,
        }),
        });

        const submitJson = await submitRes.json();

        // 400: Kratos form / credential error
        if (submitRes.status === 400) {
        const msg = extractLoginErrorMessage(submitJson as KratosErrorResponse);
        setStatus("unauthenticated");
        throw new AuthError("invalid_credentials", msg);
        }

        if (!submitRes.ok) {
        setStatus("unauthenticated");
        throw new AuthError("login_failed", "Sign-in failed unexpectedly.", JSON.stringify(submitJson));
        }

        const loginResult = submitJson as KratosSuccessfulNativeLogin;

        if (!loginResult.session_token) {
        setStatus("unauthenticated");
        throw new AuthError("missing_session_token", "Kratos did not return a session token.");
        }

        // DEV: prove you got a token (prefix only)
        console.log("[AuthProvider][login] session_token prefix", loginResult.session_token.slice(0, 12));

        // 3) Exchange for YOUR tokens
        const tokens = await exchangeSessionToken(loginResult.session_token);

        console.log("[Auth][login] exchange OK. access_token prefix:", tokens.access_token.slice(0, 12));
        console.log("[Auth][login] exchange OK. refresh_token prefix:", tokens.refresh_token.slice(0, 12));

        await storeRefreshToken(tokens.refresh_token);
        setAccessToken(tokens.access_token);
        setUser(tokens.user);
        setStatus("authenticated");
    },
    [exchangeSessionToken, storeRefreshToken],
);

  // --- Register via Kratos native flow (NEW) -------------------------------

  const register: AuthContextValue['register'] = useCallback(
    async (input: RegisterInput) => {
      const email = input.email.trim().toLowerCase();
      const password = input.password;
      const fullName = input.fullName?.trim();

      setStatus('checking');

      try {
        // 1) Create a native registration flow in Kratos
        const flowRes = await fetch(
          `${KRATOS_BASE_URL}/self-service/registration/api`,
          {
            method: 'GET',
            headers: { Accept: 'application/json' },
          },
        );

        if (!flowRes.ok) {
          const text = await flowRes.text();
          setStatus('unauthenticated');
          throw new AuthError(
            'create_registration_flow_failed',
            'Unable to start registration. Please try again.',
            text,
          );
        }

        const flow = (await flowRes.json()) as KratosRegistrationFlow;

        // 2) Submit registration data to the flow's action
        const submitRes = await fetch(flow.ui.action, {
          method: flow.ui.method,
          headers: {
            'Content-Type': 'application/json',
            Accept: 'application/json',
          },
          body: JSON.stringify({
            method: "password",
            identifier: email,
            password,
            traits: {
                email,
                full_name: fullName,
            },
            }),
        });

        // 3) Handle Kratos "form error" responses (status 400)
        if (submitRes.status === 400) {
          const data = (await submitRes.json()) as KratosErrorResponse;
          const msg = extractRegistrationErrorMessage(data);
          setStatus('unauthenticated');
          throw new KratosFormError(msg);
        }

        if (!submitRes.ok) {
          const text = await submitRes.text();
          setStatus('unauthenticated');
          throw new AuthError(
            'registration_failed',
            'Registration failed unexpectedly. Please try again.',
            text,
          );
        }

        const regResult =
          (await submitRes.json()) as KratosSuccessfulNativeRegistration;

        if (!regResult.session_token) {
          setStatus('unauthenticated');
          throw new AuthError(
            'missing_session_token',
            'Registration did not return a session token from the identity service.',
          );
        }
        // 4) Exchange Kratos session_token for your JWTs
        const tokens = await exchangeSessionToken(regResult.session_token);
        console.log("[AuthProvider][token-exchange] sending session token prefix", regResult.session_token.slice(0, 12));

        await storeRefreshToken(tokens.refresh_token);
        setAccessToken(tokens.access_token);
        setUser(tokens.user);
        setStatus("authenticated");
        console.log("[AuthProvider] AUTHENTICATED -> should render MainAppNavigator");
        } catch (e) {
        console.log("[AuthProvider][login] token-exchange error", String(e));
        setStatus("unauthenticated"); // <- critical: exits LOADING/GATE
        throw e;
        }
    },
    [exchangeSessionToken, storeRefreshToken],
  );

  // --- Logout --------------------------------------------------------------

  const logout: AuthContextValue['logout'] = useCallback(async () => {
  console.log("[Auth][logout] called");

  const refreshToken = await getStoredRefreshToken();
  console.log("[Auth][logout] has refresh token?", !!refreshToken);

  if (refreshToken) {
    console.log("[Auth][logout] sending token prefix:", refreshToken.slice(0, 12));
    try {
      const res = await fetch(`${API_BASE_URL}/auth/logout`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
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
  setStatus('unauthenticated');
  console.log("[Auth][logout] complete -> unauthenticated");
}, [getStoredRefreshToken, clearStoredRefreshToken]);

  // --- Bootstrap on app startup --------------------------------------------

  useEffect(() => {
    (async () => {
      try {
        await refresh();
      } catch {
        setStatus('unauthenticated');
      }
    })();
  }, [refresh]);

  const value: AuthContextValue = {
    status,
    accessToken,
    user,
    login,
    logout,
    refresh,
    register, 
  };

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = (): AuthContextValue => {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return ctx;
};
export { KratosFormError };