import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { AuthStackParamList } from "@/src/navigation/types";
import React, { useState } from 'react';
import {
  View,
  Text,
  TextInput,
  Pressable,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
} from 'react-native';
import { useAuth } from '../src/auth/AuthContext';
import { AuthError } from '../src/auth/types';

export const LoginScreen: React.FC = () => {
  const { login, status } = useAuth();
  type AuthNav = NativeStackNavigationProp<AuthStackParamList, "Login">;
  const navigation = useNavigation<AuthNav>();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onSubmit = async () => {
    setError(null);

    const trimmedEmail = email.trim();
    console.log("[LoginScreen] submit", { email, trimmedEmail, passwordLen: password.length });
    if (!trimmedEmail || !password) {
      setError('Please enter both email and password.');
      return;
    }

    if (status === "authenticated") {
      console.log("[LoginScreen] blocked login — already authenticated");
      return;
    }

    setSubmitting(true);
    try {
      await login(trimmedEmail.toLowerCase(), password);
      // AuthContext will move status to "authenticated" on success.
    } catch (e) {
      if (e instanceof AuthError) {
        setError(e.message);
      } else if (e instanceof Error) {
        setError(e.message || 'Sign-in failed. Please try again.');
      } else {
        setError('Sign-in failed. Please try again.');
      }
    } finally {
      setSubmitting(false);
    }
  };

  const isBusy = submitting;

  return (
    <KeyboardAvoidingView
    className="flex-1 bg-slate-950"
    behavior={Platform.OS === "ios" ? "padding" : undefined}
  >
    <View
      className="flex-1 px-6 justify-center"
    >
      <Text
        className="text-3xl font-semibold text-white mb-2"
      >
        Sign in
      </Text>

      <Text className="text-base text-slate-300 mb-6" >
        HR access to employment verification cases.
      </Text>

      {error && (
        <View className="mb-4 rounded-xl border border-red-500 bg-red-950/60 px-4 py-3">
          <Text className="text-sm text-red-100">{error}</Text>
        </View>
      )}
        <View className="mb-4">
          <Text className="text-sm text-slate-200 mb-2">Work email</Text>
          <TextInput
            autoCapitalize="none"
            keyboardType="email-address"
            autoCorrect={false}
            placeholder="hr@company.com"
            placeholderTextColor="#64748b"
            value={email}
            onChangeText={(t) => setEmail(t)}
            editable={!isBusy}
            className="w-full rounded-xl border border-slate-700 bg-slate-900 px-4 py-3 text-white"
          />
        </View>

        <View className="mb-6">
          <Text className="text-sm text-slate-200 mb-2">Password</Text>
          <TextInput
            secureTextEntry
            placeholder="Enter your password"
            placeholderTextColor="#64748b"
            value={password}
            onChangeText={(t) => setPassword(t)}
            editable={!isBusy}
            className="w-full rounded-xl border border-slate-700 bg-slate-900 px-4 py-3 text-white"
          />
        </View>

        <Pressable
          onPress={onSubmit}
          disabled={isBusy}
          className={`h-12 flex-row items-center justify-center rounded-xl ${
            isBusy
              ? 'bg-slate-700'
              : 'bg-sky-400 active:bg-sky-600'
          }`}
        >
          {isBusy ? (
            <ActivityIndicator />
          ) : (
            <Text className="text-base font-semibold text-slate-950">
              Sign in
            </Text>
          )}
        </Pressable>

        {/* Forgot password / help (wired later to Kratos recovery flow) */}
        <Pressable
          className="mt-4"
          onPress={() => {
            navigation.navigate("ForgotPassword")
          }}
        >
          <Text className="text-sm text-sky-400">
            Forgot your password?
          </Text>
        </Pressable>
      </View>
      <View className="mt-6 flex-row justify-center">
        <Text className="text-sm text-slate-400">New here? </Text>
        <Pressable onPress={() => navigation.navigate("Register")}>
            <Text className="text-sm font-semibold text-sky-400">
            Create an account
            </Text>
        </Pressable>
        </View>
    </KeyboardAvoidingView>
  );
};
