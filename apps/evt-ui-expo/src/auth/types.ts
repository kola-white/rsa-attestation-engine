export type AuthStatus = 'checking' | 'unauthenticated' | 'authenticated';

export interface User {
  id: string;
  email: string;
  name?: string;
  role?: string;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in?: number; // seconds (optional)
  user: User;
}

// 🔹 New input type for registration
export interface RegisterInput {
  email: string;
  password: string;
  fullName?: string;
}

export interface AuthContextValue {
  status: AuthStatus;
  accessToken: string | null;
  user: User | null;
  login(email: string, password: string): Promise<void>;
  logout(): Promise<void>;
  refresh(): Promise<void>;
  register(input: RegisterInput): Promise<void>;
}



export class AuthError extends Error {
  readonly code: string;
  readonly details?: string;

  constructor(code: string, message: string, details?: string) {
    super(message);
    this.code = code;
    this.details = details;
  }
}
