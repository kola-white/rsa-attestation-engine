import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useNavigation } from '@react-navigation/native';
import type { NativeStackNavigationProp } from '@react-navigation/native-stack';
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  Text,
  TextInput,
  View,
} from 'react-native';

useEffect(() => {
  console.log('[SettingsScreen] mounted');
}, []);

import type { AuthStackParamList } from '@/src/navigation/types';
import type { KratosUiNode } from '@/src/auth/kratosTypes';

const KRATOS_BASE_URL = 'https://auth.cvera.app';
const REDIRECT_SECONDS = 8;

type AuthNav = NativeStackNavigationProp<AuthStackParamList, 'Settings'>;

type KratosSettingsFlow = {
  id: string;
  type: 'browser' | 'api';
  ui: {
    action: string;
    method: string;
    nodes: KratosUiNode[];
    messages?: {
      id: number;
      text: string;
      type: string;
    }[];
  };
};

type KratosSettingsSubmitErrorResponse = {
  ui?: {
    messages?: {
      id?: number;
      text?: string;
      type?: string;
    }[];
  };
};

const getBrowserFlowId = (): string | null => {
  if (Platform.OS !== 'web') return null;
  if (typeof window === 'undefined') return null;

  return new URLSearchParams(window.location.search).get('flow');
};

const extractCsrfToken = (flow: KratosSettingsFlow): string | null => {
  const csrfNode = flow.ui.nodes.find(
    (node) => node.attributes?.name === 'csrf_token',
  );

  const value = csrfNode?.attributes?.value;

  return typeof value === 'string' && value.length > 0 ? value : null;
};

const hasPasswordMethod = (flow: KratosSettingsFlow): boolean => {
  return flow.ui.nodes.some(
    (node) =>
      node.group === 'password' &&
      node.attributes?.name === 'method' &&
      node.attributes?.value === 'password',
  );
};

const extractSettingsErrorMessage = (
  data: KratosSettingsSubmitErrorResponse | null,
): string => {
  return (
    data?.ui?.messages?.find((message) => Boolean(message.text))?.text ??
    'Password could not be updated. Please try again.'
  );
};

const SettingsScreen: React.FC = () => {
  const navigation = useNavigation<AuthNav>();

  const [flow, setFlow] = useState<KratosSettingsFlow | null>(null);
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordUpdated, setPasswordUpdated] = useState(false);
  const [countdownSeconds, setCountdownSeconds] = useState(REDIRECT_SECONDS);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const countdownTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const flowId = useMemo(() => getBrowserFlowId(), []);

    useEffect(() => {
    console.log('[SettingsScreen] mounted', {
        pathname:
        Platform.OS === 'web' && typeof window !== 'undefined'
            ? window.location.pathname
            : null,
        search:
        Platform.OS === 'web' && typeof window !== 'undefined'
            ? window.location.search
            : null,
        flowId,
    });
    }, [flowId]);

  const clearCountdownTimer = useCallback((): void => {
    if (countdownTimerRef.current) {
      clearInterval(countdownTimerRef.current);
      countdownTimerRef.current = null;
    }
  }, []);

  const redirectToLogin = useCallback(
    (source: 'auto' | 'manual'): void => {
      clearCountdownTimer();

      if (source === 'auto') {
        console.log('[SettingsScreen] auto redirect to login fired');
      } else {
        console.log('[SettingsScreen] manual redirect to login fired');
      }

      if (Platform.OS === 'web' && typeof window !== 'undefined') {
        window.location.assign('/login');
        return;
      }

      navigation.navigate('Login');
    },
    [clearCountdownTimer, navigation],
  );

  const loadSettingsFlow = useCallback(async (): Promise<void> => {
    console.log('[SettingsScreen] loadSettingsFlow start', { flowId });

    if (!flowId) {
        setError('Settings flow is missing. Please restart password recovery.');
        return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `${KRATOS_BASE_URL}/self-service/settings/flows?id=${encodeURIComponent(
            flowId,
        )}`,
        {
            method: 'GET',
            credentials: 'include',
            headers: {
            Accept: 'application/json',
            },
        },
        );

        console.log(
        '[SettingsScreen] settings flow response',
        response.status,
        response.ok,
        );

        if (!response.ok) {
        setError('This password reset session expired. Please restart recovery.');
        return;
      }

      const data = (await response.json()) as KratosSettingsFlow;

    console.log('[SettingsScreen] settings flow loaded', {
    id: data.id,
    type: data.type,
    action: data.ui.action,
    hasPasswordMethod: hasPasswordMethod(data),
    hasCsrfToken: Boolean(extractCsrfToken(data)),
    });

    if (!hasPasswordMethod(data)) {
    setError('Password reset is not available for this settings flow.');
    return;
    }

    setFlow(data);
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : 'Unable to load password reset settings.',
      );
    } finally {
      setLoading(false);
    }
  }, [flowId]);

  useEffect(() => {
    if (Platform.OS !== 'web') {
      setError('Password reset settings are currently available on web only.');
      return;
    }

    void loadSettingsFlow();
  }, [loadSettingsFlow]);

  useEffect(() => {
    if (!passwordUpdated || countdownTimerRef.current) {
      return;
    }

    console.log('[SettingsScreen] countdown started', REDIRECT_SECONDS);

    countdownTimerRef.current = setInterval(() => {
      setCountdownSeconds((currentSeconds) => {
        if (currentSeconds <= 1) {
          redirectToLogin('auto');
          return 0;
        }

        return currentSeconds - 1;
      });
    }, 1000);

    return clearCountdownTimer;
  }, [clearCountdownTimer, passwordUpdated, redirectToLogin]);

  const submitPassword = useCallback(async (): Promise<void> => {
    console.log('[SettingsScreen] submitPassword called', {
        hasFlow: Boolean(flow),
        passwordLength: password.length,
        confirmPasswordLength: confirmPassword.length,
        passwordsMatch: password === confirmPassword,
        passwordUpdated,
        });
    if (passwordUpdated) {
      return;
    }

    setError(null);

    if (!flow) {
      setError('Settings flow is missing. Please restart password recovery.');
      return;
    }

    if (!password || password.length < 8) {
      setError('Please enter a password with at least 8 characters.');
      return;
    }

    if (password !== confirmPassword) {
      setError('Passwords do not match.');
      return;
    }

    const csrfToken = extractCsrfToken(flow);

    if (!csrfToken) {
      setError('Security token is missing. Please restart password recovery.');
      return;
    }

    setSubmitting(true);

    try {
      const response = await fetch(flow.ui.action, {
        method: 'POST',
        credentials: 'include',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json; charset=utf-8',
        },
        body: JSON.stringify({
          method: 'password',
          password,
          csrf_token: csrfToken,
        }),
      });
      console.log('[SettingsScreen] password submit response', response.status, response.ok);
      const data = (await response.json().catch(() => null)) as
        | KratosSettingsSubmitErrorResponse
        | null;

      if (!response.ok) {
        setError(extractSettingsErrorMessage(data));
        return;
      }

      console.log('[SettingsScreen] password update succeeded');

      setPassword('');
      setConfirmPassword('');
      setCountdownSeconds(REDIRECT_SECONDS);
      setPasswordUpdated(true);
    } catch (e) {
      setError(
        e instanceof Error ? e.message : 'Password update failed. Please try again.',
      );
    } finally {
      setSubmitting(false);
    }
  }, [confirmPassword, flow, password, passwordUpdated]);

  const isBusy = loading || submitting;
  const formDisabled = isBusy || passwordUpdated;

  return (
    <KeyboardAvoidingView
      className="flex-1 bg-slate-950"
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
    >
      <ScrollView
        className="flex-1"
        contentContainerStyle={{ flexGrow: 1 }}
        keyboardShouldPersistTaps="handled"
      >
        <View className="flex-1 px-6 pt-16 pb-8 justify-center">
          <Text className="text-3xl font-semibold text-white mb-2">
            Set new password
          </Text>

          <Text className="text-base text-slate-300 mb-6">
            Enter a new password to complete account recovery.
          </Text>

          {error && (
            <View className="mb-4 rounded-xl border border-red-500 bg-red-950/60 px-4 py-3">
              <Text className="text-sm text-red-100">{error}</Text>
            </View>
          )}

          {passwordUpdated && (
            <View className="mb-4 rounded-xl border border-emerald-500 bg-emerald-950/60 px-4 py-3">
                <Text className="text-sm font-semibold text-emerald-100">
                    Password updated successfully.
                </Text>

                <Text className="mt-1 text-sm text-emerald-100">
                    You can now sign in with your new password.
                </Text>

                <Text className="mt-2 text-sm text-emerald-100">
                    Redirecting to sign in after {countdownSeconds} seconds…
                </Text>

                <Text className="mt-1 text-sm text-emerald-200">
                    Don't want to wait? Use the link below.
                </Text>
            </View>
          )}

          {loading && (
            <View className="mb-4 rounded-xl border border-slate-700 bg-slate-900 px-4 py-3">
              <View className="flex-row items-center">
                <ActivityIndicator size="small" />
                <Text className="ml-3 text-sm text-slate-200">
                  Loading password reset…
                </Text>
              </View>
            </View>
          )}

          {flow && (
            <>
              <View className="mb-4">
                <Text className="mb-1 text-sm font-medium text-slate-200">
                  New password
                </Text>
                <TextInput
                  className="h-11 rounded-xl border border-slate-700 bg-slate-900 px-3 text-sm text-slate-100"
                  placeholder="Enter new password"
                  placeholderTextColor="#64748b"
                  secureTextEntry
                  autoCapitalize="none"
                  autoCorrect={false}
                  editable={!formDisabled}
                  value={password}
                  onChangeText={setPassword}
                />
              </View>

              <View className="mb-4">
                <Text className="mb-1 text-sm font-medium text-slate-200">
                  Confirm new password
                </Text>
                <TextInput
                  className="h-11 rounded-xl border border-slate-700 bg-slate-900 px-3 text-sm text-slate-100"
                  placeholder="Re-enter new password"
                  placeholderTextColor="#64748b"
                  secureTextEntry
                  autoCapitalize="none"
                  autoCorrect={false}
                  editable={!formDisabled}
                  value={confirmPassword}
                  onChangeText={setConfirmPassword}
                />
              </View>

              <Pressable
                className="mt-2 h-11 rounded-xl bg-sky-400 items-center justify-center disabled:opacity-50"
                disabled={formDisabled}
                onPress={submitPassword}
              >
                {submitting ? (
                  <ActivityIndicator size="small" />
                ) : (
                  <Text className="text-base font-semibold text-slate-950">
                    Update password
                  </Text>
                )}
              </Pressable>
            </>
          )}

          <View className="mt-6 flex-row justify-center">
            <Pressable onPress={() => redirectToLogin('manual')}>
              <Text className="text-sm font-semibold text-sky-400">
                {passwordUpdated ? 'Sign in now' : 'Back to sign in'}
              </Text>
            </Pressable>
          </View>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
};

export default SettingsScreen;