import React, { useEffect, useMemo, useState } from 'react';
import { Platform, Pressable, Text, View } from 'react-native';
import type { NativeStackScreenProps } from '@react-navigation/native-stack';
import type { AuthStackParamList } from '../src/navigation/types';

type Props = NativeStackScreenProps<AuthStackParamList, 'Verify'>;

type VerificationState = 'loading' | 'success' | 'invalid';

function getBrowserFlowId(): string | null {
  if (Platform.OS !== 'web') return null;

  const search = globalThis.window?.location?.search ?? '';
  const params = new URLSearchParams(search);
  const flow = params.get('flow');

  return flow && flow.trim().length > 0 ? flow : null;
}

export default function VerifyScreen({ navigation }: Props): React.JSX.Element {
  const [state, setState] = useState<VerificationState>('loading');

  const flowId = useMemo(() => getBrowserFlowId(), []);

  useEffect(() => {
    if (!flowId) {
      setState('invalid');
      return;
    }

    // Placeholder only.
    // Future Kratos verification completion logic belongs here.
    setState('success');
  }, [flowId]);

  return (
    <View className="flex-1 bg-slate-950 px-6 py-10 justify-center">
      <View className="rounded-xl border border-slate-800 bg-slate-900/80 p-6">
        <Text className="text-2xl font-semibold text-white mb-3">
          Email verification
        </Text>

        {state === 'loading' && (
          <Text className="text-slate-300">
            Checking your verification link…
          </Text>
        )}

        {state === 'success' && (
          <View className="rounded-xl border border-emerald-500 bg-emerald-950/60 p-4">
            <Text className="text-white font-semibold mb-2">
              Verification link received
            </Text>
            <Text className="text-slate-300">
              Your email verification flow is ready. You can continue by signing
              in.
            </Text>
          </View>
        )}

        {state === 'invalid' && (
          <View className="rounded-xl border border-red-500 bg-red-950/60 p-4">
            <Text className="text-white font-semibold mb-2">
              This verification link is no longer valid
            </Text>
            <Text className="text-slate-300">
              The link may have expired or may already have been used. Please
              return to sign in and start again.
            </Text>
          </View>
        )}

        <Pressable
          className="mt-6 rounded-xl bg-sky-400 px-4 py-3"
          onPress={() => navigation.navigate('Login')}
        >
          <Text className="text-center font-semibold text-slate-950">
            Return to sign in
          </Text>
        </Pressable>
      </View>
    </View>
  );
}