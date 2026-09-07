import React, { useCallback, useEffect, useState } from "react";
import {
  ActivityIndicator,
  Platform,
  Pressable,
  Text,
  View,
} from "react-native";

import { useAuth } from "@/src/auth/AuthContext";
import {
  BiometricFailureReason,
  BiometricUnavailableReason,
  runBiometricCheck,
} from "@/src/auth/biometrics";

import { AuthNavigator } from "@/src/navigation/AuthNavigator";
import { MainAppNavigator } from "@/src/navigation/MainAppNavigator";
import { SessionExpiredScreen } from "screens/SessionExpiredScreen";

type BiometricGateState =
  | {
      status: "checking-session";
    }
  | {
      status: "prompting";
    }
  | {
      status: "restoring-session";
    }
  | {
      status: "cancelled";
    }
  | {
      status: "unavailable";
      reason: BiometricUnavailableReason;
    }
  | {
      status: "failed";
      reason: BiometricFailureReason;
    }
  | {
      status: "complete";
    };

function routeForRole(
  role: string | undefined
): "Recruiter" | "HRReview" | "ReqHome" | "Home" {
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
  const {
    status,
    user,
    hasStoredSession,
    restoreSession,
    beginReauth,
  } = useAuth();

  const [biometricGate, setBiometricGate] =
    useState<BiometricGateState>({
      status: "checking-session",
    });

  const continueWithPassword = useCallback(() => {
    setBiometricGate({
      status: "complete",
    });

    beginReauth();
  }, [beginReauth]);

  const runNativeBootstrap = useCallback(async () => {
    /*
     * Browser authentication has its own Kratos cookie/session hydration
     * lifecycle in AuthContext. The biometric gate is native-only.
     */
    if (Platform.OS === "web") {
      setBiometricGate({
        status: "complete",
      });
      return;
    }
    console.log("[AppShell][bootstrap] start");
    setBiometricGate({
      status: "checking-session",
    });

    try {
      /*
       * A stored refresh token means that Cvera has application-session
       * material that may be restored.
       *
       * Merely discovering it does not consume it and does not authenticate
       * the user.
       */
      const storedSessionExists = await hasStoredSession();
      console.log(
        "[AppShell][bootstrap] stored session exists?",
        storedSessionExists
      );

      /*
       * Fresh install, explicit logout, or otherwise no stored Cvera session:
       * there is nothing for Face ID to unlock.
       */
      if (!storedSessionExists) {
        setBiometricGate({
          status: "complete",
        });

        beginReauth();
        return;
      }

      /*
       * Session material exists. It MUST NOT be consumed until local
       * biometric authorization succeeds.
       */
      setBiometricGate({
        status: "prompting",
      });

      console.log(
        "[AppShell][bootstrap] starting biometric check"
      );
      const biometricResult = await runBiometricCheck();
      console.log(
        "[AppShell][bootstrap] biometric result:",
        biometricResult
      );
      if (biometricResult.status === "success") {
        setBiometricGate({
          status: "restoring-session",
        });

        /*
         * Face ID does not restore or mint the session itself.
         *
         * It authorizes this call into the existing Cvera refresh/session
         * restoration mechanism.
         */
        console.log(
          "[AppShell][bootstrap] biometric success -> restoring session"
        );
        await restoreSession();

        /*
         * restoreSession()/refresh() owns the resulting AuthContext state:
         *
         * authenticated
         * session-expired
         * unauthenticated
         */
        setBiometricGate({
          status: "complete",
        });

        return;
      }

      if (biometricResult.status === "cancelled") {
        setBiometricGate({
          status: "cancelled",
        });
        return;
      }

      if (biometricResult.status === "unavailable") {
        setBiometricGate({
          status: "unavailable",
          reason: biometricResult.reason,
        });
        return;
      }

      setBiometricGate({
        status: "failed",
        reason: biometricResult.reason,
      });
    } catch (error) {
      console.warn(
        "[AppShell] biometric bootstrap failed",
        error
      );

      setBiometricGate({
        status: "failed",
        reason: "unknown",
      });
    }
  }, [
    beginReauth,
    hasStoredSession,
    restoreSession,
  ]);

  useEffect(() => {
    void runNativeBootstrap();
  }, [runNativeBootstrap]);

  /*
   * Bootstrap states intentionally block application rendering.
   *
   * Most importantly, "restoring-session" means Face ID has already
   * succeeded and Cvera is now performing its normal refresh operation.
   */
  if (
    biometricGate.status === "checking-session" ||
    biometricGate.status === "prompting" ||
    biometricGate.status === "restoring-session"
  ) {
    return (
      <View className="flex-1 items-center justify-center bg-white dark:bg-black">
        <ActivityIndicator />
      </View>
    );
  }

  /*
   * User explicitly cancelled the biometric prompt.
   *
   * Do not restore the session.
   * Do not silently fall through into authenticated UI.
   */
  if (biometricGate.status === "cancelled") {
    return (
      <View className="flex-1 items-center justify-center bg-white px-6 dark:bg-black">
        <Text className="mb-2 text-center text-xl font-semibold text-zinc-950 dark:text-zinc-50">
          Cvera is locked
        </Text>

        <Text className="mb-6 text-center text-sm text-zinc-600 dark:text-zinc-300">
          Use Face ID to unlock your existing Cvera session.
        </Text>

        <Pressable
          className="w-full rounded-xl bg-sky-400 px-4 py-3"
          onPress={() => void runNativeBootstrap()}
        >
          <Text className="text-center font-semibold text-slate-950">
            Try Face ID again
          </Text>
        </Pressable>

        <Pressable
          className="mt-3 w-full rounded-xl border border-zinc-300 px-4 py-3 dark:border-zinc-700"
          onPress={continueWithPassword}
        >
          <Text className="text-center font-semibold text-zinc-900 dark:text-zinc-100">
            Sign in with password
          </Text>
        </Pressable>
      </View>
    );
  }

  /*
   * This device cannot currently perform biometric authentication.
   *
   * We deliberately do not restore the stored session without local
   * authorization.
   */
  if (biometricGate.status === "unavailable") {
    const message =
      biometricGate.reason === "not-enrolled"
        ? "Face ID is not enrolled on this device."
        : "Face ID is not available on this device.";

    return (
      <View className="flex-1 items-center justify-center bg-white px-6 dark:bg-black">
        <Text className="mb-2 text-center text-xl font-semibold text-zinc-950 dark:text-zinc-50">
          Face ID unavailable
        </Text>

        <Text className="mb-6 text-center text-sm text-zinc-600 dark:text-zinc-300">
          {message}
        </Text>

        <Pressable
          className="w-full rounded-xl bg-sky-400 px-4 py-3"
          onPress={continueWithPassword}
        >
          <Text className="text-center font-semibold text-slate-950">
            Sign in with password
          </Text>
        </Pressable>
      </View>
    );
  }

  /*
   * Biometric authentication was available but did not succeed.
   */
  if (biometricGate.status === "failed") {
    const isLockedOut =
      biometricGate.reason === "lockout";

    return (
      <View className="flex-1 items-center justify-center bg-white px-6 dark:bg-black">
        <Text className="mb-2 text-center text-xl font-semibold text-zinc-950 dark:text-zinc-50">
          {isLockedOut
            ? "Face ID temporarily locked"
            : "Face ID could not unlock Cvera"}
        </Text>

        <Text className="mb-6 text-center text-sm text-zinc-600 dark:text-zinc-300">
          {isLockedOut
            ? "Face ID is temporarily unavailable after unsuccessful attempts."
            : "Your existing Cvera session has not been restored."}
        </Text>

        {!isLockedOut && (
          <Pressable
            className="w-full rounded-xl bg-sky-400 px-4 py-3"
            onPress={() => void runNativeBootstrap()}
          >
            <Text className="text-center font-semibold text-slate-950">
              Try Face ID again
            </Text>
          </Pressable>
        )}

        <Pressable
          className="mt-3 w-full rounded-xl border border-zinc-300 px-4 py-3 dark:border-zinc-700"
          onPress={continueWithPassword}
        >
          <Text className="text-center font-semibold text-zinc-900 dark:text-zinc-100">
            Sign in with password
          </Text>
        </Pressable>
      </View>
    );
  }

  /*
   * From this point forward the biometric bootstrap has completed.
   * Normal AuthContext state owns application routing.
   */

  if (status === "checking") {
    return (
      <View className="flex-1 items-center justify-center bg-white dark:bg-black">
        <ActivityIndicator />
      </View>
    );
  }

  if (status === "session-expired") {
    return <SessionExpiredScreen />;
  }

  if (status === "unauthenticated") {
    return <AuthNavigator />;
  }

  if (
    status !== "authenticated" ||
    !user?.role
  ) {
    return (
      <View className="flex-1 items-center justify-center bg-white dark:bg-black">
        <ActivityIndicator />
      </View>
    );
  }

  const initialRoute = routeForRole(user.role);

  return (
    <MainAppNavigator
      key={`main:${initialRoute}`}
      initialRouteName={initialRoute}
    />
  );
};