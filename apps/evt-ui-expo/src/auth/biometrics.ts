// src/auth/biometrics.ts
import * as LocalAuthentication from 'expo-local-authentication';

export async function runBiometricCheck(): Promise<boolean> {
  const hasHardware = await LocalAuthentication.hasHardwareAsync();
  if (!hasHardware) return false;

  const enrolled = await LocalAuthentication.isEnrolledAsync();
  if (!enrolled) return false;

  const result = await LocalAuthentication.authenticateAsync({
    promptMessage: 'Unlock with Face ID',
    disableDeviceFallback: true,
  });

  return result.success;
}
