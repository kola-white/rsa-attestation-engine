import React from 'react';
import { Pressable, Text, View } from 'react-native';
import type { NativeStackScreenProps } from '@react-navigation/native-stack';
import type { AuthStackParamList } from '../src/navigation/types';

type Props = NativeStackScreenProps<AuthStackParamList, 'AuthError'>;

export default function AuthErrorScreen({
  navigation,
}: Props): React.JSX.Element {
  return (
    <View className="flex-1 bg-slate-950 px-6 py-10 justify-center">
      <View className="rounded-xl border border-slate-800 bg-slate-900/80 p-6">
        <Text className="text-2xl font-semibold text-white mb-3">
          Authentication link expired
        </Text>

        <View className="rounded-xl border border-red-500 bg-red-950/60 p-4">
          <Text className="text-white font-semibold mb-2">
            We could not continue this authentication flow
          </Text>
          <Text className="text-slate-300">
            For your security, this link may have expired or can no longer be
            used. Please start again.
          </Text>
        </View>

        <Pressable
          className="mt-6 rounded-xl bg-sky-400 px-4 py-3"
          onPress={() => navigation.navigate('ForgotPassword')}
        >
          <Text className="text-center font-semibold text-slate-950">
            Try again
          </Text>
        </Pressable>

        <Pressable
          className="mt-3 rounded-xl border border-slate-700 px-4 py-3"
          onPress={() => navigation.navigate('Login')}
        >
          <Text className="text-center font-semibold text-white">
            Return to sign in
          </Text>
        </Pressable>
      </View>
    </View>
  );
}