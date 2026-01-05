// src/navigation/types.ts
export type AuthStackParamList = {
  Login: undefined;
  Register: undefined;
  ForgotPassword: undefined;
};

export type MainStackParamList = {
  HRReview: undefined;
  Home: undefined;
  // Later: 'HrReview': { caseId: string } | undefined;
};
