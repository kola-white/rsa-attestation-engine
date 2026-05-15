import React, { useCallback, useState } from 'react';
import { useNavigation } from '@react-navigation/native';
import type { NativeStackNavigationProp } from '@react-navigation/native-stack';
import type { AuthStackParamList } from '@/src/navigation/types';
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
import { useAuth } from '../src/auth/AuthContext';
import { AuthError, KratosFormError } from '../src/auth/AuthContext';

type AuthNav = NativeStackNavigationProp<AuthStackParamList, 'Register'>;

type RegisterFormState = {
  fullName: string;
  email: string;
  password: string;
};

type RegisterNotice =
  | { kind: 'success'; message: string }
  | { kind: 'error'; message: string }
  | null;

const initialFormState: RegisterFormState = {
  fullName: '',
  email: '',
  password: '',
};

const isValidEmail = (value: string): boolean => {
  const email = value.trim();
  if (!email) return false;
  return /\S+@\S+\.\S+/.test(email);
};

export const RegisterScreen: React.FC = () => {
  const navigation = useNavigation<AuthNav>();
  const { register } = useAuth();

  const [form, setForm] = useState<RegisterFormState>(initialFormState);
  const [notice, setNotice] = useState<RegisterNotice>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleChange = useCallback(
    (field: keyof RegisterFormState, value: string) => {
      setForm((prev) => ({
        ...prev,
        [field]: value,
      }));

      if (notice) {
        setNotice(null);
      }
    },
    [notice],
  );

  const handleSubmit = useCallback(async () => {
    const fullName = form.fullName.trim();
    const email = form.email.trim().toLowerCase();
    const password = form.password;

    if (!email || !isValidEmail(email)) {
      setNotice({
        kind: 'error',
        message: 'Please check your email address and try again.',
      });
      return;
    }

    if (!password) {
      setNotice({
        kind: 'error',
        message: 'Please enter a password.',
      });
      return;
    }

    setNotice(null);
    setIsSubmitting(true);

    try {
      await register({ email, password, fullName });

      setNotice({
        kind: 'success',
        message: 'Account created. Please sign in with your new account.',
      });

      setForm((prev) => ({
        ...prev,
        password: '',
      }));
    } catch (err) {
      if (
        err instanceof AuthError &&
        err.code === 'registration_requires_login'
      ) {
        setNotice({
          kind: 'success',
          message: 'Account created. Please sign in with your new account.',
        });

        setForm((prev) => ({
          ...prev,
          password: '',
        }));

        return;
      }

      if (err instanceof KratosFormError) {
        setNotice({ kind: 'error', message: err.message });
      } else if (err instanceof Error) {
        if (err.message.toLowerCase().includes('already in use')) {
          setNotice({
            kind: 'error',
            message: 'This email is already in use. Try signing in instead.',
          });
        } else {
          setNotice({
            kind: 'error',
            message:
              'We could not create your account. Please check your details and try again.',
          });
        }
      } else {
        setNotice({
          kind: 'error',
          message: 'Something went wrong. Please try again.',
        });
      }
    } finally {
      setIsSubmitting(false);
    }
  }, [form, register]);

  const noticeStyles =
    notice?.kind === 'success'
      ? 'border-emerald-500 bg-emerald-950/60'
      : 'border-red-500 bg-red-950/60';

  const noticeTextStyles =
    notice?.kind === 'success' ? 'text-emerald-100' : 'text-red-100';

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

          {notice && (
            <View className={`mb-4 rounded-xl border px-4 py-3 ${noticeStyles}`}>
              <Text className={`text-sm ${noticeTextStyles}`}>
                {notice.message}
              </Text>
            </View>
          )}

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

          <Pressable
            className="mt-2 h-11 rounded-xl bg-sky-400 items-center justify-center disabled:opacity-50"
            disabled={isSubmitting}
            onPress={handleSubmit}
          >
            {isSubmitting ? (
              <ActivityIndicator size="small" />
            ) : (
              <Text className="text-base font-semibold text-slate-950">
                Create account
              </Text>
            )}
          </Pressable>

          <View className="mt-6 flex-row justify-center">
            <Text className="text-sm text-slate-400">
              Already have an account?{' '}
            </Text>
            <Pressable onPress={() => navigation.navigate('Login')}>
              <Text className="text-sm font-semibold text-sky-400">
                Sign in
              </Text>
            </Pressable>
          </View>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
};