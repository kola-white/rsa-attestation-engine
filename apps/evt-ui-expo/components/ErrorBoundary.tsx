import React from "react";
import { View, Text, Pressable, ScrollView } from "react-native";

type Props = {
  children: React.ReactNode;
  title?: string;
};

type State = {
  hasError: boolean;
  error?: Error;
  info?: React.ErrorInfo;
};

export default class ErrorBoundary extends React.Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // Keep it simple: log to Metro
    console.error("[ErrorBoundary] Caught error:", error);
    console.error("[ErrorBoundary] Component stack:", info?.componentStack);
    this.setState({ info });
  }

  private reset = () => {
    this.setState({ hasError: false, error: undefined, info: undefined });
  };

  render() {
    if (!this.state.hasError) return this.props.children;

    const title = this.props.title ?? "Screen crashed";

    return (
      <View style={{ flex: 1, backgroundColor: "white", padding: 16 }}>
        <Text style={{ fontSize: 20, fontWeight: "700", marginBottom: 8 }}>
          {title}
        </Text>

        <Text style={{ fontSize: 14, opacity: 0.7, marginBottom: 12 }}>
          The UI threw a runtime error. This prevents a black screen and shows the
          crash details so you can keep working.
        </Text>

        <Pressable
          onPress={this.reset}
          style={{
            alignSelf: "flex-start",
            backgroundColor: "black",
            paddingHorizontal: 14,
            paddingVertical: 10,
            borderRadius: 10,
            marginBottom: 12,
          }}
        >
          <Text style={{ color: "white", fontWeight: "600" }}>Try again</Text>
        </Pressable>

        <ScrollView
          style={{
            flex: 1,
            borderWidth: 1,
            borderColor: "rgba(0,0,0,0.1)",
            borderRadius: 12,
            padding: 12,
          }}
        >
          <Text style={{ fontWeight: "700", marginBottom: 6 }}>Error</Text>
          <Text selectable style={{ fontFamily: "monospace" }}>
            {this.state.error?.stack || this.state.error?.message || "Unknown error"}
          </Text>

          {this.state.info?.componentStack ? (
            <>
              <Text style={{ fontWeight: "700", marginTop: 12, marginBottom: 6 }}>
                Component stack
              </Text>
              <Text selectable style={{ fontFamily: "monospace" }}>
                {this.state.info.componentStack}
              </Text>
            </>
          ) : null}
        </ScrollView>
      </View>
    );
  }
}

