export interface KratosUiText {
  id: number;
  text: string;
  type: string;
  context?: Record<string, unknown>;
}

export type KratosUiMessage = KratosUiText;

export interface KratosUiNodeAttributes {
  name?: string;
  type?: string;
  value?: string;
  required?: boolean;
  disabled?: boolean;
  autocomplete?: string;
  node_type?: string;
  [key: string]: unknown;
}

export interface KratosUiNode {
  type: string;
  group: string;
  attributes?: KratosUiNodeAttributes;
  messages?: KratosUiText[];
  meta?: unknown;
}

export interface KratosUi {
  action: string;
  method: 'POST' | 'GET' | string;
  nodes: KratosUiNode[];
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
}

export type KratosUiContainer = KratosUi;

export interface KratosRegistrationFlow {
  id: string;
  type?: 'api' | 'browser';
  ui: KratosUiContainer;
}

export interface KratosSuccessfulNativeRegistration {
  session_token: string;
  session?: {
    id: string;
  };
}

export interface KratosErrorResponse {
  id?: string;
  type?: string;
  ui?: KratosUiContainer;
  error?: {
    message?: string;
  };
}

export class KratosFormError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'KratosFormError';
  }
}

export interface KratosRecoveryFlow {
  id: string;
  type: 'api' | 'browser';
  ui: KratosUiContainer;
}