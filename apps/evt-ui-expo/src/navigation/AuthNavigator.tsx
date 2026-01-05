import React from 'react';
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import { LoginScreen } from 'screens/LoginScreen';
import { RegisterScreen } from 'screens/RegisterScreen';
import { AuthStackParamList } from './types';


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
    </Stack.Navigator>
  );
};
