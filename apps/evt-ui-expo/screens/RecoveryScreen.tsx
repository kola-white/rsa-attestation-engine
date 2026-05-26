import React, {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
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

import type { AuthStackParamList } from '@/src/navigation/types';
import { AuthError } from '@/src/auth/types';
import type { KratosRecoveryFlow } from '@/src/auth/kratosTypes';
import {
  recoveryReducer,
  type RecoveryUiState,
} from '@/src/auth/recoveryStateMachine';

const KRATOS_BASE_URL = 'https://auth.cvera.app';

type AuthNav = NativeStackNavigationProp<AuthStackParamList, 'ForgotPassword'>;

type KratosContinueWith =
  | {
      action: 'redirect_browser_to';
      redirect_browser_to: string;
    }
  | {
      action: 'show_settings_ui';
      flow?: {
        id?: string;
        url?: string;
      };
    }
  | {
      action: string;
      [key: string]: unknown;
    };

type KratosRecoveryCodeSubmitResponse =
  | KratosRecoveryFlow
  | {
      continue_with?: KratosContinueWith[];
      redirect_browser_to?: string;
    };

const getBrowserFlowId = (): string | null => {
  if (Platform.OS !== 'web') {
    return null;
  }

  if (typeof window === 'undefined') {
    return null;
  }

  return new URLSearchParams(window.location.search).get('flow');
};

const isValidEmail = (value: string): boolean => {
  const email = value.trim();
  return Boolean(email && /\S+@\S+\.\S+/.test(email));
};

const extractCsrfToken = (flow: KratosRecoveryFlow): string => {
  const csrfNode = flow.ui.nodes.find(
    (node) => node.attributes?.name === 'csrf_token',
  );

  const value = csrfNode?.attributes?.value;

  if (typeof value !== 'string' || value.length === 0) {
    throw new AuthError(
      'missing_csrf_token',
      'Unable to continue password recovery. Please restart recovery.',
    );
  }

  return value;
};

const hasRecoveryEmailNode = (flow: KratosRecoveryFlow): boolean => {
  return flow.ui.nodes.some((node) => node.attributes?.name === 'email');
};

const mapBrowserFlowToAction = (
  flow: KratosRecoveryFlow,
  email?: string,
):
  | {
      type: 'ENTER_EMAIL';
      flowId: string;
      canResend: boolean;
    }
  | {
      type: 'EMAIL_SENT';
      flowId: string;
      email?: string;
      canResend: boolean;
    }
  | {
      type: 'FLOW_EXPIRED';
      reason?: string;
    } => {
  if (flow.state === 'choose_method') {
    return {
      type: 'ENTER_EMAIL',
      flowId: flow.id,
      canResend: false,
    };
  }

  if (flow.state === 'sent_email') {
    return {
      type: 'EMAIL_SENT',
      flowId: flow.id,
      email,
      canResend: hasRecoveryEmailNode(flow),
    };
  }

  return {
    type: 'FLOW_EXPIRED',
    reason: `Unhandled recovery state: ${flow.state}`,
  };
};

const getContinueWithRedirect = (
  response: KratosRecoveryCodeSubmitResponse,
): string | null => {
  if ('redirect_browser_to' in response) {
    return typeof response.redirect_browser_to === 'string'
      ? response.redirect_browser_to
      : null;
  }

  if (!('continue_with' in response) || !Array.isArray(response.continue_with)) {
    return null;
  }

  for (const item of response.continue_with) {
    if (
      item.action === 'redirect_browser_to' &&
      typeof item.redirect_browser_to === 'string'
    ) {
      return item.redirect_browser_to;
    }
  }

  return null;
};



export const RecoveryScreen: React.FC = () => {
  const navigation = useNavigation<AuthNav>();

  const [uiState, dispatch] = useReducer(recoveryReducer, {
    type: 'idle',
  } satisfies RecoveryUiState);

  const [protocolFlow, setProtocolFlow] = useState<KratosRecoveryFlow | null>(
    null,
  );
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);

  const isWeb = Platform.OS === 'web';

  const browserFlowId = useMemo(() => {
    return getBrowserFlowId();
  }, []);

  const isBusy =
    uiState.type === 'bootstrappingBrowserFlow' ||
    uiState.type === 'loadingBrowserFlow' ||
    uiState.type === 'submittingEmail' ||
    uiState.type === 'submittingCode' ||
    uiState.type === 'restartingFlow';

  const loadBrowserRecoveryFlow = useCallback(
    async (flowId: string): Promise<void> => {
      try {
        const response = await fetch(
          `${KRATOS_BASE_URL}/self-service/recovery/flows?id=${encodeURIComponent(
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

        if (!response.ok) {
          dispatch({
            type: 'FLOW_EXPIRED',
            reason: `Flow load failed: ${response.status}`,
          });
          return;
        }

        const data = (await response.json()) as KratosRecoveryFlow;
        setProtocolFlow(data);
        dispatch(mapBrowserFlowToAction(data, email || undefined));
      } catch (e) {
        dispatch({
          type: 'FLOW_EXPIRED',
          reason:
            e instanceof Error ? e.message : 'Unknown recovery flow error.',
        });
      }
    },
    [email],
  );

  useEffect(() => {
    if (!isWeb) {
      dispatch({ type: 'USE_NATIVE_API_RECOVERY' });
      return;
    }

    if (!browserFlowId) {
      dispatch({ type: 'BOOTSTRAP_BROWSER_FLOW' });

      window.location.assign(
        `${KRATOS_BASE_URL}/self-service/recovery/browser`,
      );
      return;
    }

    dispatch({ type: 'LOAD_BROWSER_FLOW' });
    void loadBrowserRecoveryFlow(browserFlowId);
  }, [browserFlowId, isWeb, loadBrowserRecoveryFlow]);

  const submitBrowserRecoveryEmail = useCallback(
    async (normalizedEmail: string): Promise<void> => {
      if (!protocolFlow) {
        dispatch({
          type: 'FLOW_EXPIRED',
          reason: 'Recovery flow is missing.',
        });
        return;
      }

      const csrfToken = extractCsrfToken(protocolFlow);

      dispatch({
        type: 'SUBMIT_EMAIL',
        flowId: protocolFlow.id,
      });

      const response = await fetch(protocolFlow.ui.action, {
        method: 'POST',
        credentials: 'include',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json; charset=utf-8',
        },
        body: JSON.stringify({
          method: 'code',
          email: normalizedEmail,
          csrf_token: csrfToken,
        }),
      });

      if (response.status === 403 || response.status === 410) {
        dispatch({
          type: 'FLOW_EXPIRED',
          reason: `Recovery flow is no longer valid: ${response.status}`,
        });
        return;
      }

      const data = (await response.json().catch(() => null)) as
        | KratosRecoveryFlow
        | null;

      if (!data) {
        dispatch({
          type: 'FLOW_EXPIRED',
          reason: 'Recovery response was empty.',
        });
        return;
      }

      setProtocolFlow(data);
      setInfo('If that email exists, you’ll receive a recovery code shortly.');
      dispatch(mapBrowserFlowToAction(data, normalizedEmail));
    },
    [protocolFlow],
  );

  const submitNativeRecoveryEmail = useCallback(
    async (normalizedEmail: string): Promise<void> => {
      const flowResponse = await fetch(
        `${KRATOS_BASE_URL}/self-service/recovery/api`,
        {
          method: 'GET',
          headers: {
            Accept: 'application/json',
          },
        },
      );

      if (!flowResponse.ok) {
        throw new AuthError(
          'create_recovery_flow_failed',
          'Unable to start password recovery. Please try again.',
        );
      }

      const nativeFlow = (await flowResponse.json()) as KratosRecoveryFlow;

      const submitResponse = await fetch(nativeFlow.ui.action, {
        method: 'POST',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json; charset=utf-8',
        },
        body: JSON.stringify({
          method: 'code',
          email: normalizedEmail,
        }),
      });

      if (!submitResponse.ok && submitResponse.status !== 400) {
        throw new AuthError(
          'recovery_submit_failed',
          'We couldn’t send recovery instructions. Please try again.',
        );
      }

      setInfo(
        'If that email exists, you’ll receive password reset instructions shortly.',
      );
    },
    [],
  );

  const submitRecoveryEmail = useCallback(async (): Promise<void> => {
    const normalizedEmail = email.trim().toLowerCase();

    setError(null);
    setInfo(null);

    if (!isValidEmail(normalizedEmail)) {
      setError('Please check your email address and try again.');
      return;
    }

    try {
      if (isWeb) {
        await submitBrowserRecoveryEmail(normalizedEmail);
        return;
      }

      dispatch({ type: 'USE_NATIVE_API_RECOVERY' });
      await submitNativeRecoveryEmail(normalizedEmail);
    } catch (e) {
      console.warn('[RecoveryScreen] recovery email submit failed', e);
      setError(null);
      setInfo(
        'If that email exists, you’ll receive password reset instructions shortly.',
      );
    }
  }, [
    email,
    isWeb,
    submitBrowserRecoveryEmail,
    submitNativeRecoveryEmail,
  ]);

  const submitRecoveryCode = useCallback(async (): Promise<void> => {
    setError(null);
    setInfo(null);

    if (!isWeb) {
      return;
    }

    if (!protocolFlow) {
      dispatch({
        type: 'FLOW_EXPIRED',
        reason: 'Recovery flow is missing.',
      });
      return;
    }

    const trimmedCode = code.trim();

    if (!trimmedCode) {
      setError('Please enter the recovery code from your email.');
      return;
    }

    try {
      const csrfToken = extractCsrfToken(protocolFlow);

      dispatch({
        type: 'SUBMIT_CODE',
        flowId: protocolFlow.id,
      });

      const response = await fetch(protocolFlow.ui.action, {
        method: 'POST',
        credentials: 'include',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json; charset=utf-8',
        },
        body: JSON.stringify({
          method: 'code',
          code: trimmedCode,
          csrf_token: csrfToken,
        }),
      });

      if (response.status === 403 || response.status === 410) {
        dispatch({
          type: 'FLOW_EXPIRED',
          reason: `Recovery flow is no longer valid: ${response.status}`,
        });
        return;
      }

      if (!response.ok) {
        setError('That recovery code could not be accepted. Please try again.');
        dispatch({
          type: 'EMAIL_SENT',
          flowId: protocolFlow.id,
          email: email.trim().toLowerCase() || undefined,
          canResend: hasRecoveryEmailNode(protocolFlow),
        });
        return;
      }

      const data = (await response.json().catch(() => null)) as
        | KratosRecoveryCodeSubmitResponse
        | null;

      if (data) {
        const redirectUrl = getContinueWithRedirect(data);

        if (redirectUrl && Platform.OS === 'web') {
          window.location.assign(redirectUrl);
          return;
        }

        if ('ui' in data && 'id' in data && 'state' in data) {
          setProtocolFlow(data);
        }
      }

      dispatch({ type: 'FLOW_COMPLETE' });
      setInfo(
        'Recovery code accepted. Continue with the password reset step shown by Kratos.',
      );
    } catch (e) {
      console.warn('[RecoveryScreen] recovery code submit failed', e);
      dispatch({
        type: 'FLOW_EXPIRED',
        reason: e instanceof Error ? e.message : 'Recovery code failed.',
      });
    }
  }, [code, email, isWeb, protocolFlow]);

  const restartRecoveryFlow = useCallback((): void => {
    dispatch({ type: 'RESTART_FLOW' });

    if (Platform.OS === 'web') {
      window.location.assign(
        `${KRATOS_BASE_URL}/self-service/recovery/browser`,
      );
    }
  }, []);

  const showEmailForm =
    uiState.type === 'enteringEmail' ||
    uiState.type === 'nativeApiRecovery';

  const showCodeForm = uiState.type === 'emailSentEnterCode';

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
            Reset password
          </Text>

          <Text className="text-base text-slate-300 mb-6">
            Enter your work email and we’ll send reset instructions.
          </Text>

          {error && (
            <View className="mb-4 rounded-xl border border-red-500 bg-red-950/60 px-4 py-3">
              <Text className="text-sm text-red-100">{error}</Text>
            </View>
          )}

          {info && (
            <View className="mb-4 rounded-xl border border-slate-700 bg-slate-900 px-4 py-3">
              <Text className="text-sm text-slate-200">{info}</Text>
            </View>
          )}

          {isBusy && (
            <View className="mb-4 rounded-xl border border-slate-700 bg-slate-900 px-4 py-3">
              <View className="flex-row items-center">
                <ActivityIndicator size="small" />
                <Text className="ml-3 text-sm text-slate-200">
                  Preparing password recovery…
                </Text>
              </View>
            </View>
          )}

          {showEmailForm && (
            <>
              <View className="mb-4">
                <Text className="mb-1 text-sm font-medium text-slate-200">
                  Work email
                </Text>

                <TextInput
                  className="h-11 rounded-xl border border-slate-700 bg-slate-900 px-3 text-sm text-slate-100"
                  placeholder="name@company.com"
                  placeholderTextColor="#64748b"
                  autoCapitalize="none"
                  autoCorrect={false}
                  keyboardType="email-address"
                  editable={!isBusy}
                  value={email}
                  onChangeText={setEmail}
                />
              </View>

              <Pressable
                className="mt-2 h-11 rounded-xl bg-sky-400 items-center justify-center disabled:opacity-50"
                disabled={isBusy}
                onPress={submitRecoveryEmail}
              >
                {isBusy ? (
                  <ActivityIndicator size="small" />
                ) : (
                  <Text className="text-base font-semibold text-slate-950">
                    Reset password
                  </Text>
                )}
              </Pressable>
            </>
          )}

          {showCodeForm && (
            <>
              <View className="mb-4">
                <Text className="mb-1 text-sm font-medium text-slate-200">
                  Recovery code
                </Text>

                <TextInput
                  className="h-11 rounded-xl border border-slate-700 bg-slate-900 px-3 text-sm text-slate-100"
                  placeholder="Enter recovery code"
                  placeholderTextColor="#64748b"
                  autoCapitalize="none"
                  autoCorrect={false}
                  editable={!isBusy}
                  value={code}
                  onChangeText={setCode}
                />
              </View>

              <Pressable
                className="mt-2 h-11 rounded-xl bg-sky-400 items-center justify-center disabled:opacity-50"
                disabled={isBusy}
                onPress={submitRecoveryCode}
              >
                {isBusy ? (
                  <ActivityIndicator size="small" />
                ) : (
                  <Text className="text-base font-semibold text-slate-950">
                    Continue
                  </Text>
                )}
              </Pressable>

              {uiState.canResend && (
                <Pressable
                  className="mt-4 h-11 rounded-xl border border-slate-700 items-center justify-center disabled:opacity-50"
                  disabled={isBusy}
                  onPress={submitRecoveryEmail}
                >
                  <Text className="text-base font-semibold text-sky-400">
                    Resend code
                  </Text>
                </Pressable>
              )}
            </>
          )}

          {uiState.type === 'expiredOrInvalidFlow' && (
            <>
              <View className="mb-4 rounded-xl border border-amber-500 bg-amber-950/60 px-4 py-3">
                <Text className="text-sm text-amber-100">
                  This recovery session expired or is no longer valid. Please
                  restart password recovery.
                </Text>
              </View>

              <Pressable
                className="mt-2 h-11 rounded-xl bg-sky-400 items-center justify-center"
                onPress={restartRecoveryFlow}
              >
                <Text className="text-base font-semibold text-slate-950">
                  Restart recovery
                </Text>
              </Pressable>
            </>
          )}

          {uiState.type === 'complete' && (
            <View className="mb-4 rounded-xl border border-slate-700 bg-slate-900 px-4 py-3">
              <Text className="text-sm text-slate-200">
                Recovery code accepted. Continue with the next password reset
                step.
              </Text>
            </View>
          )}

          <View className="mt-6 flex-row justify-center">
            <Pressable
              disabled={isBusy}
              onPress={() => navigation.navigate('Login')}
            >
              <Text className="text-sm font-semibold text-sky-400">
                Back to sign in
              </Text>
            </Pressable>
          </View>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
};