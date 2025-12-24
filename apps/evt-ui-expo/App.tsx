import "./global.css";
import { View, useColorScheme } from "react-native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import * as SystemUI from "expo-system-ui";
import HRReviewScreenSettingsStyle from "./screens/HRReviewScreen.SettingsStyle";

export default function App() {
  const colorScheme = useColorScheme() ?? `light`;
  SystemUI.setBackgroundColorAsync(colorScheme === "dark" ? "#000000" : "#ffffff");

  return (
    <SafeAreaProvider>
      <View className={`${colorScheme === "dark" ? "dark" : ""} flex-1 bg-background`}
      style={{ backgroundColor: colorScheme === "dark" ? "#000000" : "#9ca3af" }}
      >
        <StatusBar style={colorScheme === "dark" ? "light" : "dark"} />
      <HRReviewScreenSettingsStyle />
      </View>
    </SafeAreaProvider>
  );
}
