// src/navigation/RequestorNavigator.tsx
import React from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import type { RequestorStackParamList } from "@/src/navigation/requestorTypes";
import { RequestorHomeScreen } from "@/screens/RequestorHomeScreen";
import { RequestorNewRequestScreen } from "@/screens/RequestorNewRequestScreen";
import { RequestorRequestDetailScreen } from "@/screens/RequestorRequestDetailScreen";

const Stack = createNativeStackNavigator<RequestorStackParamList>();

export const RequestorNavigator: React.FC = () => {
  return (
    <Stack.Navigator
      initialRouteName="RequestorHome"
      screenOptions={{
        headerShown: true,
        headerLargeTitle: true,
      }}
    >
      <Stack.Screen
        name="RequestorHome"
        component={RequestorHomeScreen}
        options={{ title: "Request" }}
      />
      
      <Stack.Screen
        name="RequestorNewRequest"
        component={RequestorNewRequestScreen}
        options={{ title: "New request" }}
      />
      <Stack.Screen
        name="RequestorRequestDetail"
        component={RequestorRequestDetailScreen}
        options={{ title: "Status" }}
      />
    </Stack.Navigator>
  );
};
