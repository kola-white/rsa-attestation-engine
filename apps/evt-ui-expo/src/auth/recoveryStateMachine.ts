// src/auth/recoveryStateMachine.ts

export type RecoveryUiState =
  | {
      type: 'idle';
    }
  | {
      type: 'bootstrappingBrowserFlow';
    }
  | {
      type: 'loadingBrowserFlow';
    }
  | {
      type: 'enteringEmail';
      flowId: string;
      canResend: boolean;
    }
  | {
      type: 'submittingEmail';
      flowId: string;
    }
  | {
      type: 'emailSentEnterCode';
      flowId: string;
      email?: string;
      canResend: boolean;
    }
  | {
      type: 'submittingCode';
      flowId: string;
    }
  | {
      type: 'restartingFlow';
    }
  | {
      type: 'expiredOrInvalidFlow';
      reason?: string;
    }
  | {
      type: 'complete';
    }
  | {
      type: 'nativeApiRecovery';
    };

export type RecoveryAction =
  | {
      type: 'BOOTSTRAP_BROWSER_FLOW';
    }
  | {
      type: 'LOAD_BROWSER_FLOW';
    }
  | {
      type: 'ENTER_EMAIL';
      flowId: string;
      canResend: boolean;
    }
  | {
      type: 'SUBMIT_EMAIL';
      flowId: string;
    }
  | {
      type: 'EMAIL_SENT';
      flowId: string;
      email?: string;
      canResend: boolean;
    }
  | {
      type: 'SUBMIT_CODE';
      flowId: string;
    }
  | {
      type: 'FLOW_COMPLETE';
    }
  | {
      type: 'FLOW_EXPIRED';
      reason?: string;
    }
  | {
      type: 'RESTART_FLOW';
    }
  | {
      type: 'USE_NATIVE_API_RECOVERY';
    }
  | {
      type: 'RESET';
    };

export function recoveryReducer(
  state: RecoveryUiState,
  action: RecoveryAction,
): RecoveryUiState {
  switch (action.type) {
    case 'BOOTSTRAP_BROWSER_FLOW':
      return {
        type: 'bootstrappingBrowserFlow',
      };

    case 'LOAD_BROWSER_FLOW':
      return {
        type: 'loadingBrowserFlow',
      };

    case 'ENTER_EMAIL':
      return {
        type: 'enteringEmail',
        flowId: action.flowId,
        canResend: action.canResend,
      };

    case 'SUBMIT_EMAIL':
      return {
        type: 'submittingEmail',
        flowId: action.flowId,
      };

    case 'EMAIL_SENT':
      return {
        type: 'emailSentEnterCode',
        flowId: action.flowId,
        email: action.email,
        canResend: action.canResend,
      };

    case 'SUBMIT_CODE':
      return {
        type: 'submittingCode',
        flowId: action.flowId,
      };

    case 'FLOW_COMPLETE':
      return {
        type: 'complete',
      };

    case 'FLOW_EXPIRED':
      return {
        type: 'expiredOrInvalidFlow',
        reason: action.reason,
      };

    case 'RESTART_FLOW':
      return {
        type: 'restartingFlow',
      };

    case 'USE_NATIVE_API_RECOVERY':
      return {
        type: 'nativeApiRecovery',
      };

    case 'RESET':
      return {
        type: 'idle',
      };

    default:
      return state;
  }
}