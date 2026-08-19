import { useEffect, useRef, useState } from 'preact/hooks';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';
import {
  Start, Stop, Restart, StartAll, StopAll,
  ListProjects, AddProject, UpdateUrl, RemoveProject, PickFolder,
  GetPhpPath, SetPhpPath, OpenURL,
} from '../wailsjs/go/main/App';
import './App.css';

type Status = { apache: boolean; mariadb: boolean };
type Proc = { pid: number; cpu: number; ram: number };
type Procs = { apache: Proc[]; mariadb: Proc[] };

type LogEntry = { service: 'apache' | 'mariadb' | 'system'; line: string };

type Project = { id: string; path: string; url: string };

const tabs = ['Services', 'Projects', 'Databases', 'Config', 'Path', 'About'] as const;
type Tab = (typeof tabs)[number];

const dbs = ['app_werd', 'blog'];
const ports = ['80 (httpd)', '3306 (mysql)'];

export const App = () => {
  const [tab, setTab] = useState<Tab>(() => (localStorage.getItem('werd-tab') as Tab) || 'Services');
  const changeTab = (t: Tab) => {
    setTab(t);
    localStorage.setItem('werd-tab', t);
  };
  const [status, setStatus] = useState<Status>({ apache: false, mariadb: false });
  const [procs, setProcs] = useState<Procs>({ apache: [], mariadb: [] });
  const [loading, setLoading] = useState<{ apache: boolean; mariadb: boolean }>({
    apache: false,
    mariadb: false,
  });
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const [projects, setProjects] = useState<Project[]>([]);

  useEffect(() => {
    const onStatus = (msg: { service: 'apache' | 'mariadb'; running: boolean; procs?: Proc[] }) => {
      setStatus((s) => ({ ...s, [msg.service]: msg.running }));
      setProcs((p) => ({ ...p, [msg.service]: msg.procs || [] }));
      setLoading((l) => ({ ...l, [msg.service]: false }));
    };
    const onLog = (msg: { service: 'apache' | 'mariadb'; line: string }) =>
      setLogs((l) => [...l.slice(-199), { service: msg.service, line: msg.line }]);
    const onError = (msg: { service?: string; message: string }) =>
      setLogs((l) => [...l.slice(-199), { service: 'system', line: `[error] ${msg.message}` }]);
    const onProjects = (msg: { projects: Project[] }) => setProjects(msg.projects || []);

    EventsOn('status', onStatus);
    EventsOn('log', onLog);
    EventsOn('error', onError);
    EventsOn('projects', onProjects);
    setConnected(true);
    ListProjects().then(setProjects);
    return () => {
      EventsOff('status', 'log', 'error', 'projects');
    };
  }, []);

  const control = (service: 'apache' | 'mariadb', action: 'start' | 'stop' | 'restart') => {
    setLoading((l) => ({ ...l, [service]: true }));
    if (action === 'start') Start(service);
    else if (action === 'stop') Stop(service);
    else Restart(service);
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100 antialiased">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-6 py-8">
        <Header running={connected && status.apache && status.mariadb} connected={connected} />
        <Tabs current={tab} onSelect={changeTab} />
        {tab === 'Services' && (
          <Services
            status={status}
            procs={procs}
            loading={loading}
            onControl={control}
            onAll={(a) => {
              setLoading({ apache: true, mariadb: true });
              if (a === 'startAll') StartAll();
              else StopAll();
            }}
          />
        )}
        {tab === 'Projects' && <Projects projects={projects} />}
        {tab === 'Databases' && <Databases />}
        {tab === 'Config' && <Config />}
        {tab === 'Path' && <PathPane />}
        {tab === 'About' && <About />}
        <Logs logs={logs} />
      </div>
    </div>
  );
};

const Header = ({ running, connected }: { running: boolean; connected: boolean }) => (
  <header className="flex items-center justify-between">
    <div>
      <h1 className="text-2xl font-semibold tracking-tight">WERD Panel</h1>
      <p className="text-sm text-zinc-400">localhost · control panel</p>
    </div>
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium ${
        running
          ? 'bg-emerald-500/15 text-emerald-400'
          : connected
            ? 'bg-amber-500/15 text-amber-400'
            : 'bg-red-500/15 text-red-400'
      }`}
    >
      <span
        className={`size-2 rounded-full ${
          running ? 'bg-emerald-400' : connected ? 'bg-amber-400' : 'bg-red-400'
        }`}
      />
      {!connected ? 'Offline' : running ? 'Running' : 'Stopped'}
    </span>
  </header>
);

const Tabs = ({ current, onSelect }: { current: Tab; onSelect: (t: Tab) => void }) => (
  <nav className="flex gap-1 rounded-lg border border-zinc-800 bg-zinc-900 p-1">
    {tabs.map((t) => (
      <button
        key={t}
        onClick={() => onSelect(t)}
        className={`rounded-lg px-4 py-2 text-sm font-medium transition-colors ${
          current === t
            ? 'bg-zinc-800 text-zinc-50'
            : 'text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200'
        }`}
      >
        {t}
      </button>
    ))}
  </nav>
);

const ServiceCard = ({
  name,
  version,
  port,
  dir,
  running,
  procs,
  loading,
  onControl,
}: {
  name: string;
  version: string;
  port: string;
  dir: string;
  running: boolean;
  procs: Proc[];
  loading: boolean;
  onControl: (a: 'start' | 'stop' | 'restart') => void;
}) => (
  <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-900 p-4">
    <div className="flex items-center gap-3">
      <span className={`size-2.5 rounded-full ${running ? 'bg-emerald-500' : 'bg-red-500'}`} />
      <div>
        <div className="text-sm font-medium text-zinc-100">{name}</div>
        <div className="text-xs text-zinc-400">
          {version} · port {port}
        </div>
        {running && procs.length > 0 && (
          <div className="mt-1 text-[11px] text-zinc-500">
            {procs.length > 1 ? `${procs.length} processes` : `pid ${procs[0].pid}`}
            {' · '}cpu {Math.round(procs.reduce((s, p) => s + p.cpu, 0) * 10) / 10}%
            {' · '}ram {Math.round(procs.reduce((s, p) => s + p.ram, 0) * 10) / 10} MB
          </div>
        )}
      </div>
    </div>
    <div className="flex items-center gap-2">
      <span className="text-xs text-zinc-500">{dir}</span>
      <button
        disabled={loading || running}
        onClick={() => onControl('start')}
        title={running ? 'Already running' : loading ? 'Busy' : undefined}
        className="rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-medium text-emerald-950 hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? <Spinner /> : 'Start'}
      </button>
      <button
        disabled={loading || !running}
        onClick={() => onControl('restart')}
        title={!running ? 'Not running' : loading ? 'Busy' : undefined}
        className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-300 hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? <Spinner /> : 'Restart'}
      </button>
      <button
        disabled={loading || !running}
        onClick={() => onControl('stop')}
        title={!running ? 'Not running' : loading ? 'Busy' : undefined}
        className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-300 hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {loading ? <Spinner /> : 'Stop'}
      </button>
    </div>
  </div>
);

const Services = ({
  status,
  procs,
  loading,
  onControl,
  onAll,
}: {
  status: Status;
  procs: Procs;
  loading: { apache: boolean; mariadb: boolean };
  onControl: (s: 'apache' | 'mariadb', a: 'start' | 'stop' | 'restart') => void;
  onAll: (a: 'startAll' | 'stopAll') => void;
}) => (
  <div className="grid gap-4">
    <div className="flex items-center gap-2">
      <button
        disabled={loading.apache || loading.mariadb || (status.apache && status.mariadb)}
        onClick={() => onAll('startAll')}
        title={
          status.apache && status.mariadb
            ? 'All services already running'
            : loading.apache || loading.mariadb
              ? 'Busy'
              : undefined
        }
        className="rounded-lg bg-emerald-600 px-4 py-2 text-xs font-medium text-emerald-950 hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-50"
      >
        Start All
      </button>
      <button
        disabled={loading.apache || loading.mariadb || (!status.apache && !status.mariadb)}
        onClick={() => onAll('stopAll')}
        title={
          !status.apache && !status.mariadb
            ? 'All services already stopped'
            : loading.apache || loading.mariadb
              ? 'Busy'
              : undefined
        }
        className="rounded-lg border border-zinc-700 px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-50"
      >
        Stop All
      </button>
    </div>
    <ServiceCard
      name="Apache"
      version="2.4.66"
      port="80"
      dir="bin/httpd-2.4.66"
      running={status.apache}
      procs={procs.apache}
      loading={loading.apache}
      onControl={(a) => onControl('apache', a)}
    />
    <ServiceCard
      name="MySQL"
      version="8.4.11"
      port="3306"
      dir="bin/mysql-8.4.11"
      running={status.mariadb}
      procs={procs.mariadb}
      loading={loading.mariadb}
      onControl={(a) => onControl('mariadb', a)}
    />
    <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-900 p-4 opacity-60">
      <div className="flex items-center gap-3">
        <span className="size-2.5 rounded-full bg-emerald-500" />
        <div>
          <div className="text-sm font-medium text-zinc-100">PHP</div>
          <div className="text-xs text-zinc-400">8.4.23 · mod_php</div>
        </div>
      </div>
      <span className="text-xs text-zinc-500">via Apache</span>
    </div>
    <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-900 p-4 opacity-60">
      <div className="flex items-center gap-3">
        <span className="size-2.5 rounded-full bg-emerald-500" />
        <div>
          <div className="text-sm font-medium text-zinc-100">phpMyAdmin</div>
          <div className="text-xs text-zinc-400">5.2.3 · :80</div>
        </div>
      </div>
      <button
        onClick={() => OpenURL('http://localhost/phpmyadmin')}
        className="rounded-lg border border-zinc-700 px-3 py-1.5 text-xs font-medium text-zinc-300 hover:bg-zinc-800"
      >
        Open
      </button>
    </div>
  </div>
);

const Projects = ({ projects }: { projects: Project[] }) => {
  const [picking, setPicking] = useState(false);
  const [pending, setPending] = useState<{ path: string; url: string } | null>(null);

  const openExplorer = async () => {
    setPicking(true);
    const p = await PickFolder();
    setPicking(false);
    if (!p) return;
    const parts = p.replace(/\\/g, '/').split('/').filter(Boolean);
    const folder = parts[parts.length - 1] || '';
    const host = folder.toLowerCase().replace(/[^a-z0-9.-]+/g, '-');
    setPending({ path: p, url: `http://${host || 'project'}.localhost` });
  };

  const save = async () => {
    if (!pending) return;
    const ps = await AddProject(pending.path, pending.url);
    setProjects(ps);
    setPending(null);
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium text-zinc-300">Projects</h2>
        <button
          onClick={openExplorer}
          disabled={picking}
          className="rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-medium text-emerald-950 hover:bg-emerald-500 disabled:opacity-50"
        >
          {picking ? '...' : '+ Add Project'}
        </button>
      </div>
      <div className="overflow-hidden rounded-lg border border-zinc-800">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-zinc-800 bg-zinc-900 text-xs text-zinc-500">
            <tr>
              <th className="px-4 py-2 font-medium">Path</th>
              <th className="px-4 py-2 font-medium">URL</th>
              <th className="px-4 py-2 font-medium text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800 bg-zinc-950">
            {projects.length === 0 && (
              <tr>
                <td className="px-4 py-4 text-xs text-zinc-500" colSpan={3}>
                  Belum ada project. Klik + Add Project.
                </td>
              </tr>
            )}
            {projects.map((p) => (
              <tr key={p.id}>
                <td className="px-4 py-2 text-xs text-zinc-300">{p.path}</td>
                <td className="px-4 py-2">
                  <input
                    value={p.url}
                    onChange={async (e: any) => {
                      const ps = await UpdateUrl(p.id, e.target.value);
                      setProjects(ps);
                    }}
                    placeholder="http://project.localhost"
                    className="w-full rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1 text-xs text-zinc-300 outline-none focus:border-emerald-600"
                  />
                </td>
                <td className="px-4 py-2 text-right">
                  <div className="flex justify-end gap-2">
                    <button
                      onClick={() => OpenURL(p.url)}
                      className="rounded-lg border border-zinc-700 px-3 py-1 text-xs font-medium text-zinc-300 hover:bg-zinc-800"
                    >
                      Open
                    </button>
                    <button
                      onClick={async () => {
                        const ps = await RemoveProject(p.id);
                        setProjects(ps);
                      }}
                      className="rounded-lg border border-red-800 px-3 py-1 text-xs font-medium text-red-400 hover:bg-red-950"
                    >
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {pending && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="w-full max-w-md rounded-lg border border-zinc-800 bg-zinc-900 p-4">
            <h3 className="mb-4 text-sm font-medium text-zinc-200">Add Project</h3>
            <div className="mb-3">
              <label className="mb-1 block text-xs text-zinc-500">Path</label>
              <input
                value={pending.path}
                readOnly
                className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-xs text-zinc-300 outline-none"
              />
            </div>
            <div className="mb-4">
              <label className="mb-1 block text-xs text-zinc-500">URL</label>
              <input
                value={pending.url}
                onChange={(e: any) => setPending({ ...pending, url: e.target.value })}
                className="w-full rounded-md border border-zinc-800 bg-zinc-950 px-3 py-2 text-xs text-zinc-300 outline-none focus:border-emerald-600"
              />
            </div>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setPending(null)}
                className="rounded-md px-3 py-1.5 text-xs text-zinc-400 hover:bg-zinc-800"
              >
                Cancel
              </button>
              <button
                onClick={save}
                className="rounded-md bg-emerald-600 px-3 py-1.5 text-xs font-medium text-emerald-950 hover:bg-emerald-500"
              >
                Save
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

const Spinner = () => (
  <span className="inline-block size-3 animate-spin rounded-full border-2 border-current border-t-transparent align-[-1px]" />
);

const Logs = ({ logs }: { logs: LogEntry[] }) => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight;
  }, [logs]);
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950">
      <div className="flex items-center gap-2 border-b border-zinc-800 px-4 py-2 text-xs text-zinc-400">
        <span className="size-1.5 rounded-full bg-zinc-500" />
        Logs
      </div>
      <div
        ref={ref}
        className="h-64 overflow-y-auto px-4 py-3 font-mono text-xs leading-relaxed text-zinc-400"
      >
        {logs.length === 0 ? (
          <div className="text-zinc-600">no output yet</div>
        ) : (
          logs.map((l, i) => (
            <div key={i} className="whitespace-pre-wrap">
              <span
                className={
                  l.service === 'apache'
                    ? 'text-sky-400'
                    : l.service === 'mariadb'
                      ? 'text-emerald-400'
                      : 'text-zinc-500'
                }
              >
                [{l.service}]
              </span>{' '}
              {l.line}
            </div>
          ))
        )}
      </div>
    </div>
  );
};

const Databases = () => (
  <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-4">
    <div className="grid gap-2">
      {dbs.map((db) => (
        <div key={db} className="flex items-center justify-between rounded-lg px-2 py-2 hover:bg-zinc-800/60">
          <span className="text-sm text-zinc-100">{db}</span>
          <button className="rounded-lg border border-zinc-700 px-3 py-1 text-xs text-zinc-300 hover:bg-zinc-800">
            Open
          </button>
        </div>
      ))}
    </div>
  </div>
);

const Config = () => (
  <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-4">
    <h2 className="mb-3 text-sm font-semibold text-zinc-100">Configuration</h2>
    <div className="grid gap-2">
      {ports.map((p) => (
        <div key={p} className="rounded-lg px-2 py-1 text-sm text-zinc-300">
          {p}
        </div>
      ))}
    </div>
  </div>
);

const PathPane = () => {
  const [on, setOn] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    GetPhpPath().then(setOn);
  }, []);
  const toggle = async () => {
    setBusy(true);
    setOn(await SetPhpPath(!on));
    setBusy(false);
  };
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-4">
      <h2 className="mb-1 text-sm font-semibold text-zinc-100">Tools in user PATH</h2>
      <p className="mb-3 text-xs text-zinc-400">
        Add/remove these tools to your Windows user environment PATH so they are available in every terminal.
      </p>
      <div className="mb-3 grid gap-2">
        {[
          ['PHP', 'php', 'bin/php-8.4.23'],
          ['Composer', 'composer', 'config/'],
          ['MySQL', 'mysql, mysqldump', 'bin/mysql-8.4.11/bin'],
        ].map(([name, cmds, dir]) => (
          <div key={name} className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-950/50 px-3 py-2">
            <div>
              <div className="text-sm text-zinc-100">{name}</div>
              <div className="text-xs text-zinc-500">
                <code className="text-zinc-400">{cmds}</code> · {dir}
              </div>
            </div>
            <span className={`size-2.5 rounded-full ${on ? 'bg-emerald-500' : 'bg-zinc-600'}`} />
          </div>
        ))}
      </div>
      <button
        onClick={toggle}
        disabled={busy}
        className={`rounded-lg px-4 py-2 text-sm font-medium transition-colors ${
          on ? 'bg-emerald-600 text-white hover:bg-emerald-700' : 'bg-zinc-700 text-zinc-200 hover:bg-zinc-600'
        } disabled:opacity-60`}
      >
        {on ? 'Enabled — click to remove' : 'Disabled — click to enable'}
      </button>
      <p className="mt-3 text-xs text-zinc-500">Applies to new terminal/processes. Restart your shell after toggling.</p>
    </div>
  );
};

const About = () => (
  <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-4">
    <h2 className="mb-1 text-sm font-semibold text-zinc-100">WERD Panel</h2>
    <p className="text-sm text-zinc-400">Control panel — alternative to XAMPP / Laragon.</p>
    <p className="mt-2 text-xs text-zinc-500">Apache · MySQL · PHP · phpMyAdmin</p>
  </div>
);

export default App;