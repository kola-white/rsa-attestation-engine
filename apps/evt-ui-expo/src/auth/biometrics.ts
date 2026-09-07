import * as LocalAuthentication from "expo-local-authentication";

export type BiometricModality =
  | "face"
  | "fingerprint"
  | "iris"
  | "unknown";

export type BiometricUnavailableReason =
  | "no-hardware"
  | "not-enrolled";

export type BiometricCancelledReason =
  | "user-cancel"
  | "system-cancel"
  | "app-cancel";

export type BiometricFailureReason =
  | "lockout"
  | "no-space"
  | "timeout"
  | "unable-to-process"
  | "not-available"
  | "passcode-not-set"
  | "authentication-failed"
  | "user-fallback"
  | "invalid-context"
  | "unknown";

export type BiometricAvailability =
  | {
      status: "available";
      modalities: BiometricModality[];
    }
  | {
      status: "unavailable";
      reason: BiometricUnavailableReason;
    };

export type BiometricCheckResult =
  | {
      status: "success";
    }
  | {
      status: "unavailable";
      reason: BiometricUnavailableReason;
    }
  | {
      status: "cancelled";
      reason: BiometricCancelledReason;
    }
  | {
      status: "failed";
      reason: BiometricFailureReason;
      warning?: string;
    };

function mapAuthenticationType(
  type: LocalAuthentication.AuthenticationType
): BiometricModality {
  switch (type) {
    case LocalAuthentication.AuthenticationType.FACIAL_RECOGNITION:
      return "face";

    case LocalAuthentication.AuthenticationType.FINGERPRINT:
      return "fingerprint";

    case LocalAuthentication.AuthenticationType.IRIS:
      return "iris";

    default:
      return "unknown";
  }
}

function mapAuthenticationError(
  error: LocalAuthentication.LocalAuthenticationError
): Exclude<BiometricCheckResult, { status: "success" }> {
  switch (error) {
    case "not_enrolled":
      return {
        status: "unavailable",
        reason: "not-enrolled",
      };

    case "user_cancel":
      return {
        status: "cancelled",
        reason: "user-cancel",
      };

    case "system_cancel":
      return {
        status: "cancelled",
        reason: "system-cancel",
      };

    case "app_cancel":
      return {
        status: "cancelled",
        reason: "app-cancel",
      };

    case "lockout":
      return {
        status: "failed",
        reason: "lockout",
      };

    case "no_space":
      return {
        status: "failed",
        reason: "no-space",
      };

    case "timeout":
      return {
        status: "failed",
        reason: "timeout",
      };

    case "unable_to_process":
      return {
        status: "failed",
        reason: "unable-to-process",
      };

    case "not_available":
      return {
        status: "failed",
        reason: "not-available",
      };

    case "passcode_not_set":
      return {
        status: "failed",
        reason: "passcode-not-set",
      };

    case "authentication_failed":
      return {
        status: "failed",
        reason: "authentication-failed",
      };

    case "user_fallback":
      return {
        status: "failed",
        reason: "user-fallback",
      };

    case "invalid_context":
      return {
        status: "failed",
        reason: "invalid-context",
      };

    case "unknown":
    default:
      return {
        status: "failed",
        reason: "unknown",
      };
  }
}

export async function getBiometricAvailability(): Promise<BiometricAvailability> {
  const hasHardware = await LocalAuthentication.hasHardwareAsync();

  console.log(
    "[Biometrics] hasHardware:",
    hasHardware
  );

  if (!hasHardware) {
    return {
      status: "unavailable",
      reason: "no-hardware",
    };
  }

  const authenticationTypes =
    await LocalAuthentication.supportedAuthenticationTypesAsync();

  console.log(
    "[Biometrics] supportedAuthenticationTypes:",
    authenticationTypes
  );

  return {
    status: "available",
    modalities: authenticationTypes.map(mapAuthenticationType),
  };
}

export async function runBiometricCheck(): Promise<BiometricCheckResult> {
  console.log("[Biometrics] runBiometricCheck: start");

  const availability = await getBiometricAvailability();

  console.log(
    "[Biometrics] availability:",
    availability
  );

  if (availability.status === "unavailable") {
    return availability;
  }

  try {
    console.log(
      "[Biometrics] authenticateAsync: invoking"
    );

    const result = await LocalAuthentication.authenticateAsync({
      promptMessage: "Unlock Cvera",
      cancelLabel: "Cancel",
      fallbackLabel: "",
      disableDeviceFallback: true,
    });

    console.log(
      "[Biometrics] authenticateAsync result:",
      result
    );

    if (result.success) {
      return {
        status: "success",
      };
    }

    const mapped = mapAuthenticationError(result.error);

    if (mapped.status === "failed" && result.warning) {
      return {
        ...mapped,
        warning: result.warning,
      };
    }

    return mapped;
  } catch (error) {
    console.warn(
      "[Biometrics] authenticateAsync threw:",
      error
    );

    return {
      status: "failed",
      reason: "unknown",
    };
  }
}