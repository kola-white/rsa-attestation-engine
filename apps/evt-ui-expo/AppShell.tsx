import React, { useEffect, useState } from "react";
import { View, ActivityIndicator } from "react-native";
import { useAuth } from "@/src/auth/AuthContext";
import { runBiometricCheck } from "@/src/auth/biometrics";
import { AuthNavigator } from "@/src/navigation/AuthNavigator";
import { MainAppNavigator } from "@/src/navigation/MainAppNavigator";
import { SessionExpiredScreen } from "screens/SessionExpiredScreen";

function routeForRole(role: string | undefined): "Recruiter" | "HRReview" | "ReqHome" | "Home" {
  switch (role) {
    case "recruiter":
      return "Recruiter";
    case "hr_reviewer":
      return "HRReview";
    case "requestor":
      return "ReqHome";
    case "cvera":
      return "Recruiter";
    default:
      return "Home";
  }
}

export const AppShell: React.FC = () => {
  const { status, user } = useAuth();
  const [biometricGateDone, setBiometricGateDone] = useState(false);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      const biometricsEnabled = true;

      if (status !== "authenticated" || !biometricsEnabled) {
        if (!cancelled) setBiometricGateDone(true);
        return;
      }

      try {
        console.log("[AppShell] biometrics: starting gate check");
        await runBiometricCheck();
        console.log("[AppShell] biometrics: completed");
      } catch (e) {
        console.warn("[AppShell] biometrics: failed (proceeding)", e);
      } finally {
        if (!cancelled) setBiometricGateDone(true);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [status]);

  const shouldBlockForBiometrics = status === "authenticated" && !biometricGateDone;

  if (status === "checking" || shouldBlockForBiometrics) {
    return (
      <View className="flex-1 items-center justify-center bg-white dark:bg-black">
        <ActivityIndicator />
      </View>
    );
  }

    if (status === "session-expired") return <SessionExpiredScreen />;

    if (status === "unauthenticated") return <AuthNavigator />;

    const authenticatedUser = user;

    if (status !== "authenticated" || !authenticatedUser?.role) {
      return (
        <View className="flex-1 items-center justify-center bg-white dark:bg-black">
          <ActivityIndicator />
        </View>
      );
    }

    const initialRoute = routeForRole(authenticatedUser.role);

    return (
      <MainAppNavigator
        key={`main:${initialRoute}`}
        initialRouteName={initialRoute}
      />
    );
  };