import React from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";

import { HomeScreen } from "@/screens/HomeScreen";
import { HRReviewScreenSettingsStyle } from "@/screens/HRReviewScreen.SettingsStyle";
import { RecruiterNavigator } from "@/src/navigation/RecruiterNavigator";
import { RequestorNavigator } from "@/src/navigation/RequestorNavigator";

export type AppStackParamList = {
  Home: undefined;
  HRReview: undefined;
  Recruiter: undefined;
  ReqHome: undefined;
};

const Stack = createNativeStackNavigator<AppStackParamList>();

type Props = {
  initialRouteName: keyof AppStackParamList;
};

export const MainAppNavigator: React.FC<Props> = ({ initialRouteName }) => {
  console.log("[MainAppNavigator] render");

  return (
    <Stack.Navigator initialRouteName={initialRouteName}>
      <Stack.Screen
        name="Home"
        component={HomeScreen}
        options={{ headerShown: false }}
      />

      <Stack.Screen
        name="HRReview"
        component={HRReviewScreenSettingsStyle}
        options={{ headerShown: true }}
      />

      <Stack.Screen
        name="Recruiter"
        component={RecruiterNavigator}
        options={{ headerShown: false }}
      />

      <Stack.Screen
        name="ReqHome"
        component={RequestorNavigator}
        options={{ headerShown: false }}
      />
    </Stack.Navigator>
  );
};
