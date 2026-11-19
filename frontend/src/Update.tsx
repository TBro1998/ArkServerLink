// 自动更新：顶栏的版本按钮 / 新版本提示，以及更新对话框。
import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { UpdateService } from "../bindings/arkfgf.com/link";
import type { UpdateState } from "../bindings/arkfgf.com/link/models";
import { UpdateChanged } from "../bindings/events";
import { errText } from "./format";

export function useUpdate(): UpdateState | null {
  const [state, setState] = useState<UpdateState | null>(null);
  useEffect(() => {
    UpdateService.State().then(setState).catch(() => {});
    return Events.On(UpdateChanged, (ev) => setState(ev.data));
  }, []);
  return state;
}

/** 顶栏右侧：有新版本时显示醒目的提示，否则显示当前版本号（点击检查更新）。 */
export function UpdateBadge({ state, onOpen }: { state: UpdateState | null; onOpen: () => void }) {
  if (!state) return null;
  if (state.available && state.latest) {
    const busy = state.phase === "downloading" || state.phase === "installing";
    return (
      <button className="update-pill" onClick={onOpen} title="查看新版本">
        <span className="dot busy" />
        {busy ? "正在更新…" : `新版本 v${state.latest.version}`}
      </button>
    );
  }
  return (
    <button className="version-btn" onClick={onOpen} title="检查更新">
      v{state.current}
    </button>
  );
}

/** 把 Release 说明（Markdown）简单整理成纯文本：去掉标题井号、加粗星号、链接语法，列表换成圆点。 */
function plainNotes(md: string): string {
  return md
    .replace(/\r/g, "")
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/^\s*[-*]\s+/gm, "• ")
    .replace(/\*\*(.+?)\*\*/g, "$1")
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, "$1")
    .replace(/<!--[\s\S]*?-->/g, "")
    .trim();
}

function mb(n: number): string {
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function date(s: string | undefined): string {
  if (!s) return "";
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleDateString("zh-CN");
}

export function UpdateDialog({ state, connected, onClose }: { state: UpdateState; connected: boolean; onClose: () => void }) {
  const [actionError, setActionError] = useState("");
  const { phase, latest } = state;
  const busy = phase === "downloading" || phase === "installing";

  // 打开对话框时没检查过（或上次检查失败）就检查一次
  useEffect(() => {
    if (phase === "idle" || (phase === "error" && !state.available)) UpdateService.Check().catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const run = async (fn: () => Promise<unknown>) => {
    setActionError("");
    try {
      await fn();
    } catch (e) {
      setActionError(errText(e));
    }
  };

  const percent = state.total > 0 ? Math.min(100, (state.done / state.total) * 100) : 0;

  return (
    <div className="overlay" onClick={busy ? undefined : onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <div className="dialog-head">
          <h2>{state.available ? "发现新版本" : "检查更新"}</h2>
          {!busy && (
            <button className="ghost small" onClick={onClose}>
              关闭
            </button>
          )}
        </div>

        {phase === "checking" && (
          <p className="muted">
            <span className="spinner" /> 正在检查更新…
          </p>
        )}

        {!state.available && phase !== "checking" && (
          <div className="dialog-body">
            {phase === "latest" && <p>当前 v{state.current} 已是最新版本。</p>}
            {phase === "error" && <p className="bad">{state.error}</p>}
            <div className="btns">
              <button className="ghost" onClick={() => run(() => UpdateService.Check())}>
                重新检查
              </button>
              <button className="ghost" onClick={() => run(() => UpdateService.OpenDownloadPage())}>
                打开官网下载页
              </button>
            </div>
          </div>
        )}

        {state.available && latest && (
          <div className="dialog-body">
            <p className="version-line">
              <span className="muted">v{state.current}</span> → <b>v{latest.version}</b>
              {latest.published_at && <span className="muted small-text">（{date(latest.published_at)} 发布）</span>}
            </p>
            {latest.notes && <div className="notes">{plainNotes(latest.notes)}</div>}

            {state.can_auto ? (
              <>
                <label className="field">
                  <span>下载线路</span>
                  <select value={state.mirror} disabled={busy} onChange={(e) => run(() => UpdateService.SetMirror(e.target.value))}>
                    <option value="">自动选择最快的线路</option>
                    {(state.mirrors ?? []).map((m) => (
                      <option key={m.prefix || "direct"} value={m.prefix || "direct"}>
                        {m.name}
                      </option>
                    ))}
                  </select>
                </label>

                {phase === "downloading" && (
                  <div className="progress-box">
                    <div className="progress">
                      <div style={{ width: `${percent}%` }} />
                    </div>
                    <div className="progress-meta">
                      <span>{state.source || "准备下载…"}</span>
                      <span>{state.total > 0 ? `${mb(state.done)} / ${mb(state.total)}` : mb(state.done)}</span>
                    </div>
                  </div>
                )}
                {phase === "installing" && (
                  <p className="ok-text">
                    <span className="spinner" /> 下载完成，正在安装，联机助手会自动重新打开…
                  </p>
                )}
                {phase === "error" && <p className="bad">{state.error}</p>}

                <div className="btns">
                  {phase === "downloading" ? (
                    <button className="ghost" onClick={() => run(() => UpdateService.CancelInstall())}>
                      取消
                    </button>
                  ) : phase !== "installing" ? (
                    <button className="primary" onClick={() => run(() => UpdateService.Install())}>
                      {phase === "error" ? "重试" : "立即更新"}
                    </button>
                  ) : null}
                  <button className="ghost" disabled={busy} onClick={() => run(() => UpdateService.OpenDownloadPage())}>
                    官网下载
                  </button>
                </div>
                {!busy && (
                  <p className="muted small-text">
                    {connected ? "更新会断开当前房间。" : ""}下载后校验文件完整性，再静默安装并自动重启。下载慢可以换一条线路。
                  </p>
                )}
              </>
            ) : (
              <>
                <p className="muted">当前是便携版或开发版，不能自动安装，请到官网下载新版本。</p>
                <div className="btns">
                  <button className="primary" onClick={() => run(() => UpdateService.OpenDownloadPage())}>
                    打开官网下载页
                  </button>
                </div>
              </>
            )}
          </div>
        )}

        {actionError && <p className="bad">{actionError}</p>}
      </div>
    </div>
  );
}
