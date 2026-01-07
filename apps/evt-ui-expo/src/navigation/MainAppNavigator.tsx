// src/navigation/MainAppNavigator.tsx
import React from "react";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import { HomeScreen } from "@/screens/HomeScreen";
import {HRReviewScreenSettingsStyle} from "@/screens/HRReviewScreen.SettingsStyle";

export type AppStackParamList = {
  Home: undefined;
  HRReview: undefined;
};

const Stack = createNativeStackNavigator<AppStackParamList>();


export const MainAppNavigator: React.FC = () => {
console.log("[MainAppNavigator] render");
  return (
<Stack.Navigator initialRouteName="HRReview">
  <Stack.Screen 
    name="HRReview" 
    component={HRReviewScreenSettingsStyle} 
  />
  <Stack.Screen 
    name="Home" 
    component={HomeScreen}
    options={{ headerShown: false }} 
  />
</Stack.Navigator>

  );
};
