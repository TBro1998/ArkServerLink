// 游戏工具：画质（ASE 改 BaseDeviceProfiles.ini，ASA 生成控制台命令）和连点器。
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Clipboard, Events } from "@wailsio/runtime";
import { ToolsService } from "../bindings/arkfgf.com/link";
import type { ASEGraphicsStatus } from "../bindings/arkfgf.com/link/models";
import type { Catalog, Option } from "../bindings/arkfgf.com/link/internal/graphics/models";
import type { Config as ClickerConfig, State as ClickerState } from "../bindings/arkfgf.com/link/internal/clicker/models";
import { ClickerChanged } from "../bindings/events";
import { errText } from "./format";

type Game = "ase" | "asa";
// 应用方式：ini 只有 ASE 有（长期生效），console 是复制控制台命令（重启游戏恢复）
type Mode = "ini" | "console";

type ToolID = "graphics" | "clicker";

const TOOLS: { id: ToolID; name: string; desc: string }[] = [
  { id: "graphics", name: "画质", desc: "性能优化、战斗画质、伽马" },
  { id: "clicker", name: "连点器", desc: "热键开关的连点和按住" },
];

export function ToolsPane() {
  const [tool, setTool] = useState<ToolID>(() => (localStorage.getItem("tools-tab") as ToolID) || "graphics");
  const pick = (id: ToolID) => {
    setTool(id);
    localStorage.setItem("tools-tab", id);
  };
  return (
    <div className="body">
      <aside className="list tool-nav">
        <div className="list-head tool-nav-head">
          <span>游戏工具</span>
        </div>
        <div className="rows">
          {TOOLS.map((t) => (
            <button key={t.id} className={`row ${tool === t.id ? "selected" : ""}`} onClick={() => pick(t.id)}>
              <span className="row-name">{t.name}</span>
              <span className="row-meta">{t.desc}</span>
            </button>
          ))}
        </div>
      </aside>
      <main className="detail">
        <div className="tools">{tool === "graphics" ? <GraphicsCard /> : <ClickerCard />}</div>
      </main>
    </div>
  );
}

// ---- 画质 ----

function GraphicsCard() {
  const [game, setGame] = useState<Game>("ase");
  const [catalogs, setCatalogs] = useState<Record<Game, Catalog | null>>({ ase: null, asa: null });
  const [selected, setSelected] = useState<Record<Game, string[]>>({ ase: [], asa: [] });
  const [gamma, setGamma] = useState(0);
  const [aseMode, setAseMode] = useState<Mode>("ini");

  useEffect(() => {
    Promise.all([ToolsService.GraphicsCatalog("ase"), ToolsService.GraphicsCatalog("asa"), ToolsService.Prefs()]).then(([ase, asa, prefs]) => {
      setCatalogs({ ase, asa });
      setSelected((s) => ({ ...s, asa: prefs.asa_options ?? [] }));
      setGamma(prefs.gamma || 0);
      setAseMode(prefs.ase_mode === "console" ? "console" : "ini");
    });
  }, []);

  const cat = catalogs[game];
  const sel = selected[game];
  const setSel = (ids: string[]) => setSelected((s) => ({ ...s, [game]: ids }));
  const toggle = (id: string) => setSel(sel.includes(id) ? sel.filter((x) => x !== id) : [...sel, id]);
  const activePreset = cat?.presets?.find((p) => sameSet(p.options ?? [], sel))?.id;
  const mode: Mode = game === "asa" ? "console" : aseMode;
  const pickMode = (m: Mode) => {
    setAseMode(m);
    ToolsService.SetASEMode(m);
  };
  // ASE 用控制台时，没有控制台写法的选项只能写进 ini
  const iniOnly = game === "ase" && mode === "console" ? (cat?.options ?? []).filter((o) => sel.includes(o.id) && !(o.console?.length ?? 0)) : [];

  return (
    <section className="tool-card">
      <div className="tool-head">
        <div>
          <h2>画质</h2>
          <p className="muted small-text">提帧数、去掉云雾和水下致盲。只改你自己电脑上的画面。</p>
        </div>
        <div className="seg">
          <button className={game === "ase" ? "on" : ""} onClick={() => setGame("ase")}>
            ASE 生存进化
          </button>
          <button className={game === "asa" ? "on" : ""} onClick={() => setGame("asa")}>
            ASA 生存飞升
          </button>
        </div>
      </div>

      {!cat ? (
        <div className="list-msg">
          <span className="spinner" />
        </div>
      ) : (
        <>
          {game === "ase" && (
            <div className="presets">
              <span className="label">应用方式</span>
              <div className="seg small">
                <button className={mode === "ini" ? "on" : ""} onClick={() => pickMode("ini")} title="写进 BaseDeviceProfiles.ini，每次启动游戏都生效">
                  写入 ini（长期生效）
                </button>
                <button className={mode === "console" ? "on" : ""} onClick={() => pickMode("console")} title="不改文件，在游戏控制台里执行，重启游戏恢复">
                  控制台命令（实验性）
                </button>
              </div>
            </div>
          )}
          <div className="presets">
            <span className="label">一键档位</span>
            {(cat.presets ?? []).map((p) => (
              <button key={p.id} className={`ghost small ${activePreset === p.id ? "picked" : ""}`} title={p.desc} onClick={() => setSel(p.options ?? [])}>
                {p.name}
              </button>
            ))}
            <button className="ghost small" onClick={() => setSel([])}>
              清空
            </button>
          </div>
          <OptionGroup title="性能" game={game} mode={mode} options={(cat.options ?? []).filter((o) => o.group === "perf")} selected={sel} onToggle={toggle} />
          <OptionGroup title="战斗画质" game={game} mode={mode} options={(cat.options ?? []).filter((o) => o.group === "combat")} selected={sel} onToggle={toggle} />
          <GammaRow gamma={gamma} setGamma={setGamma} />
          {mode === "ini" ? (
            <ASEApply selected={sel} gamma={gamma} onLoaded={(ids) => setSelected((s) => ({ ...s, ase: s.ase.length ? s.ase : ids }))} />
          ) : (
            <ConsoleCommand game={game} selected={sel} gamma={gamma} iniOnly={iniOnly.map((o) => o.name)} />
          )}
        </>
      )}
    </section>
  );
}

function OptionGroup({ title, game, mode, options, selected, onToggle }: { title: string; game: Game; mode: Mode; options: Option[]; selected: string[]; onToggle: (id: string) => void }) {
  if (options.length === 0) return null;
  // ASE 用控制台时显示控制台写法，否则显示 ini（ASA 的 lines 本身就是控制台命令）
  const aseConsole = game === "ase" && mode === "console";
  return (
    <div className="opt-group">
      <h3>{title}</h3>
      <div className="opts">
        {options.map((o) => (
          <label key={o.id} className={`opt ${selected.includes(o.id) ? "on" : ""}`}>
            <input type="checkbox" checked={selected.includes(o.id)} onChange={() => onToggle(o.id)} />
            <span className="opt-body">
              <span className="opt-name">
                {o.name}
                {aseConsole && !(o.console?.length ?? 0) && <span className="opt-tag">仅 ini</span>}
              </span>
              <span className="opt-desc">{o.desc}</span>
              {o.side_effect && <span className="opt-side">副作用：{o.side_effect}</span>}
              {aseConsole && o.console_note && <span className="opt-desc">控制台：{o.console_note}</span>}
              <details className="opt-lines" onClick={(e) => e.stopPropagation()}>
                <summary>参数</summary>
                <code>{(aseConsole ? o.console ?? [] : o.lines ?? []).join("\n") || "（控制台里改不了，只能写进 ini）"}</code>
              </details>
            </span>
          </label>
        ))}
      </div>
    </div>
  );
}

function GammaRow({ gamma, setGamma }: { gamma: number; setGamma: (g: number) => void }) {
  const on = gamma > 0;
  return (
    <div className="gamma">
      <label className="check">
        <input type="checkbox" checked={on} onChange={() => setGamma(on ? 0 : 3)} />
        调整伽马（亮度）
      </label>
      <input type="range" min={1} max={5} step={0.1} value={on ? gamma : 2.2} disabled={!on} onChange={(e) => setGamma(Number(e.target.value))} />
      <span className="gamma-val">{on ? gamma.toFixed(1) : "默认"}</span>
      <span className="muted small-text">游戏默认 2.2，夜里和洞穴可以调到 3 左右</span>
    </div>
  );
}

function ASEApply({ selected, gamma, onLoaded }: { selected: string[]; gamma: number; onLoaded: (ids: string[]) => void }) {
  const [st, setSt] = useState<ASEGraphicsStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [confirm, setConfirm] = useState(false);
  const loadedOnce = useRef(false);

  const load = useCallback(() => {
    ToolsService.ASEStatus().then((s) => {
      setSt(s);
      if (!loadedOnce.current) {
        loadedOnce.current = true;
        onLoaded(s.applied ?? []);
      }
    });
  }, [onLoaded]);
  useEffect(load, []); // eslint-disable-line react-hooks/exhaustive-deps

  const run = async (fn: () => Promise<ASEGraphicsStatus>, done: string) => {
    setBusy(true);
    setMsg(null);
    try {
      const s = await fn();
      setSt(s);
      setMsg({ ok: true, text: s.game_running ? `${done}。游戏正在运行，重启游戏后生效` : `${done}，下次启动游戏时生效` });
    } catch (e) {
      setMsg({ ok: false, text: errText(e) });
      load();
    } finally {
      setBusy(false);
    }
  };

  const applied = st?.applied ?? [];
  const dirty = !sameSet(applied, selected);
  return (
    <div className="apply">
      {st && !st.found ? (
        <p className="bad small-text">{st.error}</p>
      ) : (
        <>
          <p className="muted small-text">
            写入 <code>{st?.path || "BaseDeviceProfiles.ini"}</code>
            {applied.length > 0 ? `，当前已生效 ${applied.length} 项` : "，目前没有助手的修改"}
            {st?.error && <span className="bad"> · {st.error}</span>}
          </p>
          <div className="btns">
            <button className="primary" disabled={busy || !st || selected.length === 0 || !dirty} onClick={() => run(() => ToolsService.ASEApply(selected), "已生效")}>
              {busy ? <span className="spinner" /> : null}
              {applied.length > 0 && dirty ? "更新设置" : "应用"}
            </button>
            <button className="ghost" disabled={busy || applied.length === 0} onClick={() => run(() => ToolsService.ASERestore(), "已去掉助手的修改")}>
              去掉助手的修改
            </button>
            <button className="danger" disabled={busy || !st?.backup.exists} onClick={() => setConfirm(true)}>
              恢复原文件
            </button>
          </div>
          {st?.backup.exists && (
            <p className="muted small-text">
              生效前已备份原文件：<code>{st.backup.path}</code>（{new Date(st.backup.mod_time).toLocaleString()}）
            </p>
          )}
          {gamma > 0 && <CommandBox label="伽马在游戏里调：按 Tab 打开控制台，粘贴后回车" command={`gamma ${gamma.toFixed(1)}`} />}
        </>
      )}
      {msg && <p className={`small-text ${msg.ok ? "ok-text" : "bad"}`}>{msg.text}</p>}
      {confirm && st && (
        <div className="overlay" onClick={() => setConfirm(false)}>
          <div className="dialog" onClick={(e) => e.stopPropagation()}>
            <div className="dialog-head">
              <h2>恢复原文件？</h2>
            </div>
            <p>
              用备份 <code>{st.backup.path}</code> 整份覆盖 BaseDeviceProfiles.ini，回到助手第一次修改之前的样子。
              之后你自己在这个文件里做的改动也会被覆盖。
            </p>
            <div className="btns" style={{ justifyContent: "flex-end" }}>
              <button className="ghost" onClick={() => setConfirm(false)}>
                取消
              </button>
              <button
                className="danger"
                onClick={() => {
                  setConfirm(false);
                  run(() => ToolsService.ASERestoreBackup(), "已恢复原文件");
                }}
              >
                恢复
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function ConsoleCommand({ game, selected, gamma, iniOnly }: { game: Game; selected: string[]; gamma: number; iniOnly: string[] }) {
  const [cmd, setCmd] = useState("");
  useEffect(() => {
    const t = setTimeout(() => ToolsService.ConsoleCommand(game, selected, gamma).then(setCmd), 150);
    return () => clearTimeout(t);
  }, [game, selected, gamma]);
  return (
    <div className="apply">
      {cmd ? (
        <CommandBox label="进入游戏后按 Tab 打开控制台，粘贴后回车。不改任何文件，重启游戏恢复默认" command={cmd} wrap />
      ) : (
        <p className="muted small-text">选择上面的选项或档位，生成控制台命令。</p>
      )}
      {iniOnly.length > 0 && <p className="small-text warn-text">这些选项在控制台里改不了，没有包含在命令里，要用就切到「写入 ini」：{iniOnly.join("、")}</p>}
      {game === "ase" && cmd && (
        <p className="muted small-text">
          ASE 的控制台写法还没有逐条实测，有的命令在服务器上可能不生效。如果只生效了第一条，说明不能一次输入多条，可以展开每一项的「参数」分开输入。想每次进游戏都生效，用「写入 ini」。
        </p>
      )}
    </div>
  );
}

function CommandBox({ label, command, wrap }: { label: string; command: string; wrap?: boolean }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await Clipboard.SetText(command);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <div className="cmd-box">
      <span className="muted small-text">{label}</span>
      <div className={`command ${wrap ? "wrap" : ""}`}>
        <code>{command}</code>
        <button className="primary small" onClick={copy}>
          {copied ? "已复制" : "复制命令"}
        </button>
      </div>
    </div>
  );
}

// ---- 连点器 ----

function ClickerCard() {
  const [cfg, setCfg] = useState<ClickerConfig | null>(null);
  const [st, setSt] = useState<ClickerState | null>(null);
  const [hotkeys, setHotkeys] = useState<string[]>([]);
  const [err, setErr] = useState("");
  const saved = useRef<string>("");

  useEffect(() => {
    ToolsService.Prefs().then((p) => {
      setCfg(p.clicker);
      saved.current = JSON.stringify(p.clicker);
    });
    ToolsService.ClickerHotkeys().then((h) => setHotkeys(h ?? []));
    ToolsService.ClickerState().then(setSt);
    return Events.On(ClickerChanged, (ev) => setSt(ev.data));
  }, []);

  // 运行时刷新点击次数
  useEffect(() => {
    if (!st?.running) return;
    const t = setInterval(() => ToolsService.ClickerState().then(setSt), 1000);
    return () => clearInterval(t);
  }, [st?.running]);

  // 改了设置自动保存
  useEffect(() => {
    if (!cfg) return;
    const s = JSON.stringify(cfg);
    if (s === saved.current) return;
    const t = setTimeout(() => {
      ToolsService.SetClickerConfig(cfg)
        .then((c) => {
          saved.current = JSON.stringify(c);
          setErr("");
        })
        .catch((e) => setErr(errText(e)));
    }, 400);
    return () => clearTimeout(t);
  }, [cfg]);

  const set = (patch: Partial<ClickerConfig>) => setCfg((c) => (c ? { ...c, ...patch } : c));
  const state = useMemo(() => clickerLabel(st), [st]);
  if (!cfg) return null;

  return (
    <section className="tool-card">
      <div className="tool-head">
        <div>
          <h2>连点器</h2>
          <p className="muted small-text">按热键开关。默认只在 ARK 窗口在前台时点击，切出游戏自动暂停。</p>
        </div>
        <div className="btns">
          <span className={`badge ${state.tone}`}>
            <span className={`dot ${state.tone}`} />
            {state.text}
          </span>
          <button className={st?.running ? "danger" : "primary"} onClick={() => ToolsService.ClickerToggle().then(setSt)}>
            {st?.running ? "关闭" : "开启"}
            {st?.hotkey && <kbd>{st.hotkey}</kbd>}
          </button>
        </div>
      </div>

      <div className="form-grid">
        <label className="field">
          点击
          <select value={cfg.action} onChange={(e) => set({ action: e.target.value })}>
            <option value="left">鼠标左键</option>
            <option value="right">鼠标右键</option>
            <option value="key">键盘按键</option>
          </select>
        </label>
        {cfg.action === "key" && (
          <label className="field">
            按键
            <input className="text" value={cfg.key} maxLength={5} placeholder="E" onChange={(e) => set({ key: e.target.value.toUpperCase() })} />
          </label>
        )}
        <label className="field">
          方式
          <select value={cfg.mode} onChange={(e) => set({ mode: e.target.value })}>
            <option value="click">连续点击</option>
            <option value="hold">一直按住</option>
          </select>
        </label>
        {cfg.mode === "click" && (
          <label className="field">
            间隔
            <input className="text num" type="number" min={20} max={60000} step={10} value={cfg.interval_ms} onChange={(e) => set({ interval_ms: Number(e.target.value) })} />
            毫秒
          </label>
        )}
        <label className="field">
          开关热键
          <select value={cfg.hotkey} onChange={(e) => set({ hotkey: e.target.value })}>
            {hotkeys.map((h) => (
              <option key={h} value={h}>
                {h}
              </option>
            ))}
          </select>
        </label>
        <label className="check">
          <input type="checkbox" checked={cfg.game_only} onChange={(e) => set({ game_only: e.target.checked })} />
          只在 ARK 窗口在前台时点击
        </label>
      </div>
      <p className="muted small-text">按键支持 A-Z、0-9 和 Space。「一直按住」适合自动采集、自动攻击，再按一次热键松开。</p>
      {(err || st?.error) && <p className="bad small-text">{err || st?.error}</p>}
    </section>
  );
}

function clickerLabel(st: ClickerState | null): { text: string; tone: string } {
  if (!st?.running) return { text: "已关闭", tone: "idle" };
  if (st.paused) return { text: "已暂停（ARK 不在前台）", tone: "warn" };
  if (st.holding) return { text: "按住中", tone: "ok" };
  return { text: `运行中 · ${st.clicks} 次`, tone: "ok" };
}

function sameSet(a: string[], b: string[]) {
  if (a.length !== b.length) return false;
  const s = new Set(a);
  return b.every((x) => s.has(x));
}
