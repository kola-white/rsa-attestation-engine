// /apps/evt-ui-expo/AppShell.tsx
import React, { useEffect, useState } from "react";
import { View, ActivityIndicator } from "react-native";
import { useAuth } from "@/src/auth/AuthContext";
import { runBiometricCheck } from "@/src/auth/biometrics";
import { AuthNavigator } from "@/src/navigation/AuthNavigator";
import { MainAppNavigator } from "@/src/navigation/MainAppNavigator";
import { SessionExpiredScreen } from "screens/SessionExpiredScreen";

export const AppShell: React.FC = () => {
  console.log("[AppShell] render");
  const { status } = useAuth();
  const [biometricGateDone, setBiometricGateDone] = useState(false);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      // TODO: read biometrics_enabled flag from SecureStore
      const biometricsEnabled = true;

      if (!biometricsEnabled || status !== "authenticated") {
        if (!cancelled) setBiometricGateDone(true);
        return;
      }

      const ok = await runBiometricCheck();
      // Phase 1: proceed even if biometric fails (you can enforce later)
      if (!cancelled) setBiometricGateDone(true);
    })();

    return () => {
      cancelled = true;
    };
  }, [status]);

  const shouldBlockForBiometrics = status === "authenticated" && !biometricGateDone;

  if (status === "checking" || shouldBlockForBiometrics) {
    console.log("[AppShell] LOADING/GATE", { status, biometricGateDone });

    return (
      <View className="flex-1 items-center justify-center bg-white">
        <ActivityIndicator />
      </View>
    );
  }

  console.log("[AppShell] AUTH STATUS", status);

  if (status === "session-expired") {
    console.log("[AppShell] AUTH STATUS session_expired (SessionExpiredScreen)");
    return <SessionExpiredScreen />;
  }

  if (status === "unauthenticated") {
    console.log("[AppShell] AUTH STATUS unauthenticated (AuthNavigator)");
    return <AuthNavigator />;
  }

  return <MainAppNavigator />;
};
