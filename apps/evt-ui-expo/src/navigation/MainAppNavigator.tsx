// src/navigation/MainAppNavigator.tsx
import React, { useEffect } from "react";
import { createNativeStackNavigator, NativeStackNavigationProp } from "@react-navigation/native-stack";
import { HomeScreen } from "@/screens/HomeScreen";
import { HRReviewScreenSettingsStyle } from "@/screens/HRReviewScreen.SettingsStyle";
import { RecruiterNavigator } from "@/src/navigation/RecruiterNavigator";
import { RequestorNavigator } from "@/src/navigation/RequestorNavigator";
import { useAuth } from "../auth/AuthContext";
import { CommonActions, useNavigation } from "@react-navigation/native";
import { ActivityIndicator, View } from "react-native";


export type AppStackParamList = {
  RoleGate: undefined; // determines initial route based on user role
  Home: undefined;
  HRReview: undefined;
  Recruiter: undefined; // mounts the Phase-1 recruiter stack
  ReqHome: undefined; // requestor home screen
};

type AppRoute = keyof AppStackParamList;

function routeForRole(role: string): AppRoute {
  switch (role) {
    case "recruiter":
      return "Recruiter";
    case "hr_reviewer":
      return "HRReview";
    case "requestor":
      return "ReqHome";
    case "cvera":
      return "Recruiter"; // or "Home" until you add Admin
    default:
      return "Home";
  }
}

function RoleGateScreen() {
  const { user } = useAuth();
  const nav = useNavigation<NativeStackNavigationProp<AppStackParamList>>();

  useEffect(() => {
    const role = user?.role;
    if (!role) return; // keep spinner until role exists
    const target = routeForRole(role);

    nav.dispatch(
      CommonActions.reset({
        index: 0,
        routes: [{ name: target }],
      })
    );
  }, [user?.role, nav]);

// ✅ MUST return something (otherwise your component is typed as () => void)
  return (
    <View className="flex-1 items-center justify-center bg-white dark:bg-black">
      <ActivityIndicator />
    </View>
  );
}

const Stack = createNativeStackNavigator<AppStackParamList>();

export const MainAppNavigator: React.FC = () => {
  console.log("[MainAppNavigator] render");
  return (
  <Stack.Navigator initialRouteName="RoleGate">
      <Stack.Screen
        name="RoleGate"
        component={RoleGateScreen}
        options={{ headerShown: false }}
      />      
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
