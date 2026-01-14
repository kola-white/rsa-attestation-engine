// src/navigation/MainAppNavigator.tsx
import React from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import { HomeScreen } from "@/screens/HomeScreen";
import { HRReviewScreenSettingsStyle } from "@/screens/HRReviewScreen.SettingsStyle";
import { RecruiterNavigator } from "@/src/navigation/RecruiterNavigator";
import { RequestorNavigator } from "@/src/navigation/RequestorNavigator";


export type AppStackParamList = {
  Home: undefined;
  HRReview: undefined;
  Recruiter: undefined; // mounts the Phase-1 recruiter stack
  ReqHome: undefined; // requestor home screen
};

const Stack = createNativeStackNavigator<AppStackParamList>();

export const MainAppNavigator: React.FC = () => {
  console.log("[MainAppNavigator] render");
  return (
    <Stack.Navigator initialRouteName="Home">
      <Stack.Screen
        name="Home"
        component={HomeScreen}
        options={{ headerShown: false }}
      />

      <Stack.Screen
        name="HRReview"
        component={HRReviewScreenSettingsStyle}
        options={{
          // keep HRReview header visible; button is added inside the screen via setOptions
          headerShown: true,
        }}
      />

      <Stack.Screen
        name="Recruiter"
        component={RecruiterNavigator}
        options={{
          headerShown: false, // RecruiterNavigator manages its own headers
        }}
      />

      <Stack.Screen
        name="ReqHome"
        component={RequestorNavigator}
        options={{ headerShown: false }}
      />
    </Stack.Navigator>
  );
};
