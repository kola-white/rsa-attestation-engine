export interface KratosUiText {
  id: number;
  text: string;
  type: string;
  context?: Record<string, unknown>;
}

export interface KratosUi {
  action: string;
  method: 'POST' | 'GET';
  messages?: KratosUiText[];
}

export interface KratosLoginFlow {
  id: string;
  type: 'api' | 'browser';
  ui: KratosUi;
}

export interface KratosSuccessfulNativeLogin {
  session_token?: string;
  session?: {
    id: string;
    identity: {
      id: string;
      traits: {
        email?: string;
        [key: string]: unknown;
      };
    };
  };
  // we ignore other fields for now
}

export interface KratosUiMessage {
  id: number;
  text: string;
  type: 'error' | 'success' | 'info';
  context?: unknown;
}

export interface KratosUiContainer {
  action: string; // URL to POST the registration submission to
  method: string; // usually "POST"
  messages?: KratosUiMessage[];
}

export interface KratosRegistrationFlow {
  id: string;
  ui: KratosUiContainer;
}

// Response when registration succeeds and session is created
export interface KratosSuccessfulNativeRegistration {
  session_token: string;
  session?: {
    id: string;
  };
}

// Shape of a 400 "form error" response
export interface KratosErrorResponse {
  id?: string;
  type?: string;
  ui?: KratosUiContainer;
  error?: {
    message?: string;
  };
}

// This is what the screen catches when a 400 "form error" happens.
export class KratosFormError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'KratosFormError';
  }
}