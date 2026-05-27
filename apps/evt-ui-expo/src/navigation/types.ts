// src/navigation/types.ts
export type AuthStackParamList = {
  Login: undefined;
  Register: undefined;
  ForgotPassword: undefined;
  Verify: { flow?: string } | undefined;
  AuthError: undefined;
  Settings: undefined;
};

export type MainStackParamList = {
  HRReview: undefined;
  Home: undefined;
  // Later: 'HrReview': { caseId: string } | undefined;
};
