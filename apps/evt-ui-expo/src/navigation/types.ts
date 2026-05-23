// src/navigation/types.ts
export type AuthStackParamList = {
  Login: undefined;
  Register: undefined;
  ForgotPassword: undefined;
  Verify: undefined;
  AuthError: undefined;
};

export type MainStackParamList = {
  HRReview: undefined;
  Home: undefined;
  // Later: 'HrReview': { caseId: string } | undefined;
};
