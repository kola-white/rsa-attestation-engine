// /apps/evt-ui-expo/screens/SessionExpiredScreen.tsx
import React from "react";
import { View, Text, Pressable } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useAuth } from "@/src/auth/AuthContext";

export const SessionExpiredScreen: React.FC = () => {
  const { beginReauth } = useAuth();

  return (
    <SafeAreaView className="flex-1 bg-white">
      <View className="flex-1 px-6 justify-center">
        {/* Optional lock image (core-only): large emoji is reliable on iOS, no extra deps */}
        <Text
          className="text-6xl text-center mb-6"
          accessibilityRole="image"
          accessibilityLabel="Locked"
        >
          🔒
        </Text>

        <Text
          className="text-3xl font-semibold text-center text-slate-900"
          accessibilityRole="header"
        >
          Session expired
        </Text>

        <Text className="mt-3 text-base text-center text-slate-600 leading-6">
          For your security, you need to sign in again to continue.
        </Text>

        <Pressable
          onPress={beginReauth}
          accessibilityRole="button"
          accessibilityLabel="Log back in now"
          accessibilityHint="Returns to the sign-in screen"
          hitSlop={12}
          className="mt-10 h-12 rounded-xl bg-slate-900 items-center justify-center"
        >
          <Text className="text-base font-semibold text-white">
            Log back in now
          </Text>
        </Pressable>

        {/* iOS tap target + readability: keep a little bottom breathing room */}
        <View className="h-6" />
      </View>
    </SafeAreaView>
  );
};
