import React from 'react';
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import { LoginScreen } from 'screens/LoginScreen';
import { RegisterScreen } from 'screens/RegisterScreen';
import { RecoveryScreen } from 'screens/RecoveryScreen';
import { AuthStackParamList } from './types';
import VerifyScreen from '@/screens/VerifyScreen';
import AuthErrorScreen from '@/screens/AuthErrorScreen';
import SettingsScreen from '@/screens/SettingsScreen';


const Stack = createNativeStackNavigator<AuthStackParamList>();

export const AuthNavigator: React.FC = () => {
  return (
    <Stack.Navigator
      initialRouteName="Login"
      screenOptions={{
        headerShown: false,
      }}
    >
      <Stack.Screen name="Login" component={LoginScreen} />
      <Stack.Screen name="Register" component={RegisterScreen} />
      <Stack.Screen name="ForgotPassword" component={RecoveryScreen} />
      <Stack.Screen name="Verify" component={VerifyScreen} />
      <Stack.Screen name="AuthError" component={AuthErrorScreen} />
      <Stack.Screen name="Settings" component={SettingsScreen} />
    </Stack.Navigator>
  );
};
