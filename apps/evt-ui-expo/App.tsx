import "./global.css";
import React, { useEffect } from "react";
import { View, useColorScheme } from "react-native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import * as SystemUI from "expo-system-ui";
import { NavigationContainer } from "@react-navigation/native";
import { AuthProvider } from "@/src/auth/AuthContext";
import { AppShell } from "@/AppShell";


export default function App() {
  const colorScheme = useColorScheme() ?? "dark";

  useEffect(() => {
    void SystemUI.setBackgroundColorAsync(
      colorScheme === "dark" ? "#000000" : "#ffffff",
    );
  }, [colorScheme]);

  return (
    <SafeAreaProvider>
      <View
        style={{
          flex: 1, backgroundColor: colorScheme === "dark" ? "#000000" : "#18181b",
        }}
      >
        <StatusBar style={colorScheme === "dark" ? "light" : "dark"} />

        <AuthProvider>
          <NavigationContainer>
            <AppShell />
          </NavigationContainer>
        </AuthProvider>
      </View>
    </SafeAreaProvider>
  );
}
// --- END OF FILE ---
