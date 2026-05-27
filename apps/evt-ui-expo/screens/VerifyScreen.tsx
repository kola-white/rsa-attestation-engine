import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ActivityIndicator,
  Platform,
  Pressable,
  Text,
  TextInput,
  View,
} from 'react-native';
import type { NativeStackScreenProps } from '@react-navigation/native-stack';
import type { AuthStackParamList } from '../src/navigation/types';

const KRATOS_BASE_URL = 'https://auth.cvera.app';

type Props = NativeStackScreenProps<AuthStackParamList, 'Verify'>;

type VerificationState =
  | 'loading'
  | 'enteringCode'
  | 'submitting'
  | 'success'
  | 'invalid';

type KratosUiNode = {
  attributes?: {
    name?: string;
    value?: string;
  };
};

type KratosVerificationFlow = {
  id: string;
  ui: {
    action: string;
    method?: string;
    nodes: KratosUiNode[];
  };
};

function getBrowserFlowId(routeFlow?: string): string | null {
  if (routeFlow && routeFlow.trim().length > 0) {
    return routeFlow;
  }

  if (Platform.OS !== 'web') return null;

  const search = globalThis.window?.location?.search ?? '';
  const params = new URLSearchParams(search);
  const flow = params.get('flow');

  return flow && flow.trim().length > 0 ? flow : null;
}

function extractCsrfToken(flow: KratosVerificationFlow): string | null {
  const csrfNode = flow.ui.nodes.find(
    (node) => node.attributes?.name === 'csrf_token',
  );

  return csrfNode?.attributes?.value ?? null;
}

export default function VerifyScreen({
  navigation,
  route,
}: Props): React.JSX.Element {
  const [state, setState] = useState<VerificationState>('loading');
  const [flow, setFlow] = useState<KratosVerificationFlow | null>(null);
  const [code, setCode] = useState('');
  const [notice, setNotice] = useState<string | null>(null);

  const flowId = useMemo(() => getBrowserFlowId(route.params?.flow), [route]);

  useEffect(() => {
    if (!flowId || Platform.OS !== 'web') {
      setState('invalid');
      return;
    }

    let cancelled = false;

    const loadFlow = async () => {
      try {
        const res = await fetch(
          `${KRATOS_BASE_URL}/self-service/verification/flows?id=${encodeURIComponent(
            flowId,
          )}`,
          {
            method: 'GET',
            headers: {
              Accept: 'application/json',
            },
            credentials: 'include',
          },
        );

        if (!res.ok) {
          setState('invalid');
          return;
        }

        const json = (await res.json()) as KratosVerificationFlow;

        if (cancelled) return;

        setFlow(json);
        setState('enteringCode');
      } catch {
        if (!cancelled) {
          setState('invalid');
        }
      }
    };

    void loadFlow();

    return () => {
      cancelled = true;
    };
  }, [flowId]);

  const submitCode = useCallback(async () => {
    if (!flow) return;

    const trimmedCode = code.trim();

    if (!trimmedCode) {
      setNotice('Enter the verification code from your email.');
      return;
    }

    const csrfToken = extractCsrfToken(flow);

    if (!csrfToken) {
      setNotice('Unable to verify this session. Please restart verification.');
      return;
    }

    setNotice(null);
    setState('submitting');

    try {
      const res = await fetch(flow.ui.action, {
        method: flow.ui.method ?? 'POST',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json; charset=utf-8',
          'X-CSRF-Token': csrfToken,
        },
        credentials: 'include',
        body: JSON.stringify({
          method: 'code',
          code: trimmedCode,
          csrf_token: csrfToken,
        }),
      });

      if (res.ok) {
        setState('success');
        return;
      }

      setState('enteringCode');
      setNotice('That verification code did not work. Please check it and try again.');
    } catch {
      setState('enteringCode');
      setNotice('Verification failed. Please try again.');
    }
  }, [code, flow]);

  return (
    <View className="flex-1 bg-slate-950 px-6 py-10 justify-center">
      <View className="rounded-xl border border-slate-800 bg-slate-900/80 p-6">
        <Text className="text-2xl font-semibold text-white mb-3">
          Email verification
        </Text>

        {state === 'loading' && (
          <View className="flex-row items-center">
            <ActivityIndicator />
            <Text className="ml-3 text-slate-300">
              Loading your verification flow…
            </Text>
          </View>
        )}

        {(state === 'enteringCode' || state === 'submitting') && (
          <>
            <Text className="text-slate-300 mb-4">
              Enter the verification code from your Cvera email.
            </Text>

            {notice && (
              <View className="mb-4 rounded-xl border border-amber-500 bg-amber-950/60 p-4">
                <Text className="text-amber-100">{notice}</Text>
              </View>
            )}

            <Text className="text-sm text-slate-200 mb-2">
              Verification code
            </Text>

            <TextInput
              value={code}
              onChangeText={setCode}
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="number-pad"
              placeholder="Enter code"
              placeholderTextColor="#64748b"
              editable={state !== 'submitting'}
              className="mb-4 w-full rounded-xl border border-slate-700 bg-slate-950 px-4 py-3 text-white"
            />

            <Pressable
              className={`rounded-xl px-4 py-3 ${
                state === 'submitting' ? 'bg-slate-700' : 'bg-sky-400'
              }`}
              disabled={state === 'submitting'}
              onPress={submitCode}
            >
              <Text className="text-center font-semibold text-slate-950">
                {state === 'submitting' ? 'Verifying…' : 'Verify email'}
              </Text>
            </Pressable>
          </>
        )}

        {state === 'success' && (
          <View className="rounded-xl border border-emerald-500 bg-emerald-950/60 p-4">
            <Text className="text-white font-semibold mb-2">
              Email verified
            </Text>
            <Text className="text-slate-300">
              Your email address has been verified. You can now sign in.
            </Text>
          </View>
        )}

        {state === 'invalid' && (
          <View className="rounded-xl border border-red-500 bg-red-950/60 p-4">
            <Text className="text-white font-semibold mb-2">
              This verification session is no longer valid
            </Text>
            <Text className="text-slate-300">
              The verification flow may have expired or already been used.
              Please return to sign in and start again.
            </Text>
          </View>
        )}

        {(state === 'success' || state === 'invalid') && (
          <Pressable
            className="mt-6 rounded-xl bg-sky-400 px-4 py-3"
            onPress={() => navigation.navigate('Login')}
          >
            <Text className="text-center font-semibold text-slate-950">
              Return to sign in
            </Text>
          </Pressable>
        )}
      </View>
    </View>
  );
}