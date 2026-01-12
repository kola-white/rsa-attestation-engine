import React, { useCallback, useState } from "react";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { AuthStackParamList } from "@/src/navigation/types";
import {
  View,
  Text,
  TextInput,
  Pressable,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
} from "react-native";
import { AuthError } from "@/src/auth/types";
import type { KratosRecoveryFlow, KratosErrorResponse } from "@/src/auth/kratosTypes";
import { KratosFormError } from "@/src/auth/kratosTypes";

const KRATOS_BASE_URL = "https://auth.cvera.app";

type AuthNav = NativeStackNavigationProp<AuthStackParamList, "ForgotPassword">;

const isValidEmail = (value: string): boolean => {
  const email = value.trim();
  if (!email) return false;
  return /\S+@\S+\.\S+/.test(email);
};

const extractRecoveryMessage = (data: KratosErrorResponse): string => {
  const msgs = data.ui?.messages;
  if (!msgs || msgs.length === 0) {
    return "We couldn’t start password recovery. Please try again.";
  }
  return msgs[0]?.text ?? "We couldn’t start password recovery. Please try again.";
};

export const RecoveryScreen: React.FC = () => {
  const navigation = useNavigation<AuthNav>();

  const [email, setEmail] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);

  const handleSubmit = useCallback(async () => {
    const normalizedEmail = email.trim().toLowerCase();

    setError(null);
    setInfo(null);

    if (!normalizedEmail || !isValidEmail(normalizedEmail)) {
      setError("Please check your email address and try again.");
      return;
    }

    setIsSubmitting(true);

    try {
      // 1) Create recovery flow
      const flowRes = await fetch(`${KRATOS_BASE_URL}/self-service/recovery/api`, {
        method: "GET",
        headers: { Accept: "application/json" },
      });

      if (!flowRes.ok) {
        const text = await flowRes.text();
        throw new AuthError(
          "create_recovery_flow_failed",
          "Unable to start password recovery. Please try again.",
          text
        );
      }

      const flow = (await flowRes.json()) as KratosRecoveryFlow;

      // 2) Submit email to flow
      const submitRes = await fetch(flow.ui.action, {
        method: flow.ui.method,
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json; charset=utf-8",
        },
        body: JSON.stringify({
          method: "code", // common Kratos recovery method; server decides actual behavior
          email: normalizedEmail,
        }),
      });

      if (submitRes.status === 400) {
        const data = (await submitRes.json()) as KratosErrorResponse;
        const msg = extractRecoveryMessage(data);
        throw new KratosFormError(msg);
      }

      if (!submitRes.ok) {
        const text = await submitRes.text();
        throw new AuthError(
          "recovery_submit_failed",
          "We couldn`t send recovery instructions. Please try again.",
          text
        );
      }

      await submitRes.json().catch(() => null);
      
      // For now: minimal UX
      // Always show the “safe” message (don’t confirm account existence)
      setInfo("If that email exists, you`ll receive password reset instructions shortly.");
    } catch (e) {
      if (e instanceof KratosFormError) setError(e.message);
      else if (e instanceof AuthError) setError(e.message);
      else if (e instanceof Error) setError(e.message || "Something went wrong. Please try again.");
      else setError("Something went wrong. Please try again.");
    } finally {
      setIsSubmitting(false);
    }
  }, [email]);

  return (
    <KeyboardAvoidingView
      className="flex-1 bg-slate-950"
      behavior={Platform.OS === "ios" ? "padding" : undefined}
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
              editable={!isSubmitting}
              value={email}
              onChangeText={setEmail}
            />
          </View>

          <Pressable
            className="mt-2 h-11 rounded-xl bg-sky-400 items-center justify-center disabled:opacity-50"
            disabled={isSubmitting}
            onPress={handleSubmit}
          >
            {isSubmitting ? (
              <ActivityIndicator size="small" />
            ) : (
              <Text className="text-base font-semibold text-slate-950">
                Reset password
              </Text>
            )}
          </Pressable>

          <View className="mt-6 flex-row justify-center">
            <Pressable disabled={isSubmitting} onPress={() => navigation.navigate("Login")}>
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
