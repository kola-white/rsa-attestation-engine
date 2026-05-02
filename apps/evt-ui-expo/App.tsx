import "./global.css"; // Must be first import to load env variables
import { DefaultTheme, DarkTheme } from "@react-navigation/native";
import React, { useEffect } from "react";
import { View, useColorScheme } from "react-native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import * as SystemUI from "expo-system-ui";
import { NavigationContainer } from "@react-navigation/native";
import { AuthProvider } from "@/src/auth/AuthContext";
import { AppShell } from "@/AppShell";


export default function App() {
  console.log("EVT API BASE URL:", process.env.EXPO_PUBLIC_EVT_API_BASE_URL);
  console.log("KRATOS BASE URL:", process.env.EXPO_PUBLIC_KRATOS_BASE_URL);
  const colorScheme = useColorScheme() ?? "dark";

  useEffect(() => {
    void SystemUI.setBackgroundColorAsync(
      colorScheme === "dark" ? "#000000" : "#ffffff"
    );
  }, [colorScheme]);

  return (
    <SafeAreaProvider>
      <View
        style={{
          flex: 1,
          backgroundColor: colorScheme === "dark" ? "#000000" : "#ffffff", // ✅ fix
        }}
      >
        <StatusBar style={colorScheme === "dark" ? "light" : "dark"} />

        <AuthProvider>
          <NavigationContainer theme={colorScheme === "dark" ? DarkTheme : DefaultTheme}>
            <AppShell />
          </NavigationContainer>
        </AuthProvider>
      </View>
    </SafeAreaProvider>
  );
}
