import { Platform } from "react-native";
if (Platform.OS === "web") {
  require("./global.css");
}
import { SafeAreaProvider } from "react-native-safe-area-context";

import ErrorBoundary from "./components/ErrorBoundary";
import HRReviewScreen from "./screens/HRReviewScreen";
import HRReviewScreenSettingsStyle from "./screens/HRReviewScreen.SettingsStyle";

const USE_SETTINGS_STYLE = true;

export default function App() {
  return (
    <SafeAreaProvider>
      {USE_SETTINGS_STYLE ? (
        <ErrorBoundary title="NativeWind screen crashed">
          <HRReviewScreenSettingsStyle />
        </ErrorBoundary>
      ) : (
        <HRReviewScreen />
      )}
    </SafeAreaProvider>
  );
}
