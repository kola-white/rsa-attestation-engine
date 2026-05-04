import React, { useState } from "react";
import { Pressable, Text, ViewStyle } from "react-native";
import { useAuth } from "@/src/auth/AuthContext";

type LogoutButtonProps = {
  className?: string;
  style?: ViewStyle;
};

export const LogoutButton: React.FC<LogoutButtonProps> = ({
  className,
  style,
}) => {
  const { logout, isLoggingOut } = useAuth();
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const handlePress = async () => {
    if (isLoggingOut) return;

    setErrorMessage(null);

    try {
      await logout();
    } catch (e) {
      console.log("[LogoutButton] logout failed:", String(e));
      setErrorMessage("Unable to log out. Please try again.");
    }
  };

  return (
    <>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel="Log out"
        onPress={handlePress}
        disabled={isLoggingOut}
        style={style}
        className={
          className ??
          "rounded-xl border border-zinc-300 dark:border-zinc-700 px-4 py-3 bg-white dark:bg-zinc-900"
        }
      >
        <Text className="text-zinc-900 dark:text-zinc-50 font-semibold">
          {isLoggingOut ? "Logging out..." : "Log out"}
        </Text>
      </Pressable>

      {errorMessage ? (
        <Text className="mt-2 text-sm text-red-600 dark:text-red-400">
          {errorMessage}
        </Text>
      ) : null}
    </>
  );
};