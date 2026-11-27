import { useEffect, useRef, useState } from "react";
import { Clipboard } from "@wailsio/runtime";
import { RoomService } from "../bindings/arkfgf.com/link";
import type { RoomDetail, RoomServer } from "../bindings/arkfgf.com/link/internal/arkfgf/models";
import { Phase, type ServerStatus, type Status } from "../bindings/arkfgf.com/link/internal/session/models";
import { ASABadge, isActive, isASA, LockIcon, phaseLabel, phaseTone, roomMaps, roomMode, SteamIcon, type LoginProps } from "./shared";
import { ago, errText, mapName, rate, region } from "./format";

type Props = LoginProps & {
  id: number;
  status: Status;
  autoJoin: boolean;
  onAutoJoinDone: () => void;
  onOpenPortal: (url: string) => void;
};

// 房间详情。一个房间（一个 Commander）里可以有多台服务器：加入一次（一个凭证）就能连所有服务器。
// 所以页面以服务器列表为主，每台显示自己的地图、倍率和 Mod；连上之后每台服务器旁边直接是进服按钮
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

  const servers = detail.servers ?? [];
  const multi = servers.length > 1;
  const onlineServers = servers.filter((sv) => sv.online).length;
  const where = region(detail.country, detail.province);
  const maps = roomMaps(detail);
  const allASA = servers.length > 0 && servers.every((sv) => isASA(sv.game));
  const anyPassword = servers.length > 0 ? servers.some((sv) => sv.password) : detail.password;
  // 已连接（不是失败）时，每台服务器旁边显示进服方式
  const connected = mine && status.phase !== Phase.PhaseError;

  return (
    <div className="room">
      <div className="room-head">
        <div>
          <h1>
            {!multi && anyPassword && <LockIcon />}
            {detail.name || `房间 #${detail.id}`}
          </h1>
          <div className="room-sub">
            <span className={`status-dot ${detail.online ? "ok" : "idle"}`} />
            {detail.online ? "在线" : `离线${detail.last_seen ? ` · ${ago(detail.last_seen)}在线` : ""}`}
            {multi && (
              <>
                <span className="sep" />
                <span className="hl-servers">
                  {servers.length} 台服务器 · {onlineServers} 台在线
                </span>
              </>
            )}
            <span className="sep" />
            <span className={`mode ${modeClass(roomMode(detail))}`}>{roomMode(detail)}</span>
            {maps.length > 0 && (
              <>
                <span className="sep" />
                {maps.map(mapName).join(" / ")}
              </>
            )}
            {where && (
              <>
                <span className="sep" />
                {where}
              </>
            )}
          </div>
        </div>
        <div className="big-players">
          <b>{detail.players}</b>/{detail.max_players || "?"}
          <small>{multi ? "全部服务器在线玩家" : "在线玩家"}</small>
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
              {multi
                ? `加入一次就能连接房间里的全部 ${servers.length} 台服务器，连上后在下面选一台进入。`
                : anyPassword
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

      <ServerList detail={detail} status={status} connected={connected} />

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

      {detail.portal && !(mine && status.phase === Phase.PhaseConnected) && (
        <p className="muted small-text">服主开启了玩家站（公告、服务器状态、商店），加入房间并直连后可以在助手里查看。</p>
      )}

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

      <ModList detail={detail} />
    </div>
  );
}

function modeClass(mode: string): string {
  return mode === "PvP" ? "pvp" : mode === "PvE" ? "pve" : "mixed";
}

// 房间里的一台服务器：网站的公开信息，加上连接后的本机地址（按 key 对应；旧版网站没有 key 时按名称）
type Entry = { info: RoomServer | null; conn: ServerStatus | null };

function entries(servers: RoomServer[], conns: ServerStatus[]): Entry[] {
  const used = new Set<ServerStatus>();
  const out: Entry[] = servers.map((info) => {
    const conn =
      conns.find((c) => !used.has(c) && info.key !== "" && c.key === info.key) ??
      conns.find((c) => !used.has(c) && info.key === "" && c.name === info.name && c.game === info.game) ??
      null;
    if (conn) used.add(conn);
    return { info, conn };
  });
  // 加入之后服主新加的服务器（房间详情还没刷新）也要能进
  for (const c of conns) if (!used.has(c)) out.push({ info: null, conn: c });
  return out;
}

function ServerList({ detail, status, connected }: { detail: RoomDetail; status: Status; connected: boolean }) {
  // 正在复制或进服的服务器（key）
  const [copied, setCopied] = useState("");
  const [entering, setEntering] = useState("");
  const [enterError, setEnterError] = useState("");
  const [enterHint, setEnterHint] = useState("");

  const servers = detail.servers ?? [];
  const list = entries(servers, connected ? (status.servers ?? []) : []);
  if (list.length === 0) return null;
  const multi = list.length > 1;
  const groups = new Set(servers.map((sv) => sv.cluster_group).filter((g) => g > 0)).size;
  const clusterCounts = new Map<number, number>();
  for (const sv of servers) if (sv.cluster_group > 0) clusterCounts.set(sv.cluster_group, (clusterCounts.get(sv.cluster_group) ?? 0) + 1);
  const hasCluster = [...clusterCounts.values()].some((n) => n > 1);
  const direct = status.phase === Phase.PhaseConnected;

  const copy = async (key: string, text: string) => {
    await Clipboard.SetText(text);
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
    <section className="room-servers">
      <h3>{multi ? `房间里的服务器（${list.length}）` : "服务器"}</h3>
      {multi && (
        <p className="muted small-text">
          {connected ? "都已经连好，选一台进入；换服不用重新加入。" : "加入一次就能连接这里的所有服务器。"}
          {hasCluster && " 同一集群的服务器之间可以传送角色和物品。"}
        </p>
      )}
      {list.map(({ info, conn }, i) => {
        const key = conn?.key ?? info?.key ?? String(i);
        const asa = isASA(info?.game ?? conn?.game);
        const name = info?.name || conn?.name || `服务器 ${key}`;
        return (
          <div key={key} className={`room-server ${conn ? "linked" : ""}`}>
            <div className="room-server-head">
              <span className={`status-dot ${info?.online ? "ok" : "idle"}`} />
              <span className="name">
                {info?.password && <LockIcon />}
                {name}
              </span>
              {asa && <ASABadge />}
              <span className="sv-players">{info ? (info.online ? `${info.players}/${info.max_players || "?"}` : "未运行") : ""}</span>
            </div>
            {info && <ServerFacts server={info} groups={groups} />}
            {!info && conn?.map && <div className="sv-facts">{mapName(conn.map)}</div>}
            {conn &&
              (asa ? (
                // ASA 没有 Steam 查询端口，不能用收藏或 steam://connect，只给控制台命令
                <div className="command">
                  <code>{conn.command}</code>
                  <div className="btns">
                    <button className={copied === key ? "ok small" : "ghost small"} onClick={() => copy(key, conn.command)}>
                      {copied === key ? "已复制" : "复制命令"}
                    </button>
                  </div>
                </div>
              ) : (
                <div className="command">
                  <code>{conn.address}</code>
                  <div className="btns">
                    <button className={copied === key ? "ok small" : "ghost small"} onClick={() => copy(key, conn.address)}>
                      {copied === key ? "已复制" : "复制地址"}
                    </button>
                    {direct && (
                      <button className="primary small" onClick={() => enter(key)} disabled={entering !== ""}>
                        {entering === key ? "正在启动 ARK…" : "进入游戏"}
                      </button>
                    )}
                  </div>
                </div>
              ))}
          </div>
        );
      })}
      {enterHint && <p className="ok-text small-text">{enterHint}</p>}
      {enterError && <p className="bad small-text">{enterError}</p>}
    </section>
  );
}

/** 一台服务器的模式、地图、倍率、Mod 等 */
function ServerFacts({ server: sv, groups }: { server: RoomServer; groups: number }) {
  const mods = sv.mods?.length ?? 0;
  const r = sv.rates;
  return (
    <div className="sv-facts">
      <span className={`mode ${sv.pvp ? "pvp" : "pve"}`}>{sv.pvp ? "PvP" : "PvE"}</span>
      <span>{mapName(sv.map)}</span>
      {r && (r.xp || r.tame || r.harvest) ? (
        <span>
          经验 {rate(r.xp)} · 驯服 {rate(r.tame)} · 采集 {rate(r.harvest)}
          {r.breeding ? ` · 繁殖 ${rate(r.breeding)}` : ""}
        </span>
      ) : null}
      <span>{mods > 0 ? `${mods} 个 Mod` : "无 Mod"}</span>
      {sv.version && <span>v{sv.version}</span>}
      {sv.arkapi && <span className="plugin">{isASA(sv.game) ? "AsaApi" : "ArkApi"}</span>}
      {sv.in_cluster && <span className="cluster">集群{groups > 1 && sv.cluster_group ? ` ${sv.cluster_group}` : ""}</span>}
    </div>
  );
}

/** 全房间的 Mod。房间里有多台服务器、又不是每台都装的 Mod，标出用在哪几台上 */
function ModList({ detail }: { detail: RoomDetail }) {
  const servers = detail.servers ?? [];
  const list = detail.mod_list?.length ? detail.mod_list : (detail.mods ?? []).map((m) => ({ id: m, title: "", game: "" }));
  if (list.length === 0) return null;
  const usedBy = (id: string) => {
    if (servers.length < 2) return [];
    const used = servers.filter((sv) => sv.mods?.includes(id)).map((sv) => sv.name);
    return used.length < servers.length ? used : [];
  };
  return (
    <div className="mods">
      <h3>Mod（{list.length}）</h3>
      <ul>
        {list.map((m) => {
          const used = usedBy(m.id);
          return (
            <li key={m.id} title={used.length ? `用在：${used.join(" / ")}` : undefined}>
              <span>
                {m.title || (isASA(m.game) ? "CurseForge Mod" : "未知 Mod")}
                {used.length > 0 && <small className="used-by">{used.join(" / ")}</small>}
              </span>
              <code>{m.id}</code>
            </li>
          );
        })}
      </ul>
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

// 连接状态：加入、打洞、直连、失败。每台服务器的进服方式在下面的服务器列表里
function ConnectionCard({ status, onRetry, login, loggingIn, onOpenPortal }: Props & { onRetry: () => void }) {
  const tone = phaseTone(status.phase);
  const failed = status.phase === Phase.PhaseError;
  const servers = status.servers ?? [];
  const connected = status.phase === Phase.PhaseConnected;
  const showHints = !failed && servers.length > 0;
  // ASE 通过 Steam 收藏进服，ASA 只能在游戏控制台里用 open 命令进服，房间里可能两种都有
  const anyASE = servers.some((sv) => !isASA(sv.game));
  const anyASA = servers.some((sv) => isASA(sv.game));

  return (
    <div className={`conn ${tone}`}>
      <div className="conn-head">
        <span className={`badge ${tone}`}>
          {tone === "busy" ? <span className="spinner" /> : <span className={`dot ${tone}`} />}
          {phaseLabel(status.phase)}
        </span>
        {connected && status.latency_ms > 0 && <span className="latency">延迟 {status.latency_ms < 1 ? "<1" : Math.round(status.latency_ms)} ms</span>}
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

      {connected && status.portal_url && (
        <div className="enter-row">
          <span className="muted small-text">服主开启了玩家站：公告、服务器状态和商店。</span>
          <button className="ghost small" onClick={() => onOpenPortal(status.portal_url)}>
            查看玩家站
          </button>
        </div>
      )}

      {showHints && (
        <>
          {servers.length > 1 && <p className="muted small-text">房间里的 {servers.length} 台服务器共用这一个连接，在下面的列表里选一台进入。</p>}
          {anyASE && (
            <p className="muted small-text">
              {connected
                ? "「进入游戏」会把服务器加入 Steam 收藏，并通过 Steam 启动 ARK。进入主菜单后点「加入方舟」，切到「收藏」，选这台服务器加入。也可以自己在 Steam 的「查看 → 游戏服务器 → 收藏」里添加服务器的地址。进服期间请保持助手运行。"
                : "显示「已直连」后，点服务器旁边的「进入游戏」即可进服。"}
            </p>
          )}
          {anyASA && (
            <p className="muted small-text">
              {connected
                ? "ASA（生存飞升）服务器不能从 Steam 收藏或服务器列表进入：打开 ASA，在主菜单按 Tab 打开控制台，粘贴服务器旁边的命令后回车即可进服。进服期间请保持助手运行。"
                : "ASA（生存飞升）服务器：显示「已直连」后，在游戏主菜单按 Tab 打开控制台，粘贴服务器旁边的命令回车进服。"}
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
