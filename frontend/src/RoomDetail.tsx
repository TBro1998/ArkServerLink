import { useEffect, useRef, useState } from "react";
import { Clipboard } from "@wailsio/runtime";
import { RoomService } from "../bindings/arkfgf.com/link";
import type { RoomDetail } from "../bindings/arkfgf.com/link/internal/arkfgf/models";
import { Phase, type Status } from "../bindings/arkfgf.com/link/internal/session/models";
import { ASABadge, hasASA, isActive, isASA, LockIcon, phaseLabel, phaseTone, SteamIcon, type LoginProps } from "./shared";
import { ago, errText, mapName, rate, region } from "./format";

type Props = LoginProps & {
  id: number;
  status: Status;
  autoJoin: boolean;
  onAutoJoinDone: () => void;
};

export function RoomDetailPane(props: Props) {
  const { id, status, autoJoin, onAutoJoinDone, auth } = props;
  const [detail, setDetail] = useState<RoomDetail | null>(null);
  const [error, setError] = useState("");
  const [joinError, setJoinError] = useState("");
  const [confirmSwitch, setConfirmSwitch] = useState(false);

  const load = () => {
    setError("");
    RoomService.Get(id)
      .then(setDetail)
      .catch((e) => setError(errText(e)));
  };
  useEffect(load, [id]);

  const mine = isActive(status) && status.room_id === id;
  const busyElsewhere = isActive(status) && status.room_id !== id && status.phase !== Phase.PhaseError;

  const join = async () => {
    setJoinError("");
    setConfirmSwitch(false);
    try {
      await RoomService.Join(id, detail?.name ?? "");
    } catch (e) {
      setJoinError(errText(e));
    }
  };

  // 链接唤起：房间加载完、确认已登录后自动加入；没登录就等登录完成
  useEffect(() => {
    if (!autoJoin || !detail || !auth || !auth.logged_in) return;
    onAutoJoinDone();
    if (mine && status.phase !== Phase.PhaseError) return;
    if (!detail.online) return;
    if (busyElsewhere) {
      setConfirmSwitch(true);
      return;
    }
    join();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoJoin, detail, auth]);

  if (error) {
    return (
      <div className="detail-msg">
        <p className="bad">{error}</p>
        <button className="ghost" onClick={load}>
          重试
        </button>
      </div>
    );
  }
  if (!detail) {
    return (
      <div className="detail-msg">
        <span className="spinner" /> 正在加载房间…
      </div>
    );
  }

  const where = region(detail.country, detail.province);
  const allASA = (detail.servers?.length ?? 0) > 0 && detail.servers!.every((sv) => isASA(sv.game));
  const modList = detail.mod_list?.length ? detail.mod_list : (detail.mods ?? []).map((m) => ({ id: m, title: "" }));

  return (
    <div className="room">
      <div className="room-head">
        <div>
          <h1>
            {detail.password && <LockIcon />}
            {detail.name || `房间 #${detail.id}`}
          </h1>
          <div className="room-sub">
            {hasASA(detail.servers) && <ASABadge />}
            <span className={`status-dot ${detail.online ? "ok" : "idle"}`} />
            {detail.online ? "在线" : `离线${detail.last_seen ? ` · ${ago(detail.last_seen)}在线` : ""}`}
            <span className="sep" />
            {mapName(detail.map)}
            {where && (
              <>
                <span className="sep" />
                {where}
              </>
            )}
            {detail.version && (
              <>
                <span className="sep" />v{detail.version}
              </>
            )}
          </div>
        </div>
        <div className="big-players">
          <b>{detail.players}</b>/{detail.max_players || "?"}
          <small>在线玩家</small>
        </div>
      </div>

      <section className="join-area">
        {mine ? (
          <ConnectionCard {...props} onRetry={join} />
        ) : !auth?.logged_in ? (
          <LoginPrompt {...props} />
        ) : !detail.online ? (
          <div className="join-row">
            <p className="muted">房间离线（服务器没有运行），暂时无法加入。</p>
            <button className="ghost" onClick={load}>
              刷新
            </button>
          </div>
        ) : confirmSwitch ? (
          <div className="join-row">
            <p>将断开「{status.room_name || `房间 #${status.room_id}`}」并加入这个房间。</p>
            <div className="btns">
              <button className="ghost" onClick={() => setConfirmSwitch(false)}>
                取消
              </button>
              <button className="primary" onClick={join}>
                切换
              </button>
            </div>
          </div>
        ) : (
          <div className="join-row">
            <p className="muted">
              {detail.password
                ? "这台服务器设有密码，进服时需要输入服主提供的密码。"
                : allASA
                  ? "加入后在 ASA 的控制台里输入助手给出的命令即可进服。"
                  : "加入后点「进入游戏」即可直接进服。"}
            </p>
            <button className="primary big" onClick={() => (busyElsewhere ? setConfirmSwitch(true) : join())}>
              {busyElsewhere ? "切换到这个房间" : "加入"}
            </button>
          </div>
        )}
        {joinError && <p className="bad small-text">{joinError}</p>}
      </section>

      {(detail.servers?.length ?? 0) > 1 && (
        <div className="room-servers">
          <h3>房间里的服务器（{detail.servers!.length}）</h3>
          <p className="muted small-text">加入一次就能连接这里的所有服务器。</p>
          {detail.servers!.map((sv, i) => (
            <div key={i} className="room-server">
              <span className={`status-dot ${sv.online ? "ok" : "idle"}`} />
              <span className="name">
                {sv.password && <LockIcon />}
                {sv.name}
              </span>
              {isASA(sv.game) && <ASABadge />}
              <span className="muted small-text">
                {mapName(sv.map)} · {sv.pvp ? "PvP" : "PvE"}
              </span>
              <span className="muted small-text">{sv.online ? `${sv.players}/${sv.max_players || "?"}` : "未运行"}</span>
            </div>
          ))}
        </div>
      )}

      <div className="facts">
        <Fact label="模式" value={detail.pvp ? "PvP" : "PvE"} />
        <Fact label="经验" value={rate(detail.rates.xp)} />
        <Fact label="驯服" value={rate(detail.rates.tame)} />
        <Fact label="采集" value={rate(detail.rates.harvest)} />
        <Fact label="繁殖" value={rate(detail.rates.breeding)} />
        {detail.cluster && <Fact label="集群" value={detail.cluster} />}
        {detail.arkapi && <Fact label="插件" value="ArkApi" />}
      </div>

      {(detail.tags?.length ?? 0) > 0 && (
        <div className="tags">
          {detail.tags!.map((t) => (
            <span key={t} className="tag">
              {t}
            </span>
          ))}
        </div>
      )}

      {detail.description && <p className="desc">{detail.description}</p>}

      {(detail.qq_group || detail.discord) && (
        <div className="contacts">
          {detail.qq_group && (
            <span>
              QQ 群 <code>{detail.qq_group}</code>
            </span>
          )}
          {detail.discord && (
            <span>
              Discord <code>{detail.discord}</code>
            </span>
          )}
        </div>
      )}

      {modList.length > 0 && (
        <div className="mods">
          <h3>Mod（{modList.length}）</h3>
          <ul>
            {modList.map((m) => (
              <li key={m.id}>
                <span>{m.title || "未知 Mod"}</span>
                <code>{m.id}</code>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="fact">
      <span>{label}</span>
      <b>{value}</b>
    </div>
  );
}

function LoginPrompt({ autoJoin, loggingIn, loginError, login, cancelLogin }: Props) {
  return (
    <div className="join-row">
      <div>
        <p>{autoJoin ? "登录后会自动加入这个房间。" : "加入房间需要先用 Steam 登录。"}</p>
        <p className="muted small-text">服主可以看到你的 Steam 名称，用于管理房间成员。</p>
        {loginError && <p className="bad small-text">{loginError}</p>}
      </div>
      {loggingIn ? (
        <div className="btns">
          <span className="muted">
            <span className="spinner" /> 等待在 Steam 窗口里确认登录…
          </span>
          <button className="ghost" onClick={cancelLogin}>
            取消
          </button>
        </div>
      ) : (
        <button className="steam big" onClick={login}>
          <SteamIcon /> 用 Steam 登录
        </button>
      )}
    </div>
  );
}

function ConnectionCard({ status, onRetry, login, loggingIn }: Props & { onRetry: () => void }) {
  // 正在复制或进服的服务器（key）
  const [copied, setCopied] = useState("");
  const [entering, setEntering] = useState("");
  const [enterError, setEnterError] = useState("");
  const [enterHint, setEnterHint] = useState("");
  const tone = phaseTone(status.phase);
  const failed = status.phase === Phase.PhaseError;
  const servers = status.servers ?? [];
  const showCommand = !failed && servers.length > 0;
  const connected = status.phase === Phase.PhaseConnected;
  const multi = servers.length > 1;
  // ASE 通过 Steam 收藏进服，ASA 只能在游戏控制台里用 open 命令进服，房间里可能两种都有
  const anyASE = servers.some((sv) => !isASA(sv.game));
  const anyASA = servers.some((sv) => isASA(sv.game));

  const copy = async (key: string, command: string) => {
    await Clipboard.SetText(command);
    setCopied(key);
    setTimeout(() => setCopied((k) => (k === key ? "" : k)), 1500);
  };

  const enter = async (key: string) => {
    setEnterError("");
    setEnterHint("");
    setEntering(key);
    try {
      setEnterHint(await RoomService.EnterGame(key));
    } catch (e) {
      setEnterError(errText(e));
    } finally {
      setEntering("");
    }
  };

  return (
    <div className={`conn ${tone}`}>
      <div className="conn-head">
        <span className={`badge ${tone}`}>
          {tone === "busy" ? <span className="spinner" /> : <span className={`dot ${tone}`} />}
          {phaseLabel(status.phase)}
        </span>
        {status.phase === Phase.PhaseConnected && status.latency_ms > 0 && <span className="latency">延迟 {status.latency_ms < 1 ? "<1" : Math.round(status.latency_ms)} ms</span>}
        <div className="spacer" />
        {failed ? (
          <>
            <button className="ghost small" onClick={() => RoomService.Leave()}>
              关闭
            </button>
            {status.error_code === "unauthorized" ? (
              <button className="steam small" onClick={login} disabled={loggingIn}>
                <SteamIcon /> 重新登录
              </button>
            ) : status.error_code === "banned" || status.error_code === "not_found" ? null : (
              <button className="primary small" onClick={onRetry}>
                重试
              </button>
            )}
          </>
        ) : (
          <button className="danger small" onClick={() => RoomService.Leave()}>
            断开
          </button>
        )}
      </div>

      <p className="conn-msg">
        {status.message}
        {status.retry_in > 0 && <span className="muted">（{status.retry_in} 秒后重试）</span>}
      </p>

      {showCommand && (
        <>
          {multi && <p className="muted small-text">这个房间有 {servers.length} 台服务器，都已经连好，选一台进入：</p>}
          {servers.map((sv) => {
            const asa = isASA(sv.game);
            return (
              <div key={sv.key} className="server-entry">
                {(multi || asa) && (
                  <div className="server-entry-head">
                    {multi && <b>{sv.name || `服务器 ${sv.key}`}</b>}
                    {asa && <ASABadge />}
                    {multi && sv.map && <span className="muted small-text">{mapName(sv.map)}</span>}
                  </div>
                )}
                {asa ? (
                  // ASA 没有 Steam 查询端口，不能用收藏或 steam://connect，只给控制台命令
                  <div className="command">
                    <code>{sv.command}</code>
                    <div className="btns">
                      <button className={copied === sv.key ? "ok small" : "ghost small"} onClick={() => copy(sv.key, sv.command)}>
                        {copied === sv.key ? "已复制" : "复制命令"}
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="command">
                    <code>{sv.address}</code>
                    <div className="btns">
                      <button className={copied === sv.key ? "ok small" : "ghost small"} onClick={() => copy(sv.key, sv.address)}>
                        {copied === sv.key ? "已复制" : "复制地址"}
                      </button>
                      {connected && (
                        <button className="primary small" onClick={() => enter(sv.key)} disabled={entering !== ""}>
                          {entering === sv.key ? "正在启动 ARK…" : "进入游戏"}
                        </button>
                      )}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
          {enterHint && <p className="ok-text small-text">{enterHint}</p>}
          {enterError && <p className="bad small-text">{enterError}</p>}
          {anyASE && (
            <p className="muted small-text">
              {connected
                ? "「进入游戏」会把服务器加入 Steam 收藏，并通过 Steam 启动 ARK。进入主菜单后点「加入方舟」，切到「收藏」，选这台服务器加入。也可以自己在 Steam 的「查看 → 游戏服务器 → 收藏」里添加上面的地址。进服期间请保持助手运行。"
                : "显示「已直连」后，点「进入游戏」即可进服。"}
            </p>
          )}
          {anyASA && (
            <p className="muted small-text">
              {connected
                ? "ASA（生存飞升）服务器不能从 Steam 收藏或服务器列表进入：打开 ASA，在主菜单按 Tab 打开控制台，粘贴上面的命令后回车即可进服。进服期间请保持助手运行。"
                : "ASA（生存飞升）服务器：显示「已直连」后，在游戏主菜单按 Tab 打开控制台，粘贴上面的命令回车进服。"}
            </p>
          )}
        </>
      )}

      <Logs />
    </div>
  );
}

function Logs() {
  const [open, setOpen] = useState(false);
  const [lines, setLines] = useState<string[]>([]);
  const pre = useRef<HTMLPreElement>(null);

  useEffect(() => {
    if (!open) return;
    const poll = () =>
      RoomService.Logs()
        .then((l) => setLines(l ?? []))
        .catch(() => {});
    poll();
    const t = setInterval(poll, 2000);
    return () => clearInterval(t);
  }, [open]);

  useEffect(() => {
    if (pre.current) pre.current.scrollTop = pre.current.scrollHeight;
  }, [lines]);

  return (
    <details className="logs" onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}>
      <summary>排查问题：查看运行日志</summary>
      <pre ref={pre}>{lines.length ? lines.join("\n") : "暂无日志"}</pre>
      {lines.length > 0 && (
        <button className="ghost small" onClick={() => Clipboard.SetText(lines.join("\n"))}>
          复制日志
        </button>
      )}
    </details>
  );
}
