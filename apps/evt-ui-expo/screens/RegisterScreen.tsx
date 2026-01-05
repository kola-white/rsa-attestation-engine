import React, { useCallback, useState } from 'react';
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
} from 'react-native';
import { useAuth } from '../src/auth/AuthContext'; // adjust path if needed

// If you keep KratosFormError in AuthContext, re-export it there, or just
// re-check by name string here. For simplicity, we’ll import the class.
import { KratosFormError } from '../src/auth/AuthContext'; // ensure you export it

type RegisterFormState = {
  fullName: string;
  email: string;
  password: string;
};

const initialFormState: RegisterFormState = {
  fullName: '',
  email: '',
  password: '',
};

const isValidEmail = (value: string): boolean => {
  const email = value.trim();
  if (!email) return false;
  // simple shape check is enough; Kratos enforces real validation
  return /\S+@\S+\.\S+/.test(email);
};

export const RegisterScreen: React.FC = () => {
type AuthNav = NativeStackNavigationProp<AuthStackParamList, "Register">;
const navigation = useNavigation<AuthNav>();
  const { register } = useAuth();

  const [form, setForm] = useState<RegisterFormState>(initialFormState);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleChange = useCallback(
    (field: keyof RegisterFormState, value: string) => {
      setForm((prev) => ({
        ...prev,
        [field]: value,
      }));
    },
    [],
  );

  const handleSubmit = useCallback(async () => {
    const fullName = form.fullName.trim();
    const email = form.email.trim().toLowerCase();
    const password = form.password;

    // Basic client-side validation
    if (!email || !isValidEmail(email)) {
      setError('Please check your email address and try again.');
      return;
    }

    if (!password) {
      setError('Please enter a password.');
      return;
    }

    setError(null);
    setIsSubmitting(true);

    try {
      await register({ email, password, fullName });

      // On success, the user is fully authenticated via AuthContext.
      // Navigation: replace this with your real navigation call:
      // navigation.reset({ index: 0, routes: [{ name: 'Home' }] });
    } catch (err) {
      if (err instanceof KratosFormError) {
        setError(err.message);
      } else if (err instanceof Error) {
        // Map low-level errors to HR-friendly copy
        if (err.message.toLowerCase().includes('already in use')) {
          setError('This email is already in use. Try signing in instead.');
        } else {
          setError(
            'We could not create your account. Please check your details and try again.',
          );
        }
      } else {
        setError('Something went wrong. Please try again.');
      }
    } finally {
      setIsSubmitting(false);
    }
  }, [form, register]);

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
            Create account
          </Text>
          <Text className="text-base text-slate-300 mb-6">
            HR access to employment verification cases.
          </Text>

          {error && (
            <View className="mb-4 rounded-xl border border-red-500 bg-red-950/60 px-4 py-3">
              <Text className="text-sm text-red-100">{error}</Text>
            </View>
          )}

          {/* Full name (optional) */}
          <View className="mb-4">
            <Text className="mb-1 text-sm font-medium text-slate-200">
              Full name (optional)
            </Text>
            <TextInput
              className="h-11 rounded-xl border border-slate-700 bg-slate-900 px-3 text-sm text-slate-100"
              placeholder="Jane Doe"
              placeholderTextColor="#64748b"
              autoCapitalize="words"
              autoCorrect={false}
              editable={!isSubmitting}
              value={form.fullName}
              onChangeText={(text) => handleChange('fullName', text)}
            />
          </View>

          {/* Work email */}
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
              value={form.email}
              onChangeText={(text) => handleChange('email', text)}
            />
          </View>

          {/* Password */}
          <View className="mb-2">
            <Text className="mb-1 text-sm font-medium text-slate-200">
              Password
            </Text>
            <TextInput
              className="h-11 rounded-xl border border-slate-700 bg-slate-900 px-3 text-sm text-slate-100"
              placeholder="Enter a secure password"
              placeholderTextColor="#64748b"
              secureTextEntry
              autoCapitalize="none"
              autoCorrect={false}
              editable={!isSubmitting}
              value={form.password}
              onChangeText={(text) => handleChange('password', text)}
            />
          </View>

          <Text className="text-xs text-slate-500 mb-4">
            Your password policy is enforced by your organization&apos;s
            security standards.
          </Text>

          {/* Submit button */}
          <Pressable
            className="mt-2 h-11 rounded-xl bg-sky-500 items-center justify-center disabled:opacity-50"
            disabled={isSubmitting}
            onPress={handleSubmit}
          >
            {isSubmitting ? (
              <ActivityIndicator size="small" />
            ) : (
              <Text className="text-sm font-semibold text-white">
                Create account
              </Text>
            )}
          </Pressable>

          {/* Footer: link back to Login */}
          <View className="mt-6 flex-row justify-center">
            <Text className="text-sm text-slate-400">
              Already have an account?{' '}
            </Text>
            <Pressable
              onPress={() => {
                // TODO: wire to your navigation stack, e.g.:
                // navigation.navigate('Login');
              }}
            >
            <Pressable onPress={() => navigation.navigate("Login")}>
                <Text className="text-sm font-semibold text-sky-400">Sign in</Text>
            </Pressable>
            </Pressable>
          </View>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
};
