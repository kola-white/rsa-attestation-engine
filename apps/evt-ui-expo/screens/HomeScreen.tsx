// src/screens/HomeScreen.tsx
import React from "react";
import { View, Text, Pressable } from "react-native";
import { useAuth } from "@/src/auth/AuthContext";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";
import type { AppStackParamList } from "@/src/navigation/MainAppNavigator";

type AppNav = NativeStackNavigationProp<AppStackParamList, "Home">;

export const HomeScreen: React.FC = () => {
  const { user, logout } = useAuth();
  const navigation = useNavigation<AppNav>();

  return (
    <View className="flex-1 bg-slate-950 px-6 justify-center">
      <Text className="text-3xl font-semibold text-white mb-2">
        Welcome
      </Text>

      <Text className="text-base text-slate-300 mb-6">
        {user?.email ?? "Signed in user"}
      </Text>

      <Pressable
        className="h-12 items-center justify-center rounded-xl bg-emerald-500 active:bg-emerald-600 mb-3"
        onPress={() => navigation.navigate("HRReview")}
      >
        <Text className="text-base font-semibold text-slate-950">
          Go to HR Review
        </Text>
      </Pressable>

      <Pressable
        className="h-12 items-center justify-center rounded-xl bg-slate-700 active:bg-slate-800"
        onPress={logout}
      >
        <Text className="text-base font-semibold text-white">
          Sign out
        </Text>
      </Pressable>
    </View>
  );
};
