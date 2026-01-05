// AppShell.tsx
import React, { useEffect, useState } from "react";
import { View, ActivityIndicator } from "react-native";
import { useAuth } from "@/src/auth/AuthContext";
import { runBiometricCheck } from "@/src/auth/biometrics";
import { AuthNavigator } from "@/src/navigation/AuthNavigator";
import { MainAppNavigator } from "@/src/navigation/MainAppNavigator";


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
      // In Phase 1, if biometric fails just drop back to login:
     // You can call logout() here once you're ready.
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
    <View
      style={{
        flex: 1,
        alignItems: "center",
        justifyContent: "center",
      }}
    >
      <ActivityIndicator />
    </View>
  );
}
console.log("[AppShell] AUTH STATUS", status);

if (status === "unauthenticated") {
  console.log("[AppShell] AUTH STATUS unauthenticated (AuthNavigator)");
  return <AuthNavigator />;
}

return <MainAppNavigator />;
};
