import "./global.css";
import { View, useColorScheme } from "react-native";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { StatusBar } from "expo-status-bar";
import HRReviewScreenSettingsStyle from "./screens/HRReviewScreen.SettingsStyle";

export default function App() {
  const colorScheme = useColorScheme() ?? `light`;
  return (
    <SafeAreaProvider>
      <View className={`${colorScheme === "dark" ? "dark" : ""} flex-1`}>
        <StatusBar style={colorScheme === "dark" ? "light" : "dark"} />
      <HRReviewScreenSettingsStyle />
      </View>
    </SafeAreaProvider>
  );
}
