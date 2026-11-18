// 各组件共用的状态文字、图标。
import type { AuthState } from "../bindings/arkfgf.com/link/models";
import { Phase, type Status } from "../bindings/arkfgf.com/link/internal/session/models";

export type LoginProps = {
  auth: AuthState | null;
  loggingIn: boolean;
  loginError: string;
  login: () => void;
  cancelLogin: () => void;
};

export function isActive(s: Status): boolean {
  return s.phase !== Phase.PhaseIdle && s.phase !== Phase.$zero;
}

export function phaseLabel(p: Phase): string {
  switch (p) {
    case Phase.PhaseJoining:
      return "正在加入";
    case Phase.PhaseConnecting:
      return "连接中";
    case Phase.PhaseConnected:
      return "已直连";
    case Phase.PhaseNotDirect:
      return "无法直连";
    case Phase.PhaseError:
      return "失败";
    default:
      return "未连接";
  }
}

export function phaseTone(p: Phase): string {
  switch (p) {
    case Phase.PhaseConnected:
      return "ok";
    case Phase.PhaseJoining:
    case Phase.PhaseConnecting:
      return "busy";
    case Phase.PhaseNotDirect:
      return "warn";
    case Phase.PhaseError:
      return "bad";
    default:
      return "idle";
  }
}

export function LockIcon() {
  return (
    <svg className="lock" viewBox="0 0 24 24" width="12" height="12" fill="currentColor" aria-label="有密码">
      <path d="M7 10V7a5 5 0 0 1 10 0v3h1a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1v-9a1 1 0 0 1 1-1h1Zm2 0h6V7a3 3 0 0 0-6 0v3Z" />
    </svg>
  );
}

export function SteamIcon() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor" aria-hidden="true">
      <path d="M12 2a10 10 0 0 0-9.96 9.1l5.35 2.2a2.8 2.8 0 0 1 1.6-.5h.16l2.38-3.45v-.05a3.77 3.77 0 1 1 3.77 3.77h-.09l-3.4 2.42v.13a2.83 2.83 0 0 1-5.6.56L2.4 14.6A10 10 0 1 0 12 2Zm-4.3 15.2-1.22-.5a2.12 2.12 0 1 0 1.16-2.9l1.27.52a1.56 1.56 0 1 1-1.2 2.88Zm9.62-7.85a2.51 2.51 0 1 0-2.52 2.51 2.51 2.51 0 0 0 2.52-2.5Zm-4.4 0a1.89 1.89 0 1 1 1.88 1.88 1.89 1.89 0 0 1-1.88-1.88Z" />
    </svg>
  );
}
