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

  // --- Helpers: refresh token storage --------------------------------------

  const storeRefreshToken = useCallback(async (token: string) => {
    await SecureStore.setItemAsync(REFRESH_TOKEN_KEY, token);
  }, []);

  const getStoredRefreshToken = useCallback(async (): Promise<string | null> => {
    return SecureStore.getItemAsync(REFRESH_TOKEN_KEY);
  }, []);

  const clearStoredRefreshToken = useCallback(async () => {
    await SecureStore.deleteItemAsync(REFRESH_TOKEN_KEY);
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
    const refreshToken = await getStoredRefreshToken();
    if (!refreshToken) {
      setAccessToken(null);
      setUser(null);
      setStatus('unauthenticated');
      return;
    }

    const res = await fetch(`${API_BASE_URL}/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });

    if (!res.ok) {
      await clearStoredRefreshToken();
      setAccessToken(null);
      setUser(null);
      setStatus('unauthenticated');
      return;
    }

    const data = (await res.json()) as TokenPair;
    setAccessToken(data.access_token);
    setUser(data.user);
    await storeRefreshToken(data.refresh_token);
    setStatus('authenticated');
  }, [getStoredRefreshToken, clearStoredRefreshToken, storeRefreshToken]);

  // --- Login via Kratos native flow ----------------------------------------

  const login: AuthContextValue['login'] = useCallback(
    async (email: string, password: string) => {
      setStatus('checking');

      // 1) Create a native login flow in Kratos
      const flowRes = await fetch(
        `${KRATOS_BASE_URL}/self-service/login/api`,
        {
          method: 'GET',
          headers: { Accept: 'application/json' },
        },
      );

      if (!flowRes.ok) {
        const text = await flowRes.text();
        console.log("[AuthProvider][login] create flow failed", flowRes.status, text);
        setStatus('unauthenticated');
        throw new AuthError(
          'create_login_flow_failed',
          'Unable to start sign-in. Please try again.',
          text,
        );
      }

      const flow = (await flowRes.json()) as KratosLoginFlow;

    // 2) Submit credentials to the flow's action
    const identifier = (email ?? '').trim().toLowerCase();

    const payload = {
    method: 'password',
    identifier,
    password,
    };

    const body = JSON.stringify(payload);
    console.log('[AuthProvider][login] identifier computed =', identifier);
    console.log('[AuthProvider][login] submit body =', body);

    const submitUrl = `${KRATOS_BASE_URL}/self-service/login?flow=${flow.id}`;

    const submitRes = await fetch(submitUrl, {
    method: 'POST',
    headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json; charset=utf-8',
    },
    body,
    });
        
      if (submitRes.status === 400) {
        // Validation error or bad credentials
        const data = (await submitRes.json()) as KratosErrorResponse;
        console.log("[AuthProvider][login] submit 400", JSON.stringify(data, null, 2));
        const msg = extractLoginErrorMessage(data);
        setStatus('unauthenticated');
        throw new AuthError('invalid_credentials', msg);
      }

      if (!submitRes.ok) {
        const text = await submitRes.text();
        console.log("[AuthProvider][login] submit failed", submitRes.status, text);
        setStatus('unauthenticated');
        throw new AuthError(
          'login_failed',
          'Sign-in failed unexpectedly. Please try again.',
          text,
        );
      }

      const loginResult =
        (await submitRes.json()) as KratosSuccessfulNativeLogin;

      if (!loginResult.session_token) {
        console.log("[AuthProvider][login] missing session_token", JSON.stringify(loginResult, null, 2));
        setStatus('unauthenticated');
        throw new AuthError(
          'missing_session_token',
          'Sign-in did not return a session token from the identity service.',
        );
      }

      // 3) Exchange Kratos session_token for your JWTs
      const tokens = await exchangeSessionToken(loginResult.session_token);

      await storeRefreshToken(tokens.refresh_token);
      setAccessToken(tokens.access_token);
      setUser(tokens.user);
      setStatus('authenticated');
      console.log("[AuthProvider] AUTHENTICATED -> should render MainAppNavigator");
    },
    [exchangeSessionToken, storeRefreshToken, clearStoredRefreshToken]
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
    const refreshToken = await getStoredRefreshToken();
    if (refreshToken) {
      try {
        await fetch(`${API_BASE_URL}/auth/logout`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
      } catch {
        // ignore network errors on logout
      }
    }

    await clearStoredRefreshToken();
    setAccessToken(null);
    setUser(null);
    setStatus('unauthenticated');
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