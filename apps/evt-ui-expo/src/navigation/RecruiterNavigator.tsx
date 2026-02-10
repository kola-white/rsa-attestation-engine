import React from "react";
import { Pressable, Text } from "react-native";
import { createNativeStackNavigator } from "@react-navigation/native-stack";
import { useNavigation } from "@react-navigation/native";
import type { NativeStackNavigationProp } from "@react-navigation/native-stack";

import type { RecruiterStackParamList } from "@/src/navigation/recruiterTypes";
import type { AppStackParamList } from "@/src/navigation/MainAppNavigator";

import { RecruiterCandidatesScreen } from "@/screens/RecruiterCandidates";
import { CandidateDetailScreen } from "@/screens/CandidateDetail";
import { RecruiterFiltersScreen } from "@/screens/RecruiterFilters";

console.log("[RecruiterNavigator] module loaded");

const Stack = createNativeStackNavigator<RecruiterStackParamList>();

function BackToAppButton() {
  console.log("[RecruiterNavigator] render");
  const nav = useNavigation<NativeStackNavigationProp<RecruiterStackParamList>>();


  return (
    <Pressable
      onPress={() => {
        // Parent is the App stack (once MainAppNavigator wraps RecruiterNavigator)
        const parent = nav.getParent<NativeStackNavigationProp<AppStackParamList>>();

        if (!parent) {
          console.warn("[BackToAppButton] No parent navigator found");
          if (nav.canGoBack()) nav.goBack();
          return;
        }

        parent.navigate("HRReview"); 

      }}
      accessibilityRole="button"
      accessibilityLabel="Back"
      hitSlop={10}
      style={({ pressed }) => [{ opacity: pressed ? 0.6 : 1 }]}
      className="px-2 py-1"
    >
      <Text className="text-[17px] text-sky-400 dark:text-blue-400">
        Back
      </Text>
    </Pressable>
  );
}

export const RecruiterNavigator: React.FC = () => {
  return (
    <Stack.Navigator
      initialRouteName="RecruiterCandidates"
      screenOptions={{
        headerShown: true,
        headerLargeTitle: true,
        headerShadowVisible: false,

        // Native-stack: hide label by using empty string (NOT headerBackTitleVisible)
        headerBackTitle: "Back",
        headerBackButtonMenuEnabled: false,
      }}
    >
      <Stack.Screen
        name="RecruiterCandidates"
        component={RecruiterCandidatesScreen}
        options={{
          title: "Candidates",
          // ✅ this is the “back to Home/HRReview” you’re missing
          headerLeft: () => <BackToAppButton />,
        }}
      />

      <Stack.Screen
        name="CandidateDetail"
        component={CandidateDetailScreen}
        options={{ title: "Candidate" }}
      />

      <Stack.Screen
        name="RecruiterFilters"
        component={RecruiterFiltersScreen}
        options={{ title: "Filters", presentation: "modal" }}
      />
    </Stack.Navigator>
  );
};
